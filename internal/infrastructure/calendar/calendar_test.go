package calendar

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const feed = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:daily
SUMMARY:Daily Standup
DTSTART;TZID=Europe/Berlin:20261001T093000
DTEND;TZID=Europe/Berlin:20261001T094500
RRULE:FREQ=DAILY
END:VEVENT
BEGIN:VEVENT
UID:cancelled
SUMMARY:Abgesagt
STATUS:CANCELLED
DTSTART;TZID=Europe/Berlin:20261012T110000
DTEND;TZID=Europe/Berlin:20261012T120000
END:VEVENT
BEGIN:VEVENT
UID:allday
SUMMARY:Feiertag
DTSTART;VALUE=DATE:20261013
DTEND;VALUE=DATE:20261014
END:VEVENT
END:VCALENDAR
`

func TestParseExpandsRecurrenceAndSkipsCancelled(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	now := time.Date(2026, 10, 12, 8, 0, 0, 0, berlin)
	events, err := Parse([]byte(feed), now)
	if err != nil {
		t.Fatal(err)
	}
	var standups, allDay int
	for _, e := range events {
		switch e.Summary {
		case "Daily Standup":
			standups++
			if h, m := e.Start.In(berlin).Hour(), e.Start.In(berlin).Minute(); h != 9 || m != 30 {
				t.Fatalf("standup at %v", e.Start)
			}
		case "Feiertag":
			allDay++
			if !e.AllDay {
				t.Fatal("all-day flag")
			}
		case "Abgesagt":
			t.Fatal("cancelled event listed")
		}
	}
	if standups < 7 || allDay != 1 {
		t.Fatalf("standups %d, all-day %d", standups, allDay)
	}
	for i := 1; i < len(events); i++ {
		if events[i].Start.Before(events[i-1].Start) {
			t.Fatal("not sorted")
		}
	}
}

func TestFetchFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cal.ics")
	os.WriteFile(path, []byte(feed), 0o644)
	if events, err := Fetch(path, time.Now()); err != nil || len(events) == 0 {
		t.Fatalf("%d events, %v", len(events), err)
	}
}
