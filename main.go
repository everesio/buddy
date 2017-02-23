package main

import (
	"encoding/json"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/everesio/buddy/consumers"
	"github.com/everesio/buddy/producers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/alecthomas/kingpin.v2"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

var version = "Unknown"

var params struct {
	httpAddr  string
	debugAddr string
	producer  string
	consumer  string
	debug     bool
}

func init() {
	kingpin.Flag("http-addr", "HTTP listen address").Default(":8080").StringVar(&params.httpAddr)
	kingpin.Flag("debug-addr", "Debug listen address").Default(":8081").StringVar(&params.debugAddr)
	kingpin.Flag("producer", "The endpoints producer to use.").Default("google").StringVar(&params.producer)
	kingpin.Flag("consumer", "The endpoints consumer to use.").Default("google").StringVar(&params.consumer)
	kingpin.Flag("debug", "Enable debug logging.").BoolVar(&params.debug)
}

func main() {
	kingpin.Version(version)
	kingpin.Parse()

	if params.debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Info("Starting buddy")

	producer, err := producers.New(params.producer)
	if err != nil {
		log.Fatalf("Error creating producer: %v", err)
	}
	consumer, err := consumers.New(params.consumer)
	if err != nil {
		log.Fatalf("Error creating consumer: %v", err)
	}

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

		log.Info("Debug addr ", params.debugAddr)
		errc <- http.ListenAndServe(params.debugAddr, m)
	}()

	// HTTP transport.
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.Handle("/endpoints", endpointsHandler(producer))
		http.Handle("/records", recordsHandler(producer, consumer))
		http.Handle("/sync", syncHandler(producer, consumer))
		log.Info("HTTP addr ", params.httpAddr)
		errc <- http.ListenAndServe(params.httpAddr, nil)
	}()

	// Run!
	log.Info("exit", <-errc)
}

func endpointsHandler(producer producers.Producer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		endpoints, err := producer.Endpoints()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(endpoints)
	})
}

func recordsHandler(producer producers.Producer, consumer consumers.Consumer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		records, err := consumer.Records(producer.ComputeZones())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(records)
	})
}

func syncHandler(producer producers.Producer, consumer consumers.Consumer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		endpoints, err := producer.Endpoints()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = consumer.Sync(producer.ComputeZones(), endpoints)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}