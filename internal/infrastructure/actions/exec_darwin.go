package actions

import (
	"fmt"
	"strings"
)

var macKeyCodes = map[string]int{
	"a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9,
	"b": 11, "q": 12, "w": 13, "e": 14, "r": 15, "y": 16, "t": 17,
	"1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "equal": 24, "plus": 24, "9": 25,
	"7": 26, "minus": 27, "8": 28, "0": 29, "bracketright": 30, "o": 31, "u": 32,
	"bracketleft": 33, "i": 34, "p": 35, "Return": 36, "l": 37, "j": 38, "apostrophe": 39,
	"k": 40, "semicolon": 41, "backslash": 42, "comma": 43, "slash": 44, "n": 45, "m": 46,
	"period": 47, "Tab": 48, "space": 49, "grave": 50, "BackSpace": 51, "Escape": 53,
	"Caps_Lock": 57,
	"F1":        122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97, "F7": 98, "F8": 100,
	"F9": 101, "F10": 109, "F11": 103, "F12": 111, "F13": 105, "F14": 107, "F15": 113,
	"Insert": 114, "Home": 115, "Prior": 116, "Delete": 117, "End": 119, "Next": 121,
	"Left": 123, "Right": 124, "Down": 125, "Up": 126,
}

var macModifiers = map[Modifier]string{
	ModCtrl: "control down", ModAlt: "option down", ModShift: "shift down", ModSuper: "command down",
}

// Media keys have no System Events equivalent; volume is driven through
// AppleScript, playback through the Music app.
var macMediaScripts = map[string]string{
	"XF86AudioMute":        `set volume output muted not (output muted of (get volume settings))`,
	"XF86AudioRaiseVolume": `set volume output volume ((output volume of (get volume settings)) + 6)`,
	"XF86AudioLowerVolume": `set volume output volume ((output volume of (get volume settings)) - 6)`,
	"XF86AudioMicMute":     `if input volume of (get volume settings) > 0 then set volume input volume 0 else set volume input volume 75`,
	"XF86AudioPlay":        `tell application "Music" to playpause`,
	"XF86AudioPause":       `tell application "Music" to pause`,
	"XF86AudioNext":        `tell application "Music" to next track`,
	"XF86AudioPrev":        `tell application "Music" to previous track`,
}

// sendCombo needs the Accessibility permission for ulanzi-deck in
// System Settings > Privacy & Security.
func (r *Runner) sendCombo(c Combo) error {
	if script, ok := macMediaScripts[c.Key]; ok && len(c.Modifiers) == 0 {
		return r.run("osascript", "-e", script)
	}
	code, ok := macKeyCodes[c.Key]
	if !ok {
		return fmt.Errorf("key %q is not supported on macOS", c.Key)
	}
	script := fmt.Sprintf(`tell application "System Events" to key code %d`, code)
	if len(c.Modifiers) > 0 {
		mods := make([]string, len(c.Modifiers))
		for i, m := range c.Modifiers {
			mods[i] = macModifiers[m]
		}
		script += " using {" + strings.Join(mods, ", ") + "}"
	}
	return r.run("osascript", "-e", script)
}

func (r *Runner) openURL(url string) error {
	return r.start("open", url)
}
