package application

import (
	"context"
	"errors"
	"slices"
	"time"
)

// DesktopState reports host state for display and page automation.
type DesktopState interface {
	ActiveApp() (string, error)
	Locked() (bool, error)
}

// ErrDesktopUnsupported tells the daemon to stop polling the active app.
var ErrDesktopUnsupported = errors.New("desktop query not supported")

const (
	displayInterval = time.Second
	lockInterval    = 2 * time.Second
	appInterval     = time.Second
)

func (d *Daemon) SetDesktop(s DesktopState) { d.desktop = s }

// markActivity records a key press and wakes a dimmed display at once.
func (d *Daemon) markActivity(now time.Time) {
	d.mu.Lock()
	d.lastActivity = now
	d.mu.Unlock()
	d.updateBrightness(now)
}

func (d *Daemon) updateBrightness(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	target := d.cfg.TargetBrightness(now, now.Sub(d.lastActivity), d.locked)
	d.logErr("set brightness", d.dev.SetBrightness(target, false))
}

func (d *Daemon) displayLoop(ctx context.Context) {
	ticker := time.NewTicker(displayInterval)
	defer ticker.Stop()
	var lastLockCheck time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if d.desktop != nil && now.Sub(lastLockCheck) >= lockInterval {
				lastLockCheck = now
				if locked, err := d.desktop.Locked(); err == nil {
					d.mu.Lock()
					if locked != d.locked {
						d.log.Info("screen lock changed", "locked", locked)
						if !locked {
							// Coming back counts as activity.
							d.lastActivity = now
						}
					}
					d.locked = locked
					d.mu.Unlock()
				}
			}
			d.updateBrightness(now)
		}
	}
}

// appLoop follows the focused application. It switches to the page bound
// to that app and back to where the user was once the app loses focus. A
// manual page switch in between hands control back to the user.
func (d *Daemon) appLoop(ctx context.Context) {
	if d.desktop == nil {
		return
	}
	var lastApp string
	for {
		d.mu.Lock()
		enabled := d.cfg.UsesAppPages()
		d.mu.Unlock()
		// Without app pages we only feed the "recently active" list of the
		// editor, so a slower poll is enough.
		wait := appInterval
		if !enabled {
			wait = 3 * appInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		app, err := d.desktop.ActiveApp()
		if errors.Is(err, ErrDesktopUnsupported) {
			d.log.Info("active app not available on this desktop")
			return
		}
		if err != nil || app == lastApp {
			continue
		}
		lastApp = app
		d.rememberApp(app)
		if enabled {
			d.followApp(app)
		}
	}
}

const maxRecentApps = 8

func (d *Daemon) rememberApp(app string) {
	if app == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recentApps = slices.DeleteFunc(d.recentApps, func(a string) bool { return a == app })
	d.recentApps = append([]string{app}, d.recentApps...)
	if len(d.recentApps) > maxRecentApps {
		d.recentApps = d.recentApps[:maxRecentApps]
	}
}

// RecentApps lists the most recently focused applications, newest first.
func (d *Daemon) RecentApps() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.recentApps)
}

func (d *Daemon) followApp(app string) {
	d.mu.Lock()
	target, matched := d.cfg.PageForApp(app)
	current := d.page
	// The user moved away from the auto page: stop managing it.
	if d.autoPage != "" && current != d.autoPage {
		d.autoPage, d.autoReturn = "", ""
	}
	switch {
	case matched && target != current:
		if d.autoPage == "" {
			d.autoReturn = current
		}
		d.autoPage = target
		d.mu.Unlock()
		d.log.Info("app focused, switching page", "app", app, "page", target)
		d.SwitchTo(target)
	case !matched && d.autoPage != "" && current == d.autoPage:
		back := d.autoReturn
		d.autoPage, d.autoReturn = "", ""
		d.mu.Unlock()
		if back != "" {
			d.log.Info("app lost focus, returning", "app", app, "page", back)
			d.SwitchTo(back)
		}
	default:
		d.mu.Unlock()
	}
}
