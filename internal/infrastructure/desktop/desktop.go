// Package desktop reports host state the daemon reacts to: the app in the
// foreground (for app-specific pages) and whether the screen is locked.
package desktop

import "errors"

var ErrUnsupported = errors.New("not supported on this desktop")

// ActiveApp returns an identifier of the focused application: the Wayland
// app_id or X11 class on Linux, the executable name on Windows and the
// application name on macOS. Matching is done case-insensitively.
func ActiveApp() (string, error) { return activeApp() }

// Locked reports whether the session's screen is locked.
func Locked() (bool, error) { return locked() }
