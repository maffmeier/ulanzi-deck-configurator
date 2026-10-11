package deck

import (
	"fmt"
	"slices"
	"strings"
)

type WidgetType string

const (
	WidgetClock    WidgetType = "clock"
	WidgetMetrics  WidgetType = "metrics"
	WidgetCommand  WidgetType = "command"
	WidgetGraph    WidgetType = "graph"
	WidgetImage    WidgetType = "image"
	WidgetTimer    WidgetType = "timer"
	WidgetMedia    WidgetType = "media"
	WidgetHome     WidgetType = "home_assistant"
	WidgetOBS      WidgetType = "obs"
	WidgetAudio    WidgetType = "audio"
	WidgetMeeting  WidgetType = "meeting"
	WidgetMQTT     WidgetType = "mqtt"
	WidgetCalendar WidgetType = "calendar"
	WidgetPlugin   WidgetType = "plugin"
)

// DefaultCalendarInterval is how often a calendar feed is fetched.
const DefaultCalendarInterval = 300.0

// MeetingShows are the Teams states a meeting widget can display.
var MeetingShows = []string{"mute", "camera", "hand"}

// AudioShows are the things an audio widget can display.
var AudioShows = []string{"volume", "mic", "output"}

// OBSShows are the things an obs widget can display.
var OBSShows = []string{"scene", "record", "stream"}

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
	// Home Assistant entity; Attribute shows an attribute instead of the
	// state, ActiveState is the state that counts as "on" for the colors.
	EntityID    string
	Attribute   string
	ActiveState string
	// Show selects what an obs or audio widget displays.
	Show string
	// Target is the application whose volume an audio widget shows.
	Target string
	// Topic is the MQTT topic an mqtt widget shows.
	Topic string
	// URL is the calendar (ICS) of a calendar widget.
	URL string
	// Plugin widget: Plugin names the plugin, Show its widget.
	Plugin   string
	Settings map[string]string
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
		if err := normalizeColors(w); err != nil {
			return err
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
	case WidgetHome:
		if strings.TrimSpace(w.EntityID) == "" {
			return fmt.Errorf("home_assistant widget needs entity_id")
		}
		if w.ActiveState == "" {
			w.ActiveState = "on"
		}
		if err := normalizeColors(w); err != nil {
			return err
		}
	case WidgetOBS:
		if w.Show == "" {
			w.Show = "scene"
		}
		if !slices.Contains(OBSShows, w.Show) {
			return fmt.Errorf("obs widget shows scene, record or stream, got %q", w.Show)
		}
	case WidgetAudio:
		if w.Show == "" {
			w.Show = "volume"
		}
		if !slices.Contains(AudioShows, w.Show) {
			return fmt.Errorf("audio widget shows volume, mic or output, got %q", w.Show)
		}
		if err := normalizeColors(w); err != nil {
			return err
		}
	case WidgetPlugin:
		if strings.TrimSpace(w.Plugin) == "" || strings.TrimSpace(w.Show) == "" {
			return fmt.Errorf("plugin widget needs plugin and show (the widget id)")
		}
	case WidgetCalendar:
		if strings.TrimSpace(w.URL) == "" {
			return fmt.Errorf("calendar widget needs the url of an ICS feed")
		}
		if w.IntervalS == 0 {
			w.IntervalS = DefaultCalendarInterval
		}
		if w.IntervalS < 60 {
			return fmt.Errorf("calendar feeds are fetched at most once a minute")
		}
		if err := normalizeColors(w); err != nil {
			return err
		}
	case WidgetMQTT:
		if err := ValidateMQTTTopic(w.Topic); err != nil {
			return err
		}
		if w.ActiveState == "" {
			w.ActiveState = "on"
		}
		if err := normalizeColors(w); err != nil {
			return err
		}
	case WidgetMeeting:
		if w.Show == "" {
			w.Show = "mute"
		}
		if !slices.Contains(MeetingShows, w.Show) {
			return fmt.Errorf("meeting widget shows mute, camera or hand, got %q", w.Show)
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
	case WidgetHome:
		clean.Title, clean.EntityID, clean.Attribute, clean.ActiveState = w.Title, w.EntityID, w.Attribute, w.ActiveState
		clean.OkColor, clean.FailColor, clean.IntervalS = w.OkColor, w.FailColor, w.IntervalS
	case WidgetOBS:
		clean.Title, clean.Show = w.Title, w.Show
	case WidgetMeeting:
		clean.Title, clean.Show = w.Title, w.Show
	case WidgetCalendar:
		clean.Title, clean.URL, clean.IntervalS, clean.OkColor = w.Title, w.URL, w.IntervalS, w.OkColor
	case WidgetPlugin:
		clean.Title, clean.Plugin, clean.Show, clean.Settings = w.Title, w.Plugin, w.Show, w.Settings
	case WidgetMQTT:
		clean.Title, clean.Topic, clean.Attribute, clean.ActiveState = w.Title, w.Topic, w.Attribute, w.ActiveState
		clean.OkColor, clean.FailColor = w.OkColor, w.FailColor
	case WidgetAudio:
		clean.Title, clean.Show, clean.OkColor, clean.FailColor = w.Title, w.Show, w.OkColor, w.FailColor
		if w.Show == "volume" {
			clean.Target = w.Target
		}
	}
	return clean
}

func normalizeColors(w *Widget) error {
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
	return nil
}
