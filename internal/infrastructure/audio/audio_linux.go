package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// pactl talks to PulseAudio and to PipeWire's pulse server alike.
func pactl(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pactl", args...).Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, fmt.Errorf("pactl not found: %w", ErrUnsupported)
	}
	if err != nil {
		return nil, fmt.Errorf("pactl %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type paChannel struct {
	ValuePercent string `json:"value_percent"`
}

type paNode struct {
	Index       int                  `json:"index"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Mute        bool                 `json:"mute"`
	Volume      map[string]paChannel `json:"volume"`
	Properties  map[string]any       `json:"properties"`
}

func (n paNode) percent() int {
	total, count := 0, 0
	for _, ch := range n.Volume {
		if v, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(ch.ValuePercent), "%")); err == nil {
			total += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / count
}

func (n paNode) prop(key string) string {
	s, _ := n.Properties[key].(string)
	return s
}

func (n paNode) label() string {
	if n.Description != "" {
		return n.Description
	}
	return n.Name
}

func list(kind string) ([]paNode, error) {
	out, err := pactl("-f", "json", "list", kind)
	if err != nil {
		return nil, err
	}
	var nodes []paNode
	if err := json.Unmarshal(out, &nodes); err != nil {
		return nil, fmt.Errorf("pactl %s: %w", kind, err)
	}
	return nodes, nil
}

func defaultName(kind string) string {
	out, _ := pactl("get-default-" + kind)
	return strings.TrimSpace(string(out))
}

func defaultNode(kind string) (paNode, error) {
	nodes, err := list(kind + "s")
	if err != nil {
		return paNode{}, err
	}
	name := defaultName(kind)
	for _, n := range nodes {
		if n.Name == name {
			return n, nil
		}
	}
	return paNode{}, fmt.Errorf("default %s %q not found", kind, name)
}

func appStreams(target string) ([]paNode, error) {
	inputs, err := list("sink-inputs")
	if err != nil {
		return nil, err
	}
	var matched []paNode
	for _, in := range inputs {
		if matchesApp(target, in.prop("application.name"), in.prop("application.process.binary"), in.prop("application.id"), in.prop("node.name")) {
			matched = append(matched, in)
		}
	}
	if len(matched) == 0 {
		return nil, ErrAppNotFound
	}
	return matched, nil
}

func changeVolume(target string, delta int) error {
	if target == "" {
		sink, err := defaultNode("sink")
		if err != nil {
			return err
		}
		_, err = pactl("set-sink-volume", "@DEFAULT_SINK@", fmt.Sprintf("%d%%", clampPercent(sink.percent()+delta)))
		return err
	}
	streams, err := appStreams(target)
	if err != nil {
		return err
	}
	for _, s := range streams {
		if _, err := pactl("set-sink-input-volume", strconv.Itoa(s.Index), fmt.Sprintf("%d%%", clampPercent(s.percent()+delta))); err != nil {
			return err
		}
	}
	return nil
}

func toggleMute(target string) error {
	if target == "" {
		_, err := pactl("set-sink-mute", "@DEFAULT_SINK@", "toggle")
		return err
	}
	streams, err := appStreams(target)
	if err != nil {
		return err
	}
	for _, s := range streams {
		if _, err := pactl("set-sink-input-mute", strconv.Itoa(s.Index), "toggle"); err != nil {
			return err
		}
	}
	return nil
}

func toggleMic() error {
	_, err := pactl("set-source-mute", "@DEFAULT_SOURCE@", "toggle")
	return err
}

// switchOutput changes the default sink and moves playing streams along,
// which PulseAudio (unlike PipeWire) does not do by itself.
func switchOutput(want string) error {
	sinks, err := list("sinks")
	if err != nil {
		return err
	}
	labels := make([]string, len(sinks))
	current := defaultName("sink")
	currentLabel := ""
	for i, s := range sinks {
		labels[i] = s.label()
		if s.Name == current {
			currentLabel = labels[i]
		}
	}
	i, err := nextDevice(labels, currentLabel, want)
	if err != nil {
		return err
	}
	target := sinks[i].Name
	if _, err := pactl("set-default-sink", target); err != nil {
		return err
	}
	inputs, _ := list("sink-inputs")
	for _, in := range inputs {
		_, _ = pactl("move-sink-input", strconv.Itoa(in.Index), target)
	}
	return nil
}

func current(target string) (State, error) {
	sink, err := defaultNode("sink")
	if err != nil {
		return State{}, err
	}
	st := State{Volume: sink.percent(), Muted: sink.Mute, Output: sink.label(), AppFound: true}
	if source, err := defaultNode("source"); err == nil {
		st.MicMuted = source.Mute
	}
	if target != "" {
		streams, err := appStreams(target)
		if errors.Is(err, ErrAppNotFound) {
			st.AppFound, st.Volume, st.Muted = false, 0, false
			return st, nil
		}
		if err != nil {
			return st, err
		}
		st.Volume, st.Muted = streams[0].percent(), streams[0].Mute
	}
	return st, nil
}
