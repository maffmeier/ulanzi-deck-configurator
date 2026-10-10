package application

import (
	"sync"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type fakeDesktop struct {
	mu     sync.Mutex
	app    string
	locked bool
}

func (f *fakeDesktop) ActiveApp() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.app, nil
}

func (f *fakeDesktop) Locked() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.locked, nil
}

func (f *fakeDesktop) set(app string, locked bool) {
	f.mu.Lock()
	f.app, f.locked = app, locked
	f.mu.Unlock()
}

func TestDimAfterIdleAndWakeOnPress(t *testing.T) {
	cfg := pagedConfig()
	cfg.Brightness = 70
	cfg.Display = deck.Display{DimAfterS: 1, DimBrightness: 5}
	_, dev, _ := startDaemon(t, cfg)
	eventually(t, func() bool { return dev.currentBrightness() == 70 })
	eventually(t, func() bool { return dev.currentBrightness() == 5 })
	press(dev, 0)
	eventually(t, func() bool { return dev.currentBrightness() == 70 })
}

func TestOffWhileLocked(t *testing.T) {
	cfg := pagedConfig()
	cfg.Brightness = 60
	cfg.Display = deck.Display{OffWhenLocked: true}
	desktop := &fakeDesktop{}
	dev, runner := newFakeDeck(), &fakeRunner{}
	d := newTestDaemon(t, dev, runner, cfg, desktop)
	_ = d
	eventually(t, func() bool { return dev.currentBrightness() == 60 })
	// The lock state is polled every lockInterval.
	desktop.set("", true)
	eventuallyWithin(t, 4*time.Second, func() bool { return dev.currentBrightness() == 0 })
	desktop.set("", false)
	eventuallyWithin(t, 4*time.Second, func() bool { return dev.currentBrightness() == 60 })
}

func TestFollowsFocusedApp(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages[1].Apps = []string{"obs"}
	desktop := &fakeDesktop{app: "firefox"}
	dev, runner := newFakeDeck(), &fakeRunner{}
	d := newTestDaemon(t, dev, runner, cfg, desktop)

	desktop.set("com.obsproject.Studio", false)
	eventually(t, func() bool { return d.CurrentPage() == "two" })
	desktop.set("firefox", false)
	eventually(t, func() bool { return d.CurrentPage() == "one" })

	// A manual switch while the app page is shown hands control back.
	desktop.set("obs", false)
	eventually(t, func() bool { return d.CurrentPage() == "two" })
	d.SwitchTo("one")
	desktop.set("firefox", false)
	time.Sleep(1500 * time.Millisecond)
	if d.CurrentPage() != "one" {
		t.Fatalf("page %q", d.CurrentPage())
	}
}

func eventuallyWithin(t *testing.T, wait time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
