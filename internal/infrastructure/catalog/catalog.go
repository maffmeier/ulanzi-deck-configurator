// Package catalog provides the built-in icon library: Font Awesome Free
// glyphs and Twemoji, both embedded in the binary.
package catalog

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/fogleman/gg"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

//go:embed data/fontawesome.json data/emoji.json data/twemoji.zip data/*.otf
var data embed.FS

const renderSize = 256

var ErrNotFound = errors.New("builtin asset not found")

type Icon struct {
	AssetID     string   `json:"asset_id"`
	Name        string   `json:"name"`
	Style       string   `json:"style"`
	Family      string   `json:"family"`
	SearchTerms []string `json:"search_terms"`
	glyph       string
	svgName     string
}

type Catalog struct {
	icons []Icon
	byID  map[string]*Icon
	svgs  map[string]*zip.File
	fonts map[string]*opentype.Font

	mu    sync.Mutex
	cache map[string][]byte
}

var faFiles = map[string]string{
	"solid":   "data/fa-solid.otf",
	"regular": "data/fa-regular.otf",
	"brands":  "data/fa-brands.otf",
}

func Load() (*Catalog, error) {
	c := &Catalog{byID: map[string]*Icon{}, fonts: map[string]*opentype.Font{}, svgs: map[string]*zip.File{}, cache: map[string][]byte{}}
	if err := c.loadFontAwesome(); err != nil {
		return nil, fmt.Errorf("font awesome: %w", err)
	}
	if err := c.loadEmoji(); err != nil {
		return nil, fmt.Errorf("emoji: %w", err)
	}
	sort.Slice(c.icons, func(i, j int) bool {
		a, b := c.icons[i], c.icons[j]
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		if a.Style != b.Style {
			return a.Style < b.Style
		}
		return a.Name < b.Name
	})
	for i := range c.icons {
		c.byID[c.icons[i].AssetID] = &c.icons[i]
	}
	return c, nil
}

func (c *Catalog) Icons() []Icon { return c.icons }

func (c *Catalog) loadFontAwesome() error {
	for style, file := range faFiles {
		raw, err := data.ReadFile(file)
		if err != nil {
			return err
		}
		f, err := opentype.Parse(raw)
		if err != nil {
			return err
		}
		c.fonts[style] = f
	}
	raw, err := data.ReadFile("data/fontawesome.json")
	if err != nil {
		return err
	}
	var meta map[string]struct {
		Unicode string   `json:"unicode"`
		Styles  []string `json:"styles"`
		Terms   []string `json:"terms"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	for name, m := range meta {
		code, err := strconv.ParseUint(m.Unicode, 16, 32)
		if err != nil {
			continue
		}
		terms := uniqueSorted(append(append([]string{name}, strings.Split(name, "-")...), m.Terms...))
		for _, style := range m.Styles {
			if _, ok := faFiles[style]; !ok {
				continue
			}
			c.icons = append(c.icons, Icon{
				AssetID:     "fa:" + style + ":" + name,
				Name:        name,
				Style:       style,
				Family:      "fontawesome",
				SearchTerms: terms,
				glyph:       string(rune(code)),
			})
		}
	}
	return nil
}

func (c *Catalog) loadEmoji() error {
	zipped, err := data.ReadFile("data/twemoji.zip")
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		c.svgs[strings.TrimSuffix(filepath.Base(f.Name), ".svg")] = f
	}

	raw, err := data.ReadFile("data/emoji.json")
	if err != nil {
		return err
	}
	var meta []struct {
		Unified     string   `json:"unified"`
		ShortName   string   `json:"short_name"`
		Name        string   `json:"name"`
		Category    string   `json:"category"`
		Subcategory string   `json:"subcategory"`
		ShortNames  []string `json:"short_names"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return err
	}
	for _, m := range meta {
		unified := strings.ToLower(m.Unified)
		svg := c.twemojiName(unified)
		if svg == "" || m.ShortName == "" {
			continue
		}
		terms := []string{m.ShortName, m.Name, m.Category, m.Subcategory}
		terms = append(terms, m.ShortNames...)
		terms = append(terms, strings.FieldsFunc(m.ShortName, func(r rune) bool { return r == '_' || r == '-' })...)
		c.icons = append(c.icons, Icon{
			AssetID:     "emoji:" + unified,
			Name:        strings.ReplaceAll(m.ShortName, "_", " "),
			Style:       "emoji",
			Family:      "emoji",
			SearchTerms: uniqueSorted(lowerAll(terms)),
			svgName:     svg,
		})
	}
	return nil
}

// Twemoji drops the FE0F variation selector from most file names.
func (c *Catalog) twemojiName(unified string) string {
	if _, ok := c.svgs[unified]; ok {
		return unified
	}
	stripped := strings.ReplaceAll(strings.ReplaceAll(unified, "-fe0f", ""), "fe0f-", "")
	if _, ok := c.svgs[stripped]; ok {
		return stripped
	}
	return ""
}

// PNG renders an icon at 256x256 on a transparent background.
func (c *Catalog) PNG(assetID string) ([]byte, error) {
	icon, ok := c.byID[assetID]
	if !ok {
		return nil, ErrNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.cache[assetID]; ok {
		return cached, nil
	}

	var img image.Image
	var err error
	if icon.Family == "emoji" {
		img, err = c.renderEmoji(icon)
	} else {
		img, err = c.renderGlyph(icon)
	}
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	c.cache[assetID] = buf.Bytes()
	return buf.Bytes(), nil
}

func (c *Catalog) renderGlyph(icon *Icon) (image.Image, error) {
	f := c.fonts[icon.Style]
	padding := max(12, renderSize/10)
	maxSize := renderSize - 2*padding

	var face font.Face
	var bounds image.Rectangle
	for size := renderSize - padding; size >= 24; size -= 4 {
		candidate, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(size), DPI: 72})
		if err != nil {
			return nil, err
		}
		b, _ := font.BoundString(candidate, icon.glyph)
		r := image.Rect(b.Min.X.Floor(), b.Min.Y.Floor(), b.Max.X.Ceil(), b.Max.Y.Ceil())
		if r.Dx() <= maxSize && r.Dy() <= maxSize {
			face, bounds = candidate, r
			break
		}
	}
	if face == nil {
		return nil, fmt.Errorf("glyph %s does not fit", icon.Name)
	}

	dc := gg.NewContext(renderSize, renderSize)
	dc.SetFontFace(face)
	dc.SetColor(color.RGBA{0xF8, 0xFA, 0xFC, 0xFF})
	x := float64(renderSize-bounds.Dx())/2 - float64(bounds.Min.X)
	y := float64(renderSize-bounds.Dy())/2 - float64(bounds.Min.Y)
	dc.DrawString(icon.glyph, x, y)
	return dc.Image(), nil
}

func (c *Catalog) renderEmoji(icon *Icon) (image.Image, error) {
	rc, err := c.svgs[icon.svgName].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	svg, err := oksvg.ReadIconStream(rc, oksvg.WarnErrorMode)
	if err != nil {
		return nil, err
	}
	const size, offset = 200, (renderSize - 200) / 2
	svg.SetTarget(offset, offset, size, size)
	img := image.NewRGBA(image.Rect(0, 0, renderSize, renderSize))
	scanner := rasterx.NewScannerGV(renderSize, renderSize, img, img.Bounds())
	svg.Draw(rasterx.NewDasher(renderSize, renderSize, scanner), 1)
	return img, nil
}

// Materialize writes the icon into dir (once) and returns its path, so the
// config references a regular PNG file.
func (c *Catalog) Materialize(assetID, dir string) (string, error) {
	icon, ok := c.byID[assetID]
	if !ok {
		return "", ErrNotFound
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", icon.Family, icon.Style, slugify(icon.Name)))
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}
	pngData, err := c.PNG(assetID)
	if err != nil {
		return "", err
	}
	return target, os.WriteFile(target, pngData, 0o644)
}

func slugify(s string) string {
	var parts []string
	var cur strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
			continue
		}
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	if len(parts) == 0 {
		return "asset"
	}
	return strings.Join(parts, "-")
}

func lowerAll(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = strings.ToLower(s)
	}
	return out
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range items {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
