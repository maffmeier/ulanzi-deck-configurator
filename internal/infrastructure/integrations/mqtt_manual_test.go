package integrations

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MQTT_MANUAL=mqtt://host:1883 runs against a real broker.
func TestManualMQTT(t *testing.T) {
	url := os.Getenv("MQTT_MANUAL")
	if url == "" {
		t.Skip("set MQTT_MANUAL to a broker URL")
	}
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.yaml"))
	store.Save(Secrets{MQTT: MQTTSecrets{URL: url}})
	client := NewMQTTClient(store)
	if err := client.Publish("ulanzi-deck/test", "hello", true); err != nil {
		t.Fatal(err)
	}
	reader := NewMQTTClient(store)
	deadline := time.Now().Add(3 * time.Second)
	for {
		v, ok, err := reader.Value("ulanzi-deck/test")
		if ok && v == "hello" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retained message not received: %q %v %v", v, ok, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	client.Publish("ulanzi-deck/test", "live", false)
	for deadline = time.Now().Add(3 * time.Second); ; {
		if v, _, _ := reader.Value("ulanzi-deck/test"); v == "live" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("live message not received")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
