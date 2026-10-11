package deck

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type ActionType string

const (
	ActionShell      ActionType = "shell"
	ActionShortcut   ActionType = "shortcut"
	ActionURL        ActionType = "url"
	ActionSwitchPage ActionType = "switch_page"
	ActionPredefined ActionType = "predefined_command"
	ActionTimer      ActionType = "timer"
	ActionHTTP       ActionType = "http"
	ActionTypeText   ActionType = "type_text"
	ActionMulti      ActionType = "multi"
	ActionDelay      ActionType = "delay"
	ActionHome       ActionType = "home_assistant"
	ActionOBS        ActionType = "obs"
	ActionAudio      ActionType = "audio"
	ActionMeeting    ActionType = "meeting"
	ActionMQTT       ActionType = "mqtt"
	ActionPlugin     ActionType = "plugin"
)

// MeetingOps lists what each meeting app can do from the deck.
var MeetingOps = map[string][]string{
	"teams":   {"mute", "camera", "hand", "blur", "leave"},
	"zoom":    {"mute", "camera", "hand", "share", "leave"},
	"discord": {"mute", "deafen"},
	"meet":    {"mute", "camera", "hand"},
}

// Timer operations for ActionTimer.
const (
	TimerToggle = "toggle"
	TimerStart  = "start"
	TimerPause  = "pause"
	TimerReset  = "reset"
)

// OBS operations for ActionOBS; Target names the scene or input.
var OBSOps = []string{
	"switch_scene", "toggle_record", "start_record", "stop_record",
	"toggle_stream", "start_stream", "stop_stream", "toggle_mute", "toggle_replay_buffer", "save_replay",
}

// Audio operations. Target is an application for volume and mute (empty:
// the default output) and a device name for output_set.
const (
	AudioVolumeUp   = "volume_up"
	AudioVolumeDown = "volume_down"
	AudioMute       = "mute"
	AudioMicMute    = "mic_mute"
	AudioOutputNext = "output_next"
	AudioOutputSet  = "output_set"

	DefaultAudioStep = 5
)

var AudioOps = []string{AudioVolumeUp, AudioVolumeDown, AudioMute, AudioMicMute, AudioOutputNext, AudioOutputSet}

// ValidateMQTTTopic accepts concrete topics only: actions publish to them
// and widgets show the one value they carry.
func ValidateMQTTTopic(topic string) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("mqtt needs a topic")
	}
	if strings.ContainsAny(topic, "+#") {
		return fmt.Errorf("mqtt topic %q must not contain wildcards", topic)
	}
	return nil
}

// Special switch_page targets that page relative to the current page.
const (
	PageNext     = "@next"
	PagePrevious = "@prev"
	// PageBack returns to the page shown before the current one.
	PageBack = "@back"
)

const maxMultiSteps = 50

type Action struct {
	Type      ActionType
	Cmd       string
	Keys      string
	URL       string
	Page      string
	CommandID string
	// Op is the operation of timer and obs actions.
	Op      string
	Minutes int
	// HTTP request.
	Method  string
	Body    string
	Headers map[string]string
	// Text typed by type_text.
	Text string
	// Steps of a multi action, run in order; delay steps pause.
	Steps []Action
	Ms    int
	// Home Assistant service call, e.g. light.toggle on light.kitchen.
	Service  string
	EntityID string
	// Target is the OBS scene or input an op applies to, or the app or
	// device of an audio action.
	Target string
	// Step is the volume change of an audio action in percent.
	Step int
	// MQTT message; the payload is Body.
	Topic  string
	Retain bool
	// App is the meeting application of a meeting action.
	App string
	// Plugin action: Plugin names the plugin, Op its action.
	Plugin   string
	Settings map[string]string
}

func (a Action) Validate() error {
	return a.validate(false)
}

func requireValue(t ActionType, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s action requires a value", t)
	}
	return nil
}

func (a Action) validate(inMulti bool) error {
	switch a.Type {
	case ActionShell:
		return requireValue(a.Type, a.Cmd)
	case ActionShortcut:
		return requireValue(a.Type, a.Keys)
	case ActionURL:
		return requireValue(a.Type, a.URL)
	case ActionSwitchPage:
		return requireValue(a.Type, a.Page)
	case ActionPredefined:
		return requireValue(a.Type, a.CommandID)
	case ActionTypeText:
		return requireValue(a.Type, a.Text)
	case ActionTimer:
		switch a.Op {
		case TimerToggle, TimerStart, TimerPause, TimerReset:
		default:
			return fmt.Errorf("timer action needs op toggle, start, pause or reset, got %q", a.Op)
		}
		if a.Minutes < 0 || a.Minutes > 24*60 {
			return fmt.Errorf("timer minutes must be in 0..1440, got %d", a.Minutes)
		}
	case ActionHTTP:
		if !strings.HasPrefix(a.URL, "http://") && !strings.HasPrefix(a.URL, "https://") {
			return fmt.Errorf("http action needs an http(s) url, got %q", a.URL)
		}
		switch strings.ToUpper(a.Method) {
		case "", http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return fmt.Errorf("unsupported http method %q", a.Method)
		}
	case ActionMulti:
		if inMulti {
			return fmt.Errorf("multi actions cannot be nested")
		}
		if len(a.Steps) == 0 || len(a.Steps) > maxMultiSteps {
			return fmt.Errorf("multi action needs 1 to %d steps", maxMultiSteps)
		}
		for i, step := range a.Steps {
			if err := step.validate(true); err != nil {
				return fmt.Errorf("step %d: %w", i+1, err)
			}
		}
	case ActionDelay:
		if !inMulti {
			return fmt.Errorf("delay is only allowed inside a multi action")
		}
		if a.Ms <= 0 || a.Ms > 60000 {
			return fmt.Errorf("delay must be 1..60000 ms, got %d", a.Ms)
		}
	case ActionHome:
		if !strings.Contains(a.Service, ".") {
			return fmt.Errorf("home_assistant action needs a service like light.toggle, got %q", a.Service)
		}
	case ActionOBS:
		if !slices.Contains(OBSOps, a.Op) {
			return fmt.Errorf("unknown obs op %q", a.Op)
		}
		if (a.Op == "switch_scene" || a.Op == "toggle_mute") && strings.TrimSpace(a.Target) == "" {
			return fmt.Errorf("obs %s needs a target", a.Op)
		}
	case ActionMQTT:
		if err := ValidateMQTTTopic(a.Topic); err != nil {
			return err
		}
	case ActionPlugin:
		if strings.TrimSpace(a.Plugin) == "" || strings.TrimSpace(a.Op) == "" {
			return fmt.Errorf("plugin action needs plugin and op")
		}
	case ActionMeeting:
		ops, ok := MeetingOps[a.App]
		if !ok {
			return fmt.Errorf("meeting app must be teams, zoom, discord or meet, got %q", a.App)
		}
		if !slices.Contains(ops, a.Op) {
			return fmt.Errorf("%s supports %v, not %q", a.App, ops, a.Op)
		}
	case ActionAudio:
		if !slices.Contains(AudioOps, a.Op) {
			return fmt.Errorf("unknown audio op %q", a.Op)
		}
		if a.Op == AudioOutputSet && strings.TrimSpace(a.Target) == "" {
			return fmt.Errorf("audio output_set needs the device name as target")
		}
		if a.Step < 0 || a.Step > 100 {
			return fmt.Errorf("audio step must be in 0..100, got %d", a.Step)
		}
	default:
		return fmt.Errorf("unknown action type: %q", a.Type)
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
	case ActionTimer:
		clean.Op, clean.Minutes = a.Op, a.Minutes
	case ActionHTTP:
		clean.URL, clean.Method, clean.Body = a.URL, strings.ToUpper(a.Method), a.Body
		if len(a.Headers) > 0 {
			clean.Headers = a.Headers
		}
	case ActionTypeText:
		clean.Text = a.Text
	case ActionMulti:
		for _, s := range a.Steps {
			clean.Steps = append(clean.Steps, s.Clean())
		}
	case ActionDelay:
		clean.Ms = a.Ms
	case ActionHome:
		clean.Service, clean.EntityID = a.Service, a.EntityID
	case ActionOBS:
		clean.Op, clean.Target = a.Op, a.Target
	case ActionMeeting:
		clean.App, clean.Op = a.App, a.Op
	case ActionMQTT:
		clean.Topic, clean.Body, clean.Retain = a.Topic, a.Body, a.Retain
	case ActionPlugin:
		clean.Plugin, clean.Op, clean.Settings = a.Plugin, a.Op, a.Settings
	case ActionAudio:
		clean.Op = a.Op
		switch a.Op {
		case AudioVolumeUp, AudioVolumeDown:
			clean.Target, clean.Step = a.Target, a.Step
		case AudioMute, AudioOutputSet:
			clean.Target = a.Target
		}
	}
	return clean
}
