package web

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/configfile"
)

// The JSON shapes below are the HTTP contract of the editor frontend
// (static/app.js); they mirror the ulanzi-linux web API.

type editorAction struct {
	Type      string `json:"type"`
	Cmd       string `json:"cmd"`
	Keys      string `json:"keys"`
	CommandID string `json:"command_id"`
	URL       string `json:"url"`
	Page      string `json:"page"`
}

type editorTextStyle struct {
	BackgroundColor string `json:"background_color"`
	TextColor       string `json:"text_color"`
	Bold            bool   `json:"bold"`
	Italic          bool   `json:"italic"`
	Underline       bool   `json:"underline"`
	FontFamily      string `json:"font_family"`
	FontSize        int    `json:"font_size"`
}

type editorButton struct {
	Index      int             `json:"index"`
	Label      string          `json:"label"`
	IconPath   *string         `json:"icon_path"`
	PreviewURL *string         `json:"preview_url"`
	Action     editorAction    `json:"action"`
	TextStyle  editorTextStyle `json:"text_style"`
	Press      string          `json:"press"`
	LongPress  *editorAction   `json:"long_press"`
}

type editorPage struct {
	Name    string         `json:"name"`
	Buttons []editorButton `json:"buttons"`
}

type editorSmallWindow struct {
	Enabled              bool     `json:"enabled"`
	IntervalS            float64  `json:"interval_s"`
	TimeFormat           string   `json:"time_format"`
	ShowMetrics          bool     `json:"show_metrics"`
	RotateEveryS         *float64 `json:"rotate_every_s"`
	BackgroundColor      string   `json:"background_color"`
	MetricsItems         []string `json:"metrics_items"`
	TemperatureSensors   []string `json:"temperature_sensors"`
	TemperatureSeparator string   `json:"temperature_separator"`
}

type editorConfig struct {
	Path                    string            `json:"path"`
	ConfigExists            bool              `json:"config_exists"`
	DefaultPage             string            `json:"default_page"`
	Brightness              int               `json:"brightness"`
	Pages                   []editorPage      `json:"pages"`
	FixedButtons            []editorButton    `json:"fixed_buttons"`
	SmallWindow             editorSmallWindow `json:"small_window"`
	VersionedConfigPath     *string           `json:"versioned_config_path"`
	SavedFirmwareBundlePath *string           `json:"saved_firmware_bundle_path"`
}

type editorPutRequest struct {
	DefaultPage        string            `json:"default_page"`
	Brightness         *int              `json:"brightness"`
	Pages              []editorPage      `json:"pages"`
	FixedButtons       []editorButton    `json:"fixed_buttons"`
	SmallWindow        editorSmallWindow `json:"small_window"`
	SaveFirmwareBundle bool              `json:"save_firmware_bundle"`
}

type pageSummary struct {
	Name        string `json:"name"`
	ButtonCount int    `json:"button_count"`
	Indices     []int  `json:"indices"`
}

type validationSummary struct {
	OK                      bool          `json:"ok"`
	Error                   *string       `json:"error"`
	DefaultPage             *string       `json:"default_page"`
	Pages                   []pageSummary `json:"pages"`
	FixedButtonIndices      []int         `json:"fixed_button_indices"`
	SmallWindowEnabled      bool          `json:"small_window_enabled"`
	VersionedConfigPath     *string       `json:"versioned_config_path"`
	SavedFirmwareBundlePath *string       `json:"saved_firmware_bundle_path"`
}

func failedSummary(err error) validationSummary {
	msg := err.Error()
	return validationSummary{OK: false, Error: &msg, Pages: []pageSummary{}, FixedButtonIndices: []int{}}
}

func summarize(cfg *deck.Config) validationSummary {
	s := validationSummary{OK: true, DefaultPage: &cfg.DefaultPage, SmallWindowEnabled: cfg.SmallWindow.Enabled, Pages: []pageSummary{}, FixedButtonIndices: indices(cfg.FixedButtons)}
	for _, p := range cfg.Pages {
		s.Pages = append(s.Pages, pageSummary{Name: p.Name, ButtonCount: len(p.Buttons), Indices: indices(p.Buttons)})
	}
	return s
}

func indices(buttons []deck.Button) []int {
	out := make([]int, 0, len(buttons))
	for _, b := range buttons {
		out = append(out, b.Index)
	}
	sort.Ints(out)
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func assetPreviewURL(path string) *string {
	if path == "" {
		return nil
	}
	return strPtr("/api/asset?path=" + queryEscape(path))
}

func toEditorButton(b deck.Button) editorButton {
	eb := editorButton{
		Index: b.Index,
		TextStyle: editorTextStyle{
			BackgroundColor: b.TextStyle.BackgroundColor,
			TextColor:       b.TextStyle.TextColor,
			Bold:            b.TextStyle.Bold,
			Italic:          b.TextStyle.Italic,
			Underline:       b.TextStyle.Underline,
			FontFamily:      b.TextStyle.FontFamily,
			FontSize:        b.TextStyle.FontSize,
		},
		Action: editorAction{Type: "none"},
	}
	if b.Index != deck.InfoWindowIndex {
		eb.Label = b.Label
		if b.IconPath != "" {
			compact := configfile.CompactPath(b.ResolvedIcon)
			eb.IconPath = &compact
			eb.PreviewURL = assetPreviewURL(compact)
		}
	}
	if a := b.Action; a != nil {
		eb.Action = toEditorAction(*a)
	}
	eb.Press = string(b.Press)
	if eb.Press == "" {
		eb.Press = string(deck.PressTap)
	}
	if b.LongPress != nil {
		lp := toEditorAction(*b.LongPress)
		eb.LongPress = &lp
	}
	return eb
}

func toEditorAction(a deck.Action) editorAction {
	return editorAction{Type: string(a.Type), Cmd: a.Cmd, Keys: a.Keys, CommandID: a.CommandID, URL: a.URL, Page: a.Page}
}

// fromEditorAction returns nil for "none"; only the type's own field is kept.
func fromEditorAction(ea editorAction) (*deck.Action, error) {
	if ea.Type == "" || ea.Type == "none" {
		return nil, nil
	}
	a := deck.Action{
		Type:      deck.ActionType(ea.Type),
		Cmd:       ea.Cmd,
		Keys:      ea.Keys,
		CommandID: ea.CommandID,
		URL:       ea.URL,
		Page:      ea.Page,
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	a = a.Clean()
	return &a, nil
}

func toEditorConfig(cfg *deck.Config, path string, exists bool) editorConfig {
	ec := editorConfig{
		Path:         path,
		ConfigExists: exists,
		DefaultPage:  cfg.DefaultPage,
		Brightness:   cfg.Brightness,
		Pages:        []editorPage{},
		FixedButtons: []editorButton{},
		SmallWindow: editorSmallWindow{
			Enabled:              cfg.SmallWindow.Enabled,
			IntervalS:            cfg.SmallWindow.IntervalS,
			TimeFormat:           cfg.SmallWindow.TimeFormat,
			ShowMetrics:          cfg.SmallWindow.ShowMetrics,
			RotateEveryS:         cfg.SmallWindow.RotateEveryS,
			BackgroundColor:      cfg.SmallWindow.BackgroundColor,
			MetricsItems:         nonNil(cfg.SmallWindow.MetricsItems),
			TemperatureSensors:   nonNil(cfg.SmallWindow.TemperatureSensors),
			TemperatureSeparator: cfg.SmallWindow.TemperatureSeparator,
		},
	}
	for _, p := range cfg.Pages {
		page := editorPage{Name: p.Name, Buttons: []editorButton{}}
		for _, b := range p.Buttons {
			page.Buttons = append(page.Buttons, toEditorButton(b))
		}
		ec.Pages = append(ec.Pages, page)
	}
	for _, b := range cfg.FixedButtons {
		ec.FixedButtons = append(ec.FixedButtons, toEditorButton(b))
	}
	return ec
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func defaultEditorConfig(path string) editorConfig {
	cfg := &deck.Config{
		Pages:       []deck.Page{{Name: deck.EditorDefaultPage}},
		DefaultPage: deck.EditorDefaultPage,
		SmallWindow: deck.DefaultSmallWindow(),
		Brightness:  deck.DefaultBrightness,
	}
	return toEditorConfig(cfg, path, false)
}

func fromEditorButtons(buttons []editorButton, scope string, skip map[int]bool) ([]deck.Button, error) {
	seen := map[int]bool{}
	var result []deck.Button
	for _, eb := range buttons {
		if eb.Index < 0 || eb.Index >= deck.SlotCount {
			return nil, fmt.Errorf("button index %d is outside the visual editor layout (0..%d)", eb.Index, deck.SlotCount-1)
		}
		if seen[eb.Index] {
			return nil, fmt.Errorf("duplicate button index %d in %s", eb.Index, scope)
		}
		seen[eb.Index] = true
		if skip[eb.Index] {
			continue
		}

		b := deck.Button{Index: eb.Index, TextStyle: deck.DefaultTextStyle()}
		if eb.Index != deck.InfoWindowIndex {
			b.Label = eb.Label
			if eb.IconPath != nil {
				b.IconPath = strings.TrimSpace(*eb.IconPath)
			}
			ts := eb.TextStyle
			style := deck.TextStyle{
				BackgroundColor: orDefault(ts.BackgroundColor, deck.DefaultTextBg),
				TextColor:       orDefault(ts.TextColor, deck.DefaultTextColor),
				Bold:            ts.Bold,
				Italic:          ts.Italic,
				Underline:       ts.Underline,
				FontFamily:      orDefault(ts.FontFamily, deck.DefaultFontFamily),
				FontSize:        ts.FontSize,
			}
			if style.FontSize == 0 {
				style.FontSize = deck.DefaultFontSize
			}
			normalized, err := style.Normalized()
			if err != nil {
				return nil, err
			}
			b.TextStyle = normalized
		}
		where := fmt.Sprintf("%s, Taste %d", scope, eb.Index+1)
		action, err := fromEditorAction(eb.Action)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		b.Action = action
		if eb.LongPress != nil {
			if b.LongPress, err = fromEditorAction(*eb.LongPress); err != nil {
				return nil, fmt.Errorf("%s, langer Druck: %w", where, err)
			}
		}
		b.Press = deck.PressMode(eb.Press)
		if err := b.ValidateBehavior(); err != nil {
			return nil, fmt.Errorf("%s: %w", where, err)
		}
		result = append(result, b)
	}
	return result, nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// fromEditor converts the request into YAML text; validation then runs the
// very same loader the daemon uses, so "valid in the editor" can never
// diverge from "loadable by the daemon".
func fromEditor(req editorPutRequest) ([]byte, error) {
	fixed, err := fromEditorButtons(req.FixedButtons, "fixed_buttons", nil)
	if err != nil {
		return nil, err
	}
	fixedIdx := map[int]bool{}
	for _, b := range fixed {
		fixedIdx[b.Index] = true
	}

	cfg := &deck.Config{FixedButtons: fixed, Brightness: deck.DefaultBrightness}
	if req.Brightness != nil {
		cfg.Brightness = *req.Brightness
	}
	var names []string
	for _, p := range req.Pages {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			return nil, errors.New("page name cannot be empty")
		}
		if slices.Contains(names, name) {
			return nil, fmt.Errorf("duplicate page name: %s", name)
		}
		names = append(names, name)
		buttons, err := fromEditorButtons(p.Buttons, fmt.Sprintf("page %q", name), fixedIdx)
		if err != nil {
			return nil, err
		}
		cfg.Pages = append(cfg.Pages, deck.Page{Name: name, Buttons: buttons})
	}
	if len(cfg.Pages) == 0 {
		return nil, errors.New("at least one page is required")
	}
	cfg.DefaultPage = strings.TrimSpace(req.DefaultPage)
	if cfg.DefaultPage == "" {
		return nil, errors.New("default_page cannot be empty")
	}

	sw := req.SmallWindow
	cfg.SmallWindow = deck.SmallWindow{
		Enabled:              sw.Enabled,
		IntervalS:            sw.IntervalS,
		TimeFormat:           sw.TimeFormat,
		ShowMetrics:          sw.ShowMetrics,
		RotateEveryS:         sw.RotateEveryS,
		BackgroundColor:      orDefault(sw.BackgroundColor, deck.DefaultSmallWindowBg),
		MetricsItems:         sw.MetricsItems,
		TemperatureSensors:   sw.TemperatureSensors,
		TemperatureSeparator: orDefault(sw.TemperatureSeparator, " "),
	}
	if cfg.SmallWindow.IntervalS == 0 {
		cfg.SmallWindow.IntervalS = 2
	}
	return configfile.Marshal(cfg)
}
