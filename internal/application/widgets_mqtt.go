package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/maffmeier/ulanzi-deck-configurator/internal/domain/deck"
)

// MQTTSource returns the last message of a topic; it is optional for the
// engine. ok is false until a message arrived.
type MQTTSource interface {
	MQTTValue(topic string) (payload string, ok bool, err error)
}

type mqttReading struct {
	payload string
	ok      bool
}

const mqttRefresh = 500 * time.Millisecond

// ExpireMQTT refetches MQTT values on the next render.
func (e *WidgetEngine) ExpireMQTT() { expire(e, e.mqtt) }

func (e *WidgetEngine) renderMQTT(wd deck.Widget, w, h int, bg string, now time.Time) []byte {
	title := wd.Title
	if title == "" {
		title = wd.Topic[strings.LastIndex(wd.Topic, "/")+1:]
	}
	src, ok := e.sources.(MQTTSource)
	if !ok {
		return e.renderer.Text(w, h, bg, title, []string{e.text("offline")}, false)
	}
	r, err, at := pollKeyed(e, e.mqtt, wd.Topic, mqttRefresh, now, func() (mqttReading, error) {
		payload, ok, err := src.MQTTValue(wd.Topic)
		return mqttReading{payload, ok}, err
	})
	switch {
	case at.IsZero() || (err == nil && !r.ok):
		return e.renderer.Text(w, h, bg, title, []string{"…"}, false)
	case err != nil:
		return e.renderer.Text(w, h, bg, title, []string{e.text("offline")}, false)
	}
	value := r.payload
	if wd.Attribute != "" {
		value = jsonAttribute(r.payload, wd.Attribute)
	}
	if strings.EqualFold(strings.TrimSpace(value), wd.ActiveState) {
		bg = orColor(wd.OkColor, bg)
	} else if wd.FailColor != "" {
		bg = wd.FailColor
	}
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > 4 {
		lines = lines[:4]
	}
	return e.renderer.Text(w, h, bg, title, lines, false)
}

// jsonAttribute reads a dotted path like "state" or "sensor.temperature"
// from a JSON payload, e.g. a zigbee2mqtt device state.
func jsonAttribute(payload, path string) string {
	var v any
	if json.Unmarshal([]byte(payload), &v) != nil {
		return ""
	}
	for _, part := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[part]
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", x), "0"), ".")
	default:
		out, _ := json.Marshal(x)
		return string(out)
	}
}
