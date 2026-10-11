package application

import (
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// MeetingInfo is the Teams meeting state as widgets show it.
type MeetingInfo struct {
	InMeeting  bool
	Muted      bool
	VideoOn    bool
	HandRaised bool
}

// MeetingSource reads the meeting state; it is optional for the engine.
type MeetingSource interface {
	MeetingState() (MeetingInfo, error)
}

const (
	meetingRefresh = time.Second
	handColor      = "#A16207"
)

// ExpireMeeting refetches the meeting state on the next render.
func (e *WidgetEngine) ExpireMeeting() { expire(e, e.meeting) }

func (e *WidgetEngine) renderMeeting(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	title := wd.Title
	if title == "" {
		title = "Teams"
	}
	src, ok := e.sources.(MeetingSource)
	if !ok {
		return e.renderer.Text(w, h, bg, title, []string{e.text("offline")}, false)
	}
	info, err, at := pollKeyed(e, e.meeting, "teams", meetingRefresh, now, src.MeetingState)
	switch {
	case at.IsZero():
		return e.renderer.Text(w, h, bg, title, []string{"…"}, false)
	case err != nil:
		return e.renderer.Text(w, h, bg, title, []string{e.text("offline")}, false)
	case !info.InMeeting:
		return e.renderer.Text(w, h, bg, title, []string{e.text("noMeeting")}, false)
	}
	switch wd.Show {
	case "camera":
		if info.VideoOn {
			return e.renderer.Text(w, h, defaultActive, e.text("camera"), []string{e.text("on")}, false)
		}
		return e.renderer.Text(w, h, bg, e.text("camera"), []string{e.text("off")}, false)
	case "hand":
		if info.HandRaised {
			return e.renderer.Text(w, h, handColor, e.text("hand"), []string{e.text("handUp")}, false)
		}
		return e.renderer.Text(w, h, bg, e.text("hand"), []string{"–"}, false)
	}
	if info.Muted {
		return e.renderer.Text(w, h, mutedColor, e.text("mic"), []string{e.text("micOff")}, false)
	}
	return e.renderer.Text(w, h, defaultActive, e.text("mic"), []string{e.text("micOn")}, false)
}
