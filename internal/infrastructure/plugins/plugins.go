// Package plugins runs external programs that add actions and widgets.
//
// A plugin is a directory below plugins/ next to deck.yaml with a
// plugin.yaml manifest. The program it names is started on first use and
// talks JSON lines over stdin/stdout (see docs/plugins.md):
//
//	app → plugin: {"event":"start", ...}
//	              {"event":"action","action":"id","settings":{...}}
//	              {"event":"widget","widget":"id","instance":"…","settings":{...}}
//	plugin → app: {"update":{"instance":"…","title":"…","lines":["…"],"color":"#…"}}
//	              {"log":"text"}
package plugins

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	manifestName = "plugin.yaml"
	restartDelay = 5 * time.Second
	maxLine      = 1 << 20
)

var ErrUnknownPlugin = errors.New("plugin not installed")

// Setting is one input the editor shows for an action or widget.
type Setting struct {
	Key         string `yaml:"key" json:"key"`
	Label       string `yaml:"label" json:"label"`
	Placeholder string `yaml:"placeholder" json:"placeholder"`
}

// Item is an action or widget a plugin offers.
type Item struct {
	ID       string    `yaml:"id" json:"id"`
	Label    string    `yaml:"label" json:"label"`
	Icon     string    `yaml:"icon" json:"icon"`
	Settings []Setting `yaml:"settings" json:"settings"`
}

// Manifest is plugin.yaml.
type Manifest struct {
	ID      string   `yaml:"-" json:"id"`
	Name    string   `yaml:"name" json:"name"`
	Command []string `yaml:"-" json:"-"`
	Actions []Item   `yaml:"actions" json:"actions"`
	Widgets []Item   `yaml:"widgets" json:"widgets"`
	Dir     string   `yaml:"-" json:"-"`
}

// command accepts `command: [python3, main.py]` or `command: ./main`.
type rawManifest struct {
	Manifest `yaml:",inline"`
	Command  yaml.Node `yaml:"command"`
}

// View is what a plugin widget shows.
type View struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
	Color string   `json:"color"`
}

type message struct {
	Update *struct {
		Instance string `json:"instance"`
		View
	} `json:"update"`
	Log string `json:"log"`
}

// Host starts plugins on demand and restarts them when they exit.
type Host struct {
	dir     string
	version string
	log     *slog.Logger

	mu      sync.Mutex
	running map[string]*process
}

type process struct {
	stdin   io.WriteCloser
	views   map[string]View
	widgets map[string]map[string]any // instance -> widget event
	exited  bool
	started time.Time
}

func NewHost(dir, version string, log *slog.Logger) *Host {
	return &Host{dir: dir, version: version, log: log, running: map[string]*process{}}
}

// Dir is where plugins are installed.
func (h *Host) Dir() string { return h.dir }

// Manifests lists the installed plugins, sorted by name.
func (h *Host) Manifests() []Manifest {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return nil
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := h.manifest(e.Name())
		if err != nil {
			h.log.Warn("plugin ignored", "plugin", e.Name(), "error", err)
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (h *Host) manifest(id string) (Manifest, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return Manifest{}, ErrUnknownPlugin
	}
	dir := filepath.Join(h.dir, id)
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{}, ErrUnknownPlugin
	}
	if err != nil {
		return Manifest{}, err
	}
	var raw rawManifest
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Manifest{}, err
	}
	m := raw.Manifest
	m.ID, m.Dir = id, dir
	if m.Name == "" {
		m.Name = id
	}
	switch raw.Command.Kind {
	case yaml.ScalarNode:
		m.Command = strings.Fields(raw.Command.Value)
	case yaml.SequenceNode:
		if err := raw.Command.Decode(&m.Command); err != nil {
			return Manifest{}, err
		}
	}
	if len(m.Command) == 0 {
		return Manifest{}, errors.New("plugin.yaml needs a command")
	}
	return m, nil
}

// ensureLocked starts the plugin unless it is running; a plugin that exited
// is started again after restartDelay.
func (h *Host) ensureLocked(id string) (*process, error) {
	if p := h.running[id]; p != nil && (!p.exited || time.Since(p.started) < restartDelay) {
		if p.exited {
			return nil, fmt.Errorf("plugin %s stopped, restarting soon", id)
		}
		return p, nil
	}
	m, err := h.manifest(id)
	if err != nil {
		return nil, err
	}
	name := m.Command[0]
	if strings.HasPrefix(name, "./") || strings.HasPrefix(name, `.\`) {
		name = filepath.Join(m.Dir, name)
	}
	cmd := exec.Command(name, m.Command[1:]...)
	cmd.Dir = m.Dir
	cmd.Env = append(os.Environ(), "ULANZI_DECK_VERSION="+h.version)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin %s: %w", id, err)
	}
	previous := h.running[id]
	p := &process{stdin: stdin, views: map[string]View{}, widgets: map[string]map[string]any{}, started: time.Now()}
	h.running[id] = p
	log := h.log.With("plugin", id)
	log.Info("plugin started", "command", m.Command)

	go h.readLoop(p, stdout, log)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			log.Warn("plugin stderr", "line", s.Text())
		}
	}()
	go func() {
		err := cmd.Wait()
		h.mu.Lock()
		p.exited = true
		h.mu.Unlock()
		log.Warn("plugin exited", "error", err)
	}()

	if err := h.sendLocked(p, map[string]any{"event": "start", "version": h.version}); err != nil {
		return nil, err
	}
	// Widgets shown before a restart are announced again.
	if previous != nil {
		for instance, ev := range previous.widgets {
			p.widgets[instance] = ev
			_ = h.sendLocked(p, ev)
		}
	}
	return p, nil
}

func (h *Host) sendLocked(p *process, msg map[string]any) error {
	line, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = p.stdin.Write(append(line, '\n'))
	return err
}

func (h *Host) readLoop(p *process, stdout io.Reader, log *slog.Logger) {
	s := bufio.NewScanner(stdout)
	s.Buffer(make([]byte, 64<<10), maxLine)
	for s.Scan() {
		var msg message
		if err := json.Unmarshal(s.Bytes(), &msg); err != nil {
			log.Warn("plugin sent invalid JSON", "line", s.Text())
			continue
		}
		if msg.Log != "" {
			log.Info("plugin", "message", msg.Log)
		}
		if u := msg.Update; u != nil && u.Instance != "" {
			h.mu.Lock()
			p.views[u.Instance] = u.View
			h.mu.Unlock()
		}
	}
}

// RunAction sends an action event to the plugin.
func (h *Host) RunAction(id, action string, settings map[string]string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, err := h.ensureLocked(id)
	if err != nil {
		return err
	}
	return h.sendLocked(p, map[string]any{"event": "action", "action": action, "settings": nonNil(settings)})
}

// InstanceID identifies a widget with its settings, so two keys showing
// the same widget with different settings get separate updates.
func InstanceID(widget string, settings map[string]string) string {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	sum := sha256.New()
	sum.Write([]byte(widget))
	for _, k := range keys {
		fmt.Fprintf(sum, "\x00%s=%s", k, settings[k])
	}
	return widget + "-" + hex.EncodeToString(sum.Sum(nil))[:10]
}

// Widget returns the last view the plugin sent for a widget; the first
// call announces the widget to the plugin. ok is false until it answered.
func (h *Host) Widget(id, widget string, settings map[string]string) (View, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	p, err := h.ensureLocked(id)
	if err != nil {
		return View{}, false, err
	}
	instance := InstanceID(widget, settings)
	if _, known := p.widgets[instance]; !known {
		ev := map[string]any{"event": "widget", "widget": widget, "instance": instance, "settings": nonNil(settings)}
		p.widgets[instance] = ev
		if err := h.sendLocked(p, ev); err != nil {
			return View{}, false, err
		}
	}
	v, ok := p.views[instance]
	return v, ok, nil
}

// Stop ends all plugin processes by closing their input.
func (h *Host) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, p := range h.running {
		p.stdin.Close()
	}
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}
