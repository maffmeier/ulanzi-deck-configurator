package actions

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// virtualKeyboard is a minimal Wayland client for the
// zwp_virtual_keyboard_v1 protocol (wlroots compositors: sway, Hyprland,
// river, ...). Unlike wtype it stays connected, so a key can be held down
// for as long as the deck key is pressed.
type virtualKeyboard struct {
	log  *slog.Logger
	mu   sync.Mutex
	conn *net.UnixConn
	// ids are allocated client side, starting after wl_display (1).
	nextID   uint32
	keyboard uint32
	start    time.Time
	failed   error
	// held releases the currently pressed combo. The one-key keymap is
	// replaced on every press, so only one combo can be down at a time.
	held func() error
}

const (
	wlDisplayID = 1

	opDisplaySync        = 0
	opDisplayGetRegistry = 1
	opRegistryBind       = 0
	opVKManagerCreate    = 0
	opVKKeymap           = 0
	opVKKey              = 1
	opVKModifiers        = 2

	evDisplayError  = 0
	evRegistryGlobl = 0
	evCallbackDone  = 0

	keymapFormatXKBv1 = 1
	keyPressed        = 1
	keyReleased       = 0
)

// Modifier masks of the standard xkb "complete" compat map.
var wlModifierMask = map[Modifier]uint32{
	ModShift: 1 << 0,
	ModCtrl:  1 << 2,
	ModAlt:   1 << 3,
	ModSuper: 1 << 6,
}

func newVirtualKeyboard(log *slog.Logger) *virtualKeyboard {
	return &virtualKeyboard{log: log, nextID: 2, start: time.Now()}
}

func waylandSocket() (string, error) {
	display := os.Getenv("WAYLAND_DISPLAY")
	if display == "" {
		return "", errors.New("WAYLAND_DISPLAY not set")
	}
	if filepath.IsAbs(display) {
		return display, nil
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		return "", errors.New("XDG_RUNTIME_DIR not set")
	}
	return filepath.Join(runtime, display), nil
}

func (v *virtualKeyboard) newID() uint32 {
	id := v.nextID
	v.nextID++
	return id
}

// connectLocked binds the seat and creates the virtual keyboard once.
func (v *virtualKeyboard) connectLocked() error {
	if v.conn != nil {
		return nil
	}
	if v.failed != nil {
		return v.failed
	}
	path, err := waylandSocket()
	if err != nil {
		return err
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}

	registry := v.newID()
	if err := writeMsg(conn, wlDisplayID, opDisplayGetRegistry, nil, u32(registry)); err != nil {
		conn.Close()
		return err
	}
	globals, err := roundtrip(conn, v.newID(), registry)
	if err != nil {
		conn.Close()
		return err
	}
	seat, okSeat := globals["wl_seat"]
	manager, okManager := globals["zwp_virtual_keyboard_manager_v1"]
	if !okSeat || !okManager {
		conn.Close()
		// Remember: GNOME and KDE don't offer the protocol, retrying is pointless.
		v.failed = errors.New("compositor does not support zwp_virtual_keyboard_v1")
		return v.failed
	}

	seatID, managerID, keyboardID := v.newID(), v.newID(), v.newID()
	msgs := []struct {
		obj  uint32
		op   uint16
		args []byte
	}{
		{registry, opRegistryBind, bindArgs(seat.name, "wl_seat", 1, seatID)},
		{registry, opRegistryBind, bindArgs(manager.name, "zwp_virtual_keyboard_manager_v1", 1, managerID)},
		{managerID, opVKManagerCreate, append(u32(seatID), u32(keyboardID)...)},
	}
	for _, m := range msgs {
		if err := writeMsg(conn, m.obj, m.op, nil, m.args); err != nil {
			conn.Close()
			return err
		}
	}
	v.conn, v.keyboard = conn, keyboardID
	go v.drain(conn)
	return nil
}

// drain consumes compositor events so the socket never fills up, and logs
// protocol errors.
func (v *virtualKeyboard) drain(conn *net.UnixConn) {
	for {
		obj, op, payload, err := readMsg(conn)
		if err != nil {
			v.mu.Lock()
			if v.conn == conn {
				v.conn = nil
			}
			v.mu.Unlock()
			return
		}
		if obj == wlDisplayID && op == evDisplayError && len(payload) >= 8 {
			v.log.Error("wayland protocol error", "message", readString(payload[8:]))
		}
	}
}

func (v *virtualKeyboard) now() uint32 {
	return uint32(time.Since(v.start).Milliseconds())
}

// Press holds the combo down until the returned release is called.
func (v *virtualKeyboard) Press(c Combo) (func() error, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.held != nil {
		_ = v.releaseLocked()
	}
	if err := v.connectLocked(); err != nil {
		return nil, err
	}
	keymap := xkbKeymap(c.Key)
	fd, err := memfd(keymap)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)

	var mask uint32
	for _, m := range c.Modifiers {
		mask |= wlModifierMask[m]
	}
	// The keymap maps the key to evdev code 1 (xkb keycode 9).
	const evdevCode = 1
	steps := []struct {
		op   uint16
		fds  []int
		args []byte
	}{
		{opVKKeymap, []int{fd}, append(u32(keymapFormatXKBv1), u32(uint32(len(keymap)+1))...)},
		{opVKModifiers, nil, concat(u32(mask), u32(0), u32(0), u32(0))},
		{opVKKey, nil, concat(u32(v.now()), u32(evdevCode), u32(keyPressed))},
	}
	for _, s := range steps {
		if err := writeMsg(v.conn, v.keyboard, s.op, s.fds, s.args); err != nil {
			v.resetLocked()
			return nil, err
		}
	}

	var once sync.Once
	var result error
	release := func() error {
		once.Do(func() { result = v.sendRelease() })
		return result
	}
	v.held = release
	return func() error {
		v.mu.Lock()
		defer v.mu.Unlock()
		return release()
	}, nil
}

func (v *virtualKeyboard) releaseLocked() error {
	held := v.held
	v.held = nil
	return held()
}

// sendRelease lifts the key and clears the modifiers; v.mu must be held.
func (v *virtualKeyboard) sendRelease() error {
	v.held = nil
	if v.conn == nil {
		return errors.New("wayland connection lost")
	}
	const evdevCode = 1
	if err := writeMsg(v.conn, v.keyboard, opVKKey, nil, concat(u32(v.now()), u32(evdevCode), u32(keyReleased))); err != nil {
		v.resetLocked()
		return err
	}
	if err := writeMsg(v.conn, v.keyboard, opVKModifiers, nil, concat(u32(0), u32(0), u32(0), u32(0))); err != nil {
		v.resetLocked()
		return err
	}
	return nil
}

// Tap presses and releases the combo.
func (v *virtualKeyboard) Tap(c Combo) error {
	release, err := v.Press(c)
	if err != nil {
		return err
	}
	return release()
}

func (v *virtualKeyboard) resetLocked() {
	if v.conn != nil {
		v.conn.Close()
		v.conn = nil
	}
	v.nextID = 2
}

// xkbKeymap builds a one-key keymap, the same approach wtype uses: the
// compositor only needs to translate our single keycode to the keysym.
func xkbKeymap(keysym string) string {
	return fmt.Sprintf(`xkb_keymap {
xkb_keycodes "(unnamed)" { minimum = 8; maximum = 10; <K1> = 9; };
xkb_types "(unnamed)" { include "complete" };
xkb_compatibility "(unnamed)" { include "complete" };
xkb_symbols "(unnamed)" { key <K1> {[ %s ]}; };
};
`, keysym)
}

func memfd(content string) (int, error) {
	fd, err := unix.MemfdCreate("ulanzi-deck-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		return -1, err
	}
	// The protocol expects a NUL-terminated keymap.
	data := append([]byte(content), 0)
	if _, err := unix.Write(fd, data); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

type global struct {
	name    uint32
	version uint32
}

// roundtrip collects registry globals until wl_display.sync completes.
func roundtrip(conn *net.UnixConn, callback, registry uint32) (map[string]global, error) {
	if err := writeMsg(conn, wlDisplayID, opDisplaySync, nil, u32(callback)); err != nil {
		return nil, err
	}
	globals := map[string]global{}
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetReadDeadline(time.Time{})
	for {
		obj, op, payload, err := readMsg(conn)
		if err != nil {
			return nil, err
		}
		switch {
		case obj == callback && op == evCallbackDone:
			return globals, nil
		case obj == registry && op == evRegistryGlobl && len(payload) >= 8:
			name := binary.LittleEndian.Uint32(payload)
			iface, rest := readStringRest(payload[4:])
			if len(rest) >= 4 {
				globals[iface] = global{name: name, version: binary.LittleEndian.Uint32(rest)}
			}
		case obj == wlDisplayID && op == evDisplayError && len(payload) >= 8:
			return nil, fmt.Errorf("wayland error: %s", readString(payload[8:]))
		}
	}
}

// Wire format: object id, (size << 16 | opcode), then 32-bit aligned args.
// Wayland uses the host byte order; all supported targets are little endian.
func writeMsg(conn *net.UnixConn, obj uint32, op uint16, fds []int, args []byte) error {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header, obj)
	binary.LittleEndian.PutUint32(header[4:], uint32(8+len(args))<<16|uint32(op))
	msg := append(header, args...)
	var oob []byte
	if len(fds) > 0 {
		oob = unix.UnixRights(fds...)
	}
	_, _, err := conn.WriteMsgUnix(msg, oob, nil)
	return err
}

func readMsg(r io.Reader) (uint32, uint16, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, 0, nil, err
	}
	obj := binary.LittleEndian.Uint32(header)
	sizeOp := binary.LittleEndian.Uint32(header[4:])
	size := int(sizeOp >> 16)
	if size < 8 {
		return 0, 0, nil, fmt.Errorf("invalid wayland message size %d", size)
	}
	payload := make([]byte, size-8)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, 0, nil, err
	}
	return obj, uint16(sizeOp & 0xffff), payload, nil
}

func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func concat(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func wlString(s string) []byte {
	data := append([]byte(s), 0)
	for len(data)%4 != 0 {
		data = append(data, 0)
	}
	return append(u32(uint32(len(s)+1)), data...)
}

func bindArgs(name uint32, iface string, version, id uint32) []byte {
	return concat(u32(name), wlString(iface), u32(version), u32(id))
}

func readStringRest(b []byte) (string, []byte) {
	if len(b) < 4 {
		return "", nil
	}
	n := int(binary.LittleEndian.Uint32(b))
	padded := (n + 3) &^ 3
	if len(b) < 4+padded {
		return "", nil
	}
	return strings.TrimRight(string(b[4:4+n]), "\x00"), b[4+padded:]
}

func readString(b []byte) string {
	s, _ := readStringRest(b)
	return s
}
