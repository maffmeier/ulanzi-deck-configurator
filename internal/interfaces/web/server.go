// Package web serves the browser editor and its JSON API on localhost.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"ulanzi-deck/internal/domain/deck"
	"ulanzi-deck/internal/infrastructure/actions"
	"ulanzi-deck/internal/infrastructure/catalog"
	"ulanzi-deck/internal/infrastructure/configfile"
	"ulanzi-deck/internal/infrastructure/d200"
	"ulanzi-deck/internal/infrastructure/metrics"
	"ulanzi-deck/internal/infrastructure/render"
)

//go:embed static
var staticFiles embed.FS

type Server struct {
	ConfigPath string
	Version    string
	Catalog    *catalog.Catalog
	Metrics    *metrics.Reader
	Connected  func() bool
	Log        *slog.Logger
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
	mux.HandleFunc("GET /api/small-window/preview", s.smallWindowPreview)
	mux.HandleFunc("GET /api/temperature-sensors", s.temperatureSensors)
	mux.HandleFunc("GET /api/predefined", s.predefined)
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
	for _, asset := range []string{"app.js", "app.css", "alpine.min.js"} {
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

func (s *Server) configExists() bool {
	_, err := os.Stat(s.ConfigPath)
	return err == nil
}

func (s *Server) configDir() string { return filepath.Dir(s.ConfigPath) }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	found := 0
	if devices, err := d200.Enumerate(); err == nil {
		found = len(devices)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"version":        s.Version,
		"config_path":    s.ConfigPath,
		"config_exists":  s.configExists(),
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

func (s *Server) getEditor(w http.ResponseWriter, _ *http.Request) {
	if !s.configExists() {
		writeJSON(w, http.StatusOK, defaultEditorConfig(s.ConfigPath))
		return
	}
	cfg, err := configfile.Load(s.ConfigPath)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "config parse failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEditorConfig(cfg, s.ConfigPath, true))
}

func (s *Server) parseYAML(text []byte) (*deck.Config, error) {
	return configfile.Parse(text, s.configDir())
}

func (s *Server) validateEditor(w http.ResponseWriter, r *http.Request) {
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
	cfg, err := s.parseYAML(text)
	if err != nil {
		writeJSON(w, http.StatusOK, failedSummary(err))
		return
	}
	writeJSON(w, http.StatusOK, summarize(cfg))
}

func (s *Server) putEditor(w http.ResponseWriter, r *http.Request) {
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
	cfg, err := s.parseYAML(text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	versioned, bundle, err := s.save(text, cfg, req.SaveFirmwareBundle)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "write failed: "+err.Error())
		return
	}
	resp := toEditorConfig(cfg, s.ConfigPath, true)
	resp.VersionedConfigPath = strPtr(versioned)
	resp.SavedFirmwareBundlePath = strPtr(bundle)
	writeJSON(w, http.StatusOK, resp)
}

// save writes a timestamped snapshot (and optionally the upload ZIP of the
// default page) before atomically replacing the config. The daemon's file
// watcher picks the change up within a second.
func (s *Server) save(text []byte, cfg *deck.Config, withBundle bool) (string, string, error) {
	now := time.Now()
	versioned := configfile.VersionedPath(s.ConfigPath, "", "", now)
	if err := os.MkdirAll(s.configDir(), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(versioned, text, 0o644); err != nil {
		return "", "", err
	}
	var bundle string
	if withBundle {
		blob, err := d200.BuildButtonsZip(cfg.ButtonsFor(cfg.DefaultPage), true)
		if err != nil {
			os.Remove(versioned)
			return "", "", err
		}
		bundle = configfile.VersionedPath(s.ConfigPath, "firmware", ".zip", now)
		if err := os.WriteFile(bundle, blob, 0o644); err != nil {
			os.Remove(versioned)
			return "", "", err
		}
	}
	if err := configfile.WriteAtomic(s.ConfigPath, text); err != nil {
		os.Remove(versioned)
		if bundle != "" {
			os.Remove(bundle)
		}
		return "", "", err
	}
	s.Log.Info("config saved", "path", s.ConfigPath, "snapshot", versioned)
	return versioned, bundle, nil
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	info, err := os.Stat(s.ConfigPath)
	if err != nil {
		writeDetail(w, http.StatusNotFound, "config not found at "+s.ConfigPath)
		return
	}
	content, err := os.ReadFile(s.ConfigPath)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": s.ConfigPath, "content": string(content),
		"mtime": float64(info.ModTime().UnixNano()) / 1e9, "size": info.Size(),
	})
}

type configText struct {
	Content            string `json:"content"`
	SaveFirmwareBundle bool   `json:"save_firmware_bundle"`
}

func (s *Server) validateConfig(w http.ResponseWriter, r *http.Request) {
	var req configText
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := s.parseYAML([]byte(req.Content))
	if err != nil {
		writeJSON(w, http.StatusOK, failedSummary(err))
		return
	}
	writeJSON(w, http.StatusOK, summarize(cfg))
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var req configText
	if err := readJSON(r, &req); err != nil {
		writeDetail(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := s.parseYAML([]byte(req.Content))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, failedSummary(err))
		return
	}
	versioned, bundle, err := s.save([]byte(req.Content), cfg, req.SaveFirmwareBundle)
	if err != nil {
		writeDetail(w, http.StatusInternalServerError, "write failed: "+err.Error())
		return
	}
	summary := summarize(cfg)
	summary.VersionedConfigPath = strPtr(versioned)
	summary.SavedFirmwareBundlePath = strPtr(bundle)
	writeJSON(w, http.StatusOK, summary)
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

func allocatePath(dir, stem string) string {
	candidate := filepath.Join(dir, stem+".png")
	for i := 1; ; i++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
		candidate = filepath.Join(dir, stem+"-"+strconv.Itoa(i)+".png")
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
	pngData, err := render.NormalizeUpload(file)
	if err != nil {
		writeDetail(w, http.StatusUnprocessableEntity, "invalid image upload: "+err.Error())
		return
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
	target := allocatePath(dir, stem)
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
