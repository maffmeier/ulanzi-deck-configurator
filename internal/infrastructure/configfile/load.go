// Package configfile reads and writes the deck.yaml format shared with the
// Python ulanzi-linux project, so existing configs keep working.
package configfile

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"ulanzi-deck/internal/domain/deck"
)

type rawAction struct {
	Type      string `yaml:"type"`
	Cmd       string `yaml:"cmd"`
	Keys      string `yaml:"keys"`
	URL       string `yaml:"url"`
	Page      string `yaml:"page"`
	CommandID string `yaml:"command_id"`
}

type rawTextStyle struct {
	BackgroundColor *string `yaml:"background_color"`
	TextColor       *string `yaml:"text_color"`
	Bold            bool    `yaml:"bold"`
	Italic          bool    `yaml:"italic"`
	Underline       bool    `yaml:"underline"`
	FontFamily      *string `yaml:"font_family"`
	FontSize        *int    `yaml:"font_size"`
}

type rawButton struct {
	Index     *int          `yaml:"index"`
	Icon      string        `yaml:"icon"`
	Label     string        `yaml:"label"`
	Action    *rawAction    `yaml:"action"`
	TextStyle *rawTextStyle `yaml:"text_style"`
}

type rawSmallWindow struct {
	Enabled              bool     `yaml:"enabled"`
	IntervalS            *float64 `yaml:"interval_s"`
	TimeFormat           *string  `yaml:"time_format"`
	ShowMetrics          *bool    `yaml:"show_metrics"`
	RotateEveryS         any      `yaml:"rotate_every_s"`
	BackgroundColor      *string  `yaml:"background_color"`
	MetricsItems         []string `yaml:"metrics_items"`
	TemperatureSensors   []string `yaml:"temperature_sensors"`
	TemperatureSeparator *string  `yaml:"temperature_separator"`
}

type rawPage struct {
	Buttons []rawButton `yaml:"buttons"`
}

type rawConfig struct {
	DefaultPage  string          `yaml:"default_page"`
	Brightness   *int            `yaml:"brightness"`
	SmallWindow  *rawSmallWindow `yaml:"small_window"`
	Pages        yaml.Node       `yaml:"pages"`
	FixedButtons []rawButton     `yaml:"fixed_buttons"`
	Buttons      []rawButton     `yaml:"buttons"`
}

// Load parses the YAML file at path.
func Load(path string) (*deck.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data, filepath.Dir(path))
}

// Parse decodes YAML text; relative icon paths resolve against baseDir.
func Parse(data []byte, baseDir string) (*deck.Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &deck.Config{Brightness: deck.DefaultBrightness}
	if raw.Brightness != nil {
		cfg.Brightness = *raw.Brightness
	}

	sw, err := parseSmallWindow(raw.SmallWindow)
	if err != nil {
		return nil, err
	}
	cfg.SmallWindow = sw

	if raw.Pages.Kind != 0 {
		if raw.Pages.Kind != yaml.MappingNode || len(raw.Pages.Content) == 0 {
			return nil, errors.New("'pages:' block is empty")
		}
		for i := 0; i < len(raw.Pages.Content); i += 2 {
			name := raw.Pages.Content[i].Value
			var page rawPage
			if err := raw.Pages.Content[i+1].Decode(&page); err != nil {
				return nil, fmt.Errorf("page %q: %w", name, err)
			}
			buttons, err := parseButtons(page.Buttons, "page:"+name, baseDir)
			if err != nil {
				return nil, err
			}
			cfg.Pages = append(cfg.Pages, deck.Page{Name: name, Buttons: buttons})
		}
		if cfg.FixedButtons, err = parseButtons(raw.FixedButtons, "fixed_buttons", baseDir); err != nil {
			return nil, err
		}
		cfg.DefaultPage = raw.DefaultPage
		if cfg.DefaultPage == "" {
			cfg.DefaultPage = cfg.Pages[0].Name
		}
	} else {
		// Legacy single-page schema with a flat "buttons:" list.
		buttons, err := parseButtons(raw.Buttons, "buttons", baseDir)
		if err != nil {
			return nil, err
		}
		cfg.Pages = []deck.Page{{Name: deck.DefaultPageName, Buttons: buttons}}
		cfg.DefaultPage = deck.DefaultPageName
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseSmallWindow(raw *rawSmallWindow) (deck.SmallWindow, error) {
	sw := deck.DefaultSmallWindow()
	if raw == nil {
		return sw, nil
	}
	sw.Enabled = raw.Enabled
	if raw.IntervalS != nil {
		sw.IntervalS = *raw.IntervalS
	}
	if raw.TimeFormat != nil {
		sw.TimeFormat = *raw.TimeFormat
	}
	if raw.ShowMetrics != nil {
		sw.ShowMetrics = *raw.ShowMetrics
	}
	if raw.BackgroundColor != nil {
		sw.BackgroundColor = *raw.BackgroundColor
	}
	if raw.TemperatureSeparator != nil {
		sw.TemperatureSeparator = *raw.TemperatureSeparator
	}
	sw.MetricsItems = raw.MetricsItems
	sw.TemperatureSensors = raw.TemperatureSensors

	switch v := raw.RotateEveryS.(type) {
	case nil:
	case int:
		f := float64(v)
		sw.RotateEveryS = &f
	case float64:
		sw.RotateEveryS = &v
	case string:
		if strings.TrimSpace(v) != "" {
			f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return sw, fmt.Errorf("small_window.rotate_every_s: %w", err)
			}
			sw.RotateEveryS = &f
		}
	default:
		return sw, fmt.Errorf("small_window.rotate_every_s has unsupported type %T", v)
	}
	return sw.Normalized()
}

func parseButtons(raws []rawButton, scope, baseDir string) ([]deck.Button, error) {
	buttons := make([]deck.Button, 0, len(raws))
	for _, raw := range raws {
		if raw.Index == nil {
			return nil, fmt.Errorf("%s: button without index", scope)
		}
		button, err := parseButton(raw, baseDir)
		if err != nil {
			return nil, fmt.Errorf("%s: button %d: %w", scope, *raw.Index, err)
		}
		if button.Index < 0 || button.Index >= deck.SlotCount {
			slog.Warn("unsupported button ignored", "scope", scope, "index", button.Index, "supported_range", "0..13")
			continue
		}
		if button.Index == deck.InfoWindowIndex && (button.Label != "" || button.IconPath != "") {
			slog.Warn("info window visual ignored", "scope", scope)
		}
		buttons = append(buttons, button)
	}
	return buttons, nil
}

func parseButton(raw rawButton, baseDir string) (deck.Button, error) {
	style := deck.DefaultTextStyle()
	if ts := raw.TextStyle; ts != nil {
		if ts.BackgroundColor != nil {
			style.BackgroundColor = *ts.BackgroundColor
		}
		if ts.TextColor != nil {
			style.TextColor = *ts.TextColor
		}
		if ts.FontFamily != nil {
			style.FontFamily = *ts.FontFamily
		}
		if ts.FontSize != nil {
			style.FontSize = *ts.FontSize
		}
		style.Bold, style.Italic, style.Underline = ts.Bold, ts.Italic, ts.Underline
	}
	style, err := style.Normalized()
	if err != nil {
		return deck.Button{}, err
	}

	button := deck.Button{
		Index:     *raw.Index,
		Label:     raw.Label,
		TextStyle: style,
		IconPath:  raw.Icon,
	}
	if raw.Icon != "" {
		button.ResolvedIcon = ResolvePath(raw.Icon, baseDir)
	}
	if raw.Action != nil {
		action := deck.Action{
			Type:      deck.ActionType(raw.Action.Type),
			Cmd:       raw.Action.Cmd,
			Keys:      raw.Action.Keys,
			URL:       raw.Action.URL,
			Page:      raw.Action.Page,
			CommandID: raw.Action.CommandID,
		}
		if err := action.Validate(); err != nil {
			return deck.Button{}, err
		}
		button.Action = &action
	}
	return button, nil
}

// ResolvePath expands "~" and makes relative paths absolute against baseDir.
func ResolvePath(path, baseDir string) string {
	path = ExpandHome(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path)
}

func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// CompactPath renders an absolute path relative to the home directory as
// "~/...", which keeps configs portable between machines.
func CompactPath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(ExpandHome(path))
	if err != nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs
	}
	if rel == "." {
		return "~"
	}
	return "~/" + filepath.ToSlash(rel)
}
