//go:build linux

package actions

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// Drives the real compositor when VK_MANUAL is set; used to verify holding
// keys against wev. Never runs in CI.
func TestVirtualKeyboardManual(t *testing.T) {
	spec := os.Getenv("VK_MANUAL")
	if spec == "" {
		t.Skip("VK_MANUAL not set")
	}
	c, err := ParseCombo(spec)
	if err != nil {
		t.Fatal(err)
	}
	vk := newVirtualKeyboard(slog.Default())
	release, err := vk.Press(c)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := release(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
}

func TestVirtualKeyboardTapManual(t *testing.T) {
	spec := os.Getenv("VK_TAP")
	if spec == "" {
		t.Skip("VK_TAP not set")
	}
	c, err := ParseCombo(spec)
	if err != nil {
		t.Fatal(err)
	}
	vk := newVirtualKeyboard(slog.Default())
	if err := vk.Tap(c); err != nil {
		t.Fatal(err)
	}
	// Keep the process (and thus the keyboard) alive like the daemon does.
	time.Sleep(3 * time.Second)
}
