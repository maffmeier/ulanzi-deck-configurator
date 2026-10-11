package application

import (
	"fmt"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

const (
	audioRefresh  = time.Second
	mutedColor    = "#B91C1C"
	defaultActive = "#14532D"
)

// ExpireAudio refetches audio state on the next render.
func (e *WidgetEngine) ExpireAudio() { expire(e, e.audio) }

func (e *WidgetEngine) renderAudio(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	src, ok := e.sources.(AudioSource)
	if !ok {
		return e.renderer.Text(w, h, bg, wd.Title, []string{e.text("error")}, false)
	}
	key := wd.Show + "\x00" + wd.Target
	info, err, at := pollKeyed(e, e.audio, key, audioRefresh, now, func() (AudioInfo, error) { return src.AudioState(wd.Target) })

	title := wd.Title
	if title == "" {
		switch wd.Show {
		case "mic":
			title = e.text("mic")
		case "output":
			title = e.text("output")
		default:
			title = wd.Target
			if title == "" {
				title = e.text("volume")
			}
		}
	}
	switch {
	case at.IsZero():
		return e.renderer.Text(w, h, bg, title, []string{"…"}, false)
	case err != nil:
		return e.renderer.Text(w, h, bg, title, []string{e.text("error")}, false)
	}

	switch wd.Show {
	case "mic":
		if info.MicMuted {
			return e.renderer.Text(w, h, orColor(wd.FailColor, mutedColor), title, []string{e.text("micOff")}, false)
		}
		if wd.OkColor != "" {
			bg = wd.OkColor
		}
		return e.renderer.Text(w, h, bg, title, []string{e.text("micOn")}, false)
	case "output":
		return e.renderer.Text(w, h, bg, title, []string{info.Output}, false)
	}
	if !info.AppFound {
		return e.renderer.Text(w, h, bg, title, []string{"–"}, false)
	}
	if info.Muted {
		return e.renderer.Text(w, h, orColor(wd.FailColor, mutedColor), title, []string{e.text("muted")}, false)
	}
	if wd.OkColor != "" {
		bg = wd.OkColor
	}
	return e.renderer.Text(w, h, bg, title, []string{fmt.Sprintf("%d %%", info.Volume)}, true)
}

func orColor(c, fallback string) string {
	if c != "" {
		return c
	}
	return fallback
}
