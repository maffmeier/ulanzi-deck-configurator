package main

import (
	"context"
	"log/slog"
	"maps"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/configfile"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/d200"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/interfaces/web"
)

const fleetInterval = 2 * time.Second

// fleet runs a device, a daemon and a config watcher for every attached
// deck. Decks use the main config unless its devices map names their own.
type fleet struct {
	mainPath  string
	log       *slog.Logger
	newDaemon func(dev application.Deck, cfg *deck.Config, log *slog.Logger) *application.Daemon
	// enumerate and open are the HID functions, replaced in tests.
	enumerate func() ([]d200.DeviceInfo, error)
	open      func(key string) d200.Opener

	mu      sync.Mutex
	decks   map[string]*deckRun
	infos   map[string]d200.DeviceInfo
	devices map[string]string
	mainMod time.Time
}

type deckRun struct {
	path   string
	dev    *d200.Device
	daemon *application.Daemon
	cancel context.CancelFunc
	done   chan struct{}
}

func newFleet(mainPath string, log *slog.Logger, newDaemon func(application.Deck, *deck.Config, *slog.Logger) *application.Daemon) *fleet {
	return &fleet{
		mainPath: mainPath, log: log, newDaemon: newDaemon, enumerate: d200.Enumerate, open: d200.OpenKey,
		decks: map[string]*deckRun{}, infos: map[string]d200.DeviceInfo{},
	}
}

func (f *fleet) Run(ctx context.Context) {
	ticker := time.NewTicker(fleetInterval)
	defer ticker.Stop()
	for {
		f.sync(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// devicesLocked rereads the devices map whenever the main config changed.
func (f *fleet) devicesLocked() map[string]string {
	info, err := os.Stat(f.mainPath)
	if err != nil || info.ModTime().Equal(f.mainMod) {
		return f.devices
	}
	if cfg, err := configfile.Load(f.mainPath); err == nil {
		f.devices, f.mainMod = cfg.Devices, info.ModTime()
	}
	return f.devices
}

func (f *fleet) sync(ctx context.Context) {
	found, err := f.enumerate()
	if err != nil {
		f.log.Debug("deck enumeration failed", "error", err)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	devices := f.devicesLocked()
	for _, info := range found {
		key := info.Key()
		f.infos[key] = info
		path := configfile.DeviceConfigPath(f.mainPath, devices, key)
		run := f.decks[key]
		if run != nil && run.path == path {
			continue
		}
		if run != nil {
			f.log.Info("deck config changed, restarting it", "deck", key, "config", path)
			run.stop()
		}
		f.decks[key] = f.start(ctx, key, path)
	}
}

func (f *fleet) start(ctx context.Context, key, path string) *deckRun {
	log := f.log.With("deck", key)
	cfg, err := configfile.Load(path)
	if err != nil {
		log.Error("config invalid, starting with an empty layout until it is fixed", "path", path, "error", err)
		cfg = fallbackConfig()
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &deckRun{path: path, dev: d200.NewDevice(f.open(key), log.With("component", "deck")), cancel: cancel, done: make(chan struct{})}
	run.daemon = f.newDaemon(run.dev, cfg, log.With("component", "daemon"))
	go func() {
		run.dev.Run(runCtx)
		close(run.done)
	}()
	go run.daemon.Run(runCtx)
	go application.WatchConfig(runCtx, path, configfile.Load, run.daemon.ApplyConfig, log.With("component", "watcher"))
	log.Info("deck started", "config", path)
	return run
}

// stop ends the deck's loops and waits briefly for the device to close, so
// the next run can open it again.
func (r *deckRun) stop() {
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(time.Second):
	}
}

func (f *fleet) Connected() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, run := range f.decks {
		if run.dev.Connected() {
			return true
		}
	}
	return false
}

// RecentApps merges what the daemons saw, newest first per deck.
func (f *fleet) RecentApps() []string {
	f.mu.Lock()
	runs := slices.Collect(maps.Values(f.decks))
	f.mu.Unlock()
	var out []string
	for _, run := range runs {
		for _, app := range run.daemon.RecentApps() {
			if !slices.Contains(out, app) {
				out = append(out, app)
			}
		}
	}
	return out
}

func (f *fleet) Decks() []web.DeckInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	devices := f.devicesLocked()
	out := []web.DeckInfo{}
	for key, run := range f.decks {
		info := f.infos[key]
		out = append(out, web.DeckInfo{
			Key: key, Product: info.Product, Serial: info.Serial, Connected: run.dev.Connected(),
			ConfigPath: run.path, OwnConfig: devices[key] != "",
		})
	}
	slices.SortFunc(out, func(a, b web.DeckInfo) int {
		if a.Key < b.Key {
			return -1
		}
		return 1
	})
	return out
}

// ConfigFor is the config file of a known deck.
func (f *fleet) ConfigFor(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.infos[key]; !ok {
		return "", false
	}
	return configfile.DeviceConfigPath(f.mainPath, f.devicesLocked(), key), true
}
