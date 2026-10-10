//go:build !windows

package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func systemLocale() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	// Apps started by launchd have no LANG.
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("defaults", "read", "-g", "AppleLocale").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}
