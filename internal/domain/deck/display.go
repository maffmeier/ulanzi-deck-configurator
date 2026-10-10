package deck

import (
	"fmt"
	"strings"
	"time"
)

// Display automates the deck brightness.
type Display struct {
	// DimAfterS dims the deck after this many seconds without a key press
	// (0 disables dimming).
	DimAfterS     int
	DimBrightness int
	// OffWhenLocked turns the display off while the computer is locked.
	OffWhenLocked bool
	Night         *NightMode
}

// NightMode lowers the brightness between From and To ("HH:MM", may wrap
// past midnight).
type NightMode struct {
	From       string
	To         string
	Brightness int
}

func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("time %q must be HH:MM", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func (d Display) Validate() error {
	if d.DimAfterS < 0 {
		return fmt.Errorf("display.dim_after_s must not be negative")
	}
	if d.DimBrightness < 0 || d.DimBrightness > 100 {
		return fmt.Errorf("display.dim_brightness must be in 0..100")
	}
	if n := d.Night; n != nil {
		if _, err := parseClock(n.From); err != nil {
			return fmt.Errorf("display.night.from: %w", err)
		}
		if _, err := parseClock(n.To); err != nil {
			return fmt.Errorf("display.night.to: %w", err)
		}
		if n.Brightness < 0 || n.Brightness > 100 {
			return fmt.Errorf("display.night.brightness must be in 0..100")
		}
	}
	return nil
}

// Active reports whether now falls into the night window.
func (n NightMode) Active(now time.Time) bool {
	from, err1 := parseClock(n.From)
	to, err2 := parseClock(n.To)
	if err1 != nil || err2 != nil || from == to {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	if from < to {
		return minute >= from && minute < to
	}
	return minute >= from || minute < to
}

// TargetBrightness is the brightness the deck should have right now; the
// lowest applicable rule wins.
func (c *Config) TargetBrightness(now time.Time, idle time.Duration, locked bool) int {
	d := c.Display
	if locked && d.OffWhenLocked {
		return 0
	}
	target := c.Brightness
	if d.Night != nil && d.Night.Active(now) {
		target = min(target, d.Night.Brightness)
	}
	if d.DimAfterS > 0 && idle >= time.Duration(d.DimAfterS)*time.Second {
		target = min(target, d.DimBrightness)
	}
	return target
}

// PageForApp returns the first page that lists app (case-insensitive
// substring match on the app identifier).
func (c *Config) PageForApp(app string) (string, bool) {
	app = strings.ToLower(strings.TrimSpace(app))
	if app == "" {
		return "", false
	}
	for _, p := range c.Pages {
		for _, pattern := range p.Apps {
			if pattern = strings.ToLower(strings.TrimSpace(pattern)); pattern != "" && strings.Contains(app, pattern) {
				return p.Name, true
			}
		}
	}
	return "", false
}

// UsesAppPages reports whether any page is bound to applications.
func (c *Config) UsesAppPages() bool {
	for _, p := range c.Pages {
		if len(p.Apps) > 0 {
			return true
		}
	}
	return false
}
