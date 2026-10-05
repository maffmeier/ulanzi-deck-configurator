// Package actions executes button actions on the host operating system.
package actions

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type Runner struct {
	log *slog.Logger
	env []string
}

func NewRunner(log *slog.Logger) *Runner {
	return &Runner{log: log, env: commandEnv(log)}
}

// Run executes an action; switch_page is handled by the daemon itself.
func (r *Runner) Run(action deck.Action) error {
	switch action.Type {
	case deck.ActionShell:
		return r.runShell(action.Cmd)
	case deck.ActionShortcut:
		combo, err := ParseCombo(action.Keys)
		if err != nil {
			return err
		}
		return r.sendCombo(combo)
	case deck.ActionURL:
		return r.openURL(NormalizeURL(action.URL))
	case deck.ActionPredefined:
		resolved, err := ResolvePredefined(action.CommandID)
		if err != nil {
			return err
		}
		return r.Run(resolved)
	}
	return fmt.Errorf("action type %q cannot be executed by the runner", action.Type)
}

// Press holds a shortcut down until the returned release is called, for
// push-to-talk style keys. Predefined commands must resolve to a shortcut.
func (r *Runner) Press(action deck.Action) (func(), error) {
	if action.Type == deck.ActionPredefined {
		resolved, err := ResolvePredefined(action.CommandID)
		if err != nil {
			return nil, err
		}
		action = resolved
	}
	if action.Type != deck.ActionShortcut {
		return nil, fmt.Errorf("holding only works with shortcuts, not %s", action.Type)
	}
	combo, err := ParseCombo(action.Keys)
	if err != nil {
		return nil, err
	}
	release, err := r.pressCombo(combo)
	if err != nil {
		return nil, err
	}
	return func() {
		if err := release(); err != nil {
			r.log.Error("releasing held shortcut failed", "keys", action.Keys, "error", err)
		}
	}, nil
}

var noSlashSchemes = map[string]bool{"about": true, "data": true, "file": true, "mailto": true, "sms": true, "tel": true}

// NormalizeURL adds https:// to bare hosts like "claude.ai".
func NormalizeURL(raw string) string {
	url := strings.TrimSpace(raw)
	switch {
	case url == "":
		return url
	case strings.HasPrefix(url, "//"):
		return "https:" + url
	case strings.Contains(url, "://"):
		return url
	}
	if scheme, _, found := strings.Cut(url, ":"); found && noSlashSchemes[strings.ToLower(scheme)] {
		return url
	}
	return "https://" + url
}

// start launches a process without waiting for it, but reaps it in the
// background and logs a non-zero exit so failures are visible.
func (r *Runner) start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = r.env
	detach(cmd)
	started := time.Now()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			r.log.Warn("action process failed", "command", name, "args", args, "error", err,
				"runtime", time.Since(started).Round(time.Millisecond))
		}
	}()
	return nil
}

// run executes a short helper (xdotool, osascript, ...) synchronously.
func (r *Runner) run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = r.env
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}
