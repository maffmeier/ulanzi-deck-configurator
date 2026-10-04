package configfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

func TestLoadsPythonExampleConfigs(t *testing.T) {
	for _, name := range []string{"deck.example.yaml", "deck.multipage.yaml"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Load(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(cfg.Pages) == 0 || !cfg.HasPage(cfg.DefaultPage) {
				t.Fatalf("unexpected pages %v default %q", cfg.PageNames(), cfg.DefaultPage)
			}
		})
	}
}

func TestLegacySchemaBecomesDefaultPage(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "deck.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultPage != deck.DefaultPageName || len(cfg.Pages) != 1 {
		t.Fatalf("got pages %v default %q", cfg.PageNames(), cfg.DefaultPage)
	}
}

func TestMultipagePreservesPageOrder(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "deck.multipage.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cfg.PageNames(), ",")
	if got != "main,media,dev" {
		t.Fatalf("page order %q", got)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	original, err := Load(filepath.Join("testdata", "deck.multipage.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(data, "testdata")
	if err != nil {
		t.Fatalf("reparse: %v\n%s", err, data)
	}
	if strings.Join(again.PageNames(), ",") != strings.Join(original.PageNames(), ",") {
		t.Fatalf("pages changed: %v", again.PageNames())
	}
	for i, p := range original.Pages {
		if len(again.Pages[i].Buttons) != len(p.Buttons) {
			t.Fatalf("page %s button count changed", p.Name)
		}
	}
	if len(again.FixedButtons) != len(original.FixedButtons) {
		t.Fatal("fixed buttons changed")
	}
	if again.SmallWindow.Enabled != original.SmallWindow.Enabled {
		t.Fatal("small window changed")
	}
}

func TestRejectsInvalidConfigs(t *testing.T) {
	cases := map[string]string{
		"unknown default page": "default_page: x\npages:\n  a:\n    buttons: []\n",
		"bad color":            "buttons:\n  - index: 0\n    text_style: {background_color: red}\n",
		"fixed index clash":    "pages:\n  a:\n    buttons: [{index: 1}]\nfixed_buttons: [{index: 1}]\n",
		"unknown action":       "buttons:\n  - index: 0\n    action: {type: dance}\n",
		"empty shell command":  "buttons:\n  - index: 0\n    action: {type: shell}\n",
		"interval too slow":    "small_window: {interval_s: 9}\nbuttons: []\n",
		"too many metrics":     "small_window: {metrics_items: [cpu, memory, gpu, disk]}\nbuttons: []\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(text), "."); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestOutOfRangeButtonsAreIgnored(t *testing.T) {
	cfg, err := Parse([]byte("buttons:\n  - index: 0\n  - index: 42\n"), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Pages[0].Buttons) != 1 {
		t.Fatalf("got %d buttons", len(cfg.Pages[0].Buttons))
	}
}

func TestRelativeIconPathResolvesAgainstConfigDir(t *testing.T) {
	cfg, err := Parse([]byte("buttons:\n  - index: 0\n    icon: icons/a.png\n"), "/etc/deck")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Pages[0].Buttons[0]
	if b.IconPath != "icons/a.png" || b.ResolvedIcon != filepath.Clean("/etc/deck/icons/a.png") {
		t.Fatalf("got %q / %q", b.IconPath, b.ResolvedIcon)
	}
}

func TestCompactPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := CompactPath(filepath.Join(home, "x", "y.png")); got != "~/x/y.png" {
		t.Fatalf("got %q", got)
	}
	if got := ExpandHome("~/x"); got != filepath.Join(home, "x") {
		t.Fatalf("got %q", got)
	}
}

func TestWriteAtomicAndVersionedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deck.yaml")
	if err := WriteAtomic(path, []byte("a: 1\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "a: 1\n" {
		t.Fatalf("content %q", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}
