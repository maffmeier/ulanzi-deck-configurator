package deck

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Trigger events: a time of day, or something happening on the host.
const (
	TriggerTime    = "time"
	TriggerStartup = "startup"
	TriggerConnect = "connect"
	TriggerLock    = "lock"
	TriggerUnlock  = "unlock"
	TriggerApp     = "app"
)

var TriggerEvents = []string{TriggerTime, TriggerStartup, TriggerConnect, TriggerLock, TriggerUnlock, TriggerApp}

// Weekdays as written in the config, Sunday first like time.Weekday.
var Weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Trigger runs Action when Event happens without a key press.
type Trigger struct {
	Event string
	// At is "HH:MM" for time triggers; Days limits them (empty: every day).
	At   string
	Days []string
	// App fires an app trigger when an application containing it gets focus.
	App    string
	Action Action
}

func (t Trigger) Validate() error {
	switch t.Event {
	case TriggerTime:
		if _, err := parseClock(t.At); err != nil {
			return fmt.Errorf("time trigger: %w", err)
		}
		for _, d := range t.Days {
			if !slices.Contains(Weekdays, d) {
				return fmt.Errorf("unknown weekday %q (use %s)", d, strings.Join(Weekdays, ", "))
			}
		}
	case TriggerApp:
		if strings.TrimSpace(t.App) == "" {
			return fmt.Errorf("app trigger needs an app")
		}
	case TriggerStartup, TriggerConnect, TriggerLock, TriggerUnlock:
	default:
		return fmt.Errorf("unknown trigger event %q", t.Event)
	}
	return t.Action.Validate()
}

// DueAt reports whether a time trigger fires in the minute of now.
func (t Trigger) DueAt(now time.Time) bool {
	if t.Event != TriggerTime {
		return false
	}
	minute, err := parseClock(t.At)
	if err != nil || now.Hour()*60+now.Minute() != minute {
		return false
	}
	return len(t.Days) == 0 || slices.Contains(t.Days, Weekdays[now.Weekday()])
}

// MatchesApp reports whether an app trigger fires for the focused app.
func (t Trigger) MatchesApp(app string) bool {
	return t.Event == TriggerApp && app != "" && strings.Contains(strings.ToLower(app), strings.ToLower(strings.TrimSpace(t.App)))
}

// TriggersFor returns the triggers of an event.
func (c *Config) TriggersFor(event string) []Trigger {
	var out []Trigger
	for _, t := range c.Triggers {
		if t.Event == event {
			out = append(out, t)
		}
	}
	return out
}
