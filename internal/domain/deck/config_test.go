package deck

import "testing"

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
	if a != (Action{Type: ActionPredefined, CommandID: "media_next"}) {
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
