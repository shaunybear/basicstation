package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/shaunybear/basicstation"
)

var (
	logger zerolog.Logger
)

func main() {

	// parse command line options
	addr := ":8080"
	flag.StringVar(&addr, "address", addr, "service address")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())

	uri := "ws://127.0.0.1" + addr
	muxs := "demoServer"
	server := newDemoServer(uri, muxs)
	// Server up!
	logger.Fatal().Err(listenAndServe(ctx, server, addr)).Msg("station listener")

	// trap ctrl-c
	ctrlc := make(chan os.Signal, 1)
	signal.Notify(ctrlc, os.Interrupt)
	defer func() {
		signal.Stop(ctrlc)
	}()

	// wait for ctrl-c
	<-ctrlc

	// Stop services
	cancel()
}

func listenAndServe(ctx context.Context, server demoServer, addr string) error {

	// Initialize basic station environment
	env := &basicstation.Environment{
		Log:     logger,
		Handler: &server,
		Repo:    &server,
	}

	// Initialize basic station HTTP handlers
	dh := basicstation.DiscoveryHandler{Env: env}
	sh := basicstation.StationHandler{Env: env}

	// Set the HTTP routes
	router := mux.NewRouter()
	router.Handle("/router-info", dh)
	router.Handle("/{eui}", sh)

	// Initialize http server
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	var err error

	// Start HTTP server
	go func() {
		logger.Info().Msgf("Listening on %s", addr)
		err = srv.ListenAndServe()
	}()

	// Wait for done signal
	<-ctx.Done()

	return err
}

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05.0000"}
	logger = zerolog.New(output).With().Timestamp().Logger()
}

type demoServer struct {
	uri   string
	muxs  string
	rconf basicstation.RouterConf
}

func newDemoServer(uri, muxs string) demoServer {
	return demoServer{uri: uri, muxs: muxs}
}

func (server *demoServer) GetStation(eui uint64) (basicstation.Station, bool) {
	logger.Info().Msgf("GetStation eui=%016X", eui)

	stn := demoStation{
		eui:    eui,
		server: server,
		rconf:  server.rconf,
	}
	return stn, true
}

func (server *demoServer) Receive(station basicstation.Station, msg interface{}) {
	logger.Info().Msgf("<-%s msg=%v", station, msg)
}

type demoStation struct {
	eui     uint64
	server  *demoServer
	rconf   basicstation.RouterConf
	version basicstation.Version
}

func (stn demoStation) GetRouterConf() (basicstation.RouterConf, error) {
	logger.Info().Msgf("GetRouterConf eui=%016X", stn.eui)
	return stn.server.rconf, nil

}

func (stn demoStation) GetDiscoveryResponse() (basicstation.DiscoveryResponse, error) {
	logger.Info().Msgf("GetDiscoveryResponse eui=%016X", stn.eui)

	return basicstation.DiscoveryResponse{
		URI:  fmt.Sprintf("%s/%016X", stn.server.uri, stn.eui),
		MUXS: stn.server.muxs,
	}, nil
}

func (stn demoStation) SetVersion(version basicstation.Version) {
	logger.Info().Msgf("SetVersion eui=%016X", stn.eui)
	stn.version = version
}

func (stn demoStation) String() string {
	return fmt.Sprintf("%016X", stn.eui)

}
