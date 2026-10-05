// Package application holds the use cases: driving the deck from a config
// and editing that config.
package application

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"sync"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// Deck is the device surface the daemon needs; the D200 driver implements
// it and tests use a fake.
type Deck interface {
	Events() <-chan deck.Event
	Connected() bool
	SetBrightness(value int, force bool) error
	SetSmallWindowMode(mode deck.SmallWindowMode) error
	SetSmallWindowData(cpu, mem, gpu int, timeStr string) error
	KeepAlive() error
	SetButtons(buttons []deck.Button, partial bool) error
}

type ActionRunner interface {
	Run(action deck.Action) error
	// Press holds a shortcut until the returned release is called.
	Press(action deck.Action) (func(), error)
}

type Metrics interface {
	FormatTime(format string) string
	CPUPercent() int
	MemoryPercent() int
	MetricValue(metric string) string
	TemperatureValue(sensorIDs []string, separator string) string
	NetworkBytesPerSec() (float64, bool)
}

type Renderer interface {
	SmallWindowClock(bg, text string) []byte
	SmallWindowMetrics(bg string, lines []string) []byte
	Text(w, h int, bg, title string, lines []string, mono bool) []byte
	Graph(w, h int, bg, title, value string, samples []float64, maxValue float64) []byte
	Timer(w, h int, bg, title, text string, progress float64, running bool) []byte
	Media(w, h int, bg, title, artist string, cover image.Image) []byte
	Image(w, h int, bg string, img image.Image) []byte
}

const heartbeatInterval = 2 * time.Second

// Daemon pushes the active page to the deck, keeps the info window alive
// and turns button presses into actions.
type Daemon struct {
	dev      Deck
	runner   ActionRunner
	metrics  Metrics
	renderer Renderer
	log      *slog.Logger

	mu     sync.Mutex
	cfg    *deck.Config
	page   string
	wakeup chan struct{}

	timing pressTiming
	heldMu sync.Mutex
	held   map[int]*heldKey

	engine     *WidgetEngine
	widgetKick chan struct{}
	// liveHash remembers what each live key shows, so unchanged keys are
	// not re-uploaded every second (guarded by mu).
	liveHash map[int]string
}

func NewDaemon(dev Deck, runner ActionRunner, metrics Metrics, renderer Renderer, engine *WidgetEngine, cfg *deck.Config, log *slog.Logger) *Daemon {
	return &Daemon{
		dev:        dev,
		runner:     runner,
		metrics:    metrics,
		renderer:   renderer,
		engine:     engine,
		liveHash:   map[int]string{},
		log:        log,
		cfg:        cfg,
		page:       cfg.DefaultPage,
		wakeup:     make(chan struct{}, 1),
		widgetKick: make(chan struct{}, 1),
		timing:     defaultPressTiming,
		held:       map[int]*heldKey{},
	}
}

func (d *Daemon) CurrentPage() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.page
}

func (d *Daemon) Config() *deck.Config {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg
}

// Run blocks until ctx ends.
func (d *Daemon) Run(ctx context.Context) {
	d.mu.Lock()
	d.applyLocked()
	d.mu.Unlock()

	var wg sync.WaitGroup
	wg.Go(func() { d.eventLoop(ctx) })
	wg.Go(func() { d.statusLoop(ctx) })
	wg.Go(func() { d.engine.Run(ctx) })
	wg.Go(func() { d.liveLoop(ctx) })
	wg.Wait()
}

// ApplyConfig swaps the running config; the current page is kept when it
// still exists, otherwise the new default page is shown.
func (d *Daemon) ApplyConfig(cfg *deck.Config) {
	d.mu.Lock()
	defer d.mu.Unlock()
	previous := d.page
	d.cfg = cfg
	if !cfg.HasPage(previous) {
		d.page = cfg.DefaultPage
		d.log.Info("page no longer exists, showing default", "previous", previous, "page", d.page)
	}
	d.applyLocked()
	d.log.Info("config applied", "pages", cfg.PageNames(), "page", d.page)
	d.poke()
}

func (d *Daemon) SwitchTo(target string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	page, ok := d.cfg.ResolvePageTarget(d.page, target)
	if !ok {
		d.log.Warn("switch to unknown page", "target", target, "known", d.cfg.PageNames())
		return
	}
	if page == d.page {
		return
	}
	d.page = page
	d.pushPageLocked()
	d.log.Info("page switched", "page", page)
}

func (d *Daemon) applyLocked() {
	d.logErr("set brightness", d.dev.SetBrightness(d.cfg.Brightness, false))
	d.pushPageLocked()
}

// pushPageLocked uploads the full grid, then resets the info window to its
// background color (the status loop draws on top of it).
func (d *Daemon) pushPageLocked() {
	var visible []deck.Button
	clear(d.liveHash)
	now := time.Now()
	for _, b := range d.cfg.ButtonsFor(d.page) {
		if b.Index >= deck.ButtonCount {
			continue
		}
		if b.Live != nil {
			b.IconData = d.engine.Render(*b.Live, deck.IconSize, deck.IconSize, b.TextStyle.BackgroundColor, now)
			d.liveHash[b.Index] = pngHash(b.IconData)
		}
		visible = append(visible, b)
	}
	d.logErr("upload page", d.dev.SetButtons(visible, false))
	info := deck.Button{Index: deck.InfoWindowIndex, TextStyle: deck.DefaultTextStyle()}
	info.TextStyle.BackgroundColor = d.cfg.SmallWindow.BackgroundColor
	d.logErr("upload info window", d.dev.SetButtons([]deck.Button{info}, true))
}

// logErr ignores "not connected": the device caches state and replays it
// on reconnect, so there is nothing to report.
func (d *Daemon) logErr(op string, err error) {
	if err == nil || errors.Is(err, deck.ErrNotConnected) {
		return
	}
	d.log.Warn(op+" failed", "error", err)
}

func (d *Daemon) poke() {
	select {
	case d.wakeup <- struct{}{}:
	default:
	}
}

func (d *Daemon) eventLoop(ctx context.Context) {
	defer d.releaseAll()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-d.dev.Events():
			switch e := ev.(type) {
			case deck.ButtonEvent:
				if e.Pressed {
					d.onPress(e.Index)
				} else {
					d.onRelease(e.Index)
				}
			case deck.ConnectionEvent:
				if e.Connected {
					// The status loop restarts its mode handshake.
					d.poke()
				} else {
					// A release may never arrive; don't leave keys stuck.
					d.releaseAll()
				}
			}
		}
	}
}

type statusState struct {
	key         string
	active      *deck.SmallWindowMode
	device      *deck.SmallWindowMode
	modeStarted time.Time
	primed      bool
	connected   bool
	lastPNG     string
	lastUpload  time.Time
}

func modePtr(m deck.SmallWindowMode) *deck.SmallWindowMode { return &m }

// statusLoop drives the info window and doubles as the firmware watchdog
// heartbeat; it ticks at least every interval_s (< 5s).
func (d *Daemon) statusLoop(ctx context.Context) {
	var st statusState
	for {
		timeout := d.statusTick(&st)
		select {
		case <-ctx.Done():
			return
		case <-d.wakeup:
			st = statusState{}
		case <-d.widgetKick:
		case <-time.After(timeout):
		}
	}
}

func strategyKey(sw deck.SmallWindow) string {
	rotate := "-"
	if sw.RotateEveryS != nil {
		rotate = fmt.Sprint(*sw.RotateEveryS)
	}
	return fmt.Sprint(sw.Enabled, sw.ShowMetrics, rotate, sw.MetricsItems, fmt.Sprintf("%+v", sw.Widgets))
}

func seconds(s float64) time.Duration { return time.Duration(s * float64(time.Second)) }

func (d *Daemon) statusTick(st *statusState) time.Duration {
	d.mu.Lock()
	sw := d.cfg.SmallWindow
	d.mu.Unlock()

	connected := d.dev.Connected()
	if key := strategyKey(sw); key != st.key || connected != st.connected {
		*st = statusState{key: key, connected: connected}
	}
	if !connected {
		return heartbeatInterval
	}

	if sw.Enabled && sw.UsesWidgets() {
		return d.widgetSmallWindow(st, sw)
	}

	if !sw.Enabled {
		if st.device == nil || *st.device != deck.SmallWindowBackground {
			d.logErr("small window mode", d.dev.SetSmallWindowMode(deck.SmallWindowBackground))
			st.device = modePtr(deck.SmallWindowBackground)
			st.active = st.device
		}
		d.logErr("keep alive", d.dev.KeepAlive())
		return heartbeatInterval
	}

	now := time.Now()
	desired := deck.SmallWindowClock
	var nextSwitch *time.Duration
	if sw.ShowMetrics {
		desired = deck.SmallWindowStats
		if sw.Rotates() {
			rotate := seconds(*sw.RotateEveryS)
			switch {
			case st.active == nil || *st.active == deck.SmallWindowBackground:
				desired = deck.SmallWindowClock
				nextSwitch = &rotate
			case now.Sub(st.modeStarted) >= rotate:
				desired = deck.SmallWindowClock
				if *st.active == deck.SmallWindowClock {
					desired = deck.SmallWindowStats
				}
				nextSwitch = &rotate
			default:
				desired = *st.active
				remaining := rotate - now.Sub(st.modeStarted)
				nextSwitch = &remaining
			}
		}
	}

	if sw.UsesCustomMetrics() {
		d.customSmallWindow(st, sw, desired, now)
	} else {
		d.nativeSmallWindow(st, sw, desired, now)
	}

	timeout := seconds(sw.IntervalS)
	if nextSwitch != nil {
		timeout = min(timeout, max(*nextSwitch, 10*time.Millisecond))
	}
	return timeout
}

// customSmallWindow renders the strip on the host and pins the firmware
// to BACKGROUND mode, so its native CPU/RAM overlay cannot reappear.
func (d *Daemon) customSmallWindow(st *statusState, sw deck.SmallWindow, desired deck.SmallWindowMode, now time.Time) {
	if st.device == nil || *st.device != deck.SmallWindowBackground {
		d.logErr("small window mode", d.dev.SetSmallWindowMode(deck.SmallWindowBackground))
		st.device = modePtr(deck.SmallWindowBackground)
	}
	if st.active == nil || *st.active != desired {
		st.active = modePtr(desired)
		st.modeStarted = now
		st.primed = false
	}

	var png []byte
	if desired == deck.SmallWindowStats {
		if !st.primed {
			// CPU and network are deltas; the first sample is a baseline.
			for _, m := range sw.MetricsItems {
				if m == "cpu" || m == "network" {
					d.metrics.MetricValue(m)
				}
			}
			st.primed = true
		}
		png = d.renderer.SmallWindowMetrics(sw.BackgroundColor, d.metricLines(sw))
	} else {
		png = d.renderer.SmallWindowClock(sw.BackgroundColor, d.metrics.FormatTime(sw.TimeFormat))
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	// Skip the upload when the config changed while rendering.
	if strategyKey(d.cfg.SmallWindow) != st.key {
		return
	}
	button := deck.Button{Index: deck.InfoWindowIndex, IconData: png, TextStyle: deck.DefaultTextStyle()}
	button.TextStyle.BackgroundColor = sw.BackgroundColor
	d.logErr("upload info window", d.dev.SetButtons([]deck.Button{button}, true))
}

func (d *Daemon) metricLines(sw deck.SmallWindow) []string {
	lines := make([]string, 0, len(sw.MetricsItems))
	for _, m := range sw.MetricsItems {
		label := deck.MetricLabels[m]
		var value string
		if m == "temperature" {
			value = d.metrics.TemperatureValue(sw.TemperatureSensors, sw.TemperatureSeparator)
		} else {
			value = d.metrics.MetricValue(m)
		}
		lines = append(lines, padRight(label, 4)+" "+value)
	}
	return lines
}

func padRight(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

// nativeSmallWindow lets the firmware draw clock or CPU/RAM itself.
func (d *Daemon) nativeSmallWindow(st *statusState, sw deck.SmallWindow, desired deck.SmallWindowMode, now time.Time) {
	if st.active == nil || *st.active != desired {
		d.logErr("small window mode", d.dev.SetSmallWindowMode(desired))
		st.device = modePtr(desired)
		st.active = modePtr(desired)
		st.modeStarted = now
		st.primed = false
	}
	if sw.ShowMetrics && !st.primed {
		d.metrics.CPUPercent()
		st.primed = true
	}
	timeStr := d.wireTime(sw.TimeFormat)
	if desired == deck.SmallWindowStats {
		d.logErr("small window data", d.dev.SetSmallWindowData(d.metrics.CPUPercent(), d.metrics.MemoryPercent(), 0, timeStr))
	} else {
		d.logErr("small window data", d.dev.SetSmallWindowData(0, 0, 0, timeStr))
	}
}

// wireTime appends seconds to HH:MM, which keeps the firmware clock
// layout ticking.
func (d *Daemon) wireTime(format string) string {
	rendered := d.metrics.FormatTime(format)
	if len(rendered) == 5 && rendered[2] == ':' && isDigits(rendered[:2]) && isDigits(rendered[3:]) {
		return rendered + ":" + d.metrics.FormatTime("%S")
	}
	if rendered != "" {
		return rendered
	}
	return d.metrics.FormatTime("%H:%M:%S")
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
