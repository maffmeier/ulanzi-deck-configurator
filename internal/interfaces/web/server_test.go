package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/catalog"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/configfile"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/integrations"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/metrics"
)

func newTestServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s := &Server{
		ConfigPath: filepath.Join(t.TempDir(), "deck.yaml"),
		Version:    "test",
		Metrics:    metrics.NewReader(),
		Log:        slog.New(slog.DiscardHandler),
	}
	return s, s.Handler()
}

func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndexIsStampedAndAlpineLocal(t *testing.T) {
	_, h := newTestServer(t)
	rec := do(t, h, "GET", "/", nil)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "/static/app.js?v=") || !strings.Contains(body, "/static/alpine.min.js?v=") {
		t.Fatalf("index: %d", rec.Code)
	}
	if strings.Contains(body, "cdn.jsdelivr") {
		t.Fatal("frontend must not load from a CDN")
	}
}

func TestRejectsForeignHost(t *testing.T) {
	_, h := newTestServer(t)
	req := httptest.NewRequest("GET", "http://evil.example/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestEditorRoundTrip(t *testing.T) {
	s, h := newTestServer(t)

	rec := do(t, h, "GET", "/api/editor", nil)
	var initial editorConfig
	_ = json.Unmarshal(rec.Body.Bytes(), &initial)
	if initial.ConfigExists || initial.DefaultPage != "main" {
		t.Fatalf("default editor %+v", initial)
	}

	icon := "~/icons/x.png"
	payload := map[string]any{
		"default_page": "main",
		"brightness":   80,
		"pages": []any{
			map[string]any{"name": "main", "buttons": []any{
				map[string]any{"index": 0, "label": "Hi", "icon_path": icon,
					"action":     map[string]any{"type": "switch_page", "page": "@next"},
					"text_style": map[string]any{"background_color": "#112233", "text_color": "#ffffff", "font_family": "DejaVu Sans", "font_size": 30}},
				map[string]any{"index": 4, "label": "clash"},
			}},
			map[string]any{"name": "zwei", "buttons": []any{}},
		},
		"fixed_buttons": []any{map[string]any{"index": 4, "label": "fixed", "action": map[string]any{"type": "none"}}},
		"small_window":  map[string]any{"enabled": true, "interval_s": 2, "time_format": "%H:%M", "show_metrics": false, "background_color": "#000000", "metrics_items": []string{}, "temperature_sensors": []string{}, "temperature_separator": " "},
	}
	rec = do(t, h, "PUT", "/api/editor", payload)
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}

	cfg, err := configfile.Load(s.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brightness != 80 || len(cfg.Pages) != 2 || cfg.Pages[1].Name != "zwei" {
		t.Fatalf("saved config %+v", cfg)
	}
	main := cfg.Pages[0]
	if len(main.Buttons) != 1 || main.Buttons[0].Action.Page != "@next" || main.Buttons[0].IconPath != icon {
		t.Fatalf("page buttons %+v", main.Buttons)
	}
	if main.Buttons[0].TextStyle.TextColor != "#FFFFFF" {
		t.Fatal("colors must be normalized")
	}
	if backups, _ := configfile.ListBackups(s.ConfigPath); len(backups) != 1 {
		t.Fatalf("expected one backup, got %v", backups)
	}
}

func TestEditorValidationErrors(t *testing.T) {
	s, h := newTestServer(t)
	payload := map[string]any{
		"default_page": "missing",
		"pages":        []any{map[string]any{"name": "main", "buttons": []any{}}},
		"small_window": map[string]any{"interval_s": 2, "time_format": "%H:%M", "background_color": "#000000"},
	}
	rec := do(t, h, "POST", "/api/editor/validate", payload)
	var summary validationSummary
	_ = json.Unmarshal(rec.Body.Bytes(), &summary)
	if summary.OK || summary.Error == nil {
		t.Fatalf("expected validation error, got %s", rec.Body)
	}
	rec = do(t, h, "PUT", "/api/editor", payload)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d", rec.Code)
	}
	if _, err := os.Stat(s.ConfigPath); err == nil {
		t.Fatal("invalid config must not be written")
	}
}

func TestAssetEndpointOnlyServesImages(t *testing.T) {
	s, h := newTestServer(t)
	secret := filepath.Join(filepath.Dir(s.ConfigPath), "secret.txt")
	_ = os.WriteFile(secret, []byte("x"), 0o600)
	rec := do(t, h, "GET", "/api/asset?path="+queryEscape(secret), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestPredefinedIconsExistInCatalog(t *testing.T) {
	cat, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	s, h := newTestServer(t)
	s.Catalog = cat
	h = s.Handler()
	rec := do(t, h, "GET", "/api/predefined", nil)
	var body struct {
		Items []actions.PredefinedInfo `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Items) < 30 {
		t.Fatalf("got %d items, %v", len(body.Items), err)
	}
	for _, p := range body.Items {
		if _, err := cat.PNG(p.Icon); err != nil {
			t.Fatalf("%s: icon %s: %v", p.ID, p.Icon, err)
		}
	}
}

func TestSecretsAreWriteOnly(t *testing.T) {
	s, _ := newTestServer(t)
	s.Secrets = integrations.NewSecretStore(filepath.Join(filepath.Dir(s.ConfigPath), "secrets.yaml"))
	h := s.Handler()

	rec := do(t, h, "PUT", "/api/secrets", map[string]any{"home_assistant_url": "http://ha:8123", "home_assistant_token": "geheim", "obs_password": "pw"})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "geheim") || strings.Contains(rec.Body.String(), `"pw"`) {
		t.Fatalf("secrets leaked or failed: %d %s", rec.Code, rec.Body)
	}
	var view secretsView
	_ = json.Unmarshal(do(t, h, "GET", "/api/secrets", nil).Body.Bytes(), &view)
	if !view.HomeAssistantTokenSet || !view.OBSPasswordSet || view.HomeAssistantToken != "" {
		t.Fatalf("view %+v", view)
	}
	// Saving without a token keeps the stored one.
	do(t, h, "PUT", "/api/secrets", map[string]any{"home_assistant_url": "http://ha2:8123"})
	if got := s.Secrets.Get().HomeAssistant; got.Token != "geheim" || got.URL != "http://ha2:8123" {
		t.Fatalf("stored %+v", got)
	}
}

func TestBackupsListAndRestore(t *testing.T) {
	s, h := newTestServer(t)
	first := "default_page: main\npages:\n  main:\n    buttons:\n      - {index: 0, label: Eins}\n"
	second := strings.Replace(first, "Eins", "Zwei", 1)
	for _, text := range []string{first, second} {
		if rec := do(t, h, "PUT", "/api/config", map[string]any{"content": text}); rec.Code != 200 {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(s.ConfigPath))
	for _, e := range entries {
		if e.Name() != "deck.yaml" && e.Name() != "backups" {
			t.Fatalf("stray file next to the config: %s", e.Name())
		}
	}

	var list struct {
		Items []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"items"`
	}
	_ = json.Unmarshal(do(t, h, "GET", "/api/backups", nil).Body.Bytes(), &list)
	if len(list.Items) != 2 || !list.Items[0].Current || list.Items[1].Current {
		t.Fatalf("backups %+v", list.Items)
	}

	rec := do(t, h, "POST", "/api/backups/"+list.Items[1].Name+"/restore", nil)
	if rec.Code != 200 {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(s.ConfigPath)
	if !strings.Contains(string(data), "Eins") {
		t.Fatalf("config after restore:\n%s", data)
	}
	if rec := do(t, h, "POST", "/api/backups/secrets.yaml/restore", nil); rec.Code != 404 {
		t.Fatalf("foreign name: %d", rec.Code)
	}
}

func TestExportImportProfile(t *testing.T) {
	s, h := newTestServer(t)
	os.WriteFile(s.ConfigPath, []byte("default_page: main\npages:\n  main:\n    buttons:\n      - {index: 0, label: Eins}\n  zwei:\n    buttons: []\n"), 0o644)
	rec := do(t, h, "GET", "/api/export?page=main", nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Disposition"), "main.deckprofile") {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "main.deckprofile")
	fw.Write(rec.Body.Bytes())
	mw.Close()
	req := httptest.NewRequest("POST", "http://127.0.0.1:8765/api/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	var imported editorConfig
	_ = json.Unmarshal(res.Body.Bytes(), &imported)
	if res.Code != 200 || len(imported.Pages) != 1 || imported.Pages[0].Buttons[0].Label != "Eins" {
		t.Fatalf("import: %d %s", res.Code, res.Body)
	}
}

func TestDeckOwnConfigAndRouting(t *testing.T) {
	s, _ := newTestServer(t)
	os.WriteFile(s.ConfigPath, []byte("default_page: main\npages:\n  main:\n    buttons:\n      - {index: 0, label: Gemeinsam}\n"), 0o644)
	s.Decks = func() []DeckInfo { return []DeckInfo{{Key: "SER1", Connected: true}} }
	s.ConfigFor = func(key string) (string, bool) {
		main, _ := configfile.Load(s.ConfigPath)
		return configfile.DeviceConfigPath(s.ConfigPath, main.Devices, key), key == "SER1"
	}
	h := s.Handler()

	if rec := do(t, h, "POST", "/api/decks/SER1/own-config", nil); rec.Code != 200 {
		t.Fatalf("own config: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "POST", "/api/decks/OTHER/own-config", nil); rec.Code != 404 {
		t.Fatalf("unknown deck: %d", rec.Code)
	}
	devicePath := filepath.Join(filepath.Dir(s.ConfigPath), "deck-SER1.yaml")
	if data, _ := os.ReadFile(devicePath); !strings.Contains(string(data), "Gemeinsam") || strings.Contains(string(data), "devices") {
		t.Fatalf("device config:\n%s", data)
	}

	var ed editorConfig
	_ = json.Unmarshal(do(t, h, "GET", "/api/editor?device=SER1", nil).Body.Bytes(), &ed)
	if ed.Path != devicePath {
		t.Fatalf("editor path %q", ed.Path)
	}
	ed.Pages[0].Buttons[0].Label = "Nur Deck 1"
	if rec := do(t, h, "PUT", "/api/editor?device=SER1", ed); rec.Code != 200 {
		t.Fatalf("save device: %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(devicePath); !strings.Contains(string(data), "Nur Deck 1") {
		t.Fatal("device config not saved")
	}

	// Saving the main config from the editor keeps the devices map.
	var mainEd editorConfig
	_ = json.Unmarshal(do(t, h, "GET", "/api/editor", nil).Body.Bytes(), &mainEd)
	if mainEd.Devices["SER1"] != "deck-SER1.yaml" {
		t.Fatalf("devices %v", mainEd.Devices)
	}
	if rec := do(t, h, "PUT", "/api/editor", mainEd); rec.Code != 200 {
		t.Fatalf("save main: %d", rec.Code)
	}
	if main, _ := configfile.Load(s.ConfigPath); main.Devices["SER1"] == "" || main.Pages[0].Buttons[0].Label != "Gemeinsam" {
		t.Fatalf("main after save: %+v", main)
	}

	if rec := do(t, h, "DELETE", "/api/decks/SER1/own-config", nil); rec.Code != 200 {
		t.Fatalf("shared: %d", rec.Code)
	}
	if main, _ := configfile.Load(s.ConfigPath); len(main.Devices) != 0 {
		t.Fatalf("mapping left: %v", main.Devices)
	}
}
