// Package autostart registers the app to start at login.
package autostart

const appName = "ulanzi-deck"

// Enable registers exe (with args) to run at login; Disable removes it.
// The per-OS implementations live in autostart_<os>.go.
func Enable(exe string, args []string) error { return enable(exe, args) }
func Disable() error                         { return disable() }
func Enabled() bool                          { return enabled() }
