package basicstation

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// ForwarderStub implements basic station functionality for testing
type ForwarderStub struct {
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
func (f *ForwarderStub) DoDiscovery() (err error) {

	uri := f.TCURI + "/router-info"
	conn, _, err := websocket.DefaultDialer.Dial(uri, nil)
	if err != nil {
		f.Log.Error().
			Str("service", "discovery").
			Str("tcuri", uri).
			Err(err).
			Msg("DoDiscovery connect failed")
		return
	}
	defer conn.Close()

	// Initialize discovery request
	if f.DReq == nil {
		f.DReq = map[string]string{"Router": toString(f.EUI)}
	}

	// Configure request write delay to test listener read timeout
	if f.DiscoveryRequestWait != 0 {
		f.Log.Debug().
			Str("service", "discovery").
			Msgf("Send delay=%v", f.DiscoveryRequestWait)

		time.Sleep(f.DiscoveryRequestWait)
	}

	// Send discovery request
	f.Log.Debug().
		Str("service", "discovery").
		Interface("request", f.DReq).
		Msg("Sending request")

	if err = conn.WriteJSON(f.DReq); err != nil {
		f.Log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("write request failed")

		return
	}

	// Read response
	f.Log.Debug().
		Str("service", "discovery").
		Msg("Reading response")

	if err = conn.ReadJSON(&f.DResp); err != nil {
		f.Log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("Read response failed")

		return
	}

	f.Log.Debug().
		Str("service", "discovery").
		Str("Router", f.DResp.Router).
		Str("MUXS", f.DResp.MUXS).
		Str("URI", f.DResp.URI).
		Str("Error", f.DResp.Error).
		Msg("Discovery response")

	return err
}

// DoMuxsConnect performs initial mux connection consisting
// of sending version info and receiving the router configuration
func (f *ForwarderStub) DoMuxsConnect() (err error) {

	url := f.DResp.URI

	websocket.DefaultDialer.HandshakeTimeout = 5 * time.Second

	f.Log.Debug().
		Str("service", "muxs").
		Str("url", url).
		Msg("Dialing network")

	conn, r, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		f.Log.Error().
			Str("service", "muxs").
			Str("url", url).
			Err(err).
			Msg("Dial failed")

		return err
	}

	if r.StatusCode == http.StatusUnauthorized {
		f.Log.Error().
			Str("service", "muxs").
			Str("status", r.Status).
			Msg("connect response status != OK")

		return fmt.Errorf(r.Status)
	}

	// Configure version write delay to test listener read timeout
	if f.MuxsVersionWait != 0 {
		f.Log.Debug().
			Str("service", "muxs").
			Msgf("Send version delay=%v", f.MuxsVersionWait)
		time.Sleep(f.MuxsVersionWait)
	}

	f.conn = conn

	// Send version
	f.Log.Debug().
		Str("service", "muxs").
		Msg("Sending version")

	f.Version.MsgType = "version"
	err = conn.WriteJSON(&f.Version)
	if err != nil {
		f.Log.Error().
			Str("service", "muxs").
			Err(err).
			Interface("Version", f.Version).
			Msg("Encode version")

		return err
	}

	// Read router configuration
	f.Log.Debug().
		Str("service", "muxs").
		Msg("Read router configuration")

	err = f.conn.ReadJSON(&f.RtrConf)
	if err != nil {
		f.Log.Error().
			Str("service", "muxs").
			Err(err).
			Msg("Read router configuration")

		return err
	}

	f.Log.Debug().
		Str("service", "muxs").
		Interface("rtrconf", f.RtrConf).
		Msg("Received router configuration")

	if f.MuxsWriteIdleDuration != 0 {
		f.Log.Debug().
			Str("service", "muxs").
			Msgf("Muxs write idle duration=%s", f.MuxsWriteIdleDuration)
		time.Sleep(f.MuxsWriteIdleDuration)
	}

	f.Log.Debug().
		Str("service", "muxs").
		Msg("Done")

	return err
}

// ReadLoop reads messages from the endpoint connection
func (f *ForwarderStub) ReadLoop(ctx context.Context, rxChan chan []byte) error {
	var err error

	go func() {
		for {
			_, message, err := f.conn.ReadMessage()
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
