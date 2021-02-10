package basicstation

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

const (
	// DiscoveryURL is the Discovery URL path
	DiscoveryURL     = "/router-info"
	discoveryTimeout = 5 * time.Second
)

var upgrader = websocket.Upgrader{}

// DiscoveryResponse represents a discovery response message
type DiscoveryResponse struct {
	Router string `json:"router,omitempty"`
	URI    string `json:"uri,omitempty"`
	MUXS   string `json:"muxs,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DiscoveryHandler is the Basic Station Discovery HTTP handler
type DiscoveryHandler struct {
	Env *Environment
}

func (dh DiscoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		dh.Env.Log.Error().Err(err).Msg("discovery websocket upgrader")
		return
	}
	defer conn.Close()

	// Gateway sends its unique identifier in the first and only  message of this connection.
	// Set a reasonable read deadline in the event the gateway does not send the request in a timely manner
	conn.SetReadDeadline(time.Now().Add(discoveryTimeout))

	_, reader, err := conn.NextReader()
	if err != nil {
		dh.Env.Log.Error().Err(err).Msg("discovery websocket next reader")
		return
	}

	msg, err := decodeToMap(reader)
	if err != nil {
		dh.Env.Log.Warn().Err(err).Msg("discovery decode json")
		return
	}

	// Extract EUI from the request
	var eui uint64
	for k, v := range msg {
		switch k {
		case "router", "Router":
			eui, err = parseEUI(v)
			if err != nil {
				response := DiscoveryResponse{Error: "Missing router field"}
				conn.WriteJSON(&response)
				dh.Env.Log.Warn().Err(err).Msg("discovery request get eui")
				return
			}
		default:
			dh.Env.Log.Warn().Err(err).Msg("discovery request no eui")
			return
		}
	}

	station, ok := dh.Env.Repo.GetStation(eui)
	if !ok {
		return
	}

	response, err := station.GetDiscoveryResponse()
	if err != nil {
		return
	}

	// Write response
	writer, err := conn.NextWriter(websocket.TextMessage)
	defer writer.Close()

	if err != nil {
		dh.Env.Log.Error().Err(err).Msg("discovery next writer")
		return
	}

	enc := json.NewEncoder(writer)
	if err = enc.Encode(&response); err != nil {
		dh.Env.Log.Error().Err(err).Msg("send discovery response")
	}
}

// StationHandler is the Basic Station connection HTTP handler
type StationHandler struct {
	Env *Environment
}

func (sh StationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	v, ok := vars["eui"]
	if !ok {
		sh.Env.Log.Debug().
			Str("uri", r.RequestURI).
			Msg("malformed muxs request uri")
	}

	eui, err := parseEUI(v)
	if err != nil {
		sh.Env.Log.Debug().
			Err(err).
			Msg("Parse EUI failed")
	}

	station, ok := sh.Env.Repo.GetStation(eui)
	if !ok {
		return
	}

	// Upgrade to websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		sh.Env.Log.Warn().
			Err(err).
			Uint64("eui", eui).
			Msg("websocket upgrade failed")
		return
	}
	defer ws.Close()

	// Start station protocol runner
	ch := connHandler{
		eui:     eui,
		station: station,
		env:     sh.Env,
		ws:      ws,
	}
	ch.run()
}

// stationHandler implements the basic station connection handler read/write functionality
type connHandler struct {
	eui     uint64
	station Station
	env     *Environment
	ws      *websocket.Conn
}

func (ch connHandler) run() error {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := ch.readVersion(ctx)
	if err != nil {
		return err
	}

	// Read configuration
	conf, err := ch.station.GetRouterConf()
	if err != nil {
		return err
	}

	// Get writer
	outbound, err := ch.ws.NextWriter(websocket.TextMessage)
	if err != nil {
		return err
	}

	// Encode to writer
	enc := json.NewEncoder(outbound)
	err = enc.Encode(&conf)
	if err != nil {
		return err
	}
	// Close does the actual send
	outbound.Close()

	// Start websocket writer goroutine
	go ch.writer(ctx)

	// Read loop
	for {
		mt, inbound, err := ch.ws.NextReader()
		if err != nil {
			ch.env.Log.Debug().
				Uint64("eui", ch.eui).
				Err(err).
				Msg("websocket next reader error")

			return err
		}

		switch mt {
		case websocket.TextMessage:
			ch.readText(inbound)
		case websocket.CloseMessage:
			ch.env.Log.Debug().
				Uint64("eui", ch.eui).
				Msg("received websocket close message")
			return nil
		default:
		}
	}
}

func (ch connHandler) readText(r io.Reader) error {

	// Read the message
	msg, err := decode(r)
	if err != nil {
		ch.env.Log.Error().
			Err(err).
			Str("service", "muxs").
			Uint64("eui", ch.eui).
			Msg("muxHandler decode message failed")

		return err
	}

	ch.env.Handler.Receive(ch.station, msg)

	return nil
}

func (ch connHandler) readVersion(ctx context.Context) error {
	var version Version

	// Set a short initial read deadline to abort the connection if version is not soon received
	ch.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Reset read deadline to no timeout
	defer ch.ws.SetReadDeadline(time.Time{})

	// Read version
	_, inbound, err := ch.ws.NextReader()
	if err != nil {
		return err
	}

	dec := json.NewDecoder(inbound)
	if err = dec.Decode(&version); err != nil {
		return err
	}

	ch.station.SetVersion(version)

	return nil
}

func (ch connHandler) writer(ctx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return
		}
		/*
			select {
			case r := <-conn.WriteChan:
				var w io.WriteCloser

				w, err = ws.NextWriter(websocket.TextMessage)
				if _, err = io.Copy(w, r); err != nil {
					service.log.Debug().
						Str("eui", eui.String()).
						Err(err).
						Msg("write to connection")

					return
				}
				w.Close()

			case <-ctx.Done():
				return
			}
		*/
	}
}

// read reads a JSON-encoded message from the reader and stores the
// value in v
func decodeToMap(r io.Reader) (map[string]interface{}, error) {
	var syntaxError *json.SyntaxError
	var unmarshalTypeError *json.UnmarshalTypeError
	var msg string

	m := make(map[string]interface{})
	err := json.NewDecoder(r).Decode(&m)
	if err == nil {
		return m, nil
	}

	switch {
	// Catch any syntax errors in the JSON and send an error message
	// which interpolates the location of the problem to make it
	// easier for the client to fix.
	case errors.As(err, &syntaxError):
		msg = fmt.Sprintf("Value contains badly-formed JSON (at position %d)", syntaxError.Offset)

	// In some circumstances Decode() may also return an
	// io.ErrUnexpectedEOF error for syntax errors in the JSON. There
	// is an open issue regarding this at
	// https://github.com/golang/go/issues/25956.
	case errors.Is(err, io.ErrUnexpectedEOF):
		msg = fmt.Sprintf("Value contains badly-formed JSON")

	// Catch any type errors, like trying to assign a string in the
	// JSON request body to a int field in our Person struct. We can
	// interpolate the relevant field name and position into the error
	// message to make it easier for the client to fix.
	case errors.As(err, &unmarshalTypeError):
		msg = fmt.Sprintf("Value contains an invalid value for the %q field (at position %d)", unmarshalTypeError.Field, unmarshalTypeError.Offset)

	// Catch the error caused by extra unexpected fields in the request
	// body. We extract the field name from the error message and
	// interpolate it in our custom error message. There is an open
	// issue at https://github.com/golang/go/issues/29035 regarding
	// turning this into a sentinel error.
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
		msg = fmt.Sprintf("Value contains unknown field %s", fieldName)

	// An io.EOF error is returned by Decode() if the request body is
	// empty.
	case errors.Is(err, io.EOF):
		msg = "Value is empty"
	default:
		msg = err.Error()
	}

	return nil, errors.New(msg)
}

// ParseEUI parses EUI from the input
func parseEUI(val interface{}) (uint64, error) {
	switch val.(type) {
	case int:
		return uint64(val.(int)), nil
	case float64:
		return uint64(val.(float64)), nil
	case string:
		return fromString(val.(string))
	default:
		return 0, fmt.Errorf("ParseEUI: %T is unsupported", val)
	}
}

// Stringer interface implemented by EUI
func toString(eui uint64) string {
	return fmt.Sprintf("%016x", eui)
}

// fromString parses a EUI string
func fromString(s string) (eui uint64, err error) {
	var parsed uint64
	// Try to parse as IPv6 format
	ip := net.ParseIP("::" + s)
	if ip != nil {
		parsed = binary.BigEndian.Uint64(ip[8:])
		if parsed != 0 {
			return uint64(parsed), nil
		}
	}

	for _, sep := range []string{"", "-", ":"} {
		ss := strings.Replace(s, sep, "", -1)
		if len(ss) < 16 {
			err = fmt.Errorf("Invalid EUI format: %s", s)
			return
		}

		// Attempt to parse the EUI from the string stripped of the current separator
		if parsed, err = strconv.ParseUint(ss, 16, 64); err != nil {
			continue
		}

		return uint64(parsed), nil
	}

	return
}
