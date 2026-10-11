package application

import (
	"fmt"
	"slices"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// Integrations runs actions that talk to external services (Home
// Assistant, OBS).
type Integrations interface {
	Run(action deck.Action) error
}

func (d *Daemon) SetIntegrations(i Integrations) { d.integrations = i }

// toggleKeyLocked identifies a two-state key; fixed keys share their state
// across pages.
func (d *Daemon) toggleKeyLocked(page string, index int) string {
	if slices.ContainsFunc(d.cfg.FixedButtons, func(b deck.Button) bool { return b.Index == index }) {
		return fmt.Sprintf("*:%d", index)
	}
	return fmt.Sprintf("%s:%d", page, index)
}

// effectiveLocked returns the button as currently shown and executed.
func (d *Daemon) effectiveLocked(b deck.Button) deck.Button {
	if b.Toggle != nil && d.toggled[d.toggleKeyLocked(d.page, b.Index)] {
		return b.Flipped()
	}
	return b
}

// flipToggle switches a two-state key and redraws it.
func (d *Daemon) flipToggle(page string, index int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := d.toggleKeyLocked(page, index)
	d.toggled[key] = !d.toggled[key]
	if page != d.page || index >= deck.ButtonCount {
		return
	}
	b := d.cfg.ButtonAt(d.page, index)
	if b == nil {
		return
	}
	d.logErr("upload toggle key", d.dev.SetButtons([]deck.Button{d.shownLocked(*b)}, true))
}

// runMulti executes the steps in order in the background, so delays never
// block button handling.
func (d *Daemon) runMulti(action deck.Action, index int, page string) {
	go func() {
		for _, step := range action.Steps {
			if step.Type == deck.ActionDelay {
				time.Sleep(time.Duration(step.Ms) * time.Millisecond)
				continue
			}
			d.execute(step, index, page)
		}
	}()
}
