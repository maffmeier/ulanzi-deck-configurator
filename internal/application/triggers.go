package application

import (
	"context"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

const triggerInterval = 5 * time.Second

// fire runs the actions of all triggers of event that match; they run in
// the background so slow actions never stall the loop that noticed it.
func (d *Daemon) fire(event string, match func(deck.Trigger) bool) {
	d.mu.Lock()
	triggers := d.cfg.TriggersFor(event)
	page := d.page
	d.mu.Unlock()
	for _, t := range triggers {
		if match != nil && !match(t) {
			continue
		}
		d.log.Info("trigger fired", "event", event, "action", t.Action.Type)
		go d.execute(t.Action, -1, page)
	}
}

// triggerLoop fires time triggers once in the minute they are due. The
// minute the daemon starts in is skipped, so a restart doesn't repeat them.
func (d *Daemon) triggerLoop(ctx context.Context) {
	last := time.Now().Truncate(time.Minute)
	ticker := time.NewTicker(triggerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			minute := now.Truncate(time.Minute)
			if !minute.After(last) {
				continue
			}
			last = minute
			d.fire(deck.TriggerTime, func(t deck.Trigger) bool { return t.DueAt(now) })
		}
	}
}
