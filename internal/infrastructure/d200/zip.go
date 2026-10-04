package d200

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

const (
	firstBoundaryOffset = MaxPayloadSize
	maxDummyRetries     = 1024
)

type viewParam struct {
	Icon string `json:"Icon,omitempty"`
	Text string `json:"Text,omitempty"`
}

type manifestEntry struct {
	State     int         `json:"State"`
	ViewParam []viewParam `json:"ViewParam"`
}

type zipEntry struct {
	name string
	data []byte
}

// BuildButtonsZip renders the archive SET_BUTTONS / PARTIALLY_UPDATE_BUTTONS
// expect: manifest.json, a stored dummy.txt used as shiftable padding, the
// icon PNGs and an empty trailing sentinel.txt (the firmware may drop the
// last entry).
func BuildButtonsZip(buttons []deck.Button, fillMissing bool) ([]byte, error) {
	for _, b := range buttons {
		if b.Index < 0 || b.Index >= deck.SlotCount {
			return nil, fmt.Errorf("button index %d is outside the supported D200 grid (0..%d)", b.Index, deck.SlotCount-1)
		}
	}
	if fillMissing {
		buttons = fillLayout(buttons)
	}

	manifest := map[string]manifestEntry{}
	var icons []zipEntry
	seen := map[string]bool{}
	for _, b := range buttons {
		var vp viewParam
		if needsIconAsset(b) {
			png := render.ButtonIcon(b)
			name := archiveIconName(b, png)
			vp.Icon = name
			if !hasRealIcon(b) && b.Label != "" && b.Index != deck.InfoWindowIndex {
				vp.Text = b.Label
			}
			if !seen[name] {
				seen[name] = true
				icons = append(icons, zipEntry{name, png})
			}
		}
		key := fmt.Sprintf("%d_%d", b.Index%deck.GridColumns, b.Index/deck.GridColumns)
		manifest[key] = manifestEntry{State: 0, ViewParam: []viewParam{vp}}
	}

	manifestJSON, err := asciiJSON(manifest)
	if err != nil {
		return nil, err
	}

	for padding := 0; padding <= maxDummyRetries; padding++ {
		blob, err := assembleZip(manifestJSON, icons, padding)
		if err != nil {
			return nil, err
		}
		if boundariesSafe(blob) {
			return blob, nil
		}
	}
	return nil, fmt.Errorf("unable to produce a ZIP whose 1024-byte boundaries avoid firmware-invalid bytes after %d retries", maxDummyRetries)
}

func fillLayout(buttons []deck.Button) []deck.Button {
	byIndex := map[int]deck.Button{}
	for _, b := range buttons {
		byIndex[b.Index] = b
	}
	filled := make([]deck.Button, 0, deck.ButtonCount)
	for i := range deck.ButtonCount {
		if b, ok := byIndex[i]; ok {
			filled = append(filled, b)
		} else {
			filled = append(filled, deck.Button{Index: i, TextStyle: deck.DefaultTextStyle()})
		}
	}
	return filled
}

func hasRealIcon(b deck.Button) bool {
	if b.IconData != nil {
		return true
	}
	if b.ResolvedIcon == "" {
		return false
	}
	_, err := os.Stat(b.ResolvedIcon)
	return err == nil
}

// Label-only buttons need a generated text tile, otherwise the firmware
// shows a black button despite the manifest Text key.
func needsIconAsset(b deck.Button) bool {
	return b.Index == deck.InfoWindowIndex || hasRealIcon(b) || b.Label != ""
}

// archiveIconName includes a content hash so the firmware can never reuse a
// cached asset with the same name after a page switch or an icon edit.
func archiveIconName(b deck.Button, png []byte) string {
	sum := sha256.Sum256(png)
	digest := hex.EncodeToString(sum[:])[:12]
	if b.Index == deck.InfoWindowIndex {
		return "icons/info-window-" + digest + ".png"
	}
	if b.ResolvedIcon != "" && b.IconData == nil && hasRealIcon(b) {
		stem := strings.TrimSuffix(filepath.Base(b.ResolvedIcon), filepath.Ext(b.ResolvedIcon))
		return "icons/" + sanitizeName(stem) + "-" + digest + ".png"
	}
	return fmt.Sprintf("icons/%d-%s.png", b.Index, digest)
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (r == '-' || r == '_' || r == '.' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "icon"
	}
	return b.String()
}

// asciiJSON matches Python's json.dumps(sort_keys=True, ensure_ascii=True)
// with compact separators, which is what the firmware has been tested with.
func asciiJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	raw := bytes.TrimRight(buf.Bytes(), "\n")
	var out bytes.Buffer
	for _, r := range string(raw) {
		if r < 128 {
			out.WriteRune(r)
			continue
		}
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&out, `\u%04x`, unit)
		}
	}
	return out.Bytes(), nil
}

func assembleZip(manifest []byte, icons []zipEntry, padding int) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	now := time.Now()

	if err := writeEntry(zw, "manifest.json", manifest, zip.Deflate, now); err != nil {
		return nil, err
	}
	if err := writeEntry(zw, "dummy.txt", bytes.Repeat([]byte("A"), padding), zip.Store, now); err != nil {
		return nil, err
	}
	for _, icon := range icons {
		if err := writeEntry(zw, icon.name, icon.data, zip.Deflate, now); err != nil {
			return nil, err
		}
	}
	if err := writeEntry(zw, "sentinel.txt", nil, zip.Store, now); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeEntry uses CreateRaw so sizes land in the local header; the regular
// writer emits data descriptors, which the firmware parser was never
// tested against.
func writeEntry(zw *zip.Writer, name string, data []byte, method uint16, modified time.Time) error {
	payload := data
	if method == zip.Deflate {
		var compressed bytes.Buffer
		fw, err := flate.NewWriter(&compressed, 1)
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
		if err := fw.Close(); err != nil {
			return err
		}
		payload = compressed.Bytes()
	}
	header := &zip.FileHeader{
		Name:               name,
		Method:             method,
		Modified:           modified,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(payload)),
		UncompressedSize64: uint64(len(data)),
	}
	w, err := zw.CreateRaw(header)
	if err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

// The firmware parses the archive while it streams in; if the last byte
// before a 1024-byte frame boundary is 0x00 or 0x7C (the packet magic) the
// upload gets corrupted.
func boundariesSafe(blob []byte) bool {
	for offset := firstBoundaryOffset; offset < len(blob); offset += PacketSize {
		if blob[offset] == 0x00 || blob[offset] == 0x7C {
			return false
		}
	}
	return true
}
