package render

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// writeGIF writes a 40x20 GIF whose second frame only covers the right half.
func writeGIF(t *testing.T, frames int) string {
	t.Helper()
	anim := &gif.GIF{}
	for i := range frames {
		rect := image.Rect(0, 0, 40, 20)
		if i > 0 {
			rect = image.Rect(20, 0, 40, 20)
		}
		f := image.NewPaletted(rect, palette.Plan9)
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				f.Set(x, y, color.RGBA{R: uint8(200 * (i % 2)), G: 120, A: 255})
			}
		}
		anim.Image = append(anim.Image, f)
		anim.Delay = append(anim.Delay, 10)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, anim); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "anim.gif")
	os.WriteFile(path, buf.Bytes(), 0o644)
	return path
}

func TestAnimatedButtonIconKeepsFramesAtKeySize(t *testing.T) {
	b := deck.Button{Index: 0, ResolvedIcon: writeGIF(t, 3), TextStyle: deck.DefaultTextStyle()}
	data, ok := AnimatedButtonIcon(b)
	if !ok {
		t.Fatal("expected an animated tile")
	}
	out, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Image) != 3 || out.Image[1].Bounds() != image.Rect(0, 0, deck.IconSize, deck.IconSize) {
		t.Fatalf("%d frames, bounds %v", len(out.Image), out.Image[1].Bounds())
	}
	// The partial second frame is composed onto the first one.
	if _, _, _, a := out.Image[1].At(30, deck.IconSize/2).RGBA(); a == 0 {
		t.Fatal("left half of frame 2 lost")
	}
}

func TestSingleFrameGIFIsNotAnimated(t *testing.T) {
	b := deck.Button{Index: 0, ResolvedIcon: writeGIF(t, 1), TextStyle: deck.DefaultTextStyle()}
	if _, ok := AnimatedButtonIcon(b); ok {
		t.Fatal("single frame must use the PNG path")
	}
	data, _ := os.ReadFile(b.ResolvedIcon)
	if IsAnimatedGIF(data) {
		t.Fatal("IsAnimatedGIF")
	}
}

func TestPressedIconHasKeySize(t *testing.T) {
	img, err := DecodeImage(bytes.NewReader(PressedIcon(deck.Button{Index: 0, Label: "A", TextStyle: deck.DefaultTextStyle()})))
	if err != nil || img.Bounds().Dx() != deck.IconSize {
		t.Fatalf("pressed icon: %v %v", err, img)
	}
}
