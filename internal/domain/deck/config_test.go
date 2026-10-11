package deck

import (
	"reflect"
	"testing"
	"time"
)

func threePages() *Config {
	return &Config{
		Pages:       []Page{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		DefaultPage: "a",
		SmallWindow: DefaultSmallWindow(),
		Brightness:  DefaultBrightness,
	}
}

func TestResolvePageTargetWrapsAround(t *testing.T) {
	cfg := threePages()
	cases := []struct{ current, target, want string }{
		{"a", PageNext, "b"},
		{"c", PageNext, "a"},
		{"a", PagePrevious, "c"},
		{"b", PagePrevious, "a"},
		{"b", "c", "c"},
	}
	for _, c := range cases {
		got, ok := cfg.ResolvePageTarget(c.current, c.target)
		if !ok || got != c.want {
			t.Fatalf("%s %s: got %q, %v", c.current, c.target, got, ok)
		}
	}
	if _, ok := cfg.ResolvePageTarget("a", "missing"); ok {
		t.Fatal("unknown page must not resolve")
	}
}

func TestFixedButtonsWinAndAppearOnEveryPage(t *testing.T) {
	cfg := threePages()
	cfg.Pages[0].Buttons = []Button{{Index: 1, Label: "page"}}
	cfg.FixedButtons = []Button{{Index: 10, Label: "fixed"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(cfg.ButtonsFor("a")) != 2 || len(cfg.ButtonsFor("b")) != 1 {
		t.Fatal("fixed buttons must be added to every page")
	}
	if b := cfg.ButtonAt("c", 10); b == nil || b.Label != "fixed" {
		t.Fatal("fixed button not resolved")
	}
	cfg.Pages[1].Buttons = []Button{{Index: 10}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("index clash with fixed button must fail")
	}
}

func TestSmallWindowValidation(t *testing.T) {
	sw := DefaultSmallWindow()
	sw.MetricsItems = []string{" CPU ", "memory"}
	got, err := sw.Normalized()
	if err != nil || got.MetricsItems[0] != "cpu" {
		t.Fatalf("got %v %v", got.MetricsItems, err)
	}
	sw.MetricsItems = []string{"cpu", "cpu"}
	if _, err := sw.Normalized(); err == nil {
		t.Fatal("duplicates must fail")
	}
}

func TestActionCleanKeepsOnlyRelevantField(t *testing.T) {
	a := Action{Type: ActionPredefined, CommandID: "media_next", Page: "@next", Cmd: "x"}.Clean()
	if !reflect.DeepEqual(a, Action{Type: ActionPredefined, CommandID: "media_next"}) {
		t.Fatalf("got %+v", a)
	}
}

func TestButtonBehaviorValidation(t *testing.T) {
	shortcut := &Action{Type: ActionShortcut, Keys: "ctrl+m"}
	page := &Action{Type: ActionSwitchPage, Page: PageNext}
	cases := []struct {
		name string
		b    Button
		ok   bool
	}{
		{"default becomes tap", Button{Action: shortcut}, true},
		{"long press with tap", Button{Action: shortcut, LongPress: page}, true},
		{"long press only", Button{LongPress: page}, true},
		{"repeat shortcut", Button{Action: shortcut, Press: PressRepeat}, true},
		{"hold shortcut", Button{Action: shortcut, Press: PressHold}, true},
		{"repeat page switch", Button{Action: page, Press: PressRepeat}, false},
		{"hold page switch", Button{Action: page, Press: PressHold}, false},
		{"long press with repeat", Button{Action: shortcut, Press: PressRepeat, LongPress: page}, false},
		{"unknown mode", Button{Action: shortcut, Press: "double"}, false},
	}
	for _, c := range cases {
		err := c.b.ValidateBehavior()
		if (err == nil) != c.ok {
			t.Fatalf("%s: got %v", c.name, err)
		}
	}
	b := Button{Action: shortcut}
	_ = b.ValidateBehavior()
	if b.Press != PressTap {
		t.Fatalf("empty press must normalize to tap, got %q", b.Press)
	}
}

func TestWidgetCleanDropsForeignFields(t *testing.T) {
	w := Widget{Type: WidgetClock, Format: "%H", Metric: "cpu", Items: []string{"cpu"}, IntervalS: 9, Cmd: "x"}.Clean()
	if w.Metric != "" || w.Items != nil || w.Cmd != "" || w.IntervalS != DefaultWidgetInterval || w.Format != "%H" {
		t.Fatalf("got %+v", w)
	}
}

func TestFolderGetsBackKeyAndIsSkippedWhenPaging(t *testing.T) {
	cfg := threePages()
	cfg.Pages = append(cfg.Pages, Page{Name: "sub", Folder: true, BackIndex: 4, Buttons: []Button{{Index: 0, Label: "x"}}})
	if got, _ := cfg.ResolvePageTarget("c", PageNext); got != "a" {
		t.Fatalf("paging must skip folders, got %q", got)
	}
	if got, _ := cfg.ResolvePageTarget("sub", PageNext); got != "a" {
		t.Fatalf("paging out of a folder starts at the first page, got %q", got)
	}
	back := cfg.ButtonAt("sub", 4)
	if back == nil || back.Action.Page != PageBack {
		t.Fatalf("folder must get a back key, got %+v", back)
	}
	if len(cfg.ButtonsFor("sub")) != 2 {
		t.Fatal("back key must be rendered")
	}
	cfg.Pages[3].Buttons = append(cfg.Pages[3].Buttons, Button{Index: 4, Label: "own"})
	if b := cfg.ButtonAt("sub", 4); b.Label != "own" {
		t.Fatal("an own key at the back index wins")
	}
}

func TestToggleFlipsVisualsAndAction(t *testing.T) {
	on := &Action{Type: ActionShell, Cmd: "on"}
	off := &Action{Type: ActionShell, Cmd: "off"}
	b := Button{Label: "An", Action: on, Toggle: &ToggleState{Label: "Aus", Action: off}}
	if err := b.ValidateBehavior(); err != nil {
		t.Fatal(err)
	}
	if f := b.Flipped(); f.Label != "Aus" || f.Action.Cmd != "off" {
		t.Fatalf("flipped %+v", f)
	}
	b.Press = PressRepeat
	if err := b.ValidateBehavior(); err == nil {
		t.Fatal("toggle with repeat must fail")
	}
}

func TestActionValidation(t *testing.T) {
	ok := []Action{
		{Type: ActionHTTP, URL: "https://example.com/hook", Method: "post"},
		{Type: ActionTypeText, Text: "hallo"},
		{Type: ActionMulti, Steps: []Action{{Type: ActionShell, Cmd: "a"}, {Type: ActionDelay, Ms: 200}, {Type: ActionSwitchPage, Page: PageBack}}},
		{Type: ActionHome, Service: "light.toggle", EntityID: "light.kitchen"},
		{Type: ActionOBS, Op: "switch_scene", Target: "Kamera"},
		{Type: ActionOBS, Op: "toggle_record"},
	}
	for _, a := range ok {
		if err := a.Validate(); err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	bad := []Action{
		{Type: ActionHTTP, URL: "ftp://x"},
		{Type: ActionHTTP, URL: "https://x", Method: "TRACE"},
		{Type: ActionDelay, Ms: 100},
		{Type: ActionMulti},
		{Type: ActionMulti, Steps: []Action{{Type: ActionMulti, Steps: []Action{{Type: ActionShell, Cmd: "a"}}}}},
		{Type: ActionMulti, Steps: []Action{{Type: ActionDelay, Ms: 0}}},
		{Type: ActionHome, Service: "toggle"},
		{Type: ActionOBS, Op: "switch_scene"},
		{Type: ActionOBS, Op: "explode"},
	}
	for _, a := range bad {
		if err := a.Validate(); err == nil {
			t.Fatalf("%+v must fail", a)
		}
	}
}

func TestTargetBrightnessRules(t *testing.T) {
	cfg := threePages()
	cfg.Brightness = 80
	cfg.Display = Display{DimAfterS: 60, DimBrightness: 10, OffWhenLocked: true, Night: &NightMode{From: "22:00", To: "07:00", Brightness: 30}}
	day := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	night := time.Date(2026, 1, 1, 23, 30, 0, 0, time.UTC)
	early := time.Date(2026, 1, 1, 6, 59, 0, 0, time.UTC)
	cases := []struct {
		now    time.Time
		idle   time.Duration
		locked bool
		want   int
	}{
		{day, 0, false, 80},
		{day, 2 * time.Minute, false, 10},
		{night, 0, false, 30},
		{early, 0, false, 30},
		{night, 2 * time.Minute, false, 10},
		{day, 0, true, 0},
	}
	for i, c := range cases {
		if got := cfg.TargetBrightness(c.now, c.idle, c.locked); got != c.want {
			t.Fatalf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestPageForApp(t *testing.T) {
	cfg := threePages()
	cfg.Pages[1].Apps = []string{"code", "jetbrains"}
	cfg.Pages[2].Apps = []string{"OBS"}
	if p, ok := cfg.PageForApp("Code"); !ok || p != "b" {
		t.Fatalf("got %q", p)
	}
	if p, _ := cfg.PageForApp("com.obsproject.Studio"); p != "c" {
		t.Fatalf("got %q", p)
	}
	if _, ok := cfg.PageForApp("firefox"); ok {
		t.Fatal("no match expected")
	}
}

func TestAudioActionValidation(t *testing.T) {
	ok := []Action{
		{Type: ActionAudio, Op: AudioVolumeUp},
		{Type: ActionAudio, Op: AudioMute, Target: "spotify"},
		{Type: ActionAudio, Op: AudioOutputSet, Target: "Headset"},
	}
	for _, a := range ok {
		if err := a.Validate(); err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	bad := []Action{
		{Type: ActionAudio, Op: "louder"},
		{Type: ActionAudio, Op: AudioOutputSet},
		{Type: ActionAudio, Op: AudioVolumeUp, Step: 300},
	}
	for _, a := range bad {
		if a.Validate() == nil {
			t.Fatalf("%+v must fail", a)
		}
	}
	clean := Action{Type: ActionAudio, Op: AudioMicMute, Target: "x", Step: 5}.Clean()
	if clean.Target != "" || clean.Step != 0 {
		t.Fatalf("clean %+v", clean)
	}
}
