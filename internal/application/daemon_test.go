package application

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck/internal/domain/deck"
)

type fakeDeck struct {
	mu        sync.Mutex
	events    chan deck.Event
	uploads   [][]deck.Button
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
	}
	f.mu.Unlock()
	return nil
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
	mu  sync.Mutex
	ran []deck.Action
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
func (fakeMetrics) TemperatureValue([]string, string) string { return "40C" }

type fakeRenderer struct{}

func (fakeRenderer) SmallWindowClock(string, time.Time) []byte  { return []byte("png") }
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
	d := NewDaemon(dev, runner, fakeMetrics{}, fakeRenderer{}, cfg, slog.New(slog.DiscardHandler))
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
