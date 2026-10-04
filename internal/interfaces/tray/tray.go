//go:build !darwin || cgo

// Package tray shows a system tray icon. macOS needs cgo for it, so
// cross-compiled macOS builds fall back to tray_stub.go.
package tray

import (
	_ "embed"

	"fyne.io/systray"
)

//go:embed icon.png
var iconPNG []byte

const Available = true

type Menu struct {
	OpenEditor      func()
	AutostartActive func() bool
	ToggleAutostart func() error
	Quit            func()
}

// Run blocks on the calling (main) goroutine until Quit is chosen.
func Run(m Menu) {
	systray.Run(func() {
		systray.SetIcon(iconPNG)
		systray.SetTitle("Ulanzi Deck")
		systray.SetTooltip("Ulanzi Deck")

		open := systray.AddMenuItem("Editor öffnen", "Konfiguration im Browser bearbeiten")
		autostart := systray.AddMenuItemCheckbox("Beim Anmelden starten", "", m.AutostartActive())
		systray.AddSeparator()
		quit := systray.AddMenuItem("Beenden", "Ulanzi Deck beenden")
		systray.SetOnTapped(m.OpenEditor)

		go func() {
			for {
				select {
				case <-open.ClickedCh:
					m.OpenEditor()
				case <-autostart.ClickedCh:
					if err := m.ToggleAutostart(); err == nil {
						if m.AutostartActive() {
							autostart.Check()
						} else {
							autostart.Uncheck()
						}
					}
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, m.Quit)
}

func Stop() { systray.Quit() }
