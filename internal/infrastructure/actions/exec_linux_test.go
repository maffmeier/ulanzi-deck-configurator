package actions

import (
	"reflect"
	"testing"
)

func TestWtypeArgsReleaseModifiersInReverse(t *testing.T) {
	c, _ := ParseCombo("ctrl+alt+t")
	want := []string{"-M", "ctrl", "-M", "alt", "-k", "t", "-m", "alt", "-m", "ctrl", "-s", wtypeLinger}
	if got := wtypeArgs(c); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestXdotoolSpec(t *testing.T) {
	c, _ := ParseCombo("Win+Enter")
	if got := xdotoolSpec(c); got != "super+Return" {
		t.Fatalf("got %q", got)
	}
}
