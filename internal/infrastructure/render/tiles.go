package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"

	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"

	"github.com/maffmeier/ulanzi-deck/internal/domain/deck"
)

// opentype faces keep internal scratch buffers and are not safe for
// concurrent use; the daemon and the web preview render in parallel.
var renderMu sync.Mutex

const (
	realIconPadding  = 5
	textTilePadding  = 16
	infoTextPadding  = 24
	uploadIconMargin = 5
)

func ParseHexColor(value string) color.RGBA {
	cleaned := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(cleaned) != 6 {
		return color.RGBA{A: 255}
	}
	v, err := strconv.ParseUint(cleaned, 16, 32)
	if err != nil {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: c}, image.Point{}, draw.Src)
	return img
}

// SolidPNG renders a single-color tile, e.g. black for empty buttons.
func SolidPNG(w, h int, hex string) []byte {
	return encodePNG(solid(w, h, ParseHexColor(hex)))
}

// contain scales src to fit within maxW x maxH, keeping its aspect ratio.
func contain(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	scale := math.Min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy()))
	w := max(1, int(math.Round(float64(b.Dx())*scale)))
	h := max(1, int(math.Round(float64(b.Dy())*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// FitOnTile centers src on an opaque tile; the firmware renders
// transparent pixels unpredictably, so every upload is flattened.
func FitOnTile(src image.Image, w, h, padding int, bg string) []byte {
	tile := solid(w, h, ParseHexColor(bg))
	fitted := contain(src, w-2*padding, h-2*padding)
	fb := fitted.Bounds()
	origin := image.Pt((w-fb.Dx())/2, (h-fb.Dy())/2)
	draw.Draw(tile, fb.Add(origin), fitted, image.Point{}, draw.Over)
	return encodePNG(tile)
}

func DecodeImage(r io.Reader) (image.Image, error) {
	img, _, err := image.Decode(r)
	return img, err
}

func DecodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return DecodeImage(f)
}

// NormalizeUpload converts an uploaded image to a 196x196 PNG with a small
// transparent margin, the canonical format stored in the icons directory.
func NormalizeUpload(r io.Reader) ([]byte, error) {
	src, err := DecodeImage(r)
	if err != nil {
		return nil, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, deck.IconSize, deck.IconSize))
	inner := deck.IconSize - 2*uploadIconMargin
	fitted := contain(src, inner, inner)
	fb := fitted.Bounds()
	origin := image.Pt((deck.IconSize-fb.Dx())/2, (deck.IconSize-fb.Dy())/2)
	draw.Draw(canvas, fb.Add(origin), fitted, image.Point{}, draw.Over)
	return encodePNG(canvas), nil
}

// ButtonIcon renders the PNG for a button: its icon file or data when
// present, otherwise its label as a text tile, otherwise a blank tile.
func ButtonIcon(b deck.Button) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	w, h, padding := deck.IconSize, deck.IconSize, realIconPadding
	if b.Index == deck.InfoWindowIndex {
		w, padding = deck.InfoWindowWidth, 0
	}
	bg := b.TextStyle.BackgroundColor

	if b.IconData != nil {
		if img, err := DecodeImage(bytes.NewReader(b.IconData)); err == nil {
			return FitOnTile(img, w, h, 0, bg)
		}
	}
	if b.ResolvedIcon != "" {
		if img, err := DecodeImageFile(b.ResolvedIcon); err == nil {
			return FitOnTile(img, w, h, padding, bg)
		}
	}
	if b.Label != "" {
		maxPad := textTilePadding
		if b.Index == deck.InfoWindowIndex {
			maxPad = infoTextPadding
		}
		return textTile(b.Label, b.TextStyle, w, h, w-2*maxPad, h-2*maxPad)
	}
	if b.Index == deck.InfoWindowIndex {
		return SolidPNG(w, h, bg)
	}
	return SolidPNG(w, h, "#000000")
}

type textLine struct {
	text   string
	bounds image.Rectangle
}

func measure(face font.Face, s string) image.Rectangle {
	b, _ := font.BoundString(face, s)
	return image.Rect(b.Min.X.Floor(), b.Min.Y.Floor(), b.Max.X.Ceil(), b.Max.Y.Ceil())
}

func wrapText(text string, face font.Face, maxWidth int) []string {
	var lines []string
	paragraphs := strings.Split(text, "\n")
	for _, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		current := words[0]
		for _, word := range words[1:] {
			candidate := current + " " + word
			if measure(face, candidate).Max.X <= maxWidth {
				current = candidate
				continue
			}
			lines = append(lines, current)
			current = word
		}
		lines = append(lines, current)
	}
	return lines
}

// fitText picks the largest font size (stepping down by 2) whose wrapped
// lines fit the box; mirrors the Python renderer so tiles look alike.
func fitText(text string, style deck.TextStyle, maxW, maxH int) (font.Face, []textLine, int) {
	layout := func(size int) (font.Face, []textLine, int, int, int) {
		face := Face(style.FontFamily, style.Bold, style.Italic, float64(size))
		var lines []textLine
		usedW, totalH := 0, 0
		for _, l := range wrapText(text, face, maxW) {
			b := measure(face, l)
			lines = append(lines, textLine{l, b})
			usedW = max(usedW, b.Dx())
			totalH += b.Dy()
		}
		spacing := max(6, size/6)
		totalH += spacing * max(0, len(lines)-1)
		return face, lines, spacing, usedW, totalH
	}
	for size := style.FontSize; size > 11; size -= 2 {
		face, lines, spacing, usedW, totalH := layout(size)
		if usedW <= maxW && totalH <= maxH {
			return face, lines, spacing
		}
	}
	face, lines, _, _, _ := layout(12)
	return face, lines, 4
}

func textTile(text string, style deck.TextStyle, w, h, maxW, maxH int) []byte {
	dc := gg.NewContext(w, h)
	dc.SetColor(ParseHexColor(style.BackgroundColor))
	dc.Clear()

	face, lines, spacing := fitText(text, style, maxW, maxH)
	dc.SetFontFace(face)
	fg := ParseHexColor(style.TextColor)
	dc.SetColor(fg)

	total := 0
	for _, l := range lines {
		total += l.bounds.Dy()
	}
	total += spacing * max(0, len(lines)-1)
	y := float64(h-total) / 2

	for _, l := range lines {
		width := l.bounds.Dx()
		x := float64(w-width)/2 - float64(l.bounds.Min.X)
		baseline := y - float64(l.bounds.Min.Y)
		dc.DrawString(l.text, x, baseline)
		if style.Underline {
			uy := y + float64(l.bounds.Dy()) + 2
			dc.SetLineWidth(float64(max(1, style.FontSize/18)))
			dc.DrawLine(x+float64(l.bounds.Min.X), uy, x+float64(l.bounds.Max.X), uy)
			dc.Stroke()
		}
		y += float64(l.bounds.Dy() + spacing)
	}
	return encodePNG(dc.Image())
}
