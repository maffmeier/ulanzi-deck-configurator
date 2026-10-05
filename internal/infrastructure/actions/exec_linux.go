package actions

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
)

// wtypeLinger keeps wtype's virtual keyboard alive after the keys were
// sent. If it vanishes while a freshly launched grab client (slurp, rofi)
// starts, the client cancels immediately; slurp needs ~150ms to start.
const wtypeLinger = "500"

var wtypeModifiers = map[Modifier]string{
	ModCtrl: "ctrl", ModAlt: "alt", ModShift: "shift", ModSuper: "logo",
}

var (
	vkOnce   sync.Once
	sharedVK *virtualKeyboard
)

func (r *Runner) virtualKeyboard() *virtualKeyboard {
	vkOnce.Do(func() { sharedVK = newVirtualKeyboard(r.log) })
	return sharedVK
}

// sendCombo uses our own Wayland virtual keyboard where the compositor
// supports it (sway, Hyprland, ...): xdotool only reaches XWayland windows
// there. wtype remains the fallback, xdotool covers X11.
func (r *Runner) sendCombo(c Combo) error {
	wayland := os.Getenv("WAYLAND_DISPLAY") != ""
	if wayland {
		err := r.virtualKeyboard().Tap(c)
		if err == nil {
			return nil
		}
		r.log.Debug("virtual keyboard unavailable, falling back", "error", err)
		if _, err := exec.LookPath("wtype"); err == nil {
			// Asynchronous, so the linger delay doesn't hold up further presses.
			return r.start("wtype", wtypeArgs(c)...)
		}
	}
	if _, err := exec.LookPath("xdotool"); err == nil {
		return r.run("xdotool", "key", "--clearmodifiers", xdotoolSpec(c))
	}
	if wayland {
		return errors.New("shortcut needs a wlroots compositor, wtype or xdotool")
	}
	return errors.New("shortcut needs xdotool (X11)")
}

func (r *Runner) pressCombo(c Combo) (func() error, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		release, err := r.virtualKeyboard().Press(c)
		if err == nil {
			return release, nil
		}
		if _, lookErr := exec.LookPath("xdotool"); lookErr != nil {
			return nil, fmt.Errorf("holding keys on Wayland: %w", err)
		}
	}
	if _, err := exec.LookPath("xdotool"); err != nil {
		return nil, errors.New("holding keys needs xdotool (X11)")
	}
	spec := xdotoolSpec(c)
	if err := r.run("xdotool", "keydown", spec); err != nil {
		return nil, err
	}
	return func() error { return r.run("xdotool", "keyup", spec) }, nil
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
