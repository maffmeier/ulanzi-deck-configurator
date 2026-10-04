package render

import (
	"os"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck/internal/infrastructure/catalog"
)

// Writes sample images for visual inspection when PREVIEW_DIR is set.
func TestWritePreviews(t *testing.T) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("PREVIEW_DIR not set")
	}
	style := deck.DefaultTextStyle()
	bold := style
	bold.Bold, bold.Underline, bold.BackgroundColor = true, true, "#7C3AED"
	files := map[string][]byte{
		"text.png":    ButtonIcon(deck.Button{Index: 0, Label: "Weiter → Seite", TextStyle: style}),
		"bold.png":    ButtonIcon(deck.Button{Index: 0, Label: "Mute", TextStyle: bold}),
		"clock.png":   SmallWindowClockPNG("#000000", time.Date(2026, 10, 4, 10, 8, 30, 0, time.Local)),
		"metrics.png": SmallWindowMetricsPNG("#0B1220", []string{"CPU  12%", "MEM  40%", "TEMP 55C | 61C"}),
	}
	c, _ := catalog.Load()
	for _, id := range []string{"fa:brands:github", "emoji:1f600", "fa:solid:volume-high"} {
		files[id+".png"], _ = c.PNG(id)
	}
	for name, data := range files {
		if err := os.WriteFile(dir+"/"+name, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
