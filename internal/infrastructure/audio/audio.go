// Package audio controls volume, mute and the output device of the host.
package audio

import (
	"errors"
	"fmt"
	"strings"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// State is what an audio widget shows. Volume and Muted describe the
// default output, or the target application when one is given.
type State struct {
	Volume   int
	Muted    bool
	MicMuted bool
	Output   string
	// AppFound is false when the target application plays no audio.
	AppFound bool
}

var (
	ErrUnsupported = errors.New("not supported on this system")
	ErrAppNotFound = errors.New("application is not playing audio")
)

// Run executes an audio action.
func Run(a deck.Action) error {
	step := a.Step
	if step == 0 {
		step = deck.DefaultAudioStep
	}
	switch a.Op {
	case deck.AudioVolumeUp:
		return changeVolume(strings.TrimSpace(a.Target), step)
	case deck.AudioVolumeDown:
		return changeVolume(strings.TrimSpace(a.Target), -step)
	case deck.AudioMute:
		return toggleMute(strings.TrimSpace(a.Target))
	case deck.AudioMicMute:
		return toggleMic()
	case deck.AudioOutputNext:
		return switchOutput("")
	case deck.AudioOutputSet:
		return switchOutput(strings.TrimSpace(a.Target))
	}
	return fmt.Errorf("unknown audio op %q", a.Op)
}

// Current reads the state for an audio widget.
func Current(target string) (State, error) {
	return current(strings.TrimSpace(target))
}

func clampPercent(v int) int {
	return min(100, max(0, v))
}

// nextDevice picks the device after current, or the first one whose name
// contains want.
func nextDevice(names []string, current, want string) (int, error) {
	if len(names) == 0 {
		return 0, errors.New("no output devices found")
	}
	if want != "" {
		want = strings.ToLower(want)
		for i, n := range names {
			if strings.Contains(strings.ToLower(n), want) {
				return i, nil
			}
		}
		return 0, fmt.Errorf("no output device matches %q", want)
	}
	for i, n := range names {
		if n == current {
			return (i + 1) % len(names), nil
		}
	}
	return 0, nil
}

func matchesApp(want string, names ...string) bool {
	want = strings.ToLower(want)
	for _, n := range names {
		if n != "" && strings.Contains(strings.ToLower(n), want) {
			return true
		}
	}
	return false
}
