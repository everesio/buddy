package controller

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/everesio/buddy/consumers"
	"github.com/everesio/buddy/producers"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

var (
	synchronizeTimeSummary prometheus.Summary
	synchronizePending prometheus.Gauge
	synchronizeError prometheus.Counter
)

func init() {
	synchronizeTimeSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Subsystem: "synchronize",
		Name:      "processing_time",
		Help:      "Time in milliseconds spent on synchronization.",
	})
	prometheus.MustRegister(synchronizeTimeSummary)

	synchronizePending = prometheus.NewGauge(prometheus.GaugeOpts{
		Subsystem: "synchronize",
		Name:      "pending_ops",
		Help:      "Number of pending synchronization operations.",
	})
	prometheus.MustRegister(synchronizePending)

	synchronizeError = prometheus.NewCounter(prometheus.CounterOpts{
		Subsystem: "synchronize",
		Name:      "processing_error_count",
		Help:      "Number of synchronization errors.",
	})
	prometheus.MustRegister(synchronizeError)

}

type Options struct {
	SyncInterval time.Duration
}

type Controller struct {
	producer producers.Producer
	consumer consumers.Consumer
	options  *Options
	stop     chan error
	wg       sync.WaitGroup
}

func New(producer producers.Producer, consumer consumers.Consumer, options *Options, stop chan error) *Controller {
	if options == nil {
		options = &Options{}
	}
	return &Controller{
		producer: producer,
		consumer: consumer,
		options:  options,
		stop:     stop,
	}
}

func (c *Controller) Run() {
	if c.options.SyncInterval > 0 {
		c.syncLoop()
	} else {
		log.Warn("[Synchronize] Synchronization loop is disabled.")
	}
}

func (c *Controller) syncLoop() {
	for {
		log.Debugf("[Synchronize] Sleeping for %s...", c.options.SyncInterval)
		select {
		case <-time.After(c.options.SyncInterval):
		case <-c.stop:
			log.Info("[Synchronize] Exited synchronization loop.")
			return
		}
		err := c.Synchronize()
		if err != nil {
			log.Errorf("[Synchronize] Sync loop error: %v", err)
		}
	}
}

func (c *Controller) Synchronize() error {
	start := time.Now()
	defer func() { synchronizeTimeSummary.Observe(float64(time.Since(start) / time.Millisecond)) }()

	synchronizePending.Inc()
	defer func() {synchronizePending.Dec()}()

	c.wg.Add(1)
	defer c.wg.Done()

	log.Infoln("[Synchronize] Synchronizing DNS entries...")

	endpoints, err := c.producer.Endpoints()
	if err != nil {
		synchronizeError.Inc()
		return fmt.Errorf("[Synchronize] Error getting endpoints from producer: %v", err)
	}
	computeZones := c.producer.ComputeZones()
	err = c.consumer.Sync(computeZones, endpoints)
	if err != nil {
		synchronizeError.Inc()
		return fmt.Errorf("[Synchronize] Error consuming endpoints: %v", err)
	}
	return nil
}
