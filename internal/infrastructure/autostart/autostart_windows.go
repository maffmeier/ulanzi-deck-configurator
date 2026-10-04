package autostart

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func enable(exe string, args []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	parts := []string{`"` + exe + `"`}
	for _, a := range args {
		parts = append(parts, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
	}
	return k.SetStringValue(appName, strings.Join(parts, " "))
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(appName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(appName)
	return err == nil
}
