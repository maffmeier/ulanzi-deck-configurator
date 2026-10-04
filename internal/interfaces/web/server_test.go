package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ulanzi-deck/internal/infrastructure/actions"
	"ulanzi-deck/internal/infrastructure/catalog"
	"ulanzi-deck/internal/infrastructure/configfile"
	"ulanzi-deck/internal/infrastructure/metrics"
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
	snapshots, _ := filepath.Glob(filepath.Join(filepath.Dir(s.ConfigPath), "deck-*.yaml"))
	if len(snapshots) != 1 {
		t.Fatalf("expected one snapshot, got %v", snapshots)
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
