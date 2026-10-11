package plugins

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHelperPlugin is the plugin program: the test binary started again
// with ULANZI_PLUGIN_HELPER set.
func TestHelperPlugin(t *testing.T) {
	if os.Getenv("ULANZI_PLUGIN_HELPER") == "" {
		t.Skip("helper process")
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var ev map[string]any
		json.Unmarshal(in.Bytes(), &ev)
		switch ev["event"] {
		case "widget":
			settings := ev["settings"].(map[string]any)
			fmt.Printf(`{"update":{"instance":%q,"title":"Wetter","lines":["%v 21°"],"color":"#123456"}}`+"\n", ev["instance"], settings["city"])
		case "action":
			os.WriteFile("acted", []byte(ev["action"].(string)), 0o644)
			fmt.Println(`{"log":"done"}`)
		}
	}
	os.Exit(0)
}

func installHelper(t *testing.T) *Host {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "weather")
	os.MkdirAll(pluginDir, 0o755)
	manifest := fmt.Sprintf(`name: Wetter
command: [%q, "-test.run=TestHelperPlugin"]
actions:
  - {id: refresh, label: Aktualisieren}
widgets:
  - id: temp
    label: Temperatur
    settings: [{key: city, label: Stadt, placeholder: Berlin}]
`, os.Args[0])
	os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(dir, "broken"), nil, 0o644)
	t.Setenv("ULANZI_PLUGIN_HELPER", "1")
	h := NewHost(dir, "test", slog.New(slog.DiscardHandler))
	t.Cleanup(h.Stop)
	return h
}

func TestManifestsAndWidgets(t *testing.T) {
	h := installHelper(t)
	list := h.Manifests()
	if len(list) != 1 || list[0].ID != "weather" || list[0].Widgets[0].Settings[0].Key != "city" {
		t.Fatalf("manifests %+v", list)
	}

	settings := map[string]string{"city": "Kiel"}
	deadline := time.Now().Add(5 * time.Second)
	for {
		v, ok, err := h.Widget("weather", "temp", settings)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if v.Title != "Wetter" || len(v.Lines) != 1 || !strings.HasPrefix(v.Lines[0], "Kiel") || v.Color != "#123456" {
				t.Fatalf("view %+v", v)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no update from plugin")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := h.RunAction("weather", "refresh", nil); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(h.Dir(), "weather", "acted")
	for deadline = time.Now().Add(5 * time.Second); ; {
		if data, _ := os.ReadFile(marker); string(data) == "refresh" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("action not received")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestUnknownPlugin(t *testing.T) {
	h := installHelper(t)
	for _, id := range []string{"missing", "../weather", ""} {
		if err := h.RunAction(id, "x", nil); err == nil {
			t.Fatalf("%q must fail", id)
		}
	}
}

func TestInstanceIDDependsOnSettings(t *testing.T) {
	a := InstanceID("temp", map[string]string{"city": "Kiel"})
	b := InstanceID("temp", map[string]string{"city": "Bonn"})
	if a == b || a != InstanceID("temp", map[string]string{"city": "Kiel"}) {
		t.Fatal("instance ids")
	}
}
