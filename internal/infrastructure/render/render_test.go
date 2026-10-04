package render

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"ulanzi-deck/internal/domain/deck"
)

func decodeSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func TestTileSizes(t *testing.T) {
	style := deck.DefaultTextStyle()
	cases := []struct {
		name string
		data []byte
		w, h int
	}{
		{"text", ButtonIcon(deck.Button{Index: 0, Label: "Ein langer Text der umbricht", TextStyle: style}), 196, 196},
		{"blank", ButtonIcon(deck.Button{Index: 1, TextStyle: style}), 196, 196},
		{"info", ButtonIcon(deck.Button{Index: deck.InfoWindowIndex, TextStyle: style}), 392, 196},
		{"clock", SmallWindowClockPNG("#000000", time.Now()), 392, 196},
		{"metrics", SmallWindowMetricsPNG("#102030", []string{"CPU  12%", "MEM  40%", "TEMP 55C"}), 392, 196},
	}
	for _, c := range cases {
		if w, h := decodeSize(t, c.data); w != c.w || h != c.h {
			t.Fatalf("%s: %dx%d", c.name, w, h)
		}
	}
}

func TestMissingIconFallsBackToLabel(t *testing.T) {
	b := deck.Button{Index: 0, Label: "X", ResolvedIcon: "/does/not/exist.png", TextStyle: deck.DefaultTextStyle()}
	if w, _ := decodeSize(t, ButtonIcon(b)); w != 196 {
		t.Fatal("expected a text tile")
	}
}
