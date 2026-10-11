package main

import (
	"context"
	"image"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/audio"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/calendar"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/media"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

// widgetSources adapts the infrastructure packages to the widget engine.
type widgetSources struct {
	runner       *actions.Runner
	integrations integrationRunner
}

func (s widgetSources) HomeAssistantState(entityID string) (application.HomeState, error) {
	return s.integrations.homeState(entityID)
}

func (s widgetSources) OBSStatus() (application.OBSInfo, error) { return s.integrations.obsStatus() }

func (widgetSources) CalendarEvents(url string) ([]application.CalendarEvent, error) {
	events, err := calendar.Fetch(url, time.Now())
	out := make([]application.CalendarEvent, 0, len(events))
	for _, e := range events {
		out = append(out, application.CalendarEvent{Summary: e.Summary, Start: e.Start, End: e.End, AllDay: e.AllDay})
	}
	return out, err
}

func (s widgetSources) MQTTValue(topic string) (string, bool, error) {
	return s.integrations.mqtt.Value(topic)
}

func (s widgetSources) MeetingState() (application.MeetingInfo, error) {
	return s.integrations.meetingState()
}

func (s widgetSources) CommandOutput(ctx context.Context, cmd string) (string, bool, error) {
	return s.runner.Output(ctx, cmd)
}

func (widgetSources) NowPlaying() (application.MediaInfo, error) {
	info, err := media.Current()
	return application.MediaInfo{Title: info.Title, Artist: info.Artist, Playing: info.Playing, ArtURL: info.ArtURL}, err
}

func (widgetSources) Cover(url string) image.Image { return media.Cover(url) }

func (widgetSources) AudioState(target string) (application.AudioInfo, error) {
	st, err := audio.Current(target)
	return application.AudioInfo{Volume: st.Volume, Muted: st.Muted, MicMuted: st.MicMuted, Output: st.Output, AppFound: st.AppFound}, err
}

func (widgetSources) LoadImage(path string) (image.Image, error) { return render.DecodeImageFile(path) }
