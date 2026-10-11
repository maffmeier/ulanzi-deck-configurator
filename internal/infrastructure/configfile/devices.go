package configfile

import (
	"path/filepath"
	"strings"
)

// DeviceConfigPath returns the config a deck uses: its entry in the
// devices map of the main config, or the main config itself.
func DeviceConfigPath(mainPath string, devices map[string]string, key string) string {
	if p := strings.TrimSpace(devices[key]); p != "" {
		return ResolvePath(p, filepath.Dir(mainPath))
	}
	return mainPath
}

// DeviceFileName is the default name of a deck's own config.
func DeviceFileName(key string) string {
	var b strings.Builder
	for _, r := range key {
		if r < 128 && (r == '-' || r == '_' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return "deck-" + b.String() + ".yaml"
}
