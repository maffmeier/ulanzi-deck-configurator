package desktop

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID   = user32.NewProc("GetWindowThreadProcessId")
	procOpenInputDesktop           = user32.NewProc("OpenInputDesktop")
	procCloseDesktop               = user32.NewProc("CloseDesktop")
	procGetUserObjectInformationW  = user32.NewProc("GetUserObjectInformationW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	processQueryLimitedInformation = 0x1000
	desktopReadObjects             = 0x0001
	uoiName                        = 2
)

// activeApp returns the executable name of the foreground window, e.g.
// "Code" for Code.exe.
func activeApp() (string, error) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", nil
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	handle, _, err := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return "", err
	}
	defer procCloseHandle.Call(handle)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	ok, _, err := procQueryFullProcessImageNameW.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return "", err
	}
	name := filepath.Base(syscall.UTF16ToString(buf[:size]))
	return strings.TrimSuffix(name, filepath.Ext(name)), nil
}

// locked: while the lock screen is up the input desktop is "Winlogon"
// (or can't be opened at all) instead of "Default".
func locked() (bool, error) {
	desk, _, _ := procOpenInputDesktop.Call(0, 0, desktopReadObjects)
	if desk == 0 {
		return true, nil
	}
	defer procCloseDesktop.Call(desk)
	buf := make([]uint16, 256)
	var needed uint32
	ok, _, err := procGetUserObjectInformationW.Call(desk, uoiName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 {
		return false, err
	}
	return !strings.EqualFold(syscall.UTF16ToString(buf), "Default"), nil
}
