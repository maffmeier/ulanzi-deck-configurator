package audio

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

func osascript(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	return strings.TrimSpace(string(out)), err
}

// macOS has no per-app volume and no microphone mute; the mic is muted by
// setting its input volume to 0 and restoring the previous level.
var (
	micMu       sync.Mutex
	micRestore  = 75
	errPerApp   = fmt.Errorf("per-app volume: %w", ErrUnsupported)
	switchAudio = "SwitchAudioSource"
)

func changeVolume(target string, delta int) error {
	if target != "" {
		return errPerApp
	}
	out, err := osascript("output volume of (get volume settings)")
	if err != nil {
		return err
	}
	v, _ := strconv.Atoi(out)
	_, err = osascript(fmt.Sprintf("set volume output volume %d", clampPercent(v+delta)))
	return err
}

func toggleMute(target string) error {
	if target != "" {
		return errPerApp
	}
	_, err := osascript("set volume output muted (not (output muted of (get volume settings)))")
	return err
}

func toggleMic() error {
	micMu.Lock()
	defer micMu.Unlock()
	out, err := osascript("input volume of (get volume settings)")
	if err != nil {
		return err
	}
	v, _ := strconv.Atoi(out)
	next := 0
	if v == 0 {
		next = micRestore
	} else {
		micRestore = v
	}
	_, err = osascript(fmt.Sprintf("set volume input volume %d", next))
	return err
}

// switchOutput needs SwitchAudioSource (brew install switchaudio-osx).
func switchOutput(want string) error {
	path, err := exec.LookPath(switchAudio)
	if err != nil {
		return fmt.Errorf("switching outputs needs %s (brew install switchaudio-osx): %w", switchAudio, ErrUnsupported)
	}
	out, err := exec.Command(path, "-a", "-t", "output").Output()
	if err != nil {
		return err
	}
	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	cur, _ := exec.Command(path, "-c", "-t", "output").Output()
	i, err := nextDevice(names, strings.TrimSpace(string(cur)), want)
	if err != nil {
		return err
	}
	return exec.Command(path, "-t", "output", "-s", names[i]).Run()
}

func current(target string) (State, error) {
	out, err := osascript(`set s to get volume settings
return (output volume of s as text) & "," & (output muted of s as text) & "," & (input volume of s as text)`)
	if err != nil {
		return State{}, err
	}
	parts := strings.Split(out, ",")
	if len(parts) != 3 {
		return State{}, errors.New("unexpected volume settings: " + out)
	}
	st := State{AppFound: target == ""}
	st.Volume, _ = strconv.Atoi(parts[0])
	st.Muted = parts[1] == "true"
	in, _ := strconv.Atoi(parts[2])
	st.MicMuted = in == 0
	if path, err := exec.LookPath(switchAudio); err == nil {
		if cur, err := exec.Command(path, "-c", "-t", "output").Output(); err == nil {
			st.Output = strings.TrimSpace(string(cur))
		}
	}
	return st, nil
}
