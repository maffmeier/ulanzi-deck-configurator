package audio

import "math"

// The float goes in the second argument slot: Go's syscall on windows/amd64
// copies the first four integer arguments into XMM0-3 as well, which is
// where the Windows x64 ABI expects floats.
func setSessionVolume(vol uintptr, v float32) error {
	return check(call(vol, slotSimpleSetVolume, uintptr(math.Float32bits(v)), 0))
}
