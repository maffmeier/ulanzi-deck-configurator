package main

import (
	"errors"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/desktop"
)

type desktopState struct{}

func (desktopState) ActiveApp() (string, error) {
	app, err := desktop.ActiveApp()
	if errors.Is(err, desktop.ErrUnsupported) {
		return "", application.ErrDesktopUnsupported
	}
	return app, err
}

func (desktopState) Locked() (bool, error) { return desktop.Locked() }
