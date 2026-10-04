package application

import (
	"context"
	"log/slog"
	"os"
	"time"

	"ulanzi-deck/internal/domain/deck"
)

// WatchConfig polls the config file and applies every valid change.
// Polling (instead of inotify & co.) behaves the same on all platforms and
// survives editors that replace the file on save.
func WatchConfig(ctx context.Context, path string, load func(string) (*deck.Config, error), apply func(*deck.Config), log *slog.Logger) {
	fingerprint := func() (time.Time, int64, bool) {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, 0, false
		}
		return info.ModTime(), info.Size(), true
	}
	lastMod, lastSize, _ := fingerprint()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		mod, size, ok := fingerprint()
		if !ok || (mod.Equal(lastMod) && size == lastSize) {
			continue
		}
		lastMod, lastSize = mod, size
		cfg, err := load(path)
		if err != nil {
			// Keep the previous config running; a half-finished edit must
			// not blank the deck.
			log.Error("config reload failed, keeping previous config", "path", path, "error", err)
			continue
		}
		log.Info("config file changed, reloading", "path", path)
		apply(cfg)
	}
}
