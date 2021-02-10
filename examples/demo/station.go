// +build ignore
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/shaunybear/basicstation"
)

// ForwarderStub implements basic station functionality for testing
type station struct {
	eui                   string
	tcuri                 string
	version               basicstation.Version
	discoveryReq          interface{}
	discoveryResp         basicstation.DiscoveryResponse
	RtrConf               basicstation.RouterConf
	log                   zerolog.Logger
	discoveryRequestWait  time.Duration
	muxsVersionWait       time.Duration
	muxsWriteIdleDuration time.Duration
	conn                  *websocket.Conn
}

// DoDiscovery performs discovery transaction
func (stn *station) DoDiscovery() (err error) {

	uri := stn.tcuri + "/router-info"
	conn, _, err := websocket.DefaultDialer.Dial(uri, nil)
	if err != nil {
		stn.log.Error().
			Str("service", "discovery").
			Str("tcuri", uri).
			Err(err).
			Msg("DoDiscovery connect failed")
		return
	}
	defer conn.Close()

	// Initialize discovery request
	if stn.discoveryReq == nil {
		stn.discoveryReq = map[string]string{"Router": stn.eui}
	}

	// Configure request write delay to test listener read timeout
	if stn.discoveryRequestWait != 0 {
		stn.log.Debug().
			Str("service", "discovery").
			Msgf("Send delay=%v", stn.discoveryRequestWait)

		time.Sleep(stn.discoveryRequestWait)
	}

	// Send discovery request
	stn.log.Debug().
		Str("service", "discovery").
		Interface("request", stn.discoveryReq).
		Msg("Sending request")

	if err = conn.WriteJSON(stn.discoveryReq); err != nil {
		stn.log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("write request failed")

		return
	}

	// Read response
	stn.log.Debug().
		Str("service", "discovery").
		Msg("Reading response")

	if err = conn.ReadJSON(&stn.discoveryResp); err != nil {
		stn.log.Error().
			Str("service", "discovery").
			Err(err).
			Msg("Read response failed")

		return
	}

	stn.log.Debug().
		Str("service", "discovery").
		Str("Router", stn.discoveryResp.Router).
		Str("MUXS", stn.discoveryResp.MUXS).
		Str("URI", stn.discoveryResp.URI).
		Str("Error", stn.discoveryResp.Error).
		Msg("Discovery response")

	return err
}

// DoMuxsConnect performs initial mux connection consisting
// of sending version info and receiving the router configuration
func (stn *station) DoMuxsConnect() (err error) {

	url := stn.discoveryResp.URI

	websocket.DefaultDialer.HandshakeTimeout = 5 * time.Second

	stn.log.Debug().
		Str("service", "muxs").
		Str("url", url).
		Msg("Dialing network")

	conn, r, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		stn.log.Error().
			Str("service", "muxs").
			Str("url", url).
			Err(err).
			Msg("Dial failed")

		return err
	}

	if r.StatusCode == http.StatusUnauthorized {
		stn.log.Error().
			Str("service", "muxs").
			Str("status", r.Status).
			Msg("connect response status != OK")

		return fmt.Errorf(r.Status)
	}

	// Configure version write delay to test listener read timeout
	if stn.muxsVersionWait != 0 {
		stn.log.Debug().
			Str("service", "muxs").
			Msgf("Send version delay=%v", stn.muxsVersionWait)
		time.Sleep(stn.muxsVersionWait)
	}

	stn.conn = conn

	// Send version
	stn.log.Debug().
		Str("service", "muxs").
		Msg("Sending version")

	stn.version.MsgType = "version"
	err = conn.WriteJSON(&stn.version)
	if err != nil {
		stn.log.Error().
			Str("service", "muxs").
			Err(err).
			Interface("Version", stn.version).
			Msg("Encode version")

		return err
	}

	// Read router configuration
	stn.log.Debug().
		Str("service", "muxs").
		Msg("Read router configuration")

	err = stn.conn.ReadJSON(&stn.RtrConf)
	if err != nil {
		stn.log.Error().
			Str("service", "muxs").
			Err(err).
			Msg("Read router configuration")

		return err
	}

	stn.log.Debug().
		Str("service", "muxs").
		Interface("rtrconf", stn.RtrConf).
		Msg("Received router configuration")

	if stn.muxsWriteIdleDuration != 0 {
		stn.log.Debug().
			Str("service", "muxs").
			Msgf("Muxs write idle duration=%s", stn.muxsWriteIdleDuration)
		time.Sleep(stn.muxsWriteIdleDuration)
	}

	stn.log.Debug().
		Str("service", "muxs").
		Msg("Done")

	return err
}

// ReadLoop reads messages from the endpoint connection
func (stn *station) ReadLoop(ctx context.Context, rxChan chan []byte) error {
	var err error

	go func() {
		for {
			_, message, err := stn.conn.ReadMessage()
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

func main() {

	port := 8080
	flag.IntVar(&port, "port", port, "discovery port")
	flag.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	stn := &station{
		eui:   "1",
		tcuri: fmt.Sprintf("ws://127.0.0.1:%d", port),
		log:   zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger(),
	}

	err := stn.DoDiscovery()
	if err != nil {
		return
	}

	// Connect
	err = stn.DoMuxsConnect()
	if err != nil {
		return
	}

	// Read Loop
	rxChan := make(chan []byte)
	ctx, cancel := context.WithCancel(context.Background())

	go stn.ReadLoop(ctx, rxChan)

	go func() {
		for {
			select {
			case b := <-rxChan:
				msg := map[string]interface{}{}
				if err := json.Unmarshal(b, &msg); err != nil {
					stn.log.Error().
						Err(err).
						Msg("parse received data error")
					continue
				}

				stn.log.Debug().Msgf("received %#v", msg)
				break
			case <-ctx.Done():
				return
			}
		}
	}()

	// trap ctrl-c
	ctrlc := make(chan os.Signal, 1)
	signal.Notify(ctrlc, os.Interrupt)
	defer func() {
		signal.Stop(ctrlc)
	}()

	<-ctrlc
	cancel()
}
