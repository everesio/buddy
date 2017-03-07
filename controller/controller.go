package controller

import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/everesio/buddy/consumers"
	"github.com/everesio/buddy/pkg"
	"github.com/everesio/buddy/producers"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

var (
	synchronizeProcessingTimeSummary prometheus.Summary
	synchronizePendingOpsGauge       prometheus.Gauge
	synchronizeErrorCounter          prometheus.Counter
)

func init() {
	synchronizeProcessingTimeSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace: "buddy",
		Subsystem: "synchronize",
		Name:      "processing_time",
		Help:      "Time in milliseconds spent on synchronization.",
	})
	prometheus.MustRegister(synchronizeProcessingTimeSummary)

	synchronizePendingOpsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "buddy",
		Subsystem: "synchronize",
		Name:      "pending_ops",
		Help:      "Number of pending synchronization operations.",
	})
	prometheus.MustRegister(synchronizePendingOpsGauge)

	synchronizeErrorCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "synchronize",
		Name:      "processing_error_count",
		Help:      "Number of synchronization errors.",
	})
	prometheus.MustRegister(synchronizeErrorCounter)

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
	timer := pkg.NewTimer(prometheus.ObserverFunc(func(v float64) {
		synchronizeProcessingTimeSummary.Observe(v)
	}))
	defer timer.ObserveDuration()

	synchronizePendingOpsGauge.Inc()
	defer func() { synchronizePendingOpsGauge.Dec() }()

	c.wg.Add(1)
	defer c.wg.Done()

	log.Infoln("[Synchronize] Synchronizing DNS entries...")

	endpoints, err := c.producer.Endpoints()
	if err != nil {
		synchronizeErrorCounter.Inc()
		return fmt.Errorf("[Synchronize] Error getting endpoints from producer: %v", err)
	}
	computeZones := c.producer.ComputeZones()
	err = c.consumer.Sync(computeZones, endpoints)
	if err != nil {
		synchronizeErrorCounter.Inc()
		return fmt.Errorf("[Synchronize] Error consuming endpoints: %v", err)
	}
	return nil
}
