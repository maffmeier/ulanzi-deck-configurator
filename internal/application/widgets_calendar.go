package application

import (
	"fmt"
	"math"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// CalendarEvent is one upcoming occurrence of a calendar feed.
type CalendarEvent struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
}

// CalendarSource loads a calendar feed; it is optional for the engine.
type CalendarSource interface {
	CalendarEvents(url string) ([]CalendarEvent, error)
}

const (
	soonColor  = "#A16207"
	soonWindow = 15 * time.Minute
)

// nextEvent is the running or next timed event, or else the next all-day
// one; events are sorted by start.
func nextEvent(events []CalendarEvent, now time.Time) (CalendarEvent, bool) {
	var allDay *CalendarEvent
	for i, e := range events {
		if !e.End.After(now) {
			continue
		}
		if !e.AllDay {
			return e, true
		}
		if allDay == nil {
			allDay = &events[i]
		}
	}
	if allDay != nil {
		return *allDay, true
	}
	return CalendarEvent{}, false
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (e *WidgetEngine) eventWhen(ev CalendarEvent, now time.Time) string {
	start := ev.Start.In(now.Location())
	tomorrow := now.AddDate(0, 0, 1)
	day := e.text(fmt.Sprintf("wd%d", start.Weekday()))
	switch {
	case ev.AllDay && !start.After(now):
		return e.text("today")
	case ev.AllDay && sameDay(start, tomorrow):
		return e.text("tomorrow")
	case ev.AllDay:
		return day
	case !start.After(now):
		return e.text("now")
	case start.Sub(now) < time.Hour:
		return fmt.Sprintf(e.text("inMinutes"), int(math.Ceil(start.Sub(now).Minutes())))
	case sameDay(start, now):
		return start.Format("15:04")
	case sameDay(start, tomorrow):
		return e.text("tomorrow") + " " + start.Format("15:04")
	}
	return day + " " + start.Format("15:04")
}

func (e *WidgetEngine) renderCalendar(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	title := wd.Title
	if title == "" {
		title = e.text("calendar")
	}
	src, ok := e.sources.(CalendarSource)
	if !ok {
		return e.renderer.Text(w, h, bg, title, []string{e.text("error")}, false)
	}
	every := time.Duration(wd.IntervalS * float64(time.Second))
	events, err, at := pollKeyed(e, e.calendars, wd.URL, every, now, func() ([]CalendarEvent, error) { return src.CalendarEvents(wd.URL) })
	switch {
	case at.IsZero():
		return e.renderer.Text(w, h, bg, title, []string{"…"}, false)
	case err != nil:
		return e.renderer.Text(w, h, bg, title, []string{e.text("error")}, false)
	}
	ev, ok := nextEvent(events, now)
	if !ok {
		return e.renderer.Text(w, h, bg, title, []string{e.text("noEvents")}, false)
	}
	if !ev.AllDay && ev.Start.Sub(now) <= soonWindow {
		bg = orColor(wd.OkColor, soonColor)
	}
	return e.renderer.Text(w, h, bg, title, []string{ev.Summary, e.eventWhen(ev, now)}, false)
}
