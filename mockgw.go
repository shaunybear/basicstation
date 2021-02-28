package basicstation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	lorawan "github.com/shaunybear/lorawango"
)

// MockGW implements basic station functionality for testing
type MockGW struct {
	EUI                   uint64
	TCURI                 string
	Version               Version
	DReq                  interface{}
	DResp                 DiscoveryResponse
	RtrConf               RouterConf
	Log                   zerolog.Logger
	DiscoveryRequestWait  time.Duration
	MuxsVersionWait       time.Duration
	MuxsWriteIdleDuration time.Duration
	conn                  *websocket.Conn
}

// DoDiscovery performs discovery transaction
func (gw *MockGW) DoDiscovery() (err error) {

	uri := gw.TCURI + "/router-info"
	conn, _, err := websocket.DefaultDialer.Dial(uri, nil)
	if err != nil {
		gw.Log.Error().
			Str("service", "discovery").
			Str("tcuri", uri).
			Err(err).
			Msg("DoDiscovery connect failed")
		return
	}
	defer conn.Close()

	// Initialize discovery request
	if gw.DReq == nil {
		eui, _ := lorawan.NewEUI(gw.EUI)
		gw.DReq = map[string]string{"Router": eui.String()}
	}

	// Configure request write delay to test listener read timeout
	if gw.DiscoveryRequestWait != 0 {
		gw.Log.Debug().
			Str("service", "discovery").
			Msgf("Send delay=%v", gw.DiscoveryRequestWait)

		time.Sleep(gw.DiscoveryRequestWait)
	}

	// Send discovery request
	gw.Log.Debug().
		Str("service", "discovery").
		Interface("request", gw.DReq).
		Msg("Sending request")

	if err = conn.WriteJSON(gw.DReq); err != nil {
		gw.Log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("write request failed")

		return
	}

	// Read response
	gw.Log.Debug().
		Str("service", "discovery").
		Msg("Reading response")

	if err = conn.ReadJSON(&gw.DResp); err != nil {
		gw.Log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("Read response failed")

		return
	}

	gw.Log.Debug().
		Str("service", "discovery").
		Str("Router", gw.DResp.Router).
		Str("MUXS", gw.DResp.MUXS).
		Str("URI", gw.DResp.URI).
		Str("Error", gw.DResp.Error).
		Msg("Discovery response")

	return err
}

// DoMuxsConnect performs initial mux connection consisting
// of sending version info and receiving the router configuration
func (gw *MockGW) DoMuxsConnect() (err error) {

	url := gw.DResp.URI

	websocket.DefaultDialer.HandshakeTimeout = 5 * time.Second

	gw.Log.Debug().
		Str("service", "muxs").
		Str("url", url).
		Msg("Dialing network")

	conn, r, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		gw.Log.Error().
			Str("service", "muxs").
			Str("url", url).
			Err(err).
			Msg("Dial failed")

		return err
	}

	if r.StatusCode == http.StatusUnauthorized {
		gw.Log.Error().
			Str("service", "muxs").
			Str("status", r.Status).
			Msg("connect response status != OK")

		return fmt.Errorf(r.Status)
	}

	// Configure version write delay to test listener read timeout
	if gw.MuxsVersionWait != 0 {
		gw.Log.Debug().
			Str("service", "muxs").
			Msgf("Send version delay=%v", gw.MuxsVersionWait)
		time.Sleep(gw.MuxsVersionWait)
	}

	gw.conn = conn

	// Send version
	gw.Log.Debug().
		Str("service", "muxs").
		Msg("Sending version")

	gw.Version.MsgType = "version"
	err = conn.WriteJSON(&gw.Version)
	if err != nil {
		gw.Log.Error().
			Str("service", "muxs").
			Err(err).
			Interface("Version", gw.Version).
			Msg("Encode version")

		return err
	}

	// Read router configuration
	gw.Log.Debug().
		Str("service", "muxs").
		Msg("Read router configuration")

	err = gw.conn.ReadJSON(&gw.RtrConf)
	if err != nil {
		gw.Log.Error().
			Str("service", "muxs").
			Err(err).
			Msg("Read router configuration")

		return err
	}

	gw.Log.Debug().
		Str("service", "muxs").
		Interface("rtrconf", gw.RtrConf).
		Msg("Received router configuration")

	if gw.MuxsWriteIdleDuration != 0 {
		gw.Log.Debug().
			Str("service", "muxs").
			Msgf("Muxs write idle duration=%s", gw.MuxsWriteIdleDuration)
		time.Sleep(gw.MuxsWriteIdleDuration)
	}

	gw.Log.Debug().
		Str("service", "muxs").
		Msg("Done")

	return err
}

// ReadLoop reads messages from the endpoint connection
func (gw *MockGW) ReadLoop(ctx context.Context, rxChan chan []byte) error {
	var err error

	go func() {
		for {
			_, message, err := gw.conn.ReadMessage()
			if err != nil {
				return
			}
			rxChan <- message
		}
	}()

	select {
	case <-ctx.Done():
		err = ctx.Err()
		break
	}

	return err
}
