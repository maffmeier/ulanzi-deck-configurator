package render

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

const (
	maxGIFFrames = 120
	// The firmware animates GIFs on its own, but every page switch uploads
	// them again; larger files fall back to the first frame.
	MaxAnimatedBytes = 1536 << 10
)

// IsAnimatedGIF reports whether data is a GIF with more than one frame.
func IsAnimatedGIF(data []byte) bool {
	cfg, err := gif.DecodeAll(bytes.NewReader(data))
	return err == nil && len(cfg.Image) > 1
}

type gifKey struct {
	path, bg string
	modified time.Time
	size     int64
}

type gifResult struct {
	data []byte
	ok   bool
}

var (
	gifCacheMu sync.Mutex
	gifCache   = map[gifKey]gifResult{}
)

// AnimatedButtonIcon renders the icon GIF of b frame by frame onto the key
// tile. ok is false when the icon is not an animated GIF.
func AnimatedButtonIcon(b deck.Button) (data []byte, ok bool) {
	if b.ResolvedIcon == "" || b.Index == deck.InfoWindowIndex || !strings.EqualFold(filepath.Ext(b.ResolvedIcon), ".gif") {
		return nil, false
	}
	info, err := os.Stat(b.ResolvedIcon)
	if err != nil {
		return nil, false
	}
	key := gifKey{path: b.ResolvedIcon, bg: b.TextStyle.BackgroundColor, modified: info.ModTime(), size: info.Size()}
	gifCacheMu.Lock()
	cached, hit := gifCache[key]
	gifCacheMu.Unlock()
	if hit {
		return cached.data, cached.ok
	}

	f, err := os.Open(b.ResolvedIcon)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	data, ok = animatedTile(f, deck.IconSize, deck.IconSize, realIconPadding, b.TextStyle.BackgroundColor)
	gifCacheMu.Lock()
	if len(gifCache) > 64 {
		clear(gifCache)
	}
	gifCache[key] = gifResult{data, ok}
	gifCacheMu.Unlock()
	return data, ok
}

func animatedTile(r io.Reader, w, h, padding int, bg string) ([]byte, bool) {
	src, err := gif.DecodeAll(r)
	if err != nil || len(src.Image) < 2 {
		return nil, false
	}
	renderMu.Lock()
	defer renderMu.Unlock()

	frames := coalesce(src)
	if len(frames) > maxGIFFrames {
		frames = frames[:maxGIFFrames]
	}
	background := ParseHexColor(bg)
	out := &gif.GIF{LoopCount: src.LoopCount}
	for i, frame := range frames {
		tile := solid(w, h, background)
		fitted := contain(frame, w-2*padding, h-2*padding)
		fb := fitted.Bounds()
		draw.Draw(tile, fb.Add(image.Pt((w-fb.Dx())/2, (h-fb.Dy())/2)), fitted, image.Point{}, draw.Over)

		pal := framePalette(src.Image[i].Palette, background)
		paletted := image.NewPaletted(tile.Bounds(), pal)
		draw.FloydSteinberg.Draw(paletted, paletted.Bounds(), tile, image.Point{})
		out.Image = append(out.Image, paletted)
		out.Delay = append(out.Delay, src.Delay[i])
		out.Disposal = append(out.Disposal, gif.DisposalNone)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil || buf.Len() > MaxAnimatedBytes {
		return nil, false
	}
	return buf.Bytes(), true
}

// coalesce turns GIF frames, which may only cover part of the canvas and
// depend on the previous one, into full images.
func coalesce(g *gif.GIF) []image.Image {
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	if bounds.Empty() {
		bounds = g.Image[0].Bounds()
	}
	canvas := image.NewRGBA(bounds)
	frames := make([]image.Image, 0, len(g.Image))
	for i, frame := range g.Image {
		var previous *image.RGBA
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		if disposal == gif.DisposalPrevious {
			previous = image.NewRGBA(bounds)
			draw.Draw(previous, bounds, canvas, bounds.Min, draw.Src)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		snapshot := image.NewRGBA(bounds)
		draw.Draw(snapshot, bounds, canvas, bounds.Min, draw.Src)
		frames = append(frames, snapshot)

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = previous
		}
	}
	return frames
}

// framePalette keeps the GIF's own colors (better than a generic palette)
// and adds the tile background, which shows around the scaled image.
func framePalette(src color.Palette, bg color.RGBA) color.Palette {
	pal := make(color.Palette, 0, 256)
	pal = append(pal, bg)
	for _, c := range src {
		if len(pal) == 256 {
			break
		}
		if _, _, _, a := c.RGBA(); a == 0 {
			continue
		}
		pal = append(pal, c)
	}
	return pal
}
