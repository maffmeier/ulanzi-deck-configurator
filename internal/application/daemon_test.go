package application

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type fakeDeck struct {
	mu        sync.Mutex
	events    chan deck.Event
	uploads   [][]deck.Button
	partials  [][]deck.Button
	modes     []deck.SmallWindowMode
	data      int
	keepAlive int
}

func newFakeDeck() *fakeDeck { return &fakeDeck{events: make(chan deck.Event, 8)} }

func (f *fakeDeck) Events() <-chan deck.Event     { return f.events }
func (f *fakeDeck) Connected() bool               { return true }
func (f *fakeDeck) SetBrightness(int, bool) error { return nil }
func (f *fakeDeck) KeepAlive() error              { f.mu.Lock(); f.keepAlive++; f.mu.Unlock(); return nil }
func (f *fakeDeck) SetSmallWindowData(_, _, _ int, _ string) error {
	f.mu.Lock()
	f.data++
	f.mu.Unlock()
	return nil
}
func (f *fakeDeck) SetSmallWindowMode(m deck.SmallWindowMode) error {
	f.mu.Lock()
	f.modes = append(f.modes, m)
	f.mu.Unlock()
	return nil
}
func (f *fakeDeck) SetButtons(b []deck.Button, partial bool) error {
	f.mu.Lock()
	if !partial {
		f.uploads = append(f.uploads, b)
	} else {
		f.partials = append(f.partials, b)
	}
	f.mu.Unlock()
	return nil
}

// partialData returns the IconData uploaded for index by partial updates.
func (f *fakeDeck) partialData(index int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, batch := range f.partials {
		for _, b := range batch {
			if b.Index == index && b.IconData != nil {
				out = append(out, string(b.IconData))
			}
		}
	}
	return out
}

func (f *fakeDeck) lastUploadLabel(index int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.uploads) == 0 {
		return ""
	}
	for _, b := range f.uploads[len(f.uploads)-1] {
		if b.Index == index {
			return b.Label
		}
	}
	return ""
}

type fakeRunner struct {
	mu       sync.Mutex
	ran      []deck.Action
	pressed  []string
	released []string
}

func (r *fakeRunner) Press(a deck.Action) (func(), error) {
	r.mu.Lock()
	r.pressed = append(r.pressed, a.Keys)
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		r.released = append(r.released, a.Keys)
		r.mu.Unlock()
	}, nil
}

func (r *fakeRunner) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, a := range r.ran {
		out = append(out, a.Cmd)
	}
	return out
}

func (r *fakeRunner) Run(a deck.Action) error {
	r.mu.Lock()
	r.ran = append(r.ran, a)
	r.mu.Unlock()
	return nil
}

type fakeMetrics struct{}

func (fakeMetrics) FormatTime(string) string                 { return "12:34" }
func (fakeMetrics) CPUPercent() int                          { return 10 }
func (fakeMetrics) MemoryPercent() int                       { return 20 }
func (fakeMetrics) MetricValue(string) string                { return "1%" }
func (fakeMetrics) NetworkBytesPerSec() (float64, bool)      { return 2048, true }
func (fakeMetrics) TemperatureValue([]string, string) string { return "40C" }

type fakeRenderer struct{}

func (fakeRenderer) SmallWindowClock(string, string) []byte { return []byte("png") }
func (fakeRenderer) Text(_, _ int, bg, title string, lines []string, _ bool) []byte {
	return []byte(fmt.Sprintf("text|%s|%s|%v", bg, title, lines))
}
func (fakeRenderer) Graph(_, _ int, _, title, value string, samples []float64, _ float64) []byte {
	return []byte(fmt.Sprintf("graph|%s|%s|%d", title, value, len(samples)))
}
func (fakeRenderer) Timer(_, _ int, _, title, text string, _ float64, running bool) []byte {
	return []byte(fmt.Sprintf("timer|%s|%s|%v", title, text, running))
}
func (fakeRenderer) Media(_, _ int, _, title, artist string, _ image.Image) []byte {
	return []byte("media|" + title + "|" + artist)
}
func (fakeRenderer) Image(int, int, string, image.Image) []byte { return []byte("image") }
func (fakeRenderer) SmallWindowMetrics(string, []string) []byte { return []byte("png") }

func pagedConfig() *deck.Config {
	next := &deck.Action{Type: deck.ActionSwitchPage, Page: deck.PageNext}
	return &deck.Config{
		Pages: []deck.Page{
			{Name: "one", Buttons: []deck.Button{{Index: 0, Label: "first"}, {Index: 1, Action: &deck.Action{Type: deck.ActionShell, Cmd: "true"}}}},
			{Name: "two", Buttons: []deck.Button{{Index: 0, Label: "second"}}},
		},
		FixedButtons: []deck.Button{{Index: 12, Label: "next", Action: next}},
		DefaultPage:  "one",
		SmallWindow:  deck.DefaultSmallWindow(),
		Brightness:   50,
	}
}

func startDaemon(t *testing.T, cfg *deck.Config) (*Daemon, *fakeDeck, *fakeRunner) {
	t.Helper()
	dev, runner := newFakeDeck(), &fakeRunner{}
	log := slog.New(slog.DiscardHandler)
	engine := NewWidgetEngine(fakeMetrics{}, &fakeSources{}, fakeRenderer{}, log)
	d := NewDaemon(dev, runner, fakeMetrics{}, fakeRenderer{}, engine, cfg, log)
	d.timing = pressTiming{longPress: 80 * time.Millisecond, repeatDelay: 60 * time.Millisecond, repeatInterval: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return d, dev, runner
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func press(dev *fakeDeck, index int) {
	dev.events <- deck.ButtonEvent{Index: index, Pressed: true}
	dev.events <- deck.ButtonEvent{Index: index, Pressed: false}
}

func TestNextPageButtonCyclesThroughPages(t *testing.T) {
	d, dev, _ := startDaemon(t, pagedConfig())
	eventually(t, func() bool { return dev.lastUploadLabel(0) == "first" })

	press(dev, 12)
	eventually(t, func() bool { return d.CurrentPage() == "two" && dev.lastUploadLabel(0) == "second" })
	if dev.lastUploadLabel(12) != "next" {
		t.Fatal("fixed button missing on second page")
	}

	press(dev, 12)
	eventually(t, func() bool { return d.CurrentPage() == "one" })
}

func TestPressRunsActionOnce(t *testing.T) {
	_, dev, runner := startDaemon(t, pagedConfig())
	press(dev, 1)
	eventually(t, func() bool {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return len(runner.ran) == 1 && runner.ran[0].Cmd == "true"
	})
}

func TestApplyConfigKeepsPageOrFallsBack(t *testing.T) {
	d, dev, _ := startDaemon(t, pagedConfig())
	d.SwitchTo("two")

	cfg := pagedConfig()
	cfg.Pages[1].Buttons[0].Label = "changed"
	d.ApplyConfig(cfg)
	if d.CurrentPage() != "two" || dev.lastUploadLabel(0) != "changed" {
		t.Fatalf("page %q label %q", d.CurrentPage(), dev.lastUploadLabel(0))
	}

	cfg = pagedConfig()
	cfg.Pages = cfg.Pages[:1]
	d.ApplyConfig(cfg)
	if d.CurrentPage() != "one" {
		t.Fatalf("expected fallback to default page, got %q", d.CurrentPage())
	}
}

func TestDisabledSmallWindowSendsHeartbeat(t *testing.T) {
	_, dev, _ := startDaemon(t, pagedConfig())
	eventually(t, func() bool {
		dev.mu.Lock()
		defer dev.mu.Unlock()
		return dev.keepAlive > 0 && len(dev.modes) > 0 && dev.modes[0] == deck.SmallWindowBackground
	})
}

func TestEnabledSmallWindowPushesClock(t *testing.T) {
	cfg := pagedConfig()
	cfg.SmallWindow.Enabled = true
	cfg.SmallWindow.ShowMetrics = false
	_, dev, _ := startDaemon(t, cfg)
	eventually(t, func() bool {
		dev.mu.Lock()
		defer dev.mu.Unlock()
		return dev.data > 0 && len(dev.modes) > 0 && dev.modes[0] == deck.SmallWindowClock
	})
}

func shell(cmd string) *deck.Action { return &deck.Action{Type: deck.ActionShell, Cmd: cmd} }

func behaviorConfig() *deck.Config {
	cfg := pagedConfig()
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons,
		deck.Button{Index: 2, Action: shell("short"), LongPress: shell("long"), Press: deck.PressTap},
		deck.Button{Index: 3, Action: shell("again"), Press: deck.PressRepeat},
		deck.Button{Index: 5, Action: &deck.Action{Type: deck.ActionShortcut, Keys: "ctrl+shift+m"}, Press: deck.PressHold},
	)
	return cfg
}

func TestShortPressRunsPrimaryOnRelease(t *testing.T) {
	_, dev, runner := startDaemon(t, behaviorConfig())
	dev.events <- deck.ButtonEvent{Index: 2, Pressed: true}
	time.Sleep(20 * time.Millisecond)
	if len(runner.commands()) != 0 {
		t.Fatal("primary must wait for the release")
	}
	dev.events <- deck.ButtonEvent{Index: 2, Pressed: false}
	eventually(t, func() bool { c := runner.commands(); return len(c) == 1 && c[0] == "short" })
	time.Sleep(150 * time.Millisecond)
	if c := runner.commands(); len(c) != 1 {
		t.Fatalf("long press must not fire after a short press: %v", c)
	}
}

func TestLongPressRunsSecondaryOnly(t *testing.T) {
	_, dev, runner := startDaemon(t, behaviorConfig())
	dev.events <- deck.ButtonEvent{Index: 2, Pressed: true}
	eventually(t, func() bool { c := runner.commands(); return len(c) == 1 && c[0] == "long" })
	dev.events <- deck.ButtonEvent{Index: 2, Pressed: false}
	time.Sleep(50 * time.Millisecond)
	if c := runner.commands(); len(c) != 1 {
		t.Fatalf("release after a long press must not run the primary: %v", c)
	}
}

func TestRepeatWhileHeld(t *testing.T) {
	_, dev, runner := startDaemon(t, behaviorConfig())
	dev.events <- deck.ButtonEvent{Index: 3, Pressed: true}
	eventually(t, func() bool { return len(runner.commands()) >= 4 })
	dev.events <- deck.ButtonEvent{Index: 3, Pressed: false}
	time.Sleep(30 * time.Millisecond)
	n := len(runner.commands())
	time.Sleep(100 * time.Millisecond)
	if len(runner.commands()) != n {
		t.Fatal("repeat must stop after release")
	}
}

func TestHoldPassesPressAndRelease(t *testing.T) {
	_, dev, runner := startDaemon(t, behaviorConfig())
	dev.events <- deck.ButtonEvent{Index: 5, Pressed: true}
	eventually(t, func() bool { runner.mu.Lock(); defer runner.mu.Unlock(); return len(runner.pressed) == 1 })
	runner.mu.Lock()
	early := len(runner.released)
	runner.mu.Unlock()
	if early != 0 {
		t.Fatal("must stay pressed until release")
	}
	dev.events <- deck.ButtonEvent{Index: 5, Pressed: false}
	eventually(t, func() bool { runner.mu.Lock(); defer runner.mu.Unlock(); return len(runner.released) == 1 })
}

func TestDisconnectReleasesHeldKeys(t *testing.T) {
	_, dev, runner := startDaemon(t, behaviorConfig())
	dev.events <- deck.ButtonEvent{Index: 5, Pressed: true}
	eventually(t, func() bool { runner.mu.Lock(); defer runner.mu.Unlock(); return len(runner.pressed) == 1 })
	dev.events <- deck.ConnectionEvent{Connected: false}
	eventually(t, func() bool { runner.mu.Lock(); defer runner.mu.Unlock(); return len(runner.released) == 1 })
}
