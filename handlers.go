package basicstation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	lorawan "github.com/shaunybear/lorawango"
)

const (
	// DiscoveryURL is the Discovery URL path
	DiscoveryURL     = "/router-info"
	discoveryTimeout = 5 * time.Second
)

var upgrader = websocket.Upgrader{}

// GatewayHandler is the Basic Station HTTP handler
type GatewayHandler struct {
	Env *Environment
}

func (gh GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var gw Gateway
	var err error

	vars := mux.Vars(r)
	v, ok := vars["eui"]
	if !ok {
		gh.Env.Log.Debug().
			Str("uri", r.RequestURI).
			Msg("malformed muxs request uri")
	}

	if gw.EUI, err = lorawan.NewEUI(v); err != nil {
		gh.Env.Log.Debug().
			Err(err).
			Str("eui", v).
			Msg("parse eui from url failed")
		return
	}

	gw.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		gh.Env.Log.Warn().
			Err(err).
			Str("eui", gw.Name).
			Msg("websocket upgrade failed")
		return
	}

	// Pass gateway to the server to do with it as it pleases
	gh.Env.Server.NewConnection(&gw)
}

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

func (handler DiscoveryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var response DiscoveryResponse

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		handler.Env.Log.Error().Err(err).Msg("discovery websocket upgrader")
		return
	}
	defer conn.Close()

	// Gateway sends its unique identifier in the first and only  message of this connection.
	// Set a reasonable read deadline in the event the gateway does not send the request in a timely manner
	conn.SetReadDeadline(time.Now().Add(discoveryTimeout))

	_, reader, err := conn.NextReader()
	if err != nil {
		handler.Env.Log.Error().Err(err).Msg("discovery websocket next reader")
		return
	}

	msg, err := handler.decode(reader)
	if err != nil {
		handler.Env.Log.Warn().Err(err).Msg("discovery decode json")
		return
	}

	// Extract EUI from the request
	var eui lorawan.EUI
	for k, v := range msg {
		switch k {
		case "router", "Router":
			eui, err = lorawan.NewEUI(v)
			if err != nil {
				response.Error = "Missing router field"
				conn.WriteJSON(&response)
				handler.Env.Log.Warn().Err(err).Msg("discovery request get eui")
				return
			}
			response.Router = eui.String()
		default:
			handler.Env.Log.Warn().Err(err).Msg("discovery request no eui")
			return
		}
	}

	response, err = handler.Env.Server.GetDiscoveryResponse(eui.Uint64(), r)

	// Write response
	writer, err := conn.NextWriter(websocket.TextMessage)
	defer writer.Close()

	if err != nil {
		handler.Env.Log.Error().Err(err).Msg("discovery next writer")
		return
	}

	enc := json.NewEncoder(writer)
	if err = enc.Encode(&response); err != nil {
		handler.Env.Log.Error().Err(err).Msg("send discovery response")
	}
}

func (handler DiscoveryHandler) decode(r io.Reader) (map[string]interface{}, error) {
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
