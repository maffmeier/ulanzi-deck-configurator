package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/d200"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

type nullRunner struct{}

func (nullRunner) Run(deck.Action) error             { return nil }
func (nullRunner) Press(deck.Action) (func(), error) { return func() {}, nil }

func TestFleetStartsOneDaemonPerDeckAndFollowsMapping(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "deck.yaml")
	layout := "default_page: main\npages:\n  main:\n    buttons: []\n"
	os.WriteFile(mainPath, []byte(layout), 0o644)
	os.WriteFile(filepath.Join(dir, "deck-B.yaml"), []byte(layout), 0o644)

	log := slog.New(slog.DiscardHandler)
	engine := application.NewWidgetEngine(nil, nil, render.Renderer{}, log)
	f := newFleet(mainPath, log, func(dev application.Deck, cfg *deck.Config, log *slog.Logger) *application.Daemon {
		d := application.NewDaemon(dev, nullRunner{}, nil, render.Renderer{}, engine, cfg, log)
		d.ShareEngine()
		return d
	})
	f.enumerate = func() ([]d200.DeviceInfo, error) {
		return []d200.DeviceInfo{{Serial: "A", Product: "ulanzi"}, {Serial: "B", Product: "ulanzi"}}, nil
	}
	f.open = func(string) d200.Opener {
		return func() (d200.Transport, error) { return nil, d200.ErrDeviceNotFound }
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	f.sync(ctx)
	decks := f.Decks()
	if len(decks) != 2 || decks[0].ConfigPath != mainPath || decks[1].ConfigPath != mainPath {
		t.Fatalf("decks %+v", decks)
	}
	first := f.decks["B"]

	// Give deck B its own file; the next sync restarts only B.
	os.WriteFile(mainPath, []byte(layout+"devices:\n  B: deck-B.yaml\n"), 0o644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(mainPath, future, future)
	f.sync(ctx)
	decks = f.Decks()
	if decks[0].ConfigPath != mainPath || decks[1].ConfigPath != filepath.Join(dir, "deck-B.yaml") || !decks[1].OwnConfig {
		t.Fatalf("after mapping %+v", decks)
	}
	if f.decks["B"] == first {
		t.Fatal("deck B was not restarted")
	}
	if path, ok := f.ConfigFor("B"); !ok || path != filepath.Join(dir, "deck-B.yaml") {
		t.Fatalf("ConfigFor %q %v", path, ok)
	}
	if _, ok := f.ConfigFor("C"); ok {
		t.Fatal("unknown deck")
	}
}
