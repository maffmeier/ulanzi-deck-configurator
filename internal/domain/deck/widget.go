package deck

import (
	"fmt"
	"slices"
	"strings"
)

type WidgetType string

const (
	WidgetClock   WidgetType = "clock"
	WidgetMetrics WidgetType = "metrics"
	WidgetCommand WidgetType = "command"
	WidgetGraph   WidgetType = "graph"
	WidgetImage   WidgetType = "image"
	WidgetTimer   WidgetType = "timer"
	WidgetMedia   WidgetType = "media"
)

// GraphMetrics can be plotted over time.
var GraphMetrics = []string{"cpu", "memory", "network"}

const (
	DefaultWidgetInterval = 5.0
	MinWidgetInterval     = 0.5
)

// Widget is host-rendered content for the info window or a live key.
type Widget struct {
	Type WidgetType
	// Title is an optional heading (command, graph, timer, metrics).
	Title string
	// Format is the strftime format of a clock.
	Format string
	// Items are the metrics shown by a metrics widget.
	Items []string
	// Cmd is the shell command of a command widget; its output is shown.
	Cmd string
	// IntervalS is how often a command reruns or a slideshow advances.
	IntervalS float64
	// Metric is the value plotted by a graph widget.
	Metric string
	// Paths are the images of an image widget (slideshow when > 1).
	Paths         []string
	ResolvedPaths []string
	// OkColor and FailColor color a command widget by its exit code, e.g.
	// red while the microphone is muted.
	OkColor   string
	FailColor string
}

// Normalize validates the widget and fills defaults. Keys can't show image
// widgets: a static icon does that already.
func (w *Widget) Normalize(onKey bool) error {
	switch w.Type {
	case WidgetClock:
		if strings.TrimSpace(w.Format) == "" {
			w.Format = DefaultTimeFormat
		}
	case WidgetMetrics:
		if len(w.Items) == 0 || len(w.Items) > 3 {
			return fmt.Errorf("metrics widget needs 1 to 3 items")
		}
		for _, item := range w.Items {
			if !slices.Contains(MetricChoices, item) {
				return fmt.Errorf("unsupported metric %q", item)
			}
		}
	case WidgetCommand:
		if strings.TrimSpace(w.Cmd) == "" {
			return fmt.Errorf("command widget needs cmd")
		}
		for _, c := range []*string{&w.OkColor, &w.FailColor} {
			if *c == "" {
				continue
			}
			normalized, err := NormalizeHexColor(*c)
			if err != nil {
				return err
			}
			*c = normalized
		}
	case WidgetGraph:
		if !slices.Contains(GraphMetrics, w.Metric) {
			return fmt.Errorf("graph widget needs metric cpu, memory or network, got %q", w.Metric)
		}
	case WidgetImage:
		if onKey {
			return fmt.Errorf("image widgets are only supported in the info window")
		}
		if len(w.Paths) == 0 {
			return fmt.Errorf("image widget needs at least one path")
		}
	case WidgetTimer, WidgetMedia:
	default:
		return fmt.Errorf("unknown widget type %q", w.Type)
	}
	if w.IntervalS == 0 {
		w.IntervalS = DefaultWidgetInterval
	}
	if w.IntervalS < MinWidgetInterval {
		return fmt.Errorf("interval_s must be at least %g", MinWidgetInterval)
	}
	return nil
}

// Clean keeps only the fields the widget type uses, so editor leftovers
// (a format on a graph, a metric on a clock) don't end up in the config.
func (w Widget) Clean() Widget {
	clean := Widget{Type: w.Type, IntervalS: DefaultWidgetInterval}
	switch w.Type {
	case WidgetClock:
		clean.Title, clean.Format = w.Title, w.Format
	case WidgetMetrics:
		clean.Title, clean.Items = w.Title, w.Items
	case WidgetCommand:
		clean.Title, clean.Cmd, clean.IntervalS = w.Title, w.Cmd, w.IntervalS
		clean.OkColor, clean.FailColor = w.OkColor, w.FailColor
	case WidgetGraph:
		clean.Title, clean.Metric = w.Title, w.Metric
	case WidgetImage:
		clean.Paths, clean.ResolvedPaths, clean.IntervalS = w.Paths, w.ResolvedPaths, w.IntervalS
	case WidgetTimer:
		clean.Title = w.Title
	}
	return clean
}
