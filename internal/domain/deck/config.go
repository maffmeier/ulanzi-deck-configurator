package deck

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type ActionType string

const (
	ActionShell      ActionType = "shell"
	ActionShortcut   ActionType = "shortcut"
	ActionURL        ActionType = "url"
	ActionSwitchPage ActionType = "switch_page"
	ActionPredefined ActionType = "predefined_command"
)

type Action struct {
	Type      ActionType
	Cmd       string
	Keys      string
	URL       string
	Page      string
	CommandID string
}

func (a Action) Validate() error {
	required := map[ActionType]string{
		ActionShell:      a.Cmd,
		ActionShortcut:   a.Keys,
		ActionURL:        a.URL,
		ActionSwitchPage: a.Page,
		ActionPredefined: a.CommandID,
	}
	value, known := required[a.Type]
	if !known {
		return fmt.Errorf("unknown action type: %q", a.Type)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s action requires a value", a.Type)
	}
	return nil
}

// Clean drops fields that don't belong to the action type, e.g. a page
// left over after switching the type in the editor.
func (a Action) Clean() Action {
	clean := Action{Type: a.Type}
	switch a.Type {
	case ActionShell:
		clean.Cmd = a.Cmd
	case ActionShortcut:
		clean.Keys = a.Keys
	case ActionURL:
		clean.URL = a.URL
	case ActionSwitchPage:
		clean.Page = a.Page
	case ActionPredefined:
		clean.CommandID = a.CommandID
	}
	return clean
}

type TextStyle struct {
	BackgroundColor string
	TextColor       string
	Bold            bool
	Italic          bool
	Underline       bool
	FontFamily      string
	FontSize        int
}

func DefaultTextStyle() TextStyle {
	return TextStyle{
		BackgroundColor: DefaultTextBg,
		TextColor:       DefaultTextColor,
		FontFamily:      DefaultFontFamily,
		FontSize:        DefaultFontSize,
	}
}

var hexColorRe = regexp.MustCompile(`^#?[0-9A-Fa-f]{6}$`)

func NormalizeHexColor(value string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if !hexColorRe.MatchString(cleaned) {
		return "", fmt.Errorf("color values must be 6-digit hex strings like #112233, got %q", value)
	}
	return "#" + strings.ToUpper(strings.TrimPrefix(cleaned, "#")), nil
}

func (s TextStyle) Normalized() (TextStyle, error) {
	var err error
	if s.BackgroundColor, err = NormalizeHexColor(s.BackgroundColor); err != nil {
		return s, err
	}
	if s.TextColor, err = NormalizeHexColor(s.TextColor); err != nil {
		return s, err
	}
	s.FontFamily = strings.TrimSpace(s.FontFamily)
	if s.FontFamily == "" {
		s.FontFamily = DefaultFontFamily
	}
	if s.FontSize < 12 || s.FontSize > 96 {
		return s, errors.New("text_style.font_size must be in 12..96")
	}
	return s, nil
}

func (s TextStyle) IsDefault() bool {
	return s == DefaultTextStyle()
}

type Button struct {
	Index int
	// IconPath is the path as written in the config; ResolvedIcon is the
	// absolute path used for rendering.
	IconPath     string
	ResolvedIcon string
	// IconData carries host-rendered images (small-window strip) that never
	// touch the filesystem.
	IconData  []byte
	Label     string
	Action    *Action
	TextStyle TextStyle
	// Press decides what holding the key does; LongPress is a second action
	// fired once the key is held past the long-press threshold (tap only).
	Press     PressMode
	LongPress *Action
}

type PressMode string

const (
	// PressTap fires the action once; with LongPress set, a short press
	// fires Action on release and a long press fires LongPress instead.
	PressTap PressMode = "tap"
	// PressRepeat fires the action on press and repeats it while held.
	PressRepeat PressMode = "repeat"
	// PressHold keeps a shortcut pressed while the key is held, e.g. for
	// push-to-talk.
	PressHold PressMode = "hold"
)

// ValidateBehavior checks that press mode, action and long press fit
// together; an empty mode is normalized to tap.
func (b *Button) ValidateBehavior() error {
	switch b.Press {
	case "":
		b.Press = PressTap
	case PressTap, PressRepeat, PressHold:
	default:
		return fmt.Errorf("unknown press mode %q (tap, repeat or hold)", b.Press)
	}
	if b.LongPress != nil {
		if b.Press != PressTap {
			return fmt.Errorf("long_press cannot be combined with press: %s", b.Press)
		}
		if err := b.LongPress.Validate(); err != nil {
			return fmt.Errorf("long_press: %w", err)
		}
	}
	switch b.Press {
	case PressRepeat:
		if b.Action == nil || b.Action.Type == ActionSwitchPage {
			return errors.New("press: repeat needs an action other than switch_page")
		}
	case PressHold:
		if b.Action == nil || (b.Action.Type != ActionShortcut && b.Action.Type != ActionPredefined) {
			return errors.New("press: hold only works with shortcut or predefined_command actions")
		}
	}
	return nil
}

type Page struct {
	Name    string
	Buttons []Button
}

func (p Page) ByIndex(idx int) *Button {
	for i := range p.Buttons {
		if p.Buttons[i].Index == idx {
			return &p.Buttons[i]
		}
	}
	return nil
}

type SmallWindow struct {
	Enabled              bool
	IntervalS            float64
	TimeFormat           string
	ShowMetrics          bool
	RotateEveryS         *float64
	BackgroundColor      string
	MetricsItems         []string
	TemperatureSensors   []string
	TemperatureSeparator string
}

func DefaultSmallWindow() SmallWindow {
	return SmallWindow{
		IntervalS:            2.0,
		TimeFormat:           DefaultTimeFormat,
		ShowMetrics:          true,
		BackgroundColor:      DefaultSmallWindowBg,
		TemperatureSeparator: " ",
	}
}

func (s SmallWindow) Rotates() bool {
	return s.ShowMetrics && s.RotateEveryS != nil
}

func (s SmallWindow) UsesCustomMetrics() bool {
	return len(s.MetricsItems) > 0
}

func (s SmallWindow) Normalized() (SmallWindow, error) {
	var err error
	if s.BackgroundColor, err = NormalizeHexColor(s.BackgroundColor); err != nil {
		return s, err
	}
	if s.IntervalS < SmallWindowMinInterval || s.IntervalS > SmallWindowMaxInterval {
		return s, fmt.Errorf("small_window.interval_s=%g out of range [%g, %g]",
			s.IntervalS, SmallWindowMinInterval, SmallWindowMaxInterval)
	}
	if s.RotateEveryS != nil && *s.RotateEveryS < SmallWindowMinInterval {
		return s, fmt.Errorf("small_window.rotate_every_s=%g out of range [%g, inf)",
			*s.RotateEveryS, SmallWindowMinInterval)
	}

	metrics := make([]string, 0, len(s.MetricsItems))
	for _, item := range s.MetricsItems {
		item = strings.ToLower(strings.TrimSpace(item))
		if slices.Contains(metrics, item) {
			return s, errors.New("small_window.metrics_items contains duplicates")
		}
		if !slices.Contains(MetricChoices, item) {
			return s, fmt.Errorf("small_window.metrics_items contains unsupported value %q", item)
		}
		metrics = append(metrics, item)
	}
	if len(metrics) > 3 {
		return s, errors.New("small_window.metrics_items must select between 1 and 3 items")
	}
	s.MetricsItems = metrics

	sensors := make([]string, 0, len(s.TemperatureSensors))
	for _, sensor := range s.TemperatureSensors {
		sensor = strings.TrimSpace(sensor)
		if sensor == "" {
			continue
		}
		if slices.Contains(sensors, sensor) {
			return s, errors.New("small_window.temperature_sensors contains duplicates")
		}
		sensors = append(sensors, sensor)
	}
	if len(sensors) > MaxTemperatureSensors {
		return s, fmt.Errorf("small_window.temperature_sensors must select at most %d sensors", MaxTemperatureSensors)
	}
	s.TemperatureSensors = sensors

	if s.TemperatureSeparator != " " && s.TemperatureSeparator != "|" {
		return s, errors.New("small_window.temperature_separator must be a space or pipe")
	}
	return s, nil
}

type Config struct {
	Pages        []Page
	FixedButtons []Button
	DefaultPage  string
	SmallWindow  SmallWindow
	Brightness   int
}

func (c *Config) Validate() error {
	if len(c.Pages) == 0 {
		return errors.New("config requires at least one page")
	}
	names := make([]string, 0, len(c.Pages))
	for _, p := range c.Pages {
		if slices.Contains(names, p.Name) {
			return fmt.Errorf("duplicate page name: %s", p.Name)
		}
		names = append(names, p.Name)
	}
	if !slices.Contains(names, c.DefaultPage) {
		sort.Strings(names)
		return fmt.Errorf("default_page %q is not in pages %q", c.DefaultPage, names)
	}
	if c.Brightness < 0 || c.Brightness > 100 {
		return fmt.Errorf("brightness must be in 0..100, got %d", c.Brightness)
	}
	// Overlapping indices would silently shadow each other.
	fixed := map[int]bool{}
	for _, b := range c.FixedButtons {
		fixed[b.Index] = true
	}
	for _, p := range c.Pages {
		var clash []int
		for _, b := range p.Buttons {
			if fixed[b.Index] {
				clash = append(clash, b.Index)
			}
		}
		if len(clash) > 0 {
			sort.Ints(clash)
			return fmt.Errorf("page %q reuses fixed_button indices %v", p.Name, clash)
		}
	}
	return nil
}

func (c *Config) Page(name string) (*Page, bool) {
	for i := range c.Pages {
		if c.Pages[i].Name == name {
			return &c.Pages[i], true
		}
	}
	return nil, false
}

func (c *Config) HasPage(name string) bool {
	_, ok := c.Page(name)
	return ok
}

// Special switch_page targets that page relative to the current page.
const (
	PageNext     = "@next"
	PagePrevious = "@prev"
)

// ResolvePageTarget turns a switch_page target into a concrete page name,
// wrapping around at both ends for @next/@prev.
func (c *Config) ResolvePageTarget(current, target string) (string, bool) {
	step := 0
	switch target {
	case PageNext:
		step = 1
	case PagePrevious:
		step = -1
	default:
		return target, c.HasPage(target)
	}
	if len(c.Pages) == 0 {
		return "", false
	}
	pos := 0
	for i, p := range c.Pages {
		if p.Name == current {
			pos = i
			break
		}
	}
	n := len(c.Pages)
	return c.Pages[((pos+step)%n+n)%n].Name, true
}

func (c *Config) PageNames() []string {
	names := make([]string, len(c.Pages))
	for i, p := range c.Pages {
		names[i] = p.Name
	}
	return names
}

// ButtonsFor returns the on-screen layout of a page: its buttons plus the
// fixed buttons shown on every page.
func (c *Config) ButtonsFor(name string) []Button {
	page, ok := c.Page(name)
	if !ok {
		return slices.Clone(c.FixedButtons)
	}
	return append(slices.Clone(page.Buttons), c.FixedButtons...)
}

// ButtonAt resolves the button for a physical index; fixed buttons win.
func (c *Config) ButtonAt(pageName string, index int) *Button {
	for i := range c.FixedButtons {
		if c.FixedButtons[i].Index == index {
			return &c.FixedButtons[i]
		}
	}
	page, ok := c.Page(pageName)
	if !ok {
		return nil
	}
	return page.ByIndex(index)
}
