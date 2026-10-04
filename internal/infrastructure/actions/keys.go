package actions

import (
	"fmt"
	"strings"
)

type Modifier string

const (
	ModCtrl  Modifier = "ctrl"
	ModAlt   Modifier = "alt"
	ModShift Modifier = "shift"
	ModSuper Modifier = "super"
)

// Combo is a parsed shortcut such as "ctrl+alt+t" or "super+Return".
// Key is a canonical X11 keysym name (Return, Escape, XF86AudioPlay, a, F5).
type Combo struct {
	Modifiers []Modifier
	Key       string
}

var modifierAliases = map[string]Modifier{
	"ctrl": ModCtrl, "control": ModCtrl, "strg": ModCtrl,
	"alt": ModAlt, "option": ModAlt, "opt": ModAlt,
	"shift": ModShift,
	"super": ModSuper, "win": ModSuper, "windows": ModSuper, "meta": ModSuper,
	"cmd": ModSuper, "command": ModSuper, "logo": ModSuper, "mod4": ModSuper,
	"super_l": ModSuper, "super_r": ModSuper,
}

// keyAliases maps lower-case spellings to canonical keysym names.
var keyAliases = map[string]string{
	"enter": "Return", "return": "Return", "ret": "Return",
	"esc": "Escape", "escape": "Escape",
	"tab": "Tab", "space": "space", "spc": "space",
	"backspace": "BackSpace", "bksp": "BackSpace",
	"delete": "Delete", "del": "Delete", "entf": "Delete",
	"insert": "Insert", "ins": "Insert",
	"home": "Home", "pos1": "Home", "end": "End",
	"pageup": "Prior", "page_up": "Prior", "pgup": "Prior", "prior": "Prior",
	"pagedown": "Next", "page_down": "Next", "pgdn": "Next", "next": "Next",
	"up": "Up", "down": "Down", "left": "Left", "right": "Right",
	"print": "Print", "printscreen": "Print", "druck": "Print",
	"capslock": "Caps_Lock", "caps_lock": "Caps_Lock",
	"menu":  "Menu",
	"minus": "minus", "plus": "plus", "equal": "equal", "comma": "comma",
	"period": "period", "slash": "slash", "backslash": "backslash",
	"semicolon": "semicolon", "apostrophe": "apostrophe", "grave": "grave",
	"bracketleft": "bracketleft", "bracketright": "bracketright",
	"xf86audioplay": "XF86AudioPlay", "playpause": "XF86AudioPlay", "media_play_pause": "XF86AudioPlay",
	"xf86audiopause": "XF86AudioPause",
	"xf86audiostop":  "XF86AudioStop", "mediastop": "XF86AudioStop",
	"xf86audionext": "XF86AudioNext", "medianext": "XF86AudioNext",
	"xf86audioprev": "XF86AudioPrev", "mediaprev": "XF86AudioPrev",
	"xf86audiomute": "XF86AudioMute", "mute": "XF86AudioMute",
	"xf86audiolowervolume": "XF86AudioLowerVolume", "volumedown": "XF86AudioLowerVolume",
	"xf86audioraisevolume": "XF86AudioRaiseVolume", "volumeup": "XF86AudioRaiseVolume",
	"xf86audiomicmute": "XF86AudioMicMute", "micmute": "XF86AudioMicMute",
}

var symbolKeys = map[string]string{
	"-": "minus", "=": "equal", ",": "comma", ".": "period", "/": "slash",
	`\`: "backslash", ";": "semicolon", "'": "apostrophe", "`": "grave",
	"[": "bracketleft", "]": "bracketright",
}

func ParseCombo(spec string) (Combo, error) {
	var combo Combo
	parts := strings.Split(strings.TrimSpace(spec), "+")
	// "ctrl++" means ctrl and the plus key.
	if strings.HasSuffix(spec, "++") {
		parts = append(parts[:len(parts)-2], "plus")
	}
	for i, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			return combo, fmt.Errorf("invalid shortcut %q", spec)
		}
		last := i == len(parts)-1
		if mod, ok := modifierAliases[strings.ToLower(part)]; ok && !last {
			combo.Modifiers = append(combo.Modifiers, mod)
			continue
		}
		if !last {
			return combo, fmt.Errorf("unknown modifier %q in %q", part, spec)
		}
		combo.Key = canonicalKey(part)
	}
	if combo.Key == "" {
		return combo, fmt.Errorf("shortcut %q has no key", spec)
	}
	return combo, nil
}

func canonicalKey(part string) string {
	lower := strings.ToLower(part)
	if name, ok := keyAliases[lower]; ok {
		return name
	}
	if name, ok := symbolKeys[part]; ok {
		return name
	}
	if mod, ok := modifierAliases[lower]; ok {
		// A lone modifier like "super" is pressed as its left key.
		switch mod {
		case ModSuper:
			return "Super_L"
		case ModCtrl:
			return "Control_L"
		case ModAlt:
			return "Alt_L"
		case ModShift:
			return "Shift_L"
		}
	}
	if len(lower) >= 2 && lower[0] == 'f' {
		if n := lower[1:]; isDigits(n) {
			return "F" + n
		}
	}
	if len(part) == 1 {
		return lower
	}
	return part
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
