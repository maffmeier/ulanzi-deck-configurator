package integrations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
)

func TestHomeAssistantServiceAndState(t *testing.T) {
	var mu sync.Mutex
	var called []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/services/light/toggle":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			called = append(called, body["entity_id"])
			mu.Unlock()
			w.Write([]byte("[]"))
		case r.URL.Path == "/api/states/sensor.temp":
			w.Write([]byte(`{"state":"21.5","attributes":{"unit_of_measurement":"°C","friendly_name":"Wohnzimmer"}}`))
		case r.URL.Path == "/api/":
			w.Write([]byte(`{"message":"API running."}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	s := HomeAssistantSecrets{URL: srv.URL + "/", Token: "tok"}

	if err := CallService(s, "light.toggle", "light.kitchen"); err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 || called[0] != "light.kitchen" {
		t.Fatalf("called %v", called)
	}
	st, err := State(s, "sensor.temp")
	if err != nil || st.State != "21.5" || st.Unit != "°C" || st.FriendlyName != "Wohnzimmer" {
		t.Fatalf("state %+v %v", st, err)
	}
	if err := CheckHomeAssistant(s); err != nil {
		t.Fatal(err)
	}
	if err := CheckHomeAssistant(HomeAssistantSecrets{URL: srv.URL, Token: "wrong"}); err == nil {
		t.Fatal("bad token must fail")
	}
	if err := CallService(HomeAssistantSecrets{}, "light.toggle", "x"); err == nil {
		t.Fatal("missing config must fail")
	}
}

// fakeOBS implements the obs-websocket v5 handshake with authentication.
func fakeOBS(t *testing.T, password string, requests *[]string, mu *sync.Mutex) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"obswebsocket.json"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := context.Background()
		_ = writeJSON(ctx, conn, obsOpHello, map[string]any{
			"rpcVersion": 1, "authentication": map[string]string{"challenge": "ch", "salt": "sa"},
		})
		var identify obsMessage
		if readJSON(ctx, conn, &identify) != nil {
			return
		}
		var id struct {
			Authentication string `json:"authentication"`
		}
		_ = json.Unmarshal(identify.D, &id)
		if id.Authentication != obsAuth(password, "sa", "ch") {
			conn.Close(4009, "auth failed")
			return
		}
		_ = writeJSON(ctx, conn, obsOpIdentified, map[string]any{"negotiatedRpcVersion": 1})
		for {
			var msg obsMessage
			if readJSON(ctx, conn, &msg) != nil {
				return
			}
			var req struct {
				Type string         `json:"requestType"`
				ID   string         `json:"requestId"`
				Data map[string]any `json:"requestData"`
			}
			_ = json.Unmarshal(msg.D, &req)
			mu.Lock()
			*requests = append(*requests, req.Type)
			mu.Unlock()
			resp := map[string]any{"requestType": req.Type, "requestId": req.ID, "requestStatus": map[string]any{"result": true, "code": 100}}
			switch req.Type {
			case "GetCurrentProgramScene":
				resp["responseData"] = map[string]any{"currentProgramSceneName": "Kamera"}
			case "GetRecordStatus":
				resp["responseData"] = map[string]any{"outputActive": true, "outputTimecode": "00:01:02.345"}
			case "GetStreamStatus":
				resp["responseData"] = map[string]any{"outputActive": false, "outputTimecode": "00:00:00.000"}
			case "SetCurrentProgramScene":
				if req.Data["sceneName"] == "" {
					resp["requestStatus"] = map[string]any{"result": false, "code": 600, "comment": "missing"}
				}
			}
			_ = writeJSON(ctx, conn, obsOpResponse, resp)
		}
	}))
}

func TestOBSAuthRequestsAndStatus(t *testing.T) {
	var mu sync.Mutex
	var requests []string
	srv := fakeOBS(t, "geheim", &requests, &mu)
	defer srv.Close()
	s := OBSSecrets{URL: "ws" + strings.TrimPrefix(srv.URL, "http"), Password: "geheim"}
	c := NewOBSClient()

	if err := c.Run(s, "switch_scene", "Kamera"); err != nil {
		t.Fatal(err)
	}
	if err := c.Run(s, "toggle_record", ""); err != nil {
		t.Fatal(err)
	}
	st, err := c.Status(s)
	if err != nil || st.Scene != "Kamera" || !st.Recording || st.RecordTime != "00:01:02" || st.Streaming {
		t.Fatalf("status %+v %v", st, err)
	}
	mu.Lock()
	got := strings.Join(requests, ",")
	mu.Unlock()
	if !strings.HasPrefix(got, "SetCurrentProgramScene,ToggleRecord,GetCurrentProgramScene") {
		t.Fatalf("requests %s", got)
	}

	bad := NewOBSClient()
	if err := bad.Run(OBSSecrets{URL: s.URL, Password: "falsch"}, "toggle_record", ""); err == nil {
		t.Fatal("wrong password must fail")
	}
}

func TestSecretStoreReloadsAndProtectsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.yaml")
	store := NewSecretStore(path)
	if store.Get().HomeAssistant.Token != "" {
		t.Fatal("missing file must give empty secrets")
	}
	if err := store.Save(Secrets{HomeAssistant: HomeAssistantSecrets{URL: "http://ha", Token: "t1"}}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	if store.Get().HomeAssistant.Token != "t1" {
		t.Fatal("not loaded")
	}
}
