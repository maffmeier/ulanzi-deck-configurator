package application

import (
	"context"
	"image"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type fakeSources struct {
	mu    sync.Mutex
	calls int
	out   string
	ok    bool
}

func (f *fakeSources) CommandOutput(context.Context, string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.out, f.ok, nil
}
func (f *fakeSources) NowPlaying() (MediaInfo, error) {
	return MediaInfo{Title: "Song", Artist: "Band", Playing: true}, nil
}
func (f *fakeSources) Cover(string) image.Image { return nil }
func (f *fakeSources) LoadImage(string) (image.Image, error) {
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}

func newEngine(src *fakeSources) *WidgetEngine {
	return NewWidgetEngine(fakeMetrics{}, src, fakeRenderer{}, slog.New(slog.DiscardHandler))
}

func TestTimerToggleCountsDownAndPauses(t *testing.T) {
	e := newEngine(&fakeSources{})
	timer := deck.Widget{Type: deck.WidgetTimer, Title: "Pomodoro"}
	start := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	if got := string(e.Render(timer, 196, 196, "#000000", start)); got != "timer|Pomodoro|25:00|false" {
		t.Fatalf("idle: %s", got)
	}
	e.Timer(deck.TimerToggle, 1, start)
	if got := string(e.Render(timer, 196, 196, "#000000", start.Add(15*time.Second))); got != "timer|Pomodoro|00:45|true" {
		t.Fatalf("running: %s", got)
	}
	e.Timer(deck.TimerToggle, 0, start.Add(20*time.Second))
	if got := string(e.Render(timer, 196, 196, "#000000", start.Add(time.Hour))); got != "timer|Pomodoro|00:40|false" {
		t.Fatalf("paused: %s", got)
	}
	e.Timer(deck.TimerReset, 0, start)
	if got := string(e.Render(timer, 196, 196, "#000000", start)); got != "timer|Pomodoro|01:00|false" {
		t.Fatalf("reset: %s", got)
	}
	e.Timer(deck.TimerStart, 0, start)
	if got := string(e.Render(timer, 196, 196, "#000000", start.Add(2*time.Minute))); got != "timer|Pomodoro|Fertig|true" {
		t.Fatalf("done: %s", got)
	}
}

func TestCommandWidgetCachesAndColorsByExitCode(t *testing.T) {
	src := &fakeSources{out: "Mikro an", ok: true}
	e := newEngine(src)
	w := deck.Widget{Type: deck.WidgetCommand, Cmd: "check", IntervalS: 60, OkColor: "#00AA00", FailColor: "#AA0000"}
	now := time.Now()

	if got := string(e.Render(w, 196, 196, "#000000", now)); !strings.Contains(got, "…") {
		t.Fatalf("first render must not block: %s", got)
	}
	eventually(t, func() bool { return strings.HasPrefix(string(e.Render(w, 196, 196, "#000000", now)), "text|#00AA00") })
	e.Render(w, 196, 196, "#000000", now.Add(time.Second))
	src.mu.Lock()
	calls := src.calls
	src.mu.Unlock()
	if calls != 1 {
		t.Fatalf("command must be cached for interval_s, ran %d times", calls)
	}

	src.mu.Lock()
	src.out, src.ok = "Mikro aus", false
	src.mu.Unlock()
	e.Render(w, 196, 196, "#000000", now.Add(2*time.Minute))
	eventually(t, func() bool {
		return string(e.Render(w, 196, 196, "#000000", now.Add(2*time.Minute))) == "text|#AA0000||[Mikro aus]"
	})
}

func TestGraphCollectsSamples(t *testing.T) {
	e := newEngine(&fakeSources{})
	w := deck.Widget{Type: deck.WidgetGraph, Metric: "network"}
	e.Render(w, 392, 196, "#000000", time.Now())
	e.sample()
	e.sample()
	if got := string(e.Render(w, 392, 196, "#000000", time.Now())); got != "graph|Netzwerk|2.0 K/s|2" {
		t.Fatalf("got %s", got)
	}
}

func TestMediaRefreshesInBackground(t *testing.T) {
	e := newEngine(&fakeSources{})
	w := deck.Widget{Type: deck.WidgetMedia}
	e.Render(w, 392, 196, "#000000", time.Now())
	eventually(t, func() bool { return string(e.Render(w, 392, 196, "#000000", time.Now())) == "media|Song|Band" })
}

func TestLiveKeysAndTimerAction(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons,
		deck.Button{Index: 6, Live: &deck.Widget{Type: deck.WidgetTimer, IntervalS: 5}, TextStyle: deck.DefaultTextStyle()},
		deck.Button{Index: 7, Action: &deck.Action{Type: deck.ActionTimer, Op: deck.TimerToggle, Minutes: 5}, TextStyle: deck.DefaultTextStyle()},
	)
	_, dev, runner := startDaemon(t, cfg)
	eventually(t, func() bool {
		dev.mu.Lock()
		defer dev.mu.Unlock()
		if len(dev.uploads) == 0 {
			return false
		}
		for _, b := range dev.uploads[0] {
			if b.Index == 6 {
				return strings.HasPrefix(string(b.IconData), "timer|Timer|25:00")
			}
		}
		return false
	})
	press(dev, 7)
	eventually(t, func() bool {
		data := dev.partialData(6)
		return len(data) > 0 && strings.Contains(data[len(data)-1], "|true")
	})
	runner.mu.Lock()
	ran := len(runner.ran)
	runner.mu.Unlock()
	if ran != 0 {
		t.Fatal("timer actions are handled by the daemon, not the runner")
	}
}

func TestWidgetSmallWindowUploadsOnlyChanges(t *testing.T) {
	cfg := pagedConfig()
	cfg.SmallWindow.Enabled = true
	cfg.SmallWindow.Widgets = []deck.Widget{{Type: deck.WidgetClock, Format: "%H:%M", IntervalS: 5}}
	_, dev, _ := startDaemon(t, cfg)
	eventually(t, func() bool { return len(dev.partialData(deck.InfoWindowIndex)) >= 1 })
	time.Sleep(2500 * time.Millisecond)
	if n := len(dev.partialData(deck.InfoWindowIndex)); n != 1 {
		t.Fatalf("unchanged clock must not be re-uploaded, got %d uploads", n)
	}
	dev.mu.Lock()
	keepAlive := dev.keepAlive
	dev.mu.Unlock()
	if keepAlive == 0 {
		t.Fatal("watchdog must still be fed")
	}
}
