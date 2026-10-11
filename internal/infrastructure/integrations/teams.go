package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Microsoft Teams (the new client) offers a local "third-party app API" on
// port 8124 once it is enabled under Settings → Privacy. The first action
// sent during a meeting asks the user to allow the device; Teams then sends
// a token, which is kept in secrets.yaml for later connections.
const (
	teamsDialTimeout = 2 * time.Second
	teamsRedial      = 3 * time.Second
)

// teamsURL is a variable so tests can point it at a fake server.
var teamsURL = "ws://127.0.0.1:8124"

var ErrTeamsUnavailable = errors.New("teams: local API not reachable (enable it in Teams under Settings → Privacy → Third-party app API)")

var teamsActions = map[string]string{
	"mute":   "toggle-mute",
	"camera": "toggle-video",
	"hand":   "toggle-hand",
	"blur":   "toggle-background-blur",
	"leave":  "leave-call",
}

// TeamsState is the meeting state Teams pushes on every change.
type TeamsState struct {
	InMeeting  bool `json:"isInMeeting"`
	Muted      bool `json:"isMuted"`
	VideoOn    bool `json:"isVideoOn"`
	HandRaised bool `json:"isHandRaised"`
	Recording  bool `json:"isRecordingOn"`
	Blurred    bool `json:"isBackgroundBlurred"`
	Sharing    bool `json:"isSharing"`
}

type teamsMessage struct {
	TokenRefresh  string `json:"tokenRefresh"`
	ErrorMsg      string `json:"errorMsg"`
	MeetingUpdate *struct {
		MeetingState TeamsState `json:"meetingState"`
	} `json:"meetingUpdate"`
}

// TeamsClient keeps one connection and redials lazily when it is used.
type TeamsClient struct {
	secrets *SecretStore
	version string

	mu       sync.Mutex
	conn     *websocket.Conn
	state    TeamsState
	lastDial time.Time
	nextID   int
}

func NewTeamsClient(secrets *SecretStore, version string) *TeamsClient {
	return &TeamsClient{secrets: secrets, version: version}
}

func teamsDialURL(token, version string) string {
	q := url.Values{}
	q.Set("protocol-version", "2.0.0")
	q.Set("manufacturer", "maffmeier")
	q.Set("device", "Ulanzi D200")
	q.Set("app", "ulanzi-deck")
	q.Set("app-version", version)
	if token != "" {
		q.Set("token", token)
	}
	return teamsURL + "?" + q.Encode()
}

func (c *TeamsClient) ensureLocked() error {
	if c.conn != nil {
		return nil
	}
	if time.Since(c.lastDial) < teamsRedial {
		return ErrTeamsUnavailable
	}
	c.lastDial = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), teamsDialTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, teamsDialURL(c.secrets.Get().Teams.Token, c.version), nil)
	if err != nil {
		return ErrTeamsUnavailable
	}
	c.conn = conn
	c.state = TeamsState{}
	go c.readLoop(conn)
	return nil
}

func (c *TeamsClient) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
				c.state = TeamsState{}
			}
			c.mu.Unlock()
			return
		}
		var msg teamsMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		if msg.TokenRefresh != "" {
			c.saveToken(msg.TokenRefresh)
		}
		if msg.MeetingUpdate != nil {
			c.mu.Lock()
			c.state = msg.MeetingUpdate.MeetingState
			c.mu.Unlock()
		}
	}
}

func (c *TeamsClient) saveToken(token string) {
	s := c.secrets.Get()
	if s.Teams.Token == token {
		return
	}
	s.Teams.Token = token
	_ = c.secrets.Save(s)
}

// Run sends a meeting op (mute, camera, hand, blur, leave).
func (c *TeamsClient) Run(op string) error {
	action, ok := teamsActions[op]
	if !ok {
		return fmt.Errorf("teams: unknown op %q", op)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLocked(); err != nil {
		return err
	}
	c.nextID++
	msg, _ := json.Marshal(map[string]any{"action": action, "parameters": map[string]any{}, "requestId": c.nextID})
	ctx, cancel := context.WithTimeout(context.Background(), teamsDialTimeout)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
		c.conn.CloseNow()
		c.conn = nil
		return fmt.Errorf("teams: %w", err)
	}
	return nil
}

// State returns the last meeting state Teams reported.
func (c *TeamsClient) State() (TeamsState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureLocked(); err != nil {
		return TeamsState{}, err
	}
	return c.state, nil
}

// CheckTeams tells whether the local API answers.
func CheckTeams(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), teamsDialTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, teamsDialURL(token, "check"), nil)
	if err != nil {
		return ErrTeamsUnavailable
	}
	conn.Close(websocket.StatusNormalClosure, "")
	return nil
}
