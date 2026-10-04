package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// XDG autostart works for GNOME, KDE, XFCE & co. Tiling compositors like
// sway ignore it; there an `exec ulanzi-deck` line in the config is needed.
func desktopFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", appName+".desktop"), nil
}

func quote(arg string) string {
	if !strings.ContainsAny(arg, " \t\"'\\$`") {
		return arg
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace(arg) + `"`
}

func enable(exe string, args []string) error {
	path, err := desktopFile()
	if err != nil {
		return err
	}
	cmd := []string{quote(exe)}
	for _, a := range args {
		cmd = append(cmd, quote(a))
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Ulanzi Deck
Comment=Steuerung für den Ulanzi D200
Exec=%s
Terminal=false
X-GNOME-Autostart-enabled=true
`, strings.Join(cmd, " "))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func disable() error {
	path, err := desktopFile()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func enabled() bool {
	path, err := desktopFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
