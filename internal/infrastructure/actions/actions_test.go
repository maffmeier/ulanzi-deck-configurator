package actions

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCombo(t *testing.T) {
	cases := map[string]Combo{
		"super+Return":  {Modifiers: []Modifier{ModSuper}, Key: "Return"},
		"Win+Enter":     {Modifiers: []Modifier{ModSuper}, Key: "Return"},
		"ctrl+alt+t":    {Modifiers: []Modifier{ModCtrl, ModAlt}, Key: "t"},
		"Strg+Shift+F5": {Modifiers: []Modifier{ModCtrl, ModShift}, Key: "F5"},
		"cmd+shift+4":   {Modifiers: []Modifier{ModSuper, ModShift}, Key: "4"},
		"XF86AudioPlay": {Key: "XF86AudioPlay"},
		"ctrl++":        {Modifiers: []Modifier{ModCtrl}, Key: "plus"},
		"super":         {Key: "Super_L"},
	}
	for spec, want := range cases {
		got, err := ParseCombo(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %+v want %+v", spec, got, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "foo+a"} {
		if _, err := ParseCombo(bad); err == nil {
			t.Fatalf("%q: expected error", bad)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"claude.ai":             "https://claude.ai",
		"//example.com":         "https://example.com",
		"http://x.de":           "http://x.de",
		"mailto:a@b.de":         "mailto:a@b.de",
		"  https://y.de/path  ": "https://y.de/path",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestPredefinedAliases(t *testing.T) {
	a, err := resolvePredefined("volume_up", "linux")
	if err != nil || a.Keys != "XF86AudioRaiseVolume" {
		t.Fatalf("got %+v %v", a, err)
	}
	if _, err := ResolvePredefined("nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatal("expected error")
	}
}

func TestPredefinedResolveOnAllPlatforms(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range predefinedCommands {
		if seen[p.id] {
			t.Fatalf("duplicate id %s", p.id)
		}
		seen[p.id] = true
		if p.label == "" || p.group == "" || p.icon == "" {
			t.Fatalf("%s: label, group and icon are required", p.id)
		}
		for _, goos := range []string{"linux", "windows", "darwin"} {
			a, err := resolvePredefined(p.id, goos)
			if err != nil {
				continue // not available on this platform
			}
			if err := a.Validate(); err != nil {
				t.Fatalf("%s/%s: %v", p.id, goos, err)
			}
			if a.Type == "shortcut" {
				if _, err := ParseCombo(a.Keys); err != nil {
					t.Fatalf("%s/%s: %v", p.id, goos, err)
				}
			}
		}
	}
}

func TestModKeysUseCmdOnMac(t *testing.T) {
	mac, _ := resolvePredefined("edit_copy", "darwin")
	win, _ := resolvePredefined("edit_copy", "windows")
	if mac.Keys != "cmd+c" || win.Keys != "ctrl+c" {
		t.Fatalf("mac %q win %q", mac.Keys, win.Keys)
	}
}

// IDs written by ulanzi-linux configs must keep working.
func TestLegacyPredefinedIDs(t *testing.T) {
	for _, id := range []string{"audio_mic_mute", "audio_mute", "audio_volume_down", "audio_volume_up",
		"display_screenshot_selection", "gnome_show_applications", "gnome_terminal",
		"media_next", "media_play_pause", "media_previous"} {
		if _, err := resolvePredefined(id, "linux"); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
	}
}
