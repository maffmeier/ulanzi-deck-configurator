package application

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// MediaInfo is what the desktop is currently playing.
type MediaInfo struct {
	Title   string
	Artist  string
	Playing bool
	ArtURL  string
}

// WidgetSources are the slow, host-specific inputs of widgets.
type WidgetSources interface {
	CommandOutput(ctx context.Context, cmd string) (out string, ok bool, err error)
	NowPlaying() (MediaInfo, error)
	Cover(url string) image.Image
	LoadImage(path string) (image.Image, error)
}

const (
	graphSamples     = 60
	commandTimeout   = 10 * time.Second
	mediaRefresh     = time.Second
	defaultTimerMins = 25
)

type commandResult struct {
	out     string
	ok      bool
	at      time.Time
	running bool
	err     error
}

type timerState struct {
	duration  time.Duration
	remaining time.Duration // while paused
	endsAt    time.Time     // while running
	running   bool
}

type cachedImage struct {
	img image.Image
	at  time.Time
}

// WidgetEngine renders widgets. Rendering never blocks on slow sources:
// commands, media info and covers refresh in the background and the next
// render picks up the result.
type WidgetEngine struct {
	metrics  Metrics
	sources  WidgetSources
	renderer Renderer
	log      *slog.Logger

	mu       sync.Mutex
	history  map[string][]float64
	graphs   map[string]bool
	commands map[string]*commandResult
	timer    timerState
	media    MediaInfo
	mediaAt  time.Time
	mediaRun bool
	cover    image.Image
	images   map[string]cachedImage
}

func NewWidgetEngine(metrics Metrics, sources WidgetSources, renderer Renderer, log *slog.Logger) *WidgetEngine {
	return &WidgetEngine{
		metrics:  metrics,
		sources:  sources,
		renderer: renderer,
		log:      log,
		history:  map[string][]float64{},
		graphs:   map[string]bool{},
		commands: map[string]*commandResult{},
		images:   map[string]cachedImage{},
		timer:    timerState{duration: defaultTimerMins * time.Minute, remaining: defaultTimerMins * time.Minute},
	}
}

// Run samples graph metrics once per second until ctx ends.
func (e *WidgetEngine) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sample()
		}
	}
}

// sample only measures metrics a graph has asked for.
func (e *WidgetEngine) sample() {
	e.mu.Lock()
	wanted := make([]string, 0, len(e.graphs))
	for m := range e.graphs {
		wanted = append(wanted, m)
	}
	e.mu.Unlock()

	for _, m := range wanted {
		var v float64
		switch m {
		case "cpu":
			v = float64(e.metrics.CPUPercent())
		case "memory":
			v = float64(e.metrics.MemoryPercent())
		case "network":
			v, _ = e.metrics.NetworkBytesPerSec()
		}
		e.mu.Lock()
		h := append(e.history[m], v)
		if len(h) > graphSamples {
			h = h[len(h)-graphSamples:]
		}
		e.history[m] = h
		e.mu.Unlock()
	}
}

// Render draws a widget at w x h pixels.
func (e *WidgetEngine) Render(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	switch wd.Type {
	case deck.WidgetClock:
		return e.renderer.Text(w, h, bg, wd.Title, []string{e.metrics.FormatTime(wd.Format)}, true)
	case deck.WidgetMetrics:
		return e.renderMetrics(wd, w, h, bg)
	case deck.WidgetCommand:
		return e.renderCommand(wd, w, h, bg, now)
	case deck.WidgetGraph:
		return e.renderGraph(wd, w, h, bg)
	case deck.WidgetImage:
		return e.renderImage(wd, w, h, bg, now)
	case deck.WidgetTimer:
		return e.renderTimer(wd, w, h, bg, now)
	case deck.WidgetMedia:
		return e.renderMedia(w, h, bg, now)
	}
	return e.renderer.Text(w, h, bg, "", []string{"?"}, false)
}

func (e *WidgetEngine) renderMetrics(wd deck.Widget, w, h int, bg string) []byte {
	lines := make([]string, 0, len(wd.Items))
	for _, item := range wd.Items {
		lines = append(lines, fmt.Sprintf("%-4s %s", deck.MetricLabels[item], e.metrics.MetricValue(item)))
	}
	return e.renderer.Text(w, h, bg, wd.Title, lines, true)
}

func (e *WidgetEngine) renderCommand(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	e.mu.Lock()
	res := e.commands[wd.Cmd]
	if res == nil {
		res = &commandResult{}
		e.commands[wd.Cmd] = res
	}
	stale := now.Sub(res.at) >= time.Duration(wd.IntervalS*float64(time.Second))
	if stale && !res.running {
		res.running = true
		go e.runCommand(wd.Cmd)
	}
	out, ok, at, err := res.out, res.ok, res.at, res.err
	e.mu.Unlock()

	var lines []string
	switch {
	case at.IsZero():
		lines = []string{"…"}
	case err != nil:
		lines = []string{"Fehler"}
	case out != "":
		lines = strings.Split(out, "\n")
		if len(lines) > 4 {
			lines = lines[:4]
		}
	}
	if !at.IsZero() && err == nil {
		if ok && wd.OkColor != "" {
			bg = wd.OkColor
		}
		if !ok && wd.FailColor != "" {
			bg = wd.FailColor
		}
	}
	return e.renderer.Text(w, h, bg, wd.Title, lines, false)
}

func (e *WidgetEngine) runCommand(cmd string) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, ok, err := e.sources.CommandOutput(ctx, cmd)
	if err != nil {
		e.log.Warn("widget command failed", "cmd", cmd, "error", err)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if res := e.commands[cmd]; res != nil {
		res.out, res.ok, res.err, res.at, res.running = out, ok, err, time.Now(), false
	}
}

func (e *WidgetEngine) renderGraph(wd deck.Widget, w, h int, bg string) []byte {
	e.mu.Lock()
	e.graphs[wd.Metric] = true
	samples := append([]float64(nil), e.history[wd.Metric]...)
	e.mu.Unlock()

	title := wd.Title
	if title == "" {
		title = map[string]string{"cpu": "CPU", "memory": "RAM", "network": "Netzwerk"}[wd.Metric]
	}
	maxValue, value := 100.0, "–"
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		if wd.Metric == "network" {
			maxValue = 1024
			for _, s := range samples {
				maxValue = math.Max(maxValue, s*1.1)
			}
			value = formatRate(last)
		} else {
			value = fmt.Sprintf("%.0f%%", last)
		}
	}
	return e.renderer.Graph(w, h, bg, title, value, samples, maxValue)
}

func formatRate(bytesPerSec float64) string {
	units := []string{"B/s", "K/s", "M/s", "G/s"}
	v := bytesPerSec
	for i, u := range units {
		if v < 1024 || i == len(units)-1 {
			if i == 0 {
				return fmt.Sprintf("%.0f %s", v, u)
			}
			return fmt.Sprintf("%.1f %s", v, u)
		}
		v /= 1024
	}
	return ""
}

func (e *WidgetEngine) renderImage(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	paths := wd.ResolvedPaths
	if len(paths) == 0 {
		paths = wd.Paths
	}
	if len(paths) == 0 {
		return e.renderer.Text(w, h, bg, "", nil, false)
	}
	step := now.UnixNano() / int64(wd.IntervalS*float64(time.Second))
	path := paths[int(step%int64(len(paths)))]
	img := e.image(path, now)
	if img == nil {
		return e.renderer.Text(w, h, bg, "", []string{"Bild fehlt"}, false)
	}
	return e.renderer.Image(w, h, bg, img)
}

// image caches decoded files for a minute so edits show up eventually.
func (e *WidgetEngine) image(path string, now time.Time) image.Image {
	e.mu.Lock()
	cached, ok := e.images[path]
	e.mu.Unlock()
	if ok && now.Sub(cached.at) < time.Minute {
		return cached.img
	}
	img, err := e.sources.LoadImage(path)
	if err != nil {
		e.log.Warn("widget image failed", "path", path, "error", err)
	}
	e.mu.Lock()
	e.images[path] = cachedImage{img: img, at: now}
	e.mu.Unlock()
	return img
}

// Timer applies a timer action. minutes > 0 sets the duration whenever
// the timer (re)starts from a reset state.
func (e *WidgetEngine) Timer(op string, minutes int, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := &e.timer
	if minutes > 0 && !t.running && t.remaining == t.duration {
		t.duration = time.Duration(minutes) * time.Minute
		t.remaining = t.duration
	}
	switch op {
	case deck.TimerToggle:
		if t.running {
			e.pauseLocked(now)
		} else {
			e.startLocked(now)
		}
	case deck.TimerStart:
		if !t.running {
			e.startLocked(now)
		}
	case deck.TimerPause:
		if t.running {
			e.pauseLocked(now)
		}
	case deck.TimerReset:
		if minutes > 0 {
			t.duration = time.Duration(minutes) * time.Minute
		}
		t.running, t.remaining = false, t.duration
	}
}

func (e *WidgetEngine) startLocked(now time.Time) {
	t := &e.timer
	if t.remaining <= 0 {
		t.remaining = t.duration
	}
	t.endsAt, t.running = now.Add(t.remaining), true
}

func (e *WidgetEngine) pauseLocked(now time.Time) {
	t := &e.timer
	t.remaining, t.running = max(0, t.endsAt.Sub(now)), false
}

func (e *WidgetEngine) renderTimer(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	e.mu.Lock()
	t := e.timer
	e.mu.Unlock()

	remaining := t.remaining
	if t.running {
		remaining = max(0, t.endsAt.Sub(now))
	}
	title := wd.Title
	if title == "" {
		title = "Timer"
	}
	text := formatDuration(remaining)
	if t.running && remaining == 0 {
		text = "Fertig"
	}
	progress := 0.0
	if t.duration > 0 {
		progress = 1 - float64(remaining)/float64(t.duration)
	}
	return e.renderer.Timer(w, h, bg, title, text, progress, t.running)
}

func formatDuration(d time.Duration) string {
	total := int(math.Ceil(d.Seconds()))
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func (e *WidgetEngine) renderMedia(w, h int, bg string, now time.Time) []byte {
	e.mu.Lock()
	if now.Sub(e.mediaAt) >= mediaRefresh && !e.mediaRun {
		e.mediaRun = true
		go e.refreshMedia()
	}
	info, cover := e.media, e.cover
	e.mu.Unlock()
	return e.renderer.Media(w, h, bg, info.Title, info.Artist, cover)
}

func (e *WidgetEngine) refreshMedia() {
	info, err := e.sources.NowPlaying()
	var cover image.Image
	if err == nil && info.ArtURL != "" {
		cover = e.sources.Cover(info.ArtURL)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mediaRun, e.mediaAt = false, time.Now()
	if err != nil {
		e.media, e.cover = MediaInfo{}, nil
		return
	}
	e.media, e.cover = info, cover
}
