package application

import (
	"fmt"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// PluginView is what a plugin widget shows.
type PluginView struct {
	Title string
	Lines []string
	Color string
}

// PluginSource asks plugins for widget content; it is optional for the
// engine. ok is false until the plugin answered.
type PluginSource interface {
	PluginWidget(plugin, widget string, settings map[string]string) (view PluginView, ok bool, err error)
}

type pluginReading struct {
	view PluginView
	ok   bool
}

const pluginRefresh = 500 * time.Millisecond

// ExpirePlugins refetches plugin views on the next render.
func (e *WidgetEngine) ExpirePlugins() { expire(e, e.plugins) }

func (e *WidgetEngine) renderPlugin(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	src, ok := e.sources.(PluginSource)
	if !ok {
		return e.renderer.Text(w, h, bg, wd.Title, []string{e.text("error")}, false)
	}
	// fmt prints maps with sorted keys, so equal settings give equal keys.
	key := fmt.Sprintf("%s\x00%s\x00%v", wd.Plugin, wd.Show, wd.Settings)
	r, err, at := pollKeyed(e, e.plugins, key, pluginRefresh, now, func() (pluginReading, error) {
		v, ok, err := src.PluginWidget(wd.Plugin, wd.Show, wd.Settings)
		return pluginReading{v, ok}, err
	})
	title := wd.Title
	if title == "" && r.ok {
		title = r.view.Title
	}
	switch {
	case at.IsZero() || (err == nil && !r.ok):
		return e.renderer.Text(w, h, bg, title, []string{"…"}, false)
	case err != nil:
		return e.renderer.Text(w, h, bg, title, []string{e.text("error")}, false)
	}
	if c, err := deck.NormalizeHexColor(r.view.Color); err == nil && r.view.Color != "" {
		bg = c
	}
	lines := r.view.Lines
	if len(lines) > 4 {
		lines = lines[:4]
	}
	return e.renderer.Text(w, h, bg, title, lines, false)
}
