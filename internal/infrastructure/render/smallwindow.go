package render

import (
	"image/color"
	"math"
	"time"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

var (
	colorPrimary   = color.RGBA{248, 250, 252, 255}
	colorAccent    = color.RGBA{191, 219, 254, 255}
	colorSecondary = color.RGBA{148, 163, 184, 255}
	colorSecond    = color.RGBA{248, 113, 113, 255}
	colorTick      = color.RGBA{241, 245, 249, 255}
	colorFace      = color.RGBA{11, 18, 32, 90}
)

func drawClockFace(dc *gg.Context, cx, cy, radius float64, t time.Time) {
	dc.DrawCircle(cx, cy, radius)
	dc.SetColor(colorFace)
	dc.FillPreserve()
	dc.SetColor(colorPrimary)
	dc.SetLineWidth(4)
	dc.Stroke()

	dc.DrawCircle(cx, cy, radius-10)
	dc.SetColor(colorSecondary)
	dc.SetLineWidth(2)
	dc.Stroke()

	dc.SetColor(colorTick)
	for tick := range 60 {
		angle := gg.Radians(float64(tick*6 - 90))
		outer := radius - 10
		length, width := 7.0, 1.0
		if tick%5 == 0 {
			length, width = 14, 3
		}
		inner := outer - length
		dc.SetLineWidth(width)
		dc.DrawLine(cx+math.Cos(angle)*inner, cy+math.Sin(angle)*inner,
			cx+math.Cos(angle)*outer, cy+math.Sin(angle)*outer)
		dc.Stroke()
	}

	hand := func(deg, length, width float64, c color.Color) {
		angle := gg.Radians(deg - 90)
		dc.SetColor(c)
		dc.SetLineWidth(width)
		dc.DrawLine(cx, cy, cx+math.Cos(angle)*length, cy+math.Sin(angle)*length)
		dc.Stroke()
	}
	h, m, s := float64(t.Hour()%12), float64(t.Minute()), float64(t.Second())
	hand((h+m/60)*30, radius*0.48, 6, colorPrimary)
	hand((m+s/60)*6, radius*0.7, 4, colorAccent)
	hand(s*6, radius*0.78, 2, colorSecond)

	dc.SetColor(colorPrimary)
	dc.DrawCircle(cx, cy, 5)
	dc.Fill()
}

// drawText places text with its ink top-left at (x, y), like Pillow does.
func drawText(dc *gg.Context, face font.Face, s string, x, y float64) {
	dc.SetFontFace(face)
	b := measure(face, s)
	dc.DrawString(s, x, y-float64(b.Min.Y))
}

// SmallWindowClockPNG renders the wide strip with an analog and a digital
// clock, used when custom metrics take over the info window.
func SmallWindowClockPNG(bg string, t time.Time) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := gg.NewContext(deck.InfoWindowWidth, deck.InfoWindowHeight)
	dc.SetColor(ParseHexColor(bg))
	dc.Clear()
	drawClockFace(dc, 98, 98, 72, t)

	label := Face("DejaVu Sans", false, false, 20)
	dc.SetColor(colorAccent)
	drawText(dc, label, "CLOCK", 190, 42)
	dc.SetColor(colorSecondary)
	drawText(dc, label, t.Format("15:04:05"), 190, 136)
	dc.SetColor(colorPrimary)
	drawText(dc, Face("DejaVu Sans Mono", true, false, 54), t.Format("15:04"), 190, 74)
	return encodePNG(dc.Image())
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

func (Renderer) SmallWindowClock(bg string, t time.Time) []byte { return SmallWindowClockPNG(bg, t) }
func (Renderer) SmallWindowMetrics(bg string, lines []string) []byte {
	return SmallWindowMetricsPNG(bg, lines)
}
