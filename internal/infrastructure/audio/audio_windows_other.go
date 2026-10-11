//go:build windows && !amd64

package audio

import "fmt"

// On arm64 floats travel in separate registers that syscall.SyscallN does
// not set, so per-app volume is not available there.
func setSessionVolume(uintptr, float32) error {
	return fmt.Errorf("per-app volume on this architecture: %w", ErrUnsupported)
}
