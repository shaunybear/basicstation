package basicstation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// Repository interface ..

type testStation struct {
	conf      RouterConf
	discovery DiscoveryResponse
	version   Version
}

func (stn testStation) GetRouterConf() (RouterConf, error) {
	return stn.conf, nil

}

func (stn testStation) GetDiscoveryResponse() (DiscoveryResponse, error) {
	return stn.discovery, nil

}

func (stn testStation) SetVersion(version Version) {
	stn.version = version
}

type testRepo struct {
	station testStation
}

func (repo testRepo) GetStation(uint64) (Station, bool) {
	return repo.station, true

}

func TestDiscoveryHandler(t *testing.T) {

	tcs := []struct {
		name    string
		station testStation
		message map[string]interface{}
		reply   DiscoveryResponse
	}{
		{
			name:    "with good request containing router as number",
			message: map[string]interface{}{"router": 1},
			station: testStation{},
			reply: DiscoveryResponse{
				URI:  "ws://discovery-test.com:8080/0000000000000001",
				MUXS: "TBD",
			},
		},
	}

	for _, tt := range tcs {
		t.Run(tt.name, func(t *testing.T) {

			// Initialize test response
			tr := testRepo{station: tt.station}
			tr.station.discovery = tt.reply

			// test environment
			env := &Environment{Repo: tr}

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

			tr := testRepo{}
			env := &Environment{Repo: &tr}
			h := StationHandler{Env: env}

			tr.station.conf = tt.wantConf

			s, ws := newStationWSServer(t, tt.eui, h)
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
