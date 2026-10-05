package application

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

const liveInterval = time.Second

func pngHash(png []byte) string {
	sum := sha256.Sum256(png)
	return string(sum[:])
}

// refreshWidgets redraws live keys and the info window right away, e.g.
// after a timer action, instead of waiting for the next tick.
func (d *Daemon) refreshWidgets() {
	d.pushLiveKeys(time.Now())
	select {
	case d.widgetKick <- struct{}{}:
	default:
	}
}

func (d *Daemon) liveLoop(ctx context.Context) {
	ticker := time.NewTicker(liveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			d.pushLiveKeys(now)
		}
	}
}

// pushLiveKeys uploads, in one partial update, every live key of the
// current page whose picture changed.
func (d *Daemon) pushLiveKeys(now time.Time) {
	if !d.dev.Connected() {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	var changed []deck.Button
	for _, b := range d.cfg.ButtonsFor(d.page) {
		if b.Live == nil || b.Index >= deck.ButtonCount {
			continue
		}
		png := d.engine.Render(*b.Live, deck.IconSize, deck.IconSize, b.TextStyle.BackgroundColor, now)
		hash := pngHash(png)
		if d.liveHash[b.Index] == hash {
			continue
		}
		b.IconData = png
		changed = append(changed, b)
		d.liveHash[b.Index] = hash
	}
	if len(changed) > 0 {
		d.logErr("upload live keys", d.dev.SetButtons(changed, true))
	}
}

// widgetSmallWindow rotates through the configured widgets. It ticks every
// second (clocks, timers) but only uploads when the picture changed and
// otherwise just feeds the firmware watchdog.
func (d *Daemon) widgetSmallWindow(st *statusState, sw deck.SmallWindow) time.Duration {
	if st.device == nil || *st.device != deck.SmallWindowBackground {
		d.logErr("small window mode", d.dev.SetSmallWindowMode(deck.SmallWindowBackground))
		st.device = modePtr(deck.SmallWindowBackground)
		st.lastPNG = ""
	}
	now := time.Now()
	index := 0
	if n := len(sw.Widgets); n > 1 {
		rotate := 10.0
		if sw.RotateEveryS != nil {
			rotate = *sw.RotateEveryS
		}
		index = int(now.UnixNano()/int64(rotate*float64(time.Second))) % n
	}
	png := d.engine.Render(sw.Widgets[index], deck.InfoWindowWidth, deck.InfoWindowHeight, sw.BackgroundColor, now)
	hash := pngHash(png)

	d.mu.Lock()
	defer d.mu.Unlock()
	if strategyKey(d.cfg.SmallWindow) != st.key {
		return 0
	}
	if hash == st.lastPNG && now.Sub(st.lastUpload) < heartbeatInterval {
		return liveInterval
	}
	if hash == st.lastPNG {
		d.logErr("keep alive", d.dev.KeepAlive())
		st.lastUpload = now
		return liveInterval
	}
	button := deck.Button{Index: deck.InfoWindowIndex, IconData: png, TextStyle: deck.DefaultTextStyle()}
	button.TextStyle.BackgroundColor = sw.BackgroundColor
	d.logErr("upload info window", d.dev.SetButtons([]deck.Button{button}, true))
	st.lastPNG, st.lastUpload = hash, now
	return liveInterval
}
