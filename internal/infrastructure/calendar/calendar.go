// Package calendar reads upcoming events from an iCalendar (ICS) feed.
package calendar

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/apognu/gocal"
)

// Event is one occurrence; recurring events are expanded.
type Event struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
}

const (
	fetchTimeout = 15 * time.Second
	maxFeedBytes = 16 << 20
	lookBehind   = 24 * time.Hour
	lookAhead    = 8 * 24 * time.Hour
)

// Fetch loads the feed at source (http(s)://, webcal:// or a file path)
// and returns the events from yesterday to a week ahead, sorted by start.
func Fetch(source string, now time.Time) ([]Event, error) {
	data, err := load(strings.TrimSpace(source))
	if err != nil {
		return nil, err
	}
	return Parse(data, now)
}

func load(source string) ([]byte, error) {
	if rest, ok := strings.CutPrefix(source, "webcal://"); ok {
		source = "https://" + rest
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("calendar: %s", res.Status)
		}
		return io.ReadAll(io.LimitReader(res.Body, maxFeedBytes))
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(source, "~/") {
		source = filepath.Join(home, source[2:])
	}
	return os.ReadFile(source)
}

// Parse reads ICS text; all-day events use the local day.
func Parse(data []byte, now time.Time) ([]Event, error) {
	start, end := now.Add(-lookBehind), now.Add(lookAhead)
	parser := gocal.NewParser(bytes.NewReader(data))
	parser.Start, parser.End = &start, &end
	parser.AllDayEventsTZ = time.Local
	// Tolerate feeds with minor flaws (e.g. no DTSTAMP) instead of dropping events.
	parser.Strict.Mode = gocal.StrictModeFailAttribute
	if err := parser.Parse(); err != nil {
		return nil, fmt.Errorf("calendar: %w", err)
	}
	var events []Event
	for _, e := range parser.Events {
		if e.Start == nil || strings.EqualFold(e.Status, "CANCELLED") {
			continue
		}
		ev := Event{Summary: strings.TrimSpace(e.Summary), Start: *e.Start, End: *e.Start}
		if e.End != nil {
			ev.End = *e.End
		}
		ev.AllDay = e.RawStart.Params["VALUE"] == "DATE" || len(e.RawStart.Value) == 8
		events = append(events, ev)
	}
	slices.SortFunc(events, func(a, b Event) int { return a.Start.Compare(b.Start) })
	return events, nil
}
