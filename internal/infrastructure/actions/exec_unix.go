//go:build linux || darwin

package actions

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func hideWindow(*exec.Cmd) {}

func (r *Runner) runShell(command string) error {
	return r.start("/bin/sh", "-c", command)
}

const pathSentinel = "__ULANZI_PATH__="

// commandEnv merges the PATH of the user's login shell into the process
// environment. Autostarted processes (LaunchAgents, desktop autostart) get
// a minimal PATH that misses ~/.local/bin, Homebrew and friends.
func commandEnv(log *slog.Logger) []string {
	env := os.Environ()
	shell := os.Getenv("SHELL")
	if shell == "" {
		return env
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-l", "-c", `printf '`+pathSentinel+`%s' "$PATH"`).Output()
	if err != nil {
		log.Debug("login shell PATH unavailable", "shell", shell, "error", err)
		return env
	}
	_, loginPath, found := strings.Cut(string(out), pathSentinel)
	if !found {
		return env
	}

	var merged []string
	add := func(entries ...string) {
		for _, e := range entries {
			if e != "" && !slices.Contains(merged, e) {
				merged = append(merged, e)
			}
		}
	}
	add(filepath.SplitList(os.Getenv("PATH"))...)
	add(filepath.SplitList(strings.TrimSpace(loginPath))...)

	result := slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, "PATH=") })
	return append(result, "PATH="+strings.Join(merged, string(os.PathListSeparator)))
}

func shellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", command)
}
