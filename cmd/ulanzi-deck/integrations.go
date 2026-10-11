package main

import (
	"fmt"
	"log/slog"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/integrations"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/plugins"
)

// integrationRunner executes home_assistant, obs, meeting and mqtt actions
// with the credentials from secrets.yaml.
type integrationRunner struct {
	secrets *integrations.SecretStore
	obs     *integrations.OBSClient
	teams   *integrations.TeamsClient
	mqtt    *integrations.MQTTClient
	plugins *plugins.Host
	runner  *actions.Runner
	log     *slog.Logger
}

func (r integrationRunner) Run(a deck.Action) error {
	s := r.secrets.Get()
	switch a.Type {
	case deck.ActionHome:
		return integrations.CallService(s.HomeAssistant, a.Service, a.EntityID)
	case deck.ActionOBS:
		return r.obs.Run(s.OBS, a.Op, a.Target)
	case deck.ActionMeeting:
		return r.meeting(a)
	case deck.ActionMQTT:
		return r.mqtt.Publish(a.Topic, a.Body, a.Retain)
	case deck.ActionPlugin:
		return r.plugins.RunAction(a.Plugin, a.Op, a.Settings)
	}
	return fmt.Errorf("unsupported integration action %q", a.Type)
}

// meeting uses the Teams API when Teams answers and the app's shortcut
// otherwise.
func (r integrationRunner) meeting(a deck.Action) error {
	if a.App == "teams" {
		err := r.teams.Run(a.Op)
		if err == nil {
			return nil
		}
		r.log.Info("teams API unavailable, sending the shortcut", "error", err)
	}
	shortcut, err := actions.MeetingShortcut(a.App, a.Op)
	if err != nil {
		return err
	}
	return r.runner.Run(shortcut)
}

func (r integrationRunner) meetingState() (application.MeetingInfo, error) {
	st, err := r.teams.State()
	return application.MeetingInfo{InMeeting: st.InMeeting, Muted: st.Muted, VideoOn: st.VideoOn, HandRaised: st.HandRaised}, err
}

func (r integrationRunner) homeState(entityID string) (application.HomeState, error) {
	st, err := integrations.State(r.secrets.Get().HomeAssistant, entityID)
	return application.HomeState{State: st.State, Unit: st.Unit, Name: st.FriendlyName, Attributes: st.Attributes}, err
}

func (r integrationRunner) obsStatus() (application.OBSInfo, error) {
	st, err := r.obs.Status(r.secrets.Get().OBS)
	return application.OBSInfo{Scene: st.Scene, Recording: st.Recording, RecordTime: st.RecordTime, Streaming: st.Streaming, StreamTime: st.StreamTime}, err
}
