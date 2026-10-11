package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeTeams accepts one client, reports a meeting and hands out a token
// after the first action, like Teams does after the user allows pairing.
func fakeTeams(t *testing.T, actions *[]string, tokens *[]string, mu *sync.Mutex) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*tokens = append(*tokens, r.URL.Query().Get("token"))
		mu.Unlock()
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := context.Background()
		conn.Write(ctx, websocket.MessageText, []byte(`{"meetingUpdate":{"meetingState":{"isInMeeting":true,"isMuted":true,"isVideoOn":false}}}`))
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Action string `json:"action"`
			}
			json.Unmarshal(data, &msg)
			mu.Lock()
			*actions = append(*actions, msg.Action)
			mu.Unlock()
			conn.Write(ctx, websocket.MessageText, []byte(`{"tokenRefresh":"secret-token"}`))
			conn.Write(ctx, websocket.MessageText, []byte(`{"meetingUpdate":{"meetingState":{"isInMeeting":true,"isMuted":false}}}`))
		}
	}))
	t.Cleanup(srv.Close)
	old := teamsURL
	teamsURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	t.Cleanup(func() { teamsURL = old })
}

func TestTeamsPairingStateAndActions(t *testing.T) {
	var mu sync.Mutex
	var actions, tokens []string
	fakeTeams(t, &actions, &tokens, &mu)
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.yaml"))
	client := NewTeamsClient(store, "test")

	waitFor := func(cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatal("condition not reached")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(func() bool { st, _ := client.State(); return st.InMeeting && st.Muted })

	if err := client.Run("mute"); err != nil {
		t.Fatal(err)
	}
	waitFor(func() bool { st, _ := client.State(); return !st.Muted })
	waitFor(func() bool { return store.Get().Teams.Token == "secret-token" })
	mu.Lock()
	defer mu.Unlock()
	if len(actions) != 1 || actions[0] != "toggle-mute" || tokens[0] != "" {
		t.Fatalf("actions %v, tokens %v", actions, tokens)
	}
	if err := client.Run("dance"); err == nil {
		t.Fatal("unknown op must fail")
	}
}

func TestTeamsUnavailable(t *testing.T) {
	old := teamsURL
	teamsURL = "ws://127.0.0.1:1"
	defer func() { teamsURL = old }()
	client := NewTeamsClient(NewSecretStore(filepath.Join(t.TempDir(), "s.yaml")), "test")
	if _, err := client.State(); err != ErrTeamsUnavailable {
		t.Fatalf("got %v", err)
	}
}
