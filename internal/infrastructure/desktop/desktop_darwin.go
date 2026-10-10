package desktop

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
)

func activeApp() (string, error) {
	out, err := exec.Command("osascript", "-e",
		`tell application "System Events" to get name of first application process whose frontmost is true`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var (
	cgOnce sync.Once
	cgErr  error

	cgSessionCopyCurrentDictionary func() uintptr
	cfDictionaryGetValue           func(dict, key uintptr) uintptr
	cfStringCreateWithCString      func(alloc uintptr, s string, encoding uint32) uintptr
	cfBooleanGetValue              func(b uintptr) bool
	cfRelease                      func(ref uintptr)
	lockedKey                      uintptr
)

const cfStringEncodingUTF8 = 0x08000100

func loadCG() error {
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
		purego.RegisterLibFunc(&cgSessionCopyCurrentDictionary, cg, "CGSessionCopyCurrentDictionary")
		purego.RegisterLibFunc(&cfDictionaryGetValue, cf, "CFDictionaryGetValue")
		purego.RegisterLibFunc(&cfStringCreateWithCString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&cfBooleanGetValue, cf, "CFBooleanGetValue")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		lockedKey = cfStringCreateWithCString(0, "CGSSessionScreenIsLocked", cfStringEncodingUTF8)
	})
	return cgErr
}

// locked reads the screen-locked flag of the current login session.
func locked() (bool, error) {
	if err := loadCG(); err != nil {
		return false, err
	}
	dict := cgSessionCopyCurrentDictionary()
	if dict == 0 {
		return false, nil
	}
	defer cfRelease(dict)
	value := cfDictionaryGetValue(dict, lockedKey)
	return value != 0 && cfBooleanGetValue(value), nil
}
