package render

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
)

const (
	widgetPadding = 14
	titleSize     = 18
)

var colorMuted = color.RGBA{148, 163, 184, 255}

func newCanvas(w, h int, bg string) *gg.Context {
	dc := gg.NewContext(w, h)
	dc.SetColor(ParseHexColor(bg))
	dc.Clear()
	return dc
}

// drawTitle draws a small heading and returns the y below it.
func drawTitle(dc *gg.Context, title string) float64 {
	if title == "" {
		return widgetPadding
	}
	face := Face("DejaVu Sans", false, false, titleSize)
	title = ellipsize(face, title, dc.Width()-2*widgetPadding)
	dc.SetColor(colorAccent)
	drawText(dc, face, title, widgetPadding, widgetPadding-2)
	return widgetPadding + titleSize + 6
}

func ellipsize(face font.Face, s string, maxW int) string {
	if measure(face, s).Dx() <= maxW {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		candidate := strings.TrimRight(string(runes), " ") + "…"
		if measure(face, candidate).Dx() <= maxW {
			return candidate
		}
	}
	return "…"
}

// fitLines picks the largest size (≤ maxSize) at which all lines fit the
// box; lines that are still too wide at the minimum size get ellipsized.
func fitLines(lines []string, family string, bold bool, maxW, maxH, maxSize int) (font.Face, []string, int) {
	for size := maxSize; size >= 14; size -= 2 {
		face := Face(family, bold, false, float64(size))
		spacing := max(4, size/5)
		total, fits := 0, true
		for _, l := range lines {
			b := measure(face, l)
			if b.Dx() > maxW {
				fits = false
				break
			}
			total += lineHeight(face)
		}
		total += spacing * max(0, len(lines)-1)
		if fits && total <= maxH {
			return face, lines, spacing
		}
	}
	face := Face(family, bold, false, 14)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ellipsize(face, l, maxW)
	}
	// Drop lines that don't fit vertically at all.
	maxLines := max(1, maxH/(lineHeight(face)+4))
	if len(out) > maxLines {
		out = out[:maxLines]
	}
	return face, out, 4
}

// lineHeight uses the font metrics instead of the ink box so lines with
// and without descenders get the same spacing.
func lineHeight(face font.Face) int {
	m := face.Metrics()
	return (m.Ascent + m.Descent).Ceil()
}

func drawLinesCentered(dc *gg.Context, face font.Face, lines []string, spacing int, top, bottom float64, fg color.Color) {
	m := face.Metrics()
	lh := float64(lineHeight(face))
	total := lh*float64(len(lines)) + float64(spacing*max(0, len(lines)-1))
	y := top + (bottom-top-total)/2
	dc.SetFontFace(face)
	dc.SetColor(fg)
	for _, l := range lines {
		b := measure(face, l)
		x := float64(dc.Width()-b.Dx())/2 - float64(b.Min.X)
		dc.DrawString(l, x, y+float64(m.Ascent.Ceil()))
		y += lh + float64(spacing)
	}
}

// TextPNG renders a title and up to a few lines of text, centered and as
// large as fits. Used for clock, command output and metrics.
func TextPNG(w, h int, bg, title string, lines []string, mono bool) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := newCanvas(w, h, bg)
	top := drawTitle(dc, title)
	if len(lines) == 0 {
		return encodePNG(dc.Image())
	}
	family := "DejaVu Sans"
	if mono {
		family = "DejaVu Sans Mono"
	}
	maxSize := 120
	if len(lines) > 1 {
		maxSize = 64
	}
	face, fitted, spacing := fitLines(lines, family, true, w-2*widgetPadding, h-int(top)-widgetPadding, maxSize)
	drawLinesCentered(dc, face, fitted, spacing, top, float64(h-widgetPadding), colorPrimary)
	return encodePNG(dc.Image())
}

// GraphPNG plots samples (oldest first) scaled to maxValue, with the
// current value in the corner.
func GraphPNG(w, h int, bg, title, value string, samples []float64, maxValue float64) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := newCanvas(w, h, bg)
	top := drawTitle(dc, title)
	valueFace := Face("DejaVu Sans Mono", true, false, float64(min(40, h/5)))
	vb := measure(valueFace, value)
	dc.SetColor(colorPrimary)
	drawText(dc, valueFace, value, float64(w-widgetPadding-vb.Dx()), widgetPadding-2)
	top = math.Max(top, widgetPadding+float64(vb.Dy())+8)

	left, right := float64(widgetPadding), float64(w-widgetPadding)
	bottom := float64(h - widgetPadding)
	dc.SetColor(color.RGBA{255, 255, 255, 30})
	dc.SetLineWidth(1)
	for _, f := range []float64{0, 0.5, 1} {
		y := bottom - (bottom-top)*f
		dc.DrawLine(left, y, right, y)
		dc.Stroke()
	}
	if len(samples) < 2 || maxValue <= 0 {
		return encodePNG(dc.Image())
	}

	point := func(i int) (float64, float64) {
		x := left + (right-left)*float64(i)/float64(len(samples)-1)
		v := math.Min(1, math.Max(0, samples[i]/maxValue))
		return x, bottom - (bottom-top)*v
	}
	dc.MoveTo(left, bottom)
	for i := range samples {
		dc.LineTo(point(i))
	}
	dc.LineTo(right, bottom)
	dc.ClosePath()
	dc.SetColor(color.RGBA{96, 165, 250, 70})
	dc.Fill()

	for i := range samples {
		x, y := point(i)
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.SetColor(color.RGBA{96, 165, 250, 255})
	dc.SetLineWidth(3)
	dc.Stroke()
	return encodePNG(dc.Image())
}

// TimerPNG shows the remaining time and a progress bar.
func TimerPNG(w, h int, bg, title, text string, progress float64, running bool) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := newCanvas(w, h, bg)
	top := drawTitle(dc, title)
	barH := 10.0
	barY := float64(h-widgetPadding) - barH
	fg := color.Color(colorPrimary)
	if !running {
		fg = colorMuted
	}
	face, lines, spacing := fitLines([]string{text}, "DejaVu Sans Mono", true, w-2*widgetPadding, int(barY-top)-8, 120)
	drawLinesCentered(dc, face, lines, spacing, top, barY-8, fg)

	left, width := float64(widgetPadding), float64(w-2*widgetPadding)
	dc.SetColor(color.RGBA{255, 255, 255, 40})
	dc.DrawRoundedRectangle(left, barY, width, barH, barH/2)
	dc.Fill()
	if progress > 0 {
		dc.SetColor(color.RGBA{96, 165, 250, 255})
		dc.DrawRoundedRectangle(left, barY, width*math.Min(1, progress), barH, barH/2)
		dc.Fill()
	}
	return encodePNG(dc.Image())
}

// MediaPNG shows the cover (if any) next to title and artist. On square
// keys the cover fills the tile and the text is left out.
func MediaPNG(w, h int, bg, title, artist string, cover image.Image) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()

	dc := newCanvas(w, h, bg)
	square := w == h
	textLeft := float64(widgetPadding)
	if cover != nil {
		size := h - 2*widgetPadding
		if square {
			size = h
		}
		fitted := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.CatmullRom.Scale(fitted, fitted.Bounds(), cover, cover.Bounds(), draw.Src, nil)
		x, y := widgetPadding, widgetPadding
		if square {
			x, y = 0, 0
		}
		dc.DrawImage(fitted, x, y)
		if square {
			return encodePNG(dc.Image())
		}
		textLeft = float64(x + size + widgetPadding)
	}
	if title == "" {
		title = "Keine Wiedergabe"
	}
	maxW := w - int(textLeft) - widgetPadding
	titleFace := Face("DejaVu Sans", true, false, float64(min(30, h/6)))
	artistFace := Face("DejaVu Sans", false, false, float64(min(22, h/8)))
	lines := []struct {
		face font.Face
		text string
		col  color.Color
	}{
		{titleFace, ellipsize(titleFace, title, maxW), colorPrimary},
		{artistFace, ellipsize(artistFace, artist, maxW), colorMuted},
	}
	total := float64(lineHeight(titleFace) + 8 + lineHeight(artistFace))
	y := (float64(h) - total) / 2
	for _, l := range lines {
		if l.text == "" {
			continue
		}
		dc.SetFontFace(l.face)
		dc.SetColor(l.col)
		if square {
			b := measure(l.face, l.text)
			dc.DrawString(l.text, float64(w-b.Dx())/2-float64(b.Min.X), y+float64(l.face.Metrics().Ascent.Ceil()))
		} else {
			dc.DrawString(l.text, textLeft, y+float64(l.face.Metrics().Ascent.Ceil()))
		}
		y += float64(lineHeight(l.face) + 8)
	}
	return encodePNG(dc.Image())
}

// ImagePNG fits an image onto the widget area.
func ImagePNG(w, h int, bg string, img image.Image) []byte {
	renderMu.Lock()
	defer renderMu.Unlock()
	return FitOnTile(img, w, h, 0, bg)
}
