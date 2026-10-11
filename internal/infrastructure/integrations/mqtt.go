package integrations

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// A small MQTT 3.1.1 client: QoS 0 publish and subscribe, which is all the
// deck needs. Retained messages arrive right after subscribing, so widgets
// show the current state immediately.
const (
	mqttConnect    = 1
	mqttConnAck    = 2
	mqttPublish    = 3
	mqttSubscribe  = 8
	mqttSubAck     = 9
	mqttPingReq    = 12
	mqttPingResp   = 13
	mqttDisconnect = 14

	mqttKeepAlive   = 30 * time.Second
	mqttDialTimeout = 3 * time.Second
	mqttRedial      = 3 * time.Second
	mqttMaxPacket   = 1 << 20
)

var ErrMQTTNotConfigured = errors.New("mqtt: no broker configured")

type mqttValue struct {
	payload string
	at      time.Time
}

// MQTTClient keeps one connection, resubscribes after reconnecting and
// caches the last message per topic.
type MQTTClient struct {
	secrets *SecretStore

	mu       sync.Mutex
	conn     net.Conn
	key      string
	subs     map[string]bool
	values   map[string]mqttValue
	lastDial time.Time
	nextID   uint16
	writeMu  sync.Mutex
}

func NewMQTTClient(secrets *SecretStore) *MQTTClient {
	return &MQTTClient{secrets: secrets, subs: map[string]bool{}, values: map[string]mqttValue{}}
}

func mqttString(s string) []byte {
	b := binary.BigEndian.AppendUint16(nil, uint16(len(s)))
	return append(b, s...)
}

func mqttPacket(kind, flags byte, body []byte) []byte {
	out := []byte{kind<<4 | flags}
	n := len(body)
	for {
		digit := byte(n % 128)
		n /= 128
		if n > 0 {
			digit |= 0x80
		}
		out = append(out, digit)
		if n == 0 {
			break
		}
	}
	return append(out, body...)
}

func readPacket(r *bufio.Reader) (kind, flags byte, body []byte, err error) {
	header, err := r.ReadByte()
	if err != nil {
		return 0, 0, nil, err
	}
	length, multiplier := 0, 1
	for i := 0; ; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, 0, nil, err
		}
		length += int(b&0x7f) * multiplier
		if b&0x80 == 0 {
			break
		}
		multiplier *= 128
		if i == 3 {
			return 0, 0, nil, errors.New("mqtt: malformed length")
		}
	}
	if length > mqttMaxPacket {
		return 0, 0, nil, errors.New("mqtt: packet too large")
	}
	body = make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, 0, nil, err
	}
	return header >> 4, header & 0x0f, body, nil
}

func dialMQTT(s MQTTSecrets) (net.Conn, *bufio.Reader, error) {
	if strings.TrimSpace(s.URL) == "" {
		return nil, nil, ErrMQTTNotConfigured
	}
	raw := s.URL
	if !strings.Contains(raw, "://") {
		raw = "mqtt://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("mqtt: %w", err)
	}
	secure := false
	switch u.Scheme {
	case "mqtt", "tcp":
	case "mqtts", "ssl", "tls":
		secure = true
	default:
		return nil, nil, fmt.Errorf("mqtt: unsupported scheme %q (use mqtt:// or mqtts://)", u.Scheme)
	}
	host := u.Host
	if u.Port() == "" {
		port := "1883"
		if secure {
			port = "8883"
		}
		host = net.JoinHostPort(u.Hostname(), port)
	}
	dialer := &net.Dialer{Timeout: mqttDialTimeout}
	var conn net.Conn
	if secure {
		conn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{ServerName: u.Hostname()})
	} else {
		conn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("mqtt: %w", err)
	}

	id := make([]byte, 6)
	_, _ = rand.Read(id)
	flags := byte(0x02) // clean session
	payload := mqttString("ulanzi-deck-" + hex.EncodeToString(id))
	if s.Username != "" {
		flags |= 0x80
		payload = append(payload, mqttString(s.Username)...)
		if s.Password != "" {
			flags |= 0x40
			payload = append(payload, mqttString(s.Password)...)
		}
	}
	body := append(mqttString("MQTT"), 4, flags)
	body = binary.BigEndian.AppendUint16(body, uint16(mqttKeepAlive/time.Second))
	body = append(body, payload...)

	conn.SetDeadline(time.Now().Add(mqttDialTimeout))
	if _, err := conn.Write(mqttPacket(mqttConnect, 0, body)); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("mqtt: %w", err)
	}
	reader := bufio.NewReader(conn)
	kind, _, ack, err := readPacket(reader)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("mqtt: %w", err)
	}
	if kind != mqttConnAck || len(ack) < 2 {
		conn.Close()
		return nil, nil, errors.New("mqtt: unexpected answer to CONNECT")
	}
	if ack[1] != 0 {
		conn.Close()
		reasons := map[byte]string{1: "protocol version refused", 2: "client id rejected", 3: "server unavailable", 4: "bad user name or password", 5: "not authorized"}
		return nil, nil, fmt.Errorf("mqtt: %s", reasons[ack[1]])
	}
	conn.SetDeadline(time.Time{})
	return conn, reader, nil
}

// CheckMQTT connects once to verify URL and credentials.
func CheckMQTT(s MQTTSecrets) error {
	conn, _, err := dialMQTT(s)
	if err != nil {
		return err
	}
	conn.Write(mqttPacket(mqttDisconnect, 0, nil))
	return conn.Close()
}

func (c *MQTTClient) ensureLocked() error {
	s := c.secrets.Get().MQTT
	key := s.URL + "\x00" + s.Username + "\x00" + s.Password
	if c.conn != nil && c.key == key {
		return nil
	}
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	if time.Since(c.lastDial) < mqttRedial {
		return errors.New("mqtt: not connected")
	}
	c.lastDial = time.Now()
	conn, reader, err := dialMQTT(s)
	if err != nil {
		return err
	}
	c.conn, c.key = conn, key
	go c.readLoop(conn, reader)
	go c.pingLoop(conn)
	for topic := range c.subs {
		if err := c.subscribeLocked(topic); err != nil {
			return err
		}
	}
	return nil
}

func (c *MQTTClient) write(conn net.Conn, packet []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(mqttDialTimeout))
	_, err := conn.Write(packet)
	return err
}

func (c *MQTTClient) dropLocked(conn net.Conn) {
	if c.conn == conn {
		c.conn = nil
	}
	conn.Close()
}

func (c *MQTTClient) readLoop(conn net.Conn, r *bufio.Reader) {
	for {
		conn.SetReadDeadline(time.Now().Add(2 * mqttKeepAlive))
		kind, flags, body, err := readPacket(r)
		if err != nil {
			c.mu.Lock()
			c.dropLocked(conn)
			c.mu.Unlock()
			return
		}
		if kind != mqttPublish || len(body) < 2 {
			continue
		}
		n := int(binary.BigEndian.Uint16(body))
		if len(body) < 2+n {
			continue
		}
		topic, rest := string(body[2:2+n]), body[2+n:]
		if flags&0x06 != 0 && len(rest) >= 2 {
			rest = rest[2:] // packet id of QoS 1/2 messages
		}
		c.mu.Lock()
		c.values[topic] = mqttValue{payload: string(rest), at: time.Now()}
		c.mu.Unlock()
	}
}

func (c *MQTTClient) pingLoop(conn net.Conn) {
	ticker := time.NewTicker(mqttKeepAlive * 2 / 3)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		current := c.conn == conn
		c.mu.Unlock()
		if !current || c.write(conn, mqttPacket(mqttPingReq, 0, nil)) != nil {
			return
		}
	}
}

func (c *MQTTClient) subscribeLocked(topic string) error {
	c.nextID++
	if c.nextID == 0 {
		c.nextID = 1
	}
	body := binary.BigEndian.AppendUint16(nil, c.nextID)
	body = append(body, mqttString(topic)...)
	body = append(body, 0)
	if err := c.write(c.conn, mqttPacket(mqttSubscribe, 0x02, body)); err != nil {
		c.dropLocked(c.conn)
		return fmt.Errorf("mqtt: %w", err)
	}
	return nil
}

// Publish sends payload to topic with QoS 0.
func (c *MQTTClient) Publish(topic, payload string, retain bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLocked(); err != nil {
		return err
	}
	flags := byte(0)
	if retain {
		flags = 0x01
	}
	body := append(mqttString(topic), payload...)
	if err := c.write(c.conn, mqttPacket(mqttPublish, flags, body)); err != nil {
		c.dropLocked(c.conn)
		return fmt.Errorf("mqtt: %w", err)
	}
	return nil
}

// Value returns the last message on topic, subscribing on first use. ok is
// false until a message has arrived.
func (c *MQTTClient) Value(topic string) (payload string, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.subs[topic] {
		c.subs[topic] = true
		if c.conn != nil {
			if err := c.subscribeLocked(topic); err != nil {
				return "", false, err
			}
		}
	}
	if err := c.ensureLocked(); err != nil {
		return "", false, err
	}
	v, ok := c.values[topic]
	return v.payload, ok, nil
}
