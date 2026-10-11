package d200

import (
	"image"
	"image/color"
	"image/gif"
	"os"
	"path/filepath"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// GIF_MANUAL=1 uploads an animated GIF to key 0 through BuildButtonsZip;
// every frame has its own palette, the harder case for the firmware.
func TestManualAnimatedGIF(t *testing.T) {
	if os.Getenv("GIF_MANUAL") == "" {
		t.Skip("set GIF_MANUAL=1 with a D200 attached")
	}
	anim := &gif.GIF{}
	colors := []color.RGBA{{220, 38, 38, 255}, {234, 179, 8, 255}, {22, 163, 74, 255}, {37, 99, 235, 255}}
	for i, c := range colors {
		pal := color.Palette{c, color.White, color.Black}
		frame := image.NewPaletted(image.Rect(0, 0, 64, 64), pal)
		for y := 24; y < 40; y++ {
			for x := i * 16; x < i*16+16; x++ {
				frame.SetColorIndex(x, y, 1)
			}
		}
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 40)
	}
	path := filepath.Join(t.TempDir(), "manual.gif")
	f, _ := os.Create(path)
	if err := gif.EncodeAll(f, anim); err != nil {
		t.Fatal(err)
	}
	f.Close()

	blob, err := BuildButtonsZip([]deck.Button{{Index: 0, ResolvedIcon: path, IconPath: path, TextStyle: deck.DefaultTextStyle()}}, false)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := OpenHID()
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	for _, frame := range Chunks(CmdPartialButtons, blob) {
		if err := tr.WriteReport(frame); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("uploaded %d bytes", len(blob))
}
