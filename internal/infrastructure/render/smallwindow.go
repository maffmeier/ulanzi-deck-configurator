package render

import (
	"image"
	"image/color"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

var (
	colorPrimary = color.RGBA{248, 250, 252, 255}
	colorAccent  = color.RGBA{191, 219, 254, 255}
)

// drawText places text with its ink top-left at (x, y), like Pillow does.
func drawText(dc *gg.Context, face font.Face, s string, x, y float64) {
	dc.SetFontFace(face)
	b := measure(face, s)
	dc.DrawString(s, x, y-float64(b.Min.Y))
}

// SmallWindowClockPNG renders the already formatted time (time_format)
// centered and as large as fits, matching the editor preview.
func SmallWindowClockPNG(bg, text string) []byte {
	var lines []string
	if text != "" {
		lines = []string{text}
	}
	return TextPNG(deck.InfoWindowWidth, deck.InfoWindowHeight, bg, "", lines, true)
}

// SmallWindowMetricsPNG renders up to three "LABEL value" lines.
func SmallWindowMetricsPNG(bg string, lines []string) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := gg.NewContext(deck.InfoWindowWidth, deck.InfoWindowHeight)
	dc.SetColor(ParseHexColor(bg))
	dc.Clear()

	dc.SetColor(colorAccent)
	drawText(dc, Face("DejaVu Sans", false, false, 20), "STATS", 24, 18)

	top, spacing, defaultSize := 64, 46, 40
	if len(lines) >= 3 {
		top, spacing, defaultSize = 56, 36, 34
	}
	dc.SetColor(colorPrimary)
	for i, line := range lines {
		size := min(defaultSize, max(22, 560/max(len(line), 1)))
		drawText(dc, Face("DejaVu Sans Mono", true, false, float64(size)), line, 24, float64(top+i*spacing))
	}
	return encodePNG(dc.Image())
}

// Renderer adapts the package functions to the daemon's renderer port.
type Renderer struct{}

func (Renderer) SmallWindowClock(bg, text string) []byte { return SmallWindowClockPNG(bg, text) }
func (Renderer) SmallWindowMetrics(bg string, lines []string) []byte {
	return SmallWindowMetricsPNG(bg, lines)
}

func (Renderer) Text(w, h int, bg, title string, lines []string, mono bool) []byte {
	return TextPNG(w, h, bg, title, lines, mono)
}

func (Renderer) Graph(w, h int, bg, title, value string, samples []float64, maxValue float64) []byte {
	return GraphPNG(w, h, bg, title, value, samples, maxValue)
}

func (Renderer) Timer(w, h int, bg, title, text string, progress float64, running bool) []byte {
	return TimerPNG(w, h, bg, title, text, progress, running)
}

func (Renderer) Media(w, h int, bg, title, artist string, cover image.Image) []byte {
	return MediaPNG(w, h, bg, title, artist, cover)
}

func (Renderer) Image(w, h int, bg string, img image.Image) []byte {
	return ImagePNG(w, h, bg, img)
}
