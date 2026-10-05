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

func TestPressModesRoundTrip(t *testing.T) {
	text := `buttons:
  - index: 0
    action: {type: predefined_command, command_id: media_play_pause}
    long_press: {type: predefined_command, command_id: media_next}
  - index: 1
    action: {type: predefined_command, command_id: audio_volume_up}
    press: repeat
  - index: 2
    action: {type: shortcut, keys: ctrl+shift+m}
    press: hold
`
	cfg, err := Parse([]byte(text), ".")
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Pages[0].Buttons
	if b[0].LongPress == nil || b[0].LongPress.CommandID != "media_next" || b[1].Press != deck.PressRepeat || b[2].Press != deck.PressHold {
		t.Fatalf("parsed %+v", b)
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "press: tap") {
		t.Fatal("default press mode must be omitted")
	}
	again, err := Parse(data, ".")
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if again.Pages[0].Buttons[2].Press != deck.PressHold || again.Pages[0].Buttons[0].LongPress == nil {
		t.Fatal("round trip lost press settings")
	}
	if _, err := Parse([]byte("buttons:\n  - index: 0\n    action: {type: switch_page, page: x}\n    press: hold\n"), "."); err == nil {
		t.Fatal("hold on switch_page must fail")
	}
}

func TestWidgetsRoundTrip(t *testing.T) {
	text := `small_window:
  enabled: true
  rotate_every_s: 8
  widgets:
    - {type: clock, format: '%H:%M'}
    - {type: command, title: Wetter, cmd: "curl -s wttr.in?format=3", interval_s: 600}
    - {type: graph, metric: cpu}
    - {type: image, paths: [icons/a.png, icons/b.png], interval_s: 3}
    - {type: timer, title: Pomodoro}
    - {type: media}
buttons:
  - index: 0
    live: {type: command, cmd: "pactl get-source-mute @DEFAULT_SOURCE@ | grep -q no", ok_color: '#14532d', fail_color: '#b91c1c'}
  - index: 1
    action: {type: timer, op: toggle, minutes: 25}
`
	cfg, err := Parse([]byte(text), "/cfg")
	if err != nil {
		t.Fatal(err)
	}
	w := cfg.SmallWindow.Widgets
	if len(w) != 6 || w[1].IntervalS != 600 || w[2].IntervalS != deck.DefaultWidgetInterval {
		t.Fatalf("widgets %+v", w)
	}
	if w[3].ResolvedPaths[0] != filepath.Clean("/cfg/icons/a.png") {
		t.Fatalf("image path %q", w[3].ResolvedPaths[0])
	}
	b := cfg.Pages[0].Buttons
	if b[0].Live == nil || b[0].Live.FailColor != "#B91C1C" || b[1].Action.Op != "toggle" || b[1].Action.Minutes != 25 {
		t.Fatalf("buttons %+v", b)
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(data, "/cfg")
	if err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if len(again.SmallWindow.Widgets) != 6 || again.Pages[0].Buttons[0].Live == nil || again.Pages[0].Buttons[1].Action.Minutes != 25 {
		t.Fatalf("round trip lost data:\n%s", data)
	}
	for _, bad := range []string{
		"small_window: {widgets: [{type: radar}]}\nbuttons: []\n",
		"small_window: {widgets: [{type: graph, metric: gpu}]}\nbuttons: []\n",
		"buttons: [{index: 0, live: {type: image, paths: [a.png]}}]\n",
		"buttons: [{index: 0, action: {type: timer, op: explode}}]\n",
	} {
		if _, err := Parse([]byte(bad), "."); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
