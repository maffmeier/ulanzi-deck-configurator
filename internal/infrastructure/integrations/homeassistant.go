package integrations

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotConfigured = errors.New("not configured (secrets.yaml)")

var haClient = &http.Client{Timeout: 10 * time.Second}

// EntityState is the part of a Home Assistant state the widgets show.
type EntityState struct {
	State        string
	Unit         string
	FriendlyName string
	Attributes   map[string]any
}

func haRequest(s HomeAssistantSecrets, method, path string, body any) ([]byte, error) {
	if s.URL == "" || s.Token == "" {
		return nil, fmt.Errorf("home assistant %w", ErrNotConfigured)
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, strings.TrimRight(s.URL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := haClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("home assistant: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// CallService calls e.g. "light.toggle" for an entity.
func CallService(s HomeAssistantSecrets, service, entityID string) error {
	domain, name, ok := strings.Cut(service, ".")
	if !ok {
		return fmt.Errorf("invalid service %q", service)
	}
	payload := map[string]any{}
	if entityID != "" {
		payload["entity_id"] = entityID
	}
	_, err := haRequest(s, http.MethodPost, "/api/services/"+url.PathEscape(domain)+"/"+url.PathEscape(name), payload)
	return err
}

func State(s HomeAssistantSecrets, entityID string) (EntityState, error) {
	data, err := haRequest(s, http.MethodGet, "/api/states/"+url.PathEscape(entityID), nil)
	if err != nil {
		return EntityState{}, err
	}
	var raw struct {
		State      string         `json:"state"`
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return EntityState{}, err
	}
	st := EntityState{State: raw.State, Attributes: raw.Attributes}
	st.Unit, _ = raw.Attributes["unit_of_measurement"].(string)
	st.FriendlyName, _ = raw.Attributes["friendly_name"].(string)
	return st, nil
}

// CheckHomeAssistant verifies URL and token.
func CheckHomeAssistant(s HomeAssistantSecrets) error {
	_, err := haRequest(s, http.MethodGet, "/api/", nil)
	return err
}
