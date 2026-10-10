package desktop

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

func activeApp() (string, error) {
	switch {
	case os.Getenv("SWAYSOCK") != "":
		return swayFocused(os.Getenv("SWAYSOCK"))
	case os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != "":
		return hyprlandFocused()
	case os.Getenv("DISPLAY") != "" && os.Getenv("WAYLAND_DISPLAY") == "":
		return x11Focused()
	}
	return "", ErrUnsupported
}

// i3/sway IPC: "i3-ipc" + payload length + message type, little endian.
func swayFocused(socket string) (string, error) {
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	const getTree = 4
	msg := append([]byte("i3-ipc"), make([]byte, 8)...)
	binary.LittleEndian.PutUint32(msg[10:], getTree)
	if _, err := conn.Write(msg); err != nil {
		return "", err
	}
	header := make([]byte, 14)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	payload := make([]byte, binary.LittleEndian.Uint32(header[6:10]))
	if _, err := io.ReadFull(conn, payload); err != nil {
		return "", err
	}
	var tree swayNode
	if err := json.Unmarshal(payload, &tree); err != nil {
		return "", err
	}
	if node := tree.focused(); node != nil {
		if node.AppID != "" {
			return node.AppID, nil
		}
		return node.WindowProperties.Class, nil
	}
	return "", nil
}

type swayNode struct {
	Focused          bool       `json:"focused"`
	AppID            string     `json:"app_id"`
	Nodes            []swayNode `json:"nodes"`
	FloatingNodes    []swayNode `json:"floating_nodes"`
	WindowProperties struct {
		Class string `json:"class"`
	} `json:"window_properties"`
}

func (n *swayNode) focused() *swayNode {
	if n.Focused {
		return n
	}
	for _, list := range [][]swayNode{n.Nodes, n.FloatingNodes} {
		for i := range list {
			if f := list[i].focused(); f != nil {
				return f
			}
		}
	}
	return nil
}

func hyprlandFocused() (string, error) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	socket := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "hypr", sig, ".socket.sock")
	if _, err := os.Stat(socket); err != nil {
		socket = filepath.Join("/tmp/hypr", sig, ".socket.sock")
	}
	conn, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("j/activewindow")); err != nil {
		return "", err
	}
	data, err := io.ReadAll(conn)
	if err != nil {
		return "", err
	}
	var win struct {
		Class string `json:"class"`
	}
	if err := json.Unmarshal(data, &win); err != nil {
		return "", err
	}
	return win.Class, nil
}

var (
	xpropWindow = regexp.MustCompile(`window id # (0x[0-9a-fA-F]+)`)
	xpropClass  = regexp.MustCompile(`"([^"]*)"\s*$`)
)

func x11Focused() (string, error) {
	out, err := exec.Command("xprop", "-root", "_NET_ACTIVE_WINDOW").Output()
	if err != nil {
		return "", err
	}
	m := xpropWindow.FindSubmatch(out)
	if m == nil {
		return "", nil
	}
	out, err = exec.Command("xprop", "-id", string(m[1]), "WM_CLASS").Output()
	if err != nil {
		return "", err
	}
	if c := xpropClass.FindSubmatch(out); c != nil {
		return string(c[1]), nil
	}
	return "", nil
}

var lockers = []string{"swaylock", "hyprlock", "i3lock", "gtklock", "waylock", "xsecurelock", "slock", "physlock"}

func locked() (bool, error) {
	if hint, err := logindLockedHint(); err == nil && hint {
		return true, nil
	}
	// Tiling setups lock with a standalone locker that logind never hears of.
	return lockerRunning(), nil
}

var (
	logindOnce    sync.Once
	logindSession dbus.BusObject
	logindErr     error
)

func logindLockedHint() (bool, error) {
	logindOnce.Do(func() {
		conn, err := dbus.ConnectSystemBus()
		if err != nil {
			logindErr = err
			return
		}
		var path dbus.ObjectPath
		manager := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")
		if err := manager.Call("org.freedesktop.login1.Manager.GetSessionByPID", 0, uint32(os.Getpid())).Store(&path); err != nil {
			logindErr = err
			return
		}
		logindSession = conn.Object("org.freedesktop.login1", path)
	})
	if logindErr != nil {
		return false, logindErr
	}
	v, err := logindSession.GetProperty("org.freedesktop.login1.Session.LockedHint")
	if err != nil {
		return false, err
	}
	hint, ok := v.Value().(bool)
	if !ok {
		return false, errors.New("unexpected LockedHint type")
	}
	return hint, nil
}

func lockerRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		for _, l := range lockers {
			if name == l {
				return true
			}
		}
	}
	return false
}
