package actions

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// wtypeLinger keeps wtype's virtual keyboard alive after the keys were
// sent. If it vanishes while a freshly launched grab client (slurp, rofi)
// starts, the client cancels immediately; slurp needs ~150ms to start.
const wtypeLinger = "500"

var wtypeModifiers = map[Modifier]string{
	ModCtrl: "ctrl", ModAlt: "alt", ModShift: "shift", ModSuper: "logo",
}

// sendCombo prefers wtype on Wayland: xdotool only reaches XWayland
// windows there, so compositor bindings (sway, Hyprland) never fire.
func (r *Runner) sendCombo(c Combo) error {
	wayland := os.Getenv("WAYLAND_DISPLAY") != ""
	if wayland {
		if _, err := exec.LookPath("wtype"); err == nil {
			// Asynchronous, so the linger delay doesn't hold up further presses.
			return r.start("wtype", wtypeArgs(c)...)
		}
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		return r.run("xdotool", "key", "--clearmodifiers", xdotoolSpec(c))
	}
	if wayland {
		return errors.New("shortcut needs wtype (Wayland) or xdotool (X11)")
	}
	return errors.New("shortcut needs xdotool (X11)")
}

func wtypeArgs(c Combo) []string {
	var args []string
	for _, m := range c.Modifiers {
		args = append(args, "-M", wtypeModifiers[m])
	}
	args = append(args, "-k", c.Key)
	for _, m := range slices.Backward(c.Modifiers) {
		args = append(args, "-m", wtypeModifiers[m])
	}
	return append(args, "-s", wtypeLinger)
}

func xdotoolSpec(c Combo) string {
	parts := make([]string, 0, len(c.Modifiers)+1)
	for _, m := range c.Modifiers {
		parts = append(parts, string(m))
	}
	return strings.Join(append(parts, c.Key), "+")
}

func (r *Runner) openURL(url string) error {
	for _, candidate := range [][]string{{"gio", "open", url}, {"xdg-open", url}} {
		if _, err := exec.LookPath(candidate[0]); err == nil {
			return r.start(candidate[0], candidate[1:]...)
		}
	}
	return errors.New("no URL opener found (gio or xdg-open)")
}
