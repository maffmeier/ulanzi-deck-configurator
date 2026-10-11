// Package integrations talks to external services: Home Assistant, OBS,
// Microsoft Teams and MQTT brokers.
package integrations

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Secrets live in secrets.yaml next to deck.yaml, so the deck config can
// be shared without leaking tokens.
type Secrets struct {
	HomeAssistant HomeAssistantSecrets `yaml:"home_assistant" json:"home_assistant"`
	OBS           OBSSecrets           `yaml:"obs" json:"obs"`
	Teams         TeamsSecrets         `yaml:"teams,omitempty" json:"teams"`
	MQTT          MQTTSecrets          `yaml:"mqtt,omitempty" json:"mqtt"`
}

// TeamsSecrets holds the pairing token Teams hands out.
type TeamsSecrets struct {
	Token string `yaml:"token,omitempty" json:"token"`
}

type MQTTSecrets struct {
	URL      string `yaml:"url,omitempty" json:"url"`
	Username string `yaml:"username,omitempty" json:"username"`
	Password string `yaml:"password,omitempty" json:"password"`
}

type HomeAssistantSecrets struct {
	URL   string `yaml:"url,omitempty" json:"url"`
	Token string `yaml:"token,omitempty" json:"token"`
}

type OBSSecrets struct {
	URL      string `yaml:"url,omitempty" json:"url"`
	Password string `yaml:"password,omitempty" json:"password"`
}

func SecretsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "secrets.yaml")
}

// SecretStore reloads the file when it changes on disk.
type SecretStore struct {
	path string
	mu   sync.Mutex
	mod  time.Time
	data Secrets
}

func NewSecretStore(path string) *SecretStore { return &SecretStore{path: path} }

func (s *SecretStore) Get() Secrets {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.path)
	if err != nil {
		s.data, s.mod = Secrets{}, time.Time{}
		return s.data
	}
	if info.ModTime().Equal(s.mod) {
		return s.data
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return s.data
	}
	var data Secrets
	if yaml.Unmarshal(raw, &data) == nil {
		s.data, s.mod = data, info.ModTime()
	}
	return s.data
}

// Save writes the file readable for the owner only.
func (s *SecretStore) Save(data Secrets) error {
	raw, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
