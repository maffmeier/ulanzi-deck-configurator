package application

import (
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type pressTiming struct {
	// longPress is how long a key must be held to fire its long-press action.
	longPress time.Duration
	// repeatDelay and repeatInterval behave like keyboard auto-repeat.
	repeatDelay    time.Duration
	repeatInterval time.Duration
}

var defaultPressTiming = pressTiming{
	longPress:      500 * time.Millisecond,
	repeatDelay:    400 * time.Millisecond,
	repeatInterval: 120 * time.Millisecond,
}

// heldKey tracks one physical key between press and release. The button is
// captured at press time so a page switch while holding doesn't change
// what the release does.
type heldKey struct {
	button deck.Button
	page   string
	timer  *time.Timer
	// cancelled is set when the key was released before the long-press
	// timer fired; exactly one of short and long press runs.
	cancelled bool
	stop      chan struct{}
	release   func()
}

func (d *Daemon) onPress(index int) {
	d.markActivity(time.Now())
	d.mu.Lock()
	page := d.page
	var button *deck.Button
	if b := d.cfg.ButtonAt(page, index); b != nil {
		effective := d.effectiveLocked(*b)
		button = &effective
	}
	d.mu.Unlock()

	if button == nil || (button.Action == nil && button.LongPress == nil && button.Toggle == nil) {
		d.log.Debug("no action bound", "index", index, "page", page)
		return
	}
	// A press without a release (e.g. lost event) must not leak state.
	d.endHold(index, false)

	key := &heldKey{button: *button, page: page}
	d.log.Info("button pressed", "index", index, "page", page, "press", button.Press)

	switch {
	case button.Press == deck.PressHold:
		release, err := d.runner.Press(*button.Action)
		if err != nil {
			d.log.Error("holding shortcut failed", "index", index, "error", err)
			return
		}
		key.release = release
	case button.LongPress != nil:
		// Decide on release (short) or when the timer fires (long).
		key.timer = time.AfterFunc(d.timing.longPress, func() { d.fireLongPress(index, key) })
	case button.Press == deck.PressRepeat:
		d.execute(*button.Action, index, page)
		key.stop = make(chan struct{})
		go d.repeat(index, page, *button.Action, key.stop)
	default:
		d.tap(*button, index, page)
		return
	}

	d.heldMu.Lock()
	d.held[index] = key
	d.heldMu.Unlock()
}

func (d *Daemon) onRelease(index int) {
	d.endHold(index, true)
}

// endHold finishes a held key; fireShort runs the short-press action of a
// long-press key that was released before the threshold.
func (d *Daemon) endHold(index int, fireShort bool) {
	d.heldMu.Lock()
	key := d.held[index]
	delete(d.held, index)
	var short bool
	if key != nil && key.timer != nil && key.timer.Stop() {
		// Released before the threshold: the long press will not run.
		key.cancelled = true
		short = true
	}
	d.heldMu.Unlock()
	if key == nil {
		return
	}

	if key.release != nil {
		key.release()
	}
	if key.stop != nil {
		close(key.stop)
	}
	if short && fireShort {
		d.tap(key.button, index, key.page)
	}
}

func (d *Daemon) fireLongPress(index int, key *heldKey) {
	d.heldMu.Lock()
	cancelled := key.cancelled
	d.heldMu.Unlock()
	if cancelled {
		return
	}
	d.log.Info("long press", "index", index, "page", key.page)
	d.execute(*key.button.LongPress, index, key.page)
}

func (d *Daemon) repeat(index int, page string, action deck.Action, stop <-chan struct{}) {
	select {
	case <-stop:
		return
	case <-time.After(d.timing.repeatDelay):
	}
	ticker := time.NewTicker(d.timing.repeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			d.execute(action, index, page)
		}
	}
}

// releaseAll lets go of everything, e.g. when the deck disconnects or the
// daemon stops, so no shortcut stays pressed.
func (d *Daemon) releaseAll() {
	d.heldMu.Lock()
	indices := make([]int, 0, len(d.held))
	for i := range d.held {
		indices = append(indices, i)
	}
	d.heldMu.Unlock()
	for _, i := range indices {
		d.endHold(i, false)
	}
}

// tap runs a key's (current) action and flips two-state keys.
func (d *Daemon) tap(b deck.Button, index int, page string) {
	if b.Action != nil {
		d.execute(*b.Action, index, page)
	}
	if b.Toggle != nil {
		d.flipToggle(page, index)
	}
}

func (d *Daemon) execute(action deck.Action, index int, page string) {
	switch action.Type {
	case deck.ActionMulti:
		d.runMulti(action, index, page)
		return
	case deck.ActionHome, deck.ActionOBS:
		if d.integrations == nil {
			d.log.Error("integration not available", "action", action.Type)
			return
		}
		go func() {
			if err := d.integrations.Run(action); err != nil {
				d.log.Error("integration action failed", "index", index, "action", action.Type, "error", err)
			}
		}()
		return
	case deck.ActionSwitchPage:
		d.SwitchTo(action.Page)
		return
	case deck.ActionTimer:
		d.engine.Timer(action.Op, action.Minutes, time.Now())
		d.refreshWidgets()
		return
	}
	if err := d.runner.Run(action); err != nil {
		d.log.Error("action failed", "index", index, "page", page, "action", action.Type, "error", err)
	}
}
