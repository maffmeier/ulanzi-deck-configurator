package main

import (
	"os"
	"syscall"
)

const attachParentProcess = ^uintptr(0) // (DWORD)-1

// The Windows build uses the GUI subsystem so autostart shows no console
// window. When launched from a terminal, attach to it so CLI output works.
func attachConsole() {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if ok, _, _ := proc.Call(attachParentProcess); ok == 0 {
		return
	}
	if out, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = out
		os.Stderr = out
	}
}
