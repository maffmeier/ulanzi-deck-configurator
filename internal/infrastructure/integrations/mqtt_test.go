package integrations

import (
	"bufio"
	"encoding/binary"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBroker is just enough MQTT 3.1.1 for the client: it checks the
// password, keeps retained messages and forwards publishes to subscribers.
type fakeBroker struct {
	ln       net.Listener
	password string
	mu       sync.Mutex
	retained map[string]string
	subs     map[string][]net.Conn
}

func startBroker(t *testing.T, password string) *fakeBroker {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBroker{ln: ln, password: password, retained: map[string]string{}, subs: map[string][]net.Conn{}}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	return b
}

func (b *fakeBroker) url() string { return "mqtt://" + b.ln.Addr().String() }

func (b *fakeBroker) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		kind, flags, body, err := readPacket(r)
		if err != nil {
			return
		}
		switch kind {
		case mqttConnect:
			code := byte(0)
			if b.password != "" && !strings.Contains(string(body), b.password) {
				code = 4
			}
			conn.Write(mqttPacket(mqttConnAck, 0, []byte{0, code}))
		case mqttSubscribe:
			n := int(binary.BigEndian.Uint16(body[2:]))
			topic := string(body[4 : 4+n])
			b.mu.Lock()
			b.subs[topic] = append(b.subs[topic], conn)
			value, ok := b.retained[topic]
			b.mu.Unlock()
			conn.Write(mqttPacket(mqttSubAck, 0, append(body[:2:2], 0)))
			if ok {
				conn.Write(mqttPacket(mqttPublish, 1, append(mqttString(topic), value...)))
			}
		case mqttPublish:
			n := int(binary.BigEndian.Uint16(body))
			topic, payload := string(body[2:2+n]), body[2+n:]
			b.mu.Lock()
			if flags&1 != 0 {
				b.retained[topic] = string(payload)
			}
			targets := append([]net.Conn(nil), b.subs[topic]...)
			b.mu.Unlock()
			for _, c := range targets {
				c.Write(mqttPacket(mqttPublish, 0, body))
			}
		case mqttPingReq:
			conn.Write(mqttPacket(mqttPingResp, 0, nil))
		}
	}
}

func TestMQTTPublishSubscribeAndRetained(t *testing.T) {
	broker := startBroker(t, "pw")
	broker.retained["home/light"] = `{"state":"ON"}`
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.yaml"))
	store.Save(Secrets{MQTT: MQTTSecrets{URL: broker.url(), Username: "deck", Password: "pw"}})
	client := NewMQTTClient(store)

	waitValue := func(topic, want string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			got, ok, err := client.Value(topic)
			if err == nil && ok && got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: got %q ok=%v err=%v, want %q", topic, got, ok, err, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitValue("home/light", `{"state":"ON"}`)
	if err := client.Publish("home/light", `{"state":"OFF"}`, true); err != nil {
		t.Fatal(err)
	}
	waitValue("home/light", `{"state":"OFF"}`)
}

func TestMQTTRejectsBadPassword(t *testing.T) {
	broker := startBroker(t, "right")
	err := CheckMQTT(MQTTSecrets{URL: broker.url(), Username: "deck", Password: "wrong"})
	if err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("got %v", err)
	}
	if err := CheckMQTT(MQTTSecrets{URL: broker.url(), Username: "deck", Password: "right"}); err != nil {
		t.Fatal(err)
	}
	if err := CheckMQTT(MQTTSecrets{}); err != ErrMQTTNotConfigured {
		t.Fatalf("empty url: %v", err)
	}
}
