package basicstation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	lorawan "github.com/shaunybear/lorawango"
)

// Stats provides some basic statistics
type Stats struct {
	DecodeErrors     uint
	RecvTextMsg      uint
	RecvBinaryMsg    uint
	WriteNoConnError uint
	WriteTextOk      uint
	WriteTextError   uint
}

// Gateway will be the next gateway interface
type Gateway struct {
	EUI        lorawan.EUI
	Name       string
	conn       *websocket.Conn
	Version    Version
	RouterConf RouterConf
	Stats      Stats
}

// Server is anything that implements gateway server interface
type Server interface {
	NewConnection(gw *Gateway)
	Receive(gw *Gateway, msg interface{})
	GetRouterConf(gw *Gateway) error
	GetDiscoveryResponse(eui uint64, r *http.Request) (DiscoveryResponse, error)
}

// Run ...
func (gw *Gateway) Run(ctx context.Context, server Server, log zerolog.Logger) error {
	var err error

	// Close the connection on exit
	defer gw.conn.Close()

	// First message from the gateway is it's version information
	if err = gw.readVersion(ctx); err != nil {
		log.Debug().
			Str("eui", gw.Name).
			Err(err).
			Msg("read version failed")

		return err
	}

	// Get router configuration from the server
	if err = server.GetRouterConf(gw); err != nil {
		return err
	}

	// Send config to the gateway
	outbound, err := gw.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		// websocket closed
		return err
	}

	enc := json.NewEncoder(outbound)
	if err = enc.Encode(&gw.RouterConf); err != nil {
		// websocket closed
		return err
	}
	// Closing the writer does the send
	outbound.Close()

	// Read message loop
	go func() {
		for {
			mt, inbound, err := gw.conn.NextReader()
			if err != nil {
				log.Debug().
					Str("eui", gw.Name).
					Err(err).
					Msg("websocket next reader error")
				return
			}

			switch mt {
			case websocket.TextMessage:
				msg, err := decode(inbound)
				if err != nil {
					gw.Stats.DecodeErrors++
					log.Error().
						Err(err).
						Str("eui", gw.Name).
						Msg(" decode message failed")
					continue
				}
				server.Receive(gw, msg)
			case websocket.BinaryMessage:
				// Binary data sent by RPC sessions
				gw.Stats.RecvBinaryMsg++
				log.Debug().
					Str("eui", gw.Name).
					Msg("received websocket binary data")
			case websocket.CloseMessage:
				log.Debug().
					Str("eui", gw.Name).
					Msg("received websocket close message")
				return
			default:
			}
		}
	}()

	<-ctx.Done()

	return err
}

// WriteText writes websocket text message to gateway
func (gw Gateway) WriteText(r io.Reader) error {
	if gw.conn == nil {
		gw.Stats.WriteNoConnError++
		return errors.New("no connection")
	}

	w, err := gw.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)

	if err != nil {
		gw.Stats.WriteTextError++
	} else {
		gw.Stats.WriteTextOk++
	}

	return err
}

func (gw *Gateway) readVersion(ctx context.Context) error {

	// Set a short initial read deadline to abort the connection if version is not soon received
	gw.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Reset read deadline to no timeout
	defer gw.conn.SetReadDeadline(time.Time{})

	// Read version
	_, inbound, err := gw.conn.NextReader()
	if err != nil {
		return err
	}

	dec := json.NewDecoder(inbound)
	if err = dec.Decode(&gw.Version); err != nil {
		return err
	}

	return nil
}
