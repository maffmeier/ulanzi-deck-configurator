package actions

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

// CoreGraphics keyboard events, loaded with purego so holding keys works
// in cgo and non-cgo builds alike. Like osascript they need the
// Accessibility permission.
var (
	cgOnce sync.Once
	cgErr  error

	cgEventCreateKeyboardEvent func(source uintptr, keycode uint16, down bool) uintptr
	cgEventSetFlags            func(event uintptr, flags uint64)
	cgEventPost                func(tap uint32, event uintptr)
	cfRelease                  func(ref uintptr)
)

const cgHIDEventTap = 0

var macModifierKeyCodes = map[Modifier]uint16{ModSuper: 55, ModShift: 56, ModAlt: 58, ModCtrl: 59}

var macModifierFlags = map[Modifier]uint64{
	ModShift: 0x00020000,
	ModCtrl:  0x00040000,
	ModAlt:   0x00080000,
	ModSuper: 0x00100000,
}

func loadCoreGraphics() error {
	cgOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			cgErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			cgErr = err
			return
		}
		purego.RegisterLibFunc(&cgEventCreateKeyboardEvent, cg, "CGEventCreateKeyboardEvent")
		purego.RegisterLibFunc(&cgEventSetFlags, cg, "CGEventSetFlags")
		purego.RegisterLibFunc(&cgEventPost, cg, "CGEventPost")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	})
	return cgErr
}

func postKey(keycode uint16, down bool, flags uint64) {
	event := cgEventCreateKeyboardEvent(0, keycode, down)
	if event == 0 {
		return
	}
	cgEventSetFlags(event, flags)
	cgEventPost(cgHIDEventTap, event)
	cfRelease(event)
}

func (r *Runner) pressCombo(c Combo) (func() error, error) {
	code, ok := macKeyCodes[c.Key]
	if !ok {
		return nil, fmt.Errorf("key %q cannot be held on macOS", c.Key)
	}
	if err := loadCoreGraphics(); err != nil {
		return nil, err
	}
	var flags uint64
	for _, m := range c.Modifiers {
		flags |= macModifierFlags[m]
		postKey(macModifierKeyCodes[m], true, flags)
	}
	postKey(uint16(code), true, flags)

	return func() error {
		postKey(uint16(code), false, flags)
		for i := len(c.Modifiers) - 1; i >= 0; i-- {
			m := c.Modifiers[i]
			flags &^= macModifierFlags[m]
			postKey(macModifierKeyCodes[m], false, flags)
		}
		return nil
	}, nil
}
