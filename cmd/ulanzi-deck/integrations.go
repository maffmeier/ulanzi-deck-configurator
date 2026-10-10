package main

import (
	"fmt"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/integrations"
)

// integrationRunner executes home_assistant and obs actions with the
// credentials from secrets.yaml.
type integrationRunner struct {
	secrets *integrations.SecretStore
	obs     *integrations.OBSClient
}

func (r integrationRunner) Run(a deck.Action) error {
	s := r.secrets.Get()
	switch a.Type {
	case deck.ActionHome:
		return integrations.CallService(s.HomeAssistant, a.Service, a.EntityID)
	case deck.ActionOBS:
		return r.obs.Run(s.OBS, a.Op, a.Target)
	}
	return fmt.Errorf("unsupported integration action %q", a.Type)
}

func (r integrationRunner) homeState(entityID string) (application.HomeState, error) {
	st, err := integrations.State(r.secrets.Get().HomeAssistant, entityID)
	return application.HomeState{State: st.State, Unit: st.Unit, Name: st.FriendlyName, Attributes: st.Attributes}, err
}

func (r integrationRunner) obsStatus() (application.OBSInfo, error) {
	st, err := r.obs.Status(r.secrets.Get().OBS)
	return application.OBSInfo{Scene: st.Scene, Recording: st.Recording, RecordTime: st.RecordTime, Streaming: st.Streaming, StreamTime: st.StreamTime}, err
}
