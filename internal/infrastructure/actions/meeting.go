package actions

import (
	"fmt"
	"runtime"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// meetingKeys are the default in-app shortcuts, per app and op: the first
// entry for Windows and Linux, the second for macOS. Zoom and Discord can
// make them global in their settings, otherwise the app needs focus.
var meetingKeys = map[string]map[string][2]string{
	"teams": {
		"mute":   {"ctrl+shift+m", "cmd+shift+m"},
		"camera": {"ctrl+shift+o", "cmd+shift+o"},
		"hand":   {"ctrl+shift+k", "cmd+shift+k"},
		"blur":   {"ctrl+shift+p", "cmd+shift+p"},
		"leave":  {"ctrl+shift+h", "cmd+shift+h"},
	},
	"zoom": {
		"mute":   {"alt+a", "cmd+shift+a"},
		"camera": {"alt+v", "cmd+shift+v"},
		"hand":   {"alt+y", "alt+y"},
		"share":  {"alt+s", "cmd+shift+s"},
		"leave":  {"alt+q", "cmd+w"},
	},
	"discord": {
		"mute":   {"ctrl+shift+m", "cmd+shift+m"},
		"deafen": {"ctrl+shift+d", "cmd+shift+d"},
	},
	"meet": {
		"mute":   {"ctrl+d", "cmd+d"},
		"camera": {"ctrl+e", "cmd+e"},
		"hand":   {"ctrl+alt+h", "ctrl+cmd+h"},
	},
}

// MeetingShortcut is the shortcut action a meeting action falls back to.
func MeetingShortcut(app, op string) (deck.Action, error) {
	keys, ok := meetingKeys[app][op]
	if !ok {
		return deck.Action{}, fmt.Errorf("no shortcut for %s %s", app, op)
	}
	if runtime.GOOS == "darwin" {
		return deck.Action{Type: deck.ActionShortcut, Keys: keys[1]}, nil
	}
	return deck.Action{Type: deck.ActionShortcut, Keys: keys[0]}, nil
}
