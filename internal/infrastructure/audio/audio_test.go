package audio

import "testing"

func TestNextDevice(t *testing.T) {
	names := []string{"Speakers", "Headset", "HDMI"}
	if i, _ := nextDevice(names, "Headset", ""); i != 2 {
		t.Fatalf("next after Headset: %d", i)
	}
	if i, _ := nextDevice(names, "HDMI", ""); i != 0 {
		t.Fatalf("wraps around: %d", i)
	}
	if i, _ := nextDevice(names, "Speakers", "head"); i != 1 {
		t.Fatalf("match by name: %d", i)
	}
	if _, err := nextDevice(names, "", "bluetooth"); err == nil {
		t.Fatal("unknown device must fail")
	}
	if _, err := nextDevice(nil, "", ""); err == nil {
		t.Fatal("no devices must fail")
	}
}

func TestMatchesApp(t *testing.T) {
	if !matchesApp("spotify", "", "Spotify") || matchesApp("spotify", "firefox") {
		t.Fatal("matching is a case-insensitive substring test")
	}
}
