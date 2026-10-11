package configfile

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// A profile is a ZIP with profile.yaml (a regular deck.yaml) and the images
// it references under icons/, so layouts can be shared between machines.
const (
	ProfileExt      = ".deckprofile"
	profileManifest = "profile.yaml"
	maxProfileFiles = 500
	maxProfileBytes = 64 << 20
)

var profileImageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}

// ExportProfile packs the whole config, or only page when it is not empty.
func ExportProfile(cfg *deck.Config, page string) ([]byte, error) {
	out := *cfg
	if page != "" {
		p, ok := cfg.Page(page)
		if !ok {
			return nil, fmt.Errorf("unknown page %q", page)
		}
		out = deck.Config{
			Pages:       []deck.Page{*p},
			DefaultPage: p.Name,
			Brightness:  deck.DefaultBrightness,
			SmallWindow: deck.DefaultSmallWindow(),
		}
	}

	files := map[string]string{}
	pack := func(resolved string) string {
		if resolved == "" {
			return ""
		}
		if name, ok := files[resolved]; ok {
			return name
		}
		if _, err := os.Stat(resolved); err != nil {
			return ""
		}
		base := sanitizeProfileName(filepath.Base(resolved))
		name := "icons/" + base
		for i := 2; slices.Contains(mapValues(files), name); i++ {
			name = fmt.Sprintf("icons/%d-%s", i, base)
		}
		files[resolved] = name
		return name
	}
	packButtons := func(buttons []deck.Button) []deck.Button {
		copied := slices.Clone(buttons)
		for i := range copied {
			b := &copied[i]
			if name := pack(b.ResolvedIcon); name != "" {
				b.IconPath = name
			}
			if b.Toggle != nil {
				t := *b.Toggle
				if name := pack(t.ResolvedIcon); name != "" {
					t.IconPath = name
				}
				b.Toggle = &t
			}
		}
		return copied
	}

	out.Pages = slices.Clone(out.Pages)
	for i := range out.Pages {
		out.Pages[i].Buttons = packButtons(out.Pages[i].Buttons)
	}
	out.FixedButtons = packButtons(out.FixedButtons)
	out.SmallWindow.Widgets = slices.Clone(out.SmallWindow.Widgets)
	for i := range out.SmallWindow.Widgets {
		w := &out.SmallWindow.Widgets[i]
		var paths []string
		for j, resolved := range w.ResolvedPaths {
			if name := pack(resolved); name != "" {
				paths = append(paths, name)
			} else if j < len(w.Paths) {
				paths = append(paths, w.Paths[j])
			}
		}
		w.Paths = paths
	}

	text, err := Marshal(&out)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := addZipFile(zw, profileManifest, text); err != nil {
		return nil, err
	}
	for resolved, name := range files {
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, err
		}
		if err := addZipFile(zw, name, data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func addZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func sanitizeProfileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 128 && (r == '-' || r == '_' || r == '.' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.TrimLeft(b.String(), ".")
}

// ImportProfile unpacks the images of a profile into dir and returns its
// config with paths pointing there. Only images below icons/ are extracted.
func ImportProfile(data []byte, dir string) (*deck.Config, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a profile: %w", err)
	}
	if len(zr.File) > maxProfileFiles {
		return nil, fmt.Errorf("profile has too many files (%d)", len(zr.File))
	}
	var manifest []byte
	var total int64
	for _, f := range zr.File {
		total += int64(f.UncompressedSize64)
		if total > maxProfileBytes {
			return nil, errors.New("profile is too large")
		}
		name := path.Clean(f.Name)
		switch {
		case name == profileManifest:
			if manifest, err = readZipFile(f); err != nil {
				return nil, err
			}
		case strings.HasPrefix(name, "icons/") && !strings.Contains(name[len("icons/"):], "/") &&
			slices.Contains(profileImageExts, strings.ToLower(path.Ext(name))):
			content, err := readZipFile(f)
			if err != nil {
				return nil, err
			}
			target := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				return nil, err
			}
		}
	}
	if manifest == nil {
		return nil, errors.New("not a profile: profile.yaml is missing")
	}
	return Parse(manifest, dir)
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxProfileBytes))
}
