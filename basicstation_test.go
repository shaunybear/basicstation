package basicstation

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Repository interface ..

type testServer struct {
	conf      RouterConf
	discovery DiscoveryResponse
	version   Version
}

func (s testServer) GetRouterConf(gw *Gateway) error {
	gw.RouterConf = s.conf
	return nil
}

func (s testServer) GetDiscoveryResponse(eui uint64, r *http.Request) (DiscoveryResponse, error) {
	return s.discovery, nil
}

func (s testServer) Error(eui uint64, err error, msg string) {

}

func (s testServer) Debug(eui uint64, msg string, err error) {

}

func (s testServer) NewConnection(gw *Gateway) {
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	gw.Run(ctx, s, s)
}

func (s testServer) Receive(gw *Gateway, msg interface{}) {
}

func (s testServer) SetVersion(eui uint64, v Version) {
	s.version = v
}

func (s testServer) Write(m interface{}) {
}

func TestDiscoveryHandler(t *testing.T) {

	tcs := []struct {
		name    string
		message map[string]interface{}
		reply   DiscoveryResponse
	}{
		{
			name:    "with good request containing router as number",
			message: map[string]interface{}{"router": 1},
			reply: DiscoveryResponse{
				URI:  "ws://discovery-test.com:8080/0000000000000001",
				MUXS: "TBD",
			},
		},
	}

	for _, tt := range tcs {
		t.Run(tt.name, func(t *testing.T) {

			// Initialize test response
			ts := testServer{}
			ts.discovery = tt.reply

			// test environment
			env := &Environment{Server: ts}

			// discovery handler
			h := DiscoveryHandler{Env: env}

			// start test server
			s, ws := newDiscoveryWSServer(t, h)
			defer s.Close()
			defer ws.Close()

			sendMessage(t, ws, tt.message)

			var reply DiscoveryResponse
			receiveWSMessage(t, ws, &reply)

			if !reflect.DeepEqual(reply, tt.reply) {
				t.Fatalf("Expected '%+v', got '%+v'", tt.reply, reply)
			}
		})
	}
}

func TestStationRouterConf(t *testing.T) {

	defVer := map[string]interface{}{
		"station":  "testStation",
		"firmware": "test-fw",
		"package":  "test-package",
		"model":    "test-model",
		"protocol": 2,
		"features": "rsh",
		"msgtype":  "version",
	}

	tcs := []struct {
		name     string
		eui      string
		message  map[string]interface{}
		wantConf RouterConf
	}{
		{
			name:     "with good version",
			eui:      "0000000000000001",
			message:  defVer,
			wantConf: newRouterConf(),
		},
	}

	for _, tt := range tcs {
		t.Run(tt.name, func(t *testing.T) {

			ts := testServer{}
			env := &Environment{Server: &ts}

			ts.conf = tt.wantConf

			gh := GatewayHandler{Env: env}

			s, ws := newStationWSServer(t, tt.eui, gh)
			defer s.Close()
			defer ws.Close()

			sendMessage(t, ws, tt.message)

			var gotConf RouterConf
			receiveWSMessage(t, ws, &gotConf)

			if !reflect.DeepEqual(gotConf, tt.wantConf) {
				t.Fatalf("Expected '%+v', got '%+v'", tt.wantConf, gotConf)
			}
		})
	}
}

func TestUplink(t *testing.T) {

	// DevAddr is encoded as an int32, check mapstructure does not error on negative values
	devaddrs := []int32{-1, 100}
	m := map[string]interface{}{
		"msgtype": "updf",
	}

	for _, wantDevAddr := range devaddrs {
		m["DevAddr"] = wantDevAddr

		b := bytes.Buffer{}
		enc := json.NewEncoder(&b)
		enc.Encode(&m)

		value, err := decode(&b)
		if err != nil {
			t.Errorf("Decode uplink devaddr=%d failed", wantDevAddr)
		}

		gotDevAddr := value.(Uplink).DevAddr
		if gotDevAddr != wantDevAddr {
			t.Errorf("uplink devAddr got=%d, want=%d", gotDevAddr, wantDevAddr)
		}
	}

}

func newDiscoveryWSServer(t *testing.T, h http.Handler) (*httptest.Server, *websocket.Conn) {
	t.Helper()

	s := httptest.NewServer(h)
	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	return s, ws
}

func newStationWSServer(t *testing.T, eui string, h http.Handler) (*httptest.Server, *websocket.Conn) {
	t.Helper()

	mux := mux.NewRouter()
	mux.Handle("/{eui}", h)

	s := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(s.URL, "http") + "/" + eui

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	return s, ws
}

func sendMessage(t *testing.T, ws *websocket.Conn, msg interface{}) {
	t.Helper()

	m, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	if err := ws.WriteMessage(websocket.TextMessage, m); err != nil {
		t.Fatal(err)
	}
}

func receiveWSMessage(t *testing.T, ws *websocket.Conn, reply interface{}) {
	t.Helper()

	_, m, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	err = json.Unmarshal(m, reply)
	if err != nil {
		t.Fatal(err)
	}
}

func newRouterConf() RouterConf {
	return RouterConf{
		MessageType: "router_conf",
		Region:      "US902",
	}
}
