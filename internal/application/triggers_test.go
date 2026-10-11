package application

import (
	"slices"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

func TestTriggersRunOnStartupAppAndLock(t *testing.T) {
	cfg := pagedConfig()
	cfg.Triggers = []deck.Trigger{
		{Event: deck.TriggerStartup, Action: *shell("startup")},
		{Event: deck.TriggerApp, App: "OBS", Action: *shell("obs focused")},
		{Event: deck.TriggerLock, Action: *shell("locked")},
		{Event: deck.TriggerUnlock, Action: *shell("unlocked")},
		{Event: deck.TriggerTime, At: "00:00", Action: *shell("never in this test")},
	}
	desktop := &fakeDesktop{app: "code"}
	dev, runner := newFakeDeck(), &fakeRunner{}
	newTestDaemon(t, dev, runner, cfg, desktop)
	has := func(cmd string) func() bool {
		return func() bool { return slices.Contains(runner.commands(), cmd) }
	}
	eventually(t, has("startup"))

	desktop.set("com.obsproject.Studio", false)
	eventuallyWithin(t, 4*time.Second, has("obs focused"))
	desktop.set("code", true)
	eventuallyWithin(t, 4*time.Second, has("locked"))
	desktop.set("code", false)
	eventuallyWithin(t, 4*time.Second, has("unlocked"))
	if slices.Contains(runner.commands(), "never in this test") && time.Now().Hour() != 0 {
		t.Fatal("time trigger fired at the wrong time")
	}
}

func TestSwitchPageTrigger(t *testing.T) {
	cfg := pagedConfig()
	cfg.Triggers = []deck.Trigger{{Event: deck.TriggerStartup, Action: deck.Action{Type: deck.ActionSwitchPage, Page: "two"}}}
	d, _, _ := startDaemon(t, cfg)
	eventually(t, func() bool { return d.CurrentPage() == "two" })
}
