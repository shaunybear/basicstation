package basicstation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/gorilla/websocket"
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
	EUI        uint64
	conn       *websocket.Conn
	Version    Version
	RouterConf RouterConf
	Stats      Stats
}

// Logger interface
type Logger interface {
	Error(eui uint64, err error, msg string)
	Debug(eui uint64, msg string, err error)
}

// Handler is anything that implements gateway handler interface
type Handler interface {
	Receive(gw *Gateway, msg interface{})
	GetRouterConf(gw *Gateway) error
}

// Run ...
func (gw *Gateway) Run(ctx context.Context, handler Handler, log Logger) error {
	var err error

	// Close the connection on exit
	defer gw.conn.Close()

	// First message from the gateway is it's version information
	if err = gw.readVersion(ctx); err != nil {
		log.Error(gw.EUI, err, "read version failed")
		return err
	}

	// Get router configuration from the server
	if err = handler.GetRouterConf(gw); err != nil {
		return err
	}

	// Send config to the gateway
	outbound, err := gw.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		// websocket closed
		log.Debug(gw.EUI, "websocket closed", nil)
		return err
	}

	enc := json.NewEncoder(outbound)
	if err = enc.Encode(&gw.RouterConf); err != nil {
		// websocket closed
		return err
	}
	// Closing the writer does the send
	outbound.Close()

	done := make(chan bool)

	// Read message loop
	go func() {
		for {
			var mt int
			var inbound io.Reader

			mt, inbound, err = gw.conn.NextReader()
			if err != nil {
				log.Debug(gw.EUI, "websocket reader detected close", nil)
				done <- true
				return
			}

			switch mt {
			case websocket.TextMessage:
				var msg interface{}

				msg, err = decode(inbound)
				if err != nil {
					gw.Stats.DecodeErrors++
					log.Error(gw.EUI, err, "decode message failed")
					continue
				}
				handler.Receive(gw, msg)
			case websocket.BinaryMessage:
				// Binary data sent by RPC sessions
				gw.Stats.RecvBinaryMsg++
				log.Debug(gw.EUI, "received websocket binary data", nil)
			case websocket.CloseMessage:
				err = errors.New("received websocket close")
				log.Debug(gw.EUI, "lost connection", err)
				return
			default:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return err
		}
	}

	return err
}

// WriteJSON writes json encoded message to websocket
func (gw Gateway) WriteJSON(msg interface{}) error {
	if gw.conn == nil {
		gw.Stats.WriteNoConnError++
		return errors.New("no connection")
	}

	err := gw.conn.WriteJSON(msg)
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
