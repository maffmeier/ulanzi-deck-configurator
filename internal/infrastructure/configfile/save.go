package configfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type outAction struct {
	Type      string `yaml:"type"`
	Cmd       string `yaml:"cmd,omitempty"`
	Keys      string `yaml:"keys,omitempty"`
	CommandID string `yaml:"command_id,omitempty"`
	URL       string `yaml:"url,omitempty"`
	Page      string `yaml:"page,omitempty"`
	Op        string `yaml:"op,omitempty"`
	Minutes   int    `yaml:"minutes,omitempty"`
}

type outWidget struct {
	Type      string   `yaml:"type"`
	Title     string   `yaml:"title,omitempty"`
	Format    string   `yaml:"format,omitempty"`
	Items     []string `yaml:"items,omitempty"`
	Cmd       string   `yaml:"cmd,omitempty"`
	IntervalS float64  `yaml:"interval_s,omitempty"`
	Metric    string   `yaml:"metric,omitempty"`
	Paths     []string `yaml:"paths,omitempty"`
	OkColor   string   `yaml:"ok_color,omitempty"`
	FailColor string   `yaml:"fail_color,omitempty"`
}

type outTextStyle struct {
	BackgroundColor string `yaml:"background_color"`
	TextColor       string `yaml:"text_color"`
	Bold            bool   `yaml:"bold"`
	Italic          bool   `yaml:"italic"`
	Underline       bool   `yaml:"underline"`
	FontFamily      string `yaml:"font_family"`
	FontSize        int    `yaml:"font_size"`
}

type outButton struct {
	Index     int           `yaml:"index"`
	Label     string        `yaml:"label,omitempty"`
	Icon      string        `yaml:"icon,omitempty"`
	TextStyle *outTextStyle `yaml:"text_style,omitempty"`
	Action    *outAction    `yaml:"action,omitempty"`
	Press     string        `yaml:"press,omitempty"`
	LongPress *outAction    `yaml:"long_press,omitempty"`
	Live      *outWidget    `yaml:"live,omitempty"`
}

type outSmallWindow struct {
	Enabled              bool        `yaml:"enabled"`
	IntervalS            float64     `yaml:"interval_s"`
	TimeFormat           string      `yaml:"time_format"`
	ShowMetrics          bool        `yaml:"show_metrics"`
	BackgroundColor      string      `yaml:"background_color"`
	RotateEveryS         *float64    `yaml:"rotate_every_s,omitempty"`
	MetricsItems         []string    `yaml:"metrics_items,omitempty"`
	TemperatureSensors   []string    `yaml:"temperature_sensors,omitempty"`
	TemperatureSeparator string      `yaml:"temperature_separator,omitempty"`
	Widgets              []outWidget `yaml:"widgets,omitempty"`
}

type outPage struct {
	Buttons []outButton `yaml:"buttons"`
}

type outConfig struct {
	DefaultPage  string         `yaml:"default_page"`
	Brightness   *int           `yaml:"brightness,omitempty"`
	SmallWindow  outSmallWindow `yaml:"small_window"`
	Pages        yaml.Node      `yaml:"pages"`
	FixedButtons []outButton    `yaml:"fixed_buttons,omitempty"`
}

// Marshal renders a config in canonical form: page order is preserved,
// buttons are sorted by index and default values are omitted.
func Marshal(cfg *deck.Config) ([]byte, error) {
	out := outConfig{
		DefaultPage: cfg.DefaultPage,
		SmallWindow: outSmallWindow{
			Enabled:            cfg.SmallWindow.Enabled,
			IntervalS:          cfg.SmallWindow.IntervalS,
			TimeFormat:         cfg.SmallWindow.TimeFormat,
			ShowMetrics:        cfg.SmallWindow.ShowMetrics,
			BackgroundColor:    cfg.SmallWindow.BackgroundColor,
			RotateEveryS:       cfg.SmallWindow.RotateEveryS,
			MetricsItems:       cfg.SmallWindow.MetricsItems,
			TemperatureSensors: cfg.SmallWindow.TemperatureSensors,
		},
		Pages: yaml.Node{Kind: yaml.MappingNode},
	}
	if cfg.Brightness != deck.DefaultBrightness {
		b := cfg.Brightness
		out.Brightness = &b
	}
	for _, w := range cfg.SmallWindow.Widgets {
		out.SmallWindow.Widgets = append(out.SmallWindow.Widgets, *toOutWidget(&w))
	}
	if cfg.SmallWindow.TemperatureSeparator != " " {
		out.SmallWindow.TemperatureSeparator = cfg.SmallWindow.TemperatureSeparator
	}

	for _, page := range cfg.Pages {
		var value yaml.Node
		if err := value.Encode(outPage{Buttons: outButtons(page.Buttons)}); err != nil {
			return nil, err
		}
		out.Pages.Content = append(out.Pages.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: page.Name}, &value)
	}
	out.FixedButtons = outButtons(cfg.FixedButtons)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), enc.Close()
}

func outButtons(buttons []deck.Button) []outButton {
	sorted := append([]deck.Button(nil), buttons...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })

	result := make([]outButton, 0, len(sorted))
	for _, b := range sorted {
		ob := outButton{Index: b.Index}
		if b.Index != deck.InfoWindowIndex {
			ob.Label = b.Label
			ob.Icon = b.IconPath
			if !b.TextStyle.IsDefault() {
				s := b.TextStyle
				ob.TextStyle = &outTextStyle{
					BackgroundColor: s.BackgroundColor,
					TextColor:       s.TextColor,
					Bold:            s.Bold,
					Italic:          s.Italic,
					Underline:       s.Underline,
					FontFamily:      s.FontFamily,
					FontSize:        s.FontSize,
				}
			}
		}
		ob.Action = toOutAction(b.Action)
		ob.LongPress = toOutAction(b.LongPress)
		ob.Live = toOutWidget(b.Live)
		if b.Press != "" && b.Press != deck.PressTap {
			ob.Press = string(b.Press)
		}
		result = append(result, ob)
	}
	return result
}

// WriteAtomic replaces path via a temp file in the same directory so a
// crash never leaves a half-written config for the watcher to pick up.
func WriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// VersionedPath returns a free sibling path like deck-20261004-221500.yaml
// (or deck-firmware-...zip when label/ext are given).
func VersionedPath(path, label, ext string, now time.Time) string {
	if ext == "" {
		ext = filepath.Ext(path)
	}
	stem := trimExt(filepath.Base(path))
	if label != "" {
		stem += "-" + label
	}
	stem += "-" + now.Format("20060102-150405")
	dir := filepath.Dir(path)
	candidate := filepath.Join(dir, stem+ext)
	for i := 1; fileExists(candidate); i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%02d%s", stem, i, ext))
	}
	return candidate
}

func trimExt(name string) string {
	return name[:len(name)-len(filepath.Ext(name))]
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func toOutAction(a *deck.Action) *outAction {
	if a == nil {
		return nil
	}
	return &outAction{
		Type:      string(a.Type),
		Cmd:       a.Cmd,
		Keys:      a.Keys,
		CommandID: a.CommandID,
		URL:       a.URL,
		Page:      a.Page,
		Op:        a.Op,
		Minutes:   a.Minutes,
	}
}

// toOutWidget omits the default interval to keep configs short.
func toOutWidget(w *deck.Widget) *outWidget {
	if w == nil {
		return nil
	}
	ow := &outWidget{
		Type:      string(w.Type),
		Title:     w.Title,
		Format:    w.Format,
		Items:     w.Items,
		Cmd:       w.Cmd,
		Metric:    w.Metric,
		Paths:     w.Paths,
		OkColor:   w.OkColor,
		FailColor: w.FailColor,
	}
	if w.IntervalS != deck.DefaultWidgetInterval {
		ow.IntervalS = w.IntervalS
	}
	return ow
}
