//go:build darwin && !cgo

package tray

const Available = false

type Menu struct {
	OpenEditor      func()
	AutostartActive func() bool
	ToggleAutostart func() error
	Quit            func()
}

func Run(Menu) {}

func Stop() {}
