package main

import (
	"fmt"
	log "github.com/Sirupsen/logrus"

	"flag"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

var version = "Unknown"

func main() {
	var (
		httpAddr  = flag.String("http.addr", ":8080", "HTTP listen address")
		debugAddr = flag.String("debug.addr", ":8081", "Debug and metrics listen address")
	)
	flag.Parse()

	log.Info("Starting buddy")

	errc := make(chan error)

	// Interrupt handler.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	// Debug listener.
	go func() {
		m := http.NewServeMux()
		m.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
		m.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		m.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		m.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
		m.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

		log.Info("Debug addr", *debugAddr)
		errc <- http.ListenAndServe(*debugAddr, m)
	}()

	// HTTP transport.
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Info("HTTP addr", *httpAddr)
		errc <- http.ListenAndServe(*httpAddr, nil)
	}()

	// Run!
	log.Info("exit", <-errc)
}
