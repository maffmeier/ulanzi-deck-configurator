// Package web serves the browser editor and its JSON API on localhost.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/application"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/actions"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/catalog"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/configfile"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/d200"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/integrations"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/metrics"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/plugins"
	"github.com/maffmeier/ulanzi-deck-configurator/internal/infrastructure/render"
)

//go:embed static
var staticFiles embed.FS

// DeckInfo describes an attached deck for the editor's deck switch.
type DeckInfo struct {
	Key        string `json:"key"`
	Product    string `json:"product"`
	Serial     string `json:"serial"`
	Connected  bool   `json:"connected"`
	ConfigPath string `json:"config_path"`
	OwnConfig  bool   `json:"own_config"`
}

type Server struct {
	ConfigPath string
	Version    string
	Catalog    *catalog.Catalog
	Metrics    *metrics.Reader
	Connected  func() bool
	Widgets    *application.WidgetEngine
	Secrets    *integrations.SecretStore
	RecentApps func() []string
	// Decks lists attached decks; ConfigFor maps a deck to its config.
	Decks     func() []DeckInfo
	ConfigFor func(key string) (string, bool)
	Plugins   *plugins.Host
	Log       *slog.Logger
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFiles, "static")

	mux.HandleFunc("GET /{$}", s.index)
	mux.Handle("GET /static/", http.StripPrefix("/static/", noCache(http.FileServerFS(static))))
	mux.Handle("GET /fonts/", http.StripPrefix("/fonts/", http.FileServerFS(render.FontFS())))
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/devices", s.devices)
	mux.HandleFunc("GET /api/editor", s.getEditor)
	mux.HandleFunc("PUT /api/editor", s.putEditor)
	mux.HandleFunc("POST /api/editor/validate", s.validateEditor)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("POST /api/config/validate", s.validateConfig)
	mux.HandleFunc("GET /api/backups", s.listBackups)
	mux.HandleFunc("GET /api/export", s.exportProfile)
	mux.HandleFunc("POST /api/import", s.importProfile)
	mux.HandleFunc("POST /api/backups/{name}/restore", s.restoreBackup)
	mux.HandleFunc("GET /api/small-window/preview", s.smallWindowPreview)
	mux.HandleFunc("GET /api/temperature-sensors", s.temperatureSensors)
	mux.HandleFunc("GET /api/predefined", s.predefined)
	mux.HandleFunc("POST /api/render/widget", s.renderWidget)
	mux.HandleFunc("GET /api/secrets", s.getSecrets)
	mux.HandleFunc("GET /api/desktop/recent-apps", s.recentApps)
	mux.HandleFunc("GET /api/plugins", s.listPlugins)
	mux.HandleFunc("GET /api/decks", s.listDecks)
	mux.HandleFunc("POST /api/decks/{key}/own-config", s.ownDeckConfig)
	mux.HandleFunc("DELETE /api/decks/{key}/own-config", s.sharedDeckConfig)
	mux.HandleFunc("PUT /api/secrets", s.putSecrets)
	mux.HandleFunc("POST /api/secrets/test", s.testSecrets)
	mux.HandleFunc("POST /api/assets", s.uploadAsset)
	mux.HandleFunc("GET /api/asset", s.getAsset)
	mux.HandleFunc("GET /api/builtin-assets", s.builtinAssets)
	mux.HandleFunc("GET /api/builtin-asset", s.builtinAsset)
	mux.HandleFunc("POST /api/builtin-assets/import", s.importBuiltinAsset)
	return localOnly(mux)
}

// localOnly rejects requests whose Host is not a loopback name. The editor
// can write files, so a malicious website must not reach it through DNS
// rebinding.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if host != "localhost" && !isLoopbackIP(host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackIP(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeDetail(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20)).Decode(v)
}

func queryEscape(s string) string { return url.QueryEscape(s) }

// index stamps asset URLs with a content hash so a cached app.js never
// pairs with a newer page.
func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := string(html)
	for _, asset := range []string{"app.js", "app.css", "i18n.js", "alpine.min.js"} {
		data, err := staticFiles.ReadFile("static/" + asset)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		ref := `"/static/` + asset + `"`
		page = strings.ReplaceAll(page, ref, `"/static/`+asset+`?v=`+hex.EncodeToString(sum[:])[:12]+`"`)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(page))
}

func configExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Server) configDir() string { return filepath.Dir(s.ConfigPath) }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	found := 0
	if devices, err := d200.Enumerate(); err == nil {
		found = len(devices)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"version":        s.Version,
		"config_path":    path,
		"config_exists":  configExists(path),
		"devices_found":  found,
		"deck_connected": s.Connected != nil && s.Connected(),
	})
}

func (s *Server) devices(w http.ResponseWriter, _ *http.Request) {
	devices, err := d200.Enumerate()
	if err != nil {
		s.Log.Warn("hid enumerate failed", "error", err)
	}
	out := []map[string]any{}
	for _, d := range devices {
		out = append(out, map[string]any{
			"manufacturer": d.Manufacturer, "product": d.Product, "serial": d.Serial,
			"interface_number": 0, "path": d.Path,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getEditor(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	if !configExists(path) {
		writeJSON(w, http.StatusOK, defaultEditorConfig(path))
		return
	}
	cfg, err := configfile.Load(path)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "config parse failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEditorConfig(cfg, path, true))
}

// parseYAML resolves relative paths against the directory of the config
// the text belongs to.
func (s *Server) parseYAML(path string, text []byte) (*deck.Config, error) {
	return configfile.Parse(text, filepath.Dir(path))
}

// pathFor is the config the request edits: the main config, or the one of
// the deck named by ?device=.
func (s *Server) pathFor(r *http.Request) string {
	if key := r.URL.Query().Get("device"); key != "" && s.ConfigFor != nil {
		if path, ok := s.ConfigFor(key); ok {
			return path
		}
	}
	return s.ConfigPath
}

func (s *Server) validateEditor(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	var req editorPutRequest
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	text, err := fromEditor(req)
	if err != nil {
		writeJSON(w, http.StatusOK, failedSummary(err))
		return
	}
	cfg, err := s.parseYAML(path, text)
	if err != nil {
		writeJSON(w, http.StatusOK, failedSummary(err))
		return
	}
	writeJSON(w, http.StatusOK, summarize(cfg))
}

func (s *Server) putEditor(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	var req editorPutRequest
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	text, err := fromEditor(req)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	cfg, err := s.parseYAML(path, text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	versioned, bundle, err := s.save(path, text, cfg, req.SaveFirmwareBundle)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "write failed: "+err.Error())
		return
	}
	resp := toEditorConfig(cfg, path, true)
	resp.VersionedConfigPath = strPtr(versioned)
	resp.SavedFirmwareBundlePath = strPtr(bundle)
	writeJSON(w, http.StatusOK, resp)
}

// save keeps a version in the backup directory (and optionally the upload
// ZIP of the default page) before atomically replacing the config. The
// daemon's file watcher picks the change up within a second.
func (s *Server) save(path string, text []byte, cfg *deck.Config, withBundle bool) (string, string, error) {
	now := time.Now()
	versioned, err := configfile.WriteBackup(path, text, now)
	if err != nil {
		return "", "", err
	}
	var bundle string
	if withBundle {
		blob, err := d200.BuildButtonsZip(cfg.ButtonsFor(cfg.DefaultPage), true)
		if err != nil {
			os.Remove(versioned)
			return "", "", err
		}
		bundle = configfile.VersionedPath(filepath.Join(configfile.BackupDir(path), filepath.Base(path)), "firmware", ".zip", now)
		if err := os.WriteFile(bundle, blob, 0o644); err != nil {
			os.Remove(versioned)
			return "", "", err
		}
	}
	if err := configfile.WriteAtomic(path, text); err != nil {
		os.Remove(versioned)
		if bundle != "" {
			os.Remove(bundle)
		}
		return "", "", err
	}
	s.Log.Info("config saved", "path", path, "snapshot", versioned)
	return versioned, bundle, nil
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	info, err := os.Stat(path)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "config not found at "+path)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": path, "content": string(content),
		"mtime": float64(info.ModTime().UnixNano()) / 1e9, "size": info.Size(),
	})
}

type configText struct {
	Content            string `json:"content"`
	SaveFirmwareBundle bool   `json:"save_firmware_bundle"`
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	var req configText
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := s.parseYAML(path, []byte(req.Content))
	if err != nil {
		writeJSON(w, http.StatusOK, failedSummary(err))
		return
	}
	writeJSON(w, http.StatusOK, summarize(cfg))
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	var req configText
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := s.parseYAML(path, []byte(req.Content))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	versioned, bundle, err := s.save(path, []byte(req.Content), cfg, req.SaveFirmwareBundle)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "write failed: "+err.Error())
		return
	}
	summary := summarize(cfg)
	summary.VersionedConfigPath = strPtr(versioned)
	summary.SavedFirmwareBundlePath = strPtr(bundle)
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	backups, err := configfile.ListBackups(path)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	current, _ := os.ReadFile(path)
	items := []map[string]any{}
	for _, b := range backups {
		item := map[string]any{"name": b.Name, "saved": b.Saved.Format(time.RFC3339), "size": b.Size}
		if data, err := configfile.ReadBackup(path, b.Name); err == nil && string(data) == string(current) {
			item["current"] = true
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// restoreBackup saves an old version as the current config, so restoring is
// itself a new version and can be undone the same way.
func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	text, err := configfile.ReadBackup(path, r.PathValue("name"))
	if errors.Is(err, configfile.ErrBackupNotFound) {
		writeDetail(w, http.StatusNotFound, "backup not found")
		return
	}
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg, err := s.parseYAML(path, text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	if _, _, err := s.save(path, text, cfg, false); err != nil {
		writeDetail(w, http.StatusInternalServerError, "write failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEditorConfig(cfg, path, true))
}

// exportProfile downloads the saved config (or one page of it) with its
// images as a .deckprofile file.
func (s *Server) exportProfile(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	cfg, err := configfile.Load(path)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	page := r.URL.Query().Get("page")
	data, err := configfile.ExportProfile(cfg, page)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	name := "ulanzi-deck"
	if page != "" {
		name = strings.Trim(unsafeFilename.ReplaceAllString(page, "-"), ".-")
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + configfile.ProfileExt}))
	_, _ = w.Write(data)
}

// importProfile unpacks the images of an uploaded profile next to the
// config and returns its layout; the editor merges it and the user saves.
func (s *Server) importProfile(w http.ResponseWriter, r *http.Request) {
	path := s.pathFor(r)
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	stem := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	stem = strings.Trim(unsafeFilename.ReplaceAllString(stem, "-"), ".-")
	if stem == "" {
		stem = "profile"
	}
	dir := filepath.Join(s.configDir(), "icons", "imported", stem)
	for i := 2; fileExists(dir); i++ {
		dir = filepath.Join(s.configDir(), "icons", "imported", stem+"-"+strconv.Itoa(i))
	}
	cfg, err := configfile.ImportProfile(data, dir)
	if err != nil {
		os.RemoveAll(dir)
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEditorConfig(cfg, path, true))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (s *Server) smallWindowPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("time_format")
	if format == "" {
		format = deck.DefaultTimeFormat
	}
	separator := q.Get("temperature_separator")
	if separator == "" {
		separator = " "
	}
	var selected []string
	for _, m := range q["metrics_items"] {
		if slices.Contains(deck.MetricChoices, m) && len(selected) < 3 {
			selected = append(selected, m)
		}
	}
	items := []map[string]string{}
	for _, m := range selected {
		value := s.Metrics.MetricValue(m)
		if m == "temperature" {
			value = s.Metrics.TemperatureValue(q["temperature_sensors"], separator)
		}
		items = append(items, map[string]string{"id": m, "label": deck.MetricLabels[m], "value": value})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"time_text":   s.Metrics.FormatTime(format),
		"cpu_percent": s.Metrics.CPUPercent(),
		"mem_percent": s.Metrics.MemoryPercent(),
		"gpu_percent": 0,
		"metrics":     items,
	})
}

func (s *Server) predefined(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": actions.Predefined()})
}

func (s *Server) temperatureSensors(w http.ResponseWriter, _ *http.Request) {
	sensors := s.Metrics.TemperatureSensors()
	if sensors == nil {
		sensors = []metrics.TemperatureSensor{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": sensors})
}

var unsafeFilename = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func allocatePath(dir, stem, ext string) string {
	candidate := filepath.Join(dir, stem+ext)
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, stem+"-"+strconv.Itoa(i)+ext)
	}
}

func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 20<<20)
	file, header, err := r.FormFile("file")
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	// Animated GIFs are kept as they are; the deck plays them.
	pngData, ext := raw, ".gif"
	if !render.IsAnimatedGIF(raw) {
		ext = ".png"
		if pngData, err = render.NormalizeUpload(bytes.NewReader(raw)); err != nil {
			writeDetail(w, http.StatusUnprocessableEntity, "invalid image upload: "+err.Error())
			return
		}
	}
	stem := strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	stem = strings.Trim(unsafeFilename.ReplaceAllString(stem, "-"), ".-")
	if stem == "" {
		stem = "button-icon"
	}
	dir := filepath.Join(s.configDir(), "icons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	target := allocatePath(dir, stem, ext)
	if err := os.WriteFile(target, pngData, 0o644); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	compact := configfile.CompactPath(target)
	writeJSON(w, http.StatusOK, map[string]string{"path": compact, "preview_url": *assetPreviewURL(compact)})
}

var imageExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}

// getAsset serves icon previews; only image files are exposed.
func (s *Server) getAsset(w http.ResponseWriter, r *http.Request) {
	path := configfile.ResolvePath(r.URL.Query().Get("path"), s.configDir())
	ext := strings.ToLower(filepath.Ext(path))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !slices.Contains(imageExtensions, ext) {
		writeDetail(w, http.StatusNotFound, "asset not found")
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(ext))
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, path)
}

func (s *Server) builtinAssets(w http.ResponseWriter, _ *http.Request) {
	icons := s.Catalog.Icons()
	items := make([]map[string]any, 0, len(icons))
	for _, icon := range icons {
		items = append(items, map[string]any{
			"asset_id": icon.AssetID, "name": icon.Name, "style": icon.Style, "family": icon.Family,
			"search_terms": icon.SearchTerms,
			"preview_url":  "/api/builtin-asset?asset_id=" + queryEscape(icon.AssetID),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (s *Server) builtinAsset(w http.ResponseWriter, r *http.Request) {
	data, err := s.Catalog.PNG(r.URL.Query().Get("asset_id"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeDetail(w, http.StatusNotFound, "builtin asset not found")
		return
	}
	if err != nil {
		writeDetail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(data)
}

func (s *Server) importBuiltinAsset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetID string `json:"asset_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	target, err := s.Catalog.Materialize(req.AssetID, filepath.Join(s.configDir(), "icons", "builtin"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeDetail(w, http.StatusNotFound, "builtin asset not found")
		return
	}
	if err != nil {
		writeDetail(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	compact := configfile.CompactPath(target)
	writeJSON(w, http.StatusOK, map[string]string{"path": compact, "preview_url": *assetPreviewURL(compact)})
}

// renderWidget draws a widget exactly as the deck would show it, for the
// editor preview. It shares the daemon's engine, so timers and graphs show
// their live state.
func (s *Server) renderWidget(w http.ResponseWriter, r *http.Request) {
	if s.Widgets == nil {
		writeDetail(w, http.StatusServiceUnavailable, "widget engine not available")
		return
	}
	var req struct {
		Widget     editorWidget `json:"widget"`
		Target     string       `json:"target"`
		Background string       `json:"background"`
	}
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	onKey := req.Target == "key"
	widget, err := fromEditorWidget(req.Widget, onKey, s.configDir())
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	bg, err := deck.NormalizeHexColor(req.Background)
	if err != nil {
		bg = deck.DefaultSmallWindowBg
	}
	width, height := deck.InfoWindowWidth, deck.InfoWindowHeight
	if onKey {
		width = deck.IconSize
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.Widgets.Render(widget, width, height, bg, time.Now()))
}

// secretsView never sends tokens back to the browser; it only tells
// whether one is set. Empty fields in a PUT keep the stored value.
type secretsView struct {
	HomeAssistantURL      string `json:"home_assistant_url"`
	HomeAssistantToken    string `json:"home_assistant_token"`
	HomeAssistantTokenSet bool   `json:"home_assistant_token_set"`
	OBSURL                string `json:"obs_url"`
	OBSPassword           string `json:"obs_password"`
	OBSPasswordSet        bool   `json:"obs_password_set"`
	TeamsPaired           bool   `json:"teams_paired"`
	TeamsUnpair           bool   `json:"teams_unpair"`
	MQTTURL               string `json:"mqtt_url"`
	MQTTUsername          string `json:"mqtt_username"`
	MQTTPassword          string `json:"mqtt_password"`
	MQTTPasswordSet       bool   `json:"mqtt_password_set"`
}

func (s *Server) getSecrets(w http.ResponseWriter, _ *http.Request) {
	if s.Secrets == nil {
		writeJSON(w, http.StatusOK, secretsView{})
		return
	}
	sec := s.Secrets.Get()
	writeJSON(w, http.StatusOK, secretsView{
		HomeAssistantURL: sec.HomeAssistant.URL, HomeAssistantTokenSet: sec.HomeAssistant.Token != "",
		OBSURL: sec.OBS.URL, OBSPasswordSet: sec.OBS.Password != "",
		TeamsPaired: sec.Teams.Token != "",
		MQTTURL:     sec.MQTT.URL, MQTTUsername: sec.MQTT.Username, MQTTPasswordSet: sec.MQTT.Password != "",
	})
}

func (s *Server) mergeSecrets(r *http.Request) (integrations.Secrets, error) {
	var view secretsView
	if err := readJSON(r, &view); err != nil {
		return integrations.Secrets{}, err
	}
	sec := integrations.Secrets{}
	if s.Secrets != nil {
		sec = s.Secrets.Get()
	}
	sec.HomeAssistant.URL = strings.TrimSpace(view.HomeAssistantURL)
	sec.OBS.URL = strings.TrimSpace(view.OBSURL)
	if view.HomeAssistantToken != "" {
		sec.HomeAssistant.Token = strings.TrimSpace(view.HomeAssistantToken)
	}
	if view.OBSPassword != "" {
		sec.OBS.Password = view.OBSPassword
	}
	if view.TeamsUnpair {
		sec.Teams.Token = ""
	}
	sec.MQTT.URL = strings.TrimSpace(view.MQTTURL)
	sec.MQTT.Username = strings.TrimSpace(view.MQTTUsername)
	if view.MQTTPassword != "" {
		sec.MQTT.Password = view.MQTTPassword
	}
	return sec, nil
}

func (s *Server) putSecrets(w http.ResponseWriter, r *http.Request) {
	if s.Secrets == nil {
		writeDetail(w, http.StatusServiceUnavailable, "secrets not available")
		return
	}
	sec, err := s.mergeSecrets(r)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Secrets.Save(sec); err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.getSecrets(w, r)
}

// testSecrets checks the given (or stored) credentials without saving.
func (s *Server) testSecrets(w http.ResponseWriter, r *http.Request) {
	sec, err := s.mergeSecrets(r)
	if err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	result := map[string]string{}
	if sec.HomeAssistant.URL != "" {
		result["home_assistant"] = "ok"
		if err := integrations.CheckHomeAssistant(sec.HomeAssistant); err != nil {
			result["home_assistant"] = err.Error()
		}
	}
	if sec.OBS.URL != "" || sec.OBS.Password != "" {
		result["obs"] = "ok"
		if _, err := integrations.NewOBSClient().Request(sec.OBS, "GetVersion", nil); err != nil {
			result["obs"] = err.Error()
		}
	}
	if sec.MQTT.URL != "" {
		result["mqtt"] = "ok"
		if err := integrations.CheckMQTT(sec.MQTT); err != nil {
			result["mqtt"] = err.Error()
		}
	}
	// Teams is only reported when it is in use, it has no URL to configure.
	if err := integrations.CheckTeams(sec.Teams.Token); err == nil {
		result["teams"] = "ok"
	} else if sec.Teams.Token != "" {
		result["teams"] = err.Error()
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) decks() []DeckInfo {
	if s.Decks == nil {
		return []DeckInfo{}
	}
	return s.Decks()
}

func (s *Server) listDecks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.decks()})
}

func (s *Server) knownDeck(key string) bool {
	return slices.ContainsFunc(s.decks(), func(d DeckInfo) bool { return d.Key == key })
}

// ownDeckConfig gives a deck its own config file, starting as a copy of the
// main layout, and records it in the devices map of the main config.
func (s *Server) ownDeckConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !s.knownDeck(key) {
		writeDetail(w, http.StatusNotFound, "unknown deck")
		return
	}
	main, err := configfile.Load(s.ConfigPath)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	name := configfile.DeviceFileName(key)
	target := filepath.Join(s.configDir(), name)
	if !fileExists(target) {
		copied := *main
		copied.Devices = nil
		text, err := configfile.Marshal(&copied)
		if err == nil {
			_, _, err = s.save(target, text, &copied, false)
		}
		if err != nil {
			writeDetail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if main.Devices == nil {
		main.Devices = map[string]string{}
	}
	main.Devices[key] = name
	s.saveMain(w, main)
}

// sharedDeckConfig makes a deck use the main config again; its own file
// stays on disk.
func (s *Server) sharedDeckConfig(w http.ResponseWriter, r *http.Request) {
	main, err := configfile.Load(s.ConfigPath)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	delete(main.Devices, r.PathValue("key"))
	s.saveMain(w, main)
}

func (s *Server) saveMain(w http.ResponseWriter, main *deck.Config) {
	text, err := configfile.Marshal(main)
	if err == nil {
		_, _, err = s.save(s.ConfigPath, text, main, false)
	}
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.decks()})
}

func (s *Server) listPlugins(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"items": []plugins.Manifest{}, "dir": ""}
	if s.Plugins != nil {
		if list := s.Plugins.Manifests(); list != nil {
			out["items"] = list
		}
		out["dir"] = configfile.CompactPath(s.Plugins.Dir())
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) recentApps(w http.ResponseWriter, _ *http.Request) {
	apps := []string{}
	if s.RecentApps != nil {
		apps = append(apps, s.RecentApps()...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": apps})
}
