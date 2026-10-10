package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// OBS speaks obs-websocket v5 (built into OBS 28+).
const (
	obsOpHello      = 0
	obsOpIdentify   = 1
	obsOpIdentified = 2
	obsOpRequest    = 6
	obsOpResponse   = 7
	obsDefaultURL   = "ws://localhost:4455"
	obsTimeout      = 5 * time.Second
)

type obsMessage struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type obsResponse struct {
	RequestID     string `json:"requestId"`
	RequestStatus struct {
		Result  bool   `json:"result"`
		Code    int    `json:"code"`
		Comment string `json:"comment"`
	} `json:"requestStatus"`
	ResponseData json.RawMessage `json:"responseData"`
}

// OBSClient keeps one connection and reconnects on the next request after
// a failure.
type OBSClient struct {
	mu      sync.Mutex
	conn    *websocket.Conn
	key     string
	nextID  int
	pending map[string]chan obsResponse
	pendMu  sync.Mutex
}

func NewOBSClient() *OBSClient {
	return &OBSClient{pending: map[string]chan obsResponse{}}
}

func obsAuth(password, salt, challenge string) string {
	secret := sha256.Sum256([]byte(password + salt))
	secretB64 := base64.StdEncoding.EncodeToString(secret[:])
	auth := sha256.Sum256([]byte(secretB64 + challenge))
	return base64.StdEncoding.EncodeToString(auth[:])
}

func (c *OBSClient) connectLocked(s OBSSecrets) error {
	key := s.URL + "\x00" + s.Password
	if c.conn != nil && c.key == key {
		return nil
	}
	c.closeLocked()
	target := s.URL
	if target == "" {
		target = obsDefaultURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), obsTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, target, &websocket.DialOptions{Subprotocols: []string{"obswebsocket.json"}})
	if err != nil {
		return fmt.Errorf("obs: %w", err)
	}
	conn.SetReadLimit(1 << 20)

	var hello obsMessage
	if err := readJSON(ctx, conn, &hello); err != nil || hello.Op != obsOpHello {
		conn.CloseNow()
		return fmt.Errorf("obs: no hello: %v", err)
	}
	var helloData struct {
		RPCVersion     int `json:"rpcVersion"`
		Authentication *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	_ = json.Unmarshal(hello.D, &helloData)
	identify := map[string]any{"rpcVersion": 1, "eventSubscriptions": 0}
	if a := helloData.Authentication; a != nil {
		if s.Password == "" {
			conn.CloseNow()
			return errors.New("obs: password required (secrets.yaml)")
		}
		identify["authentication"] = obsAuth(s.Password, a.Salt, a.Challenge)
	}
	if err := writeJSON(ctx, conn, obsOpIdentify, identify); err != nil {
		conn.CloseNow()
		return err
	}
	var identified obsMessage
	if err := readJSON(ctx, conn, &identified); err != nil || identified.Op != obsOpIdentified {
		conn.CloseNow()
		return errors.New("obs: authentication failed")
	}
	c.conn, c.key = conn, key
	go c.readLoop(conn)
	return nil
}

func readJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(ctx context.Context, conn *websocket.Conn, op int, d any) error {
	data, err := json.Marshal(map[string]any{"op": op, "d": d})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func (c *OBSClient) readLoop(conn *websocket.Conn) {
	for {
		var msg obsMessage
		if err := readJSON(context.Background(), conn, &msg); err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
			}
			c.mu.Unlock()
			c.failPending()
			return
		}
		if msg.Op != obsOpResponse {
			continue
		}
		var resp obsResponse
		if json.Unmarshal(msg.D, &resp) != nil {
			continue
		}
		c.pendMu.Lock()
		ch := c.pending[resp.RequestID]
		delete(c.pending, resp.RequestID)
		c.pendMu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (c *OBSClient) failPending() {
	c.pendMu.Lock()
	defer c.pendMu.Unlock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

func (c *OBSClient) closeLocked() {
	if c.conn != nil {
		c.conn.Close(websocket.StatusNormalClosure, "")
		c.conn = nil
	}
}

// Request sends one obs-websocket request and returns its responseData.
func (c *OBSClient) Request(s OBSSecrets, requestType string, data map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	if err := c.connectLocked(s); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := strconv.Itoa(c.nextID)
	conn := c.conn
	ch := make(chan obsResponse, 1)
	c.pendMu.Lock()
	c.pending[id] = ch
	c.pendMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), obsTimeout)
	defer cancel()
	payload := map[string]any{"requestType": requestType, "requestId": id}
	if data != nil {
		payload["requestData"] = data
	}
	err := writeJSON(ctx, conn, obsOpRequest, payload)
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, errors.New("obs: connection lost")
		}
		if !resp.RequestStatus.Result {
			return nil, fmt.Errorf("obs %s: %s (code %d)", requestType, resp.RequestStatus.Comment, resp.RequestStatus.Code)
		}
		return resp.ResponseData, nil
	case <-ctx.Done():
		c.pendMu.Lock()
		delete(c.pending, id)
		c.pendMu.Unlock()
		return nil, errors.New("obs: request timed out")
	}
}

var obsRequests = map[string]string{
	"switch_scene":         "SetCurrentProgramScene",
	"toggle_record":        "ToggleRecord",
	"start_record":         "StartRecord",
	"stop_record":          "StopRecord",
	"toggle_stream":        "ToggleStream",
	"start_stream":         "StartStream",
	"stop_stream":          "StopStream",
	"toggle_mute":          "ToggleInputMute",
	"toggle_replay_buffer": "ToggleReplayBuffer",
	"save_replay":          "SaveReplayBuffer",
}

// Run executes a deck obs action.
func (c *OBSClient) Run(s OBSSecrets, op, target string) error {
	requestType, ok := obsRequests[op]
	if !ok {
		return fmt.Errorf("unknown obs op %q", op)
	}
	var data map[string]any
	switch op {
	case "switch_scene":
		data = map[string]any{"sceneName": target}
	case "toggle_mute":
		data = map[string]any{"inputName": target}
	}
	_, err := c.Request(s, requestType, data)
	return err
}

// OBSStatus is what the obs widget shows.
type OBSStatus struct {
	Scene      string
	Recording  bool
	RecordTime string
	Streaming  bool
	StreamTime string
}

func (c *OBSClient) Status(s OBSSecrets) (OBSStatus, error) {
	var st OBSStatus
	data, err := c.Request(s, "GetCurrentProgramScene", nil)
	if err != nil {
		return st, err
	}
	var scene struct {
		Name    string `json:"currentProgramSceneName"`
		NameNew string `json:"sceneName"`
	}
	_ = json.Unmarshal(data, &scene)
	st.Scene = scene.Name
	if st.Scene == "" {
		st.Scene = scene.NameNew
	}
	var output struct {
		Active   bool   `json:"outputActive"`
		Timecode string `json:"outputTimecode"`
	}
	if data, err := c.Request(s, "GetRecordStatus", nil); err == nil && json.Unmarshal(data, &output) == nil {
		st.Recording, st.RecordTime = output.Active, trimTimecode(output.Timecode)
	}
	output.Active, output.Timecode = false, ""
	if data, err := c.Request(s, "GetStreamStatus", nil); err == nil && json.Unmarshal(data, &output) == nil {
		st.Streaming, st.StreamTime = output.Active, trimTimecode(output.Timecode)
	}
	return st, nil
}

// trimTimecode turns "00:12:34.567" into "00:12:34".
func trimTimecode(tc string) string {
	if len(tc) >= 8 {
		return tc[:8]
	}
	return tc
}
