package actions

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000

	inputKeyboard    = 1
	keyEventExtended = 0x0001
	keyEventKeyUp    = 0x0002
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNewProcessGroup | createNoWindow}
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func commandEnv(*slog.Logger) []string { return os.Environ() }

func (r *Runner) runShell(command string) error {
	cmd := exec.Command("cmd.exe")
	// Pass the line verbatim: Go's argument quoting would mangle cmd syntax.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNewProcessGroup | createNoWindow,
		CmdLine:       `cmd.exe /C ` + command,
	}
	cmd.Env = r.env
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			r.log.Warn("action process failed", "command", command, "error", err)
		}
	}()
	return nil
}

func (r *Runner) openURL(url string) error {
	return r.start("rundll32.exe", "url.dll,FileProtocolHandler", url)
}

var virtualKeys = map[string]uint16{
	"Return": 0x0D, "Escape": 0x1B, "Tab": 0x09, "space": 0x20, "BackSpace": 0x08,
	"Delete": 0x2E, "Insert": 0x2D, "Home": 0x24, "End": 0x23, "Prior": 0x21, "Next": 0x22,
	"Left": 0x25, "Up": 0x26, "Right": 0x27, "Down": 0x28, "Print": 0x2C, "Caps_Lock": 0x14,
	"Menu": 0x5D, "minus": 0xBD, "equal": 0xBB, "plus": 0xBB, "comma": 0xBC, "period": 0xBE,
	"slash": 0xBF, "semicolon": 0xBA, "grave": 0xC0, "bracketleft": 0xDB, "backslash": 0xDC,
	"bracketright": 0xDD, "apostrophe": 0xDE,
	"Super_L": 0x5B, "Control_L": 0x11, "Alt_L": 0x12, "Shift_L": 0x10,
	"XF86AudioMute": 0xAD, "XF86AudioLowerVolume": 0xAE, "XF86AudioRaiseVolume": 0xAF,
	"XF86AudioNext": 0xB0, "XF86AudioPrev": 0xB1, "XF86AudioStop": 0xB2,
	"XF86AudioPlay": 0xB3, "XF86AudioPause": 0xB3,
}

var extendedKeys = map[uint16]bool{
	0x2E: true, 0x2D: true, 0x24: true, 0x23: true, 0x21: true, 0x22: true,
	0x25: true, 0x26: true, 0x27: true, 0x28: true, 0x5B: true, 0x5D: true,
	0xAD: true, 0xAE: true, 0xAF: true, 0xB0: true, 0xB1: true, 0xB2: true, 0xB3: true,
}

var modifierKeys = map[Modifier]uint16{ModCtrl: 0x11, ModAlt: 0x12, ModShift: 0x10, ModSuper: 0x5B}

func virtualKey(key string) (uint16, bool) {
	if vk, ok := virtualKeys[key]; ok {
		return vk, true
	}
	if len(key) == 1 {
		c := key[0]
		switch {
		case c >= 'a' && c <= 'z':
			return uint16(c - 'a' + 'A'), true
		case c >= '0' && c <= '9':
			return uint16(c), true
		}
	}
	var n int
	if _, err := fmt.Sscanf(key, "F%d", &n); err == nil && n >= 1 && n <= 24 {
		return uint16(0x70 + n - 1), true
	}
	return 0, false
}

type keyboardInput struct {
	vk    uint16
	scan  uint16
	flags uint32
	time  uint32
	extra uintptr
}

// input mirrors the Win32 INPUT struct on 64-bit Windows (40 bytes; the
// union is sized by MOUSEINPUT).
type input struct {
	typ uint32
	ki  keyboardInput
	_   [8]byte
}

var procSendInput = syscall.NewLazyDLL("user32.dll").NewProc("SendInput")

func keyEvent(vk uint16, up bool) input {
	var flags uint32
	if extendedKeys[vk] {
		flags |= keyEventExtended
	}
	if up {
		flags |= keyEventKeyUp
	}
	return input{typ: inputKeyboard, ki: keyboardInput{vk: vk, flags: flags}}
}

// comboEvents returns the key-down events (modifiers first) and the
// matching key-up events in reverse order.
func comboEvents(c Combo) (down, up []input, err error) {
	vk, ok := virtualKey(c.Key)
	if !ok {
		return nil, nil, fmt.Errorf("key %q is not supported on Windows", c.Key)
	}
	for _, m := range c.Modifiers {
		down = append(down, keyEvent(modifierKeys[m], false))
	}
	down = append(down, keyEvent(vk, false))
	up = append(up, keyEvent(vk, true))
	for i := len(c.Modifiers) - 1; i >= 0; i-- {
		up = append(up, keyEvent(modifierKeys[c.Modifiers[i]], true))
	}
	return down, up, nil
}

func sendInput(events []input) error {
	sent, _, err := procSendInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(events[0]))
	if int(sent) != len(events) {
		return fmt.Errorf("SendInput delivered %d of %d events: %w", sent, len(events), err)
	}
	return nil
}

func (r *Runner) sendCombo(c Combo) error {
	down, up, err := comboEvents(c)
	if err != nil {
		return err
	}
	return sendInput(append(down, up...))
}

func (r *Runner) pressCombo(c Combo) (func() error, error) {
	down, up, err := comboEvents(c)
	if err != nil {
		return nil, err
	}
	if err := sendInput(down); err != nil {
		return nil, err
	}
	return func() error { return sendInput(up) }, nil
}
