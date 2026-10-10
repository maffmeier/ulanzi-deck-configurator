package application

import (
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

type fakeIntegrations struct{ ran chan deck.Action }

func (f fakeIntegrations) Run(a deck.Action) error { f.ran <- a; return nil }

func TestToggleAlternatesActionsAndVisual(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons, deck.Button{
		Index: 7, Label: "An", Action: shell("on"), Press: deck.PressTap,
		Toggle: &deck.ToggleState{Label: "Aus", Action: shell("off")},
	})
	_, dev, runner := startDaemon(t, cfg)
	press(dev, 7)
	eventually(t, func() bool { c := runner.commands(); return len(c) == 1 && c[0] == "on" })
	eventually(t, func() bool {
		labels := dev.partialLabels(7)
		return len(labels) > 0 && labels[len(labels)-1] == "Aus"
	})
	press(dev, 7)
	eventually(t, func() bool { c := runner.commands(); return len(c) == 2 && c[1] == "off" })
	eventually(t, func() bool {
		labels := dev.partialLabels(7)
		return labels[len(labels)-1] == "An"
	})
}

func TestMultiRunsStepsInOrderWithDelay(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons, deck.Button{
		Index: 7, Press: deck.PressTap,
		Action: &deck.Action{Type: deck.ActionMulti, Steps: []deck.Action{
			*shell("a"), {Type: deck.ActionDelay, Ms: 150}, *shell("b"),
		}},
	})
	_, dev, runner := startDaemon(t, cfg)
	start := time.Now()
	press(dev, 7)
	eventually(t, func() bool { return len(runner.commands()) == 2 })
	if c := runner.commands(); c[0] != "a" || c[1] != "b" {
		t.Fatalf("order %v", c)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("delay step ignored")
	}
}

func TestFolderBackReturnsToPreviousPage(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages = append(cfg.Pages, deck.Page{Name: "folder", Folder: true, BackIndex: 4})
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons, deck.Button{Index: 7, Action: &deck.Action{Type: deck.ActionSwitchPage, Page: "folder"}, Press: deck.PressTap})
	d, dev, _ := startDaemon(t, cfg)
	press(dev, 7)
	eventually(t, func() bool { return d.CurrentPage() == "folder" && dev.lastUploadLabel(4) == "← Zurück" })
	press(dev, 4)
	eventually(t, func() bool { return d.CurrentPage() == "one" })
}

func TestIntegrationActionsGoToIntegrations(t *testing.T) {
	cfg := pagedConfig()
	cfg.Pages[0].Buttons = append(cfg.Pages[0].Buttons, deck.Button{Index: 7, Press: deck.PressTap,
		Action: &deck.Action{Type: deck.ActionHome, Service: "light.toggle", EntityID: "light.x"}})
	d, dev, runner := startDaemon(t, cfg)
	ran := make(chan deck.Action, 1)
	d.SetIntegrations(fakeIntegrations{ran: ran})
	press(dev, 7)
	select {
	case a := <-ran:
		if a.EntityID != "light.x" {
			t.Fatalf("got %+v", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("integration not called")
	}
	if len(runner.commands()) != 0 {
		t.Fatal("runner must not see integration actions")
	}
}
