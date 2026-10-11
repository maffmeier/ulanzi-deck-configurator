//go:build !linux && !darwin && !windows

package audio

func changeVolume(string, int) error { return ErrUnsupported }
func toggleMute(string) error        { return ErrUnsupported }
func toggleMic() error               { return ErrUnsupported }
func switchOutput(string) error      { return ErrUnsupported }
func current(string) (State, error)  { return State{}, ErrUnsupported }
