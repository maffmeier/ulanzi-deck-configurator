package main

import (
	"context"
	"image"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/media"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

// widgetSources adapts the infrastructure packages to the widget engine.
type widgetSources struct {
	runner *actions.Runner
}

func (s widgetSources) CommandOutput(ctx context.Context, cmd string) (string, bool, error) {
	return s.runner.Output(ctx, cmd)
}

func (widgetSources) NowPlaying() (application.MediaInfo, error) {
	info, err := media.Current()
	return application.MediaInfo{Title: info.Title, Artist: info.Artist, Playing: info.Playing, ArtURL: info.ArtURL}, err
}

func (widgetSources) Cover(url string) image.Image { return media.Cover(url) }

func (widgetSources) LoadImage(path string) (image.Image, error) { return render.DecodeImageFile(path) }
