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
	Type      string            `json:"type"`
	Cmd       string            `json:"cmd"`
	Keys      string            `json:"keys"`
	CommandID string            `json:"command_id"`
	URL       string            `json:"url"`
	Page      string            `json:"page"`
	Op        string            `json:"op"`
	Minutes   int               `json:"minutes"`
	Method    string            `json:"method"`
	Body      string            `json:"body"`
	Headers   map[string]string `json:"headers"`
	Text      string            `json:"text"`
	Steps     []editorAction    `json:"steps"`
	Ms        int               `json:"ms"`
	Service   string            `json:"service"`
	EntityID  string            `json:"entity_id"`
	Target    string            `json:"target"`
}

type editorToggle struct {
	Label      string        `json:"label"`
	IconPath   *string       `json:"icon_path"`
	PreviewURL *string       `json:"preview_url"`
	Action     *editorAction `json:"action"`
}

type editorWidget struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Format      string   `json:"format"`
	Items       []string `json:"items"`
	Cmd         string   `json:"cmd"`
	IntervalS   float64  `json:"interval_s"`
	Metric      string   `json:"metric"`
	Paths       []string `json:"paths"`
	OkColor     string   `json:"ok_color"`
	FailColor   string   `json:"fail_color"`
	EntityID    string   `json:"entity_id"`
	Attribute   string   `json:"attribute"`
	ActiveState string   `json:"active_state"`
	Show        string   `json:"show"`
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
	Live       *editorWidget   `json:"live"`
	Toggle     *editorToggle   `json:"toggle"`
}

type editorPage struct {
	Name      string         `json:"name"`
	Buttons   []editorButton `json:"buttons"`
	Folder    bool           `json:"folder"`
	BackIndex int            `json:"back_index"`
	Apps      []string       `json:"apps"`
}

type editorDisplay struct {
	DimAfterS     int    `json:"dim_after_s"`
	DimBrightness int    `json:"dim_brightness"`
	OffWhenLocked bool   `json:"off_when_locked"`
	NightEnabled  bool   `json:"night_enabled"`
	NightFrom     string `json:"night_from"`
	NightTo       string `json:"night_to"`
	NightBright   int    `json:"night_brightness"`
	PressFeedback bool   `json:"press_feedback"`
}

type editorSmallWindow struct {
	Enabled              bool           `json:"enabled"`
	IntervalS            float64        `json:"interval_s"`
	TimeFormat           string         `json:"time_format"`
	ShowMetrics          bool           `json:"show_metrics"`
	RotateEveryS         *float64       `json:"rotate_every_s"`
	BackgroundColor      string         `json:"background_color"`
	MetricsItems         []string       `json:"metrics_items"`
	TemperatureSensors   []string       `json:"temperature_sensors"`
	TemperatureSeparator string         `json:"temperature_separator"`
	Widgets              []editorWidget `json:"widgets"`
}

type editorConfig struct {
	Path                    string            `json:"path"`
	ConfigExists            bool              `json:"config_exists"`
	DefaultPage             string            `json:"default_page"`
	Brightness              int               `json:"brightness"`
	Display                 editorDisplay     `json:"display"`
	Language                string            `json:"language"`
	Pages                   []editorPage      `json:"pages"`
	FixedButtons            []editorButton    `json:"fixed_buttons"`
	SmallWindow             editorSmallWindow `json:"small_window"`
	VersionedConfigPath     *string           `json:"versioned_config_path"`
	SavedFirmwareBundlePath *string           `json:"saved_firmware_bundle_path"`
}

type editorPutRequest struct {
	DefaultPage        string            `json:"default_page"`
	Brightness         *int              `json:"brightness"`
	Display            editorDisplay     `json:"display"`
	Language           string            `json:"language"`
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
	if b.Live != nil {
		live := toEditorWidget(*b.Live)
		eb.Live = &live
	}
	if t := b.Toggle; t != nil {
		et := &editorToggle{Label: t.Label}
		if t.IconPath != "" {
			compact := configfile.CompactPath(t.ResolvedIcon)
			et.IconPath, et.PreviewURL = &compact, assetPreviewURL(compact)
		}
		if t.Action != nil {
			ta := toEditorAction(*t.Action)
			et.Action = &ta
		}
		eb.Toggle = et
	}
	return eb
}

func toEditorAction(a deck.Action) editorAction {
	ea := editorAction{
		Type: string(a.Type), Cmd: a.Cmd, Keys: a.Keys, CommandID: a.CommandID, URL: a.URL, Page: a.Page,
		Op: a.Op, Minutes: a.Minutes, Method: a.Method, Body: a.Body, Headers: a.Headers, Text: a.Text,
		Ms: a.Ms, Service: a.Service, EntityID: a.EntityID, Target: a.Target, Steps: []editorAction{},
	}
	for _, step := range a.Steps {
		ea.Steps = append(ea.Steps, toEditorAction(step))
	}
	return ea
}

func toAction(ea editorAction) deck.Action {
	a := deck.Action{
		Type: deck.ActionType(ea.Type), Cmd: ea.Cmd, Keys: ea.Keys, CommandID: ea.CommandID, URL: ea.URL,
		Page: ea.Page, Op: ea.Op, Minutes: ea.Minutes, Method: ea.Method, Body: ea.Body, Text: ea.Text,
		Ms: ea.Ms, Service: strings.TrimSpace(ea.Service), EntityID: strings.TrimSpace(ea.EntityID), Target: ea.Target,
	}
	headers := map[string]string{}
	for k, v := range ea.Headers {
		if k = strings.TrimSpace(k); k != "" {
			headers[k] = v
		}
	}
	if len(headers) > 0 {
		a.Headers = headers
	}
	for _, step := range ea.Steps {
		a.Steps = append(a.Steps, toAction(step))
	}
	return a
}

func toEditorWidget(w deck.Widget) editorWidget {
	return editorWidget{
		Type: string(w.Type), Title: w.Title, Format: w.Format, Items: nonNil(w.Items), Cmd: w.Cmd,
		IntervalS: w.IntervalS, Metric: w.Metric, Paths: nonNil(w.Paths), OkColor: w.OkColor, FailColor: w.FailColor,
		EntityID: w.EntityID, Attribute: w.Attribute, ActiveState: w.ActiveState, Show: w.Show,
	}
}

// fromEditorWidget converts and validates; paths stay as written and are
// resolved by the config loader.
func fromEditorWidget(ew editorWidget, onKey bool, baseDir string) (deck.Widget, error) {
	w := deck.Widget{
		Type: deck.WidgetType(ew.Type), Title: strings.TrimSpace(ew.Title), Format: ew.Format, Items: ew.Items,
		Cmd: ew.Cmd, IntervalS: ew.IntervalS, Metric: ew.Metric, OkColor: ew.OkColor, FailColor: ew.FailColor,
		EntityID: strings.TrimSpace(ew.EntityID), Attribute: strings.TrimSpace(ew.Attribute),
		ActiveState: strings.TrimSpace(ew.ActiveState), Show: ew.Show,
	}
	for _, p := range ew.Paths {
		if p = strings.TrimSpace(p); p != "" {
			w.Paths = append(w.Paths, p)
			w.ResolvedPaths = append(w.ResolvedPaths, configfile.ResolvePath(p, baseDir))
		}
	}
	if err := w.Normalize(onKey); err != nil {
		return w, err
	}
	return w.Clean(), nil
}

// fromEditorAction returns nil for "none"; only the type's own field is kept.
func fromEditorAction(ea editorAction) (*deck.Action, error) {
	if ea.Type == "" || ea.Type == "none" {
		return nil, nil
	}
	a := toAction(ea)
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
		Display:      toEditorDisplay(cfg.Display),
		Language:     cfg.Language,
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
			Widgets:              toEditorWidgets(cfg.SmallWindow.Widgets),
			TemperatureSensors:   nonNil(cfg.SmallWindow.TemperatureSensors),
			TemperatureSeparator: cfg.SmallWindow.TemperatureSeparator,
		},
	}
	for _, p := range cfg.Pages {
		page := editorPage{Name: p.Name, Buttons: []editorButton{}, Folder: p.Folder, BackIndex: p.BackIndex, Apps: nonNil(p.Apps)}
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

func toEditorWidgets(ws []deck.Widget) []editorWidget {
	out := []editorWidget{}
	for _, w := range ws {
		out = append(out, toEditorWidget(w))
	}
	return out
}

func cleanList(items []string) []string {
	var out []string
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toEditorDisplay(d deck.Display) editorDisplay {
	ed := editorDisplay{DimAfterS: d.DimAfterS, DimBrightness: d.DimBrightness, OffWhenLocked: d.OffWhenLocked,
		NightFrom: "22:00", NightTo: "07:00", NightBright: 20, PressFeedback: d.PressFeedback}
	if n := d.Night; n != nil {
		ed.NightEnabled, ed.NightFrom, ed.NightTo, ed.NightBright = true, n.From, n.To, n.Brightness
	}
	return ed
}

func fromEditorDisplay(ed editorDisplay) deck.Display {
	d := deck.Display{DimAfterS: ed.DimAfterS, DimBrightness: ed.DimBrightness, OffWhenLocked: ed.OffWhenLocked, PressFeedback: ed.PressFeedback}
	if ed.NightEnabled {
		d.Night = &deck.NightMode{From: ed.NightFrom, To: ed.NightTo, Brightness: ed.NightBright}
	}
	return d
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
		if et := eb.Toggle; et != nil && eb.Index != deck.InfoWindowIndex {
			toggle := &deck.ToggleState{Label: et.Label}
			if et.IconPath != nil {
				toggle.IconPath = strings.TrimSpace(*et.IconPath)
			}
			if et.Action != nil {
				if toggle.Action, err = fromEditorAction(*et.Action); err != nil {
					return nil, fmt.Errorf("%s, zweiter Zustand: %w", where, err)
				}
			}
			b.Toggle = toggle
		}
		if eb.Live != nil && eb.Live.Type != "" && eb.Live.Type != "none" && eb.Index != deck.InfoWindowIndex {
			live, err := fromEditorWidget(*eb.Live, true, "")
			if err != nil {
				return nil, fmt.Errorf("%s, Live-Anzeige: %w", where, err)
			}
			b.Live = &live
		}
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
	cfg.Display = fromEditorDisplay(req.Display)
	cfg.Language = req.Language
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
		cfg.Pages = append(cfg.Pages, deck.Page{Name: name, Buttons: buttons, Folder: p.Folder, BackIndex: p.BackIndex, Apps: cleanList(p.Apps)})
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
	for i, ew := range sw.Widgets {
		w, err := fromEditorWidget(ew, false, "")
		if err != nil {
			return nil, fmt.Errorf("Infofenster, Widget %d: %w", i+1, err)
		}
		cfg.SmallWindow.Widgets = append(cfg.SmallWindow.Widgets, w)
	}
	return configfile.Marshal(cfg)
}
