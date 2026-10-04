package catalog

import (
	"bytes"
	"image/png"
	"testing"
)

func TestCatalogRendersFontAwesomeAndEmoji(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Icons()) < 3000 {
		t.Fatalf("only %d icons", len(c.Icons()))
	}
	for _, id := range []string{"fa:solid:house", "fa:brands:github", "emoji:1f600", "emoji:2764-fe0f"} {
		data, err := c.PNG(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil || img.Bounds().Dx() != renderSize {
			t.Fatalf("%s: bad png", id)
		}
		opaque := 0
		for y := 0; y < renderSize; y += 4 {
			for x := 0; x < renderSize; x += 4 {
				if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
					opaque++
				}
			}
		}
		if opaque < 200 {
			t.Fatalf("%s: almost nothing drawn (%d samples)", id, opaque)
		}
	}
	if _, err := c.PNG("fa:solid:nope"); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestMaterializeWritesOnce(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	p1, err := c.Materialize("fa:solid:house", dir)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := c.Materialize("fa:solid:house", dir)
	if p1 != p2 {
		t.Fatal("expected same path")
	}
}
