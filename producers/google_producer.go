package producers

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/everesio/buddy/pkg"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

const (
	keyInternalIPDNSZone  = "internal-ip-dns-zone"
	keyExternalIPDNSZone  = "external-ip-dns-zone"
	keyInternalIPHostname = "internal-ip-hostname"
	keyExternalIPHostname = "external-ip-hostname"
)

var (
	internalEndpointsGauge *prometheus.GaugeVec
	externalEndpointsGauge *prometheus.GaugeVec
)

func init() {
	internalEndpointsGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "buddy",
		Subsystem: "google_producer",
		Name:      "internal_endpoints",
		Help:      "Number of instances with internal IP.",
	},
		[]string{"compute_zone"},
	)
	prometheus.MustRegister(internalEndpointsGauge)

	externalEndpointsGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "buddy",
		Subsystem: "google_producer",
		Name:      "external_endpoints",
		Help:      "Number of instances with external IP.",
	},
		[]string{"compute_zone"},
	)
	prometheus.MustRegister(externalEndpointsGauge)
}

// GoogleProducer reads data from compute engine
type GoogleProducer struct {
	computeZones         []string
	externalIPDNSZone    string
	internalIPDNSZone    string
	computeEngineService *computeEngineService
}

// NewGoogleProducer creates new GoogleProducer
func NewGoogleProducer() (*GoogleProducer, error) {
	if pkg.GoogleConfig.Project == "" {
		return nil, errors.New("Please provide --google-project")
	}

	client, err := google.DefaultClient(context.Background(), compute.ComputeReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("[Compute Engine] Unable to create google oauth2 http client %v", err)
	}

	computeEngineService, err := newComputeEngineService(pkg.GoogleConfig.Project, client)
	if err != nil {
		return nil, fmt.Errorf("[Compute Engine] Unable to create compute engine service: %v", err)
	}

	var computeZones []string
	if computeZones, err = getComputeZones(computeEngineService); err != nil {
		return nil, err
	}

	if pkg.GoogleConfig.ExternalIPDNSZone == pkg.GoogleConfig.InternalIPDNSZone && pkg.GoogleConfig.ExternalIPDNSZone != "" {
		return nil, fmt.Errorf("[Compute Engine] internalIP and externalIP DNS Zone names are the same: %s", pkg.GoogleConfig.InternalIPDNSZone)
	}

	log.Printf("[Compute Engine] Google producer: project %s, compute zones %v", pkg.GoogleConfig.Project, computeZones)
	return &GoogleProducer{
		computeZones:         computeZones,
		externalIPDNSZone:    pkg.GoogleConfig.ExternalIPDNSZone,
		internalIPDNSZone:    pkg.GoogleConfig.InternalIPDNSZone,
		computeEngineService: computeEngineService}, nil
}

func getComputeZones(computeEngineService *computeEngineService) ([]string, error) {
	switch {
	case pkg.GoogleConfig.Zone == "" && pkg.GoogleConfig.Region == "":
		return nil, errors.New("Please provide --google-zone or --google-region")
	case pkg.GoogleConfig.Zone != "" && pkg.GoogleConfig.Region != "":
		return nil, errors.New("Please provide either --google-zone or --google-region")
	case pkg.GoogleConfig.Zone != "":
		if _, err := computeEngineService.getRegion(pkg.GoogleConfig.Zone); err != nil {
			return nil, err
		}
		return []string{pkg.GoogleConfig.Zone}, nil
	case pkg.GoogleConfig.Region != "":
		var managedZones []string
		var err error
		if managedZones, err = computeEngineService.getZones(pkg.GoogleConfig.Region); err != nil {
			return nil, err
		}
		return managedZones, nil

	}
	return nil, errors.New("getManagedZones: Internal error")
}

// Endpoints provides endpoints read from compute engine.
func (gp *GoogleProducer) Endpoints() ([]*pkg.Endpoint, error) {
	endpoints := make([]*pkg.Endpoint, 0, 16)
	for _, zone := range gp.computeZones {
		googleInstances, err := gp.computeEngineService.getInstances(zone)
		if err != nil {
			return nil, err
		}
		var internalEndpoints int
		var externalEndpoints int
		for _, googleInstance := range googleInstances {
			internalEndpoint := newEndpoint(&googleInstance, keyInternalIPHostname, keyInternalIPDNSZone, gp.internalIPDNSZone, googleInstance.InternalIP)
			externalEndpoint := newEndpoint(&googleInstance, keyExternalIPHostname, keyExternalIPDNSZone, gp.externalIPDNSZone, googleInstance.ExternalIP)
			if internalEndpoint != nil && externalEndpoint != nil && internalEndpoint.DNSZone == externalEndpoint.DNSZone && internalEndpoint.Hostname == externalEndpoint.Hostname {
				log.Warnf("Instance %s has the same dns name for externalIP %s and internalIP %s", googleInstance.Name, externalEndpoint.IP, internalEndpoint.IP)
				continue
			}
			if internalEndpoint != nil {
				endpoints = append(endpoints, internalEndpoint)
				internalEndpoints++
			}
			if externalEndpoint != nil {
				endpoints = append(endpoints, externalEndpoint)
				externalEndpoints++
			}
		}
		internalEndpointsGauge.WithLabelValues(zone).Set(float64(internalEndpoints))
		externalEndpointsGauge.WithLabelValues(zone).Set(float64(externalEndpoints))
	}
	return endpoints, nil
}

// ComputeZones provides all compute zones managed by the producer
func (gp *GoogleProducer) ComputeZones() []string {
	return gp.computeZones
}

func newEndpoint(googleInstance *googleInstance, keyHostname string, keyDNSZone string, defaultDNSZone string, ip string) *pkg.Endpoint {
	if ip != "" {
		// is mata data or tag present ?
		hostname, ok1 := googleInstance.Metadata[keyHostname]
		dnsZone, ok2 := googleInstance.Metadata[keyDNSZone]
		_, ok3 := googleInstance.Tags[keyHostname]
		_, ok4 := googleInstance.Tags[keyDNSZone]
		if ok1 || ok2 || ok3 || ok4 {
			if hostname == "" {
				hostname = googleInstance.Name
			}
			if dnsZone == "" {
				dnsZone = defaultDNSZone
			}
			if dnsZone == "" || hostname == "" {
				log.Warningf("Skip record. Default DNS ComputeZone was not configured: instance name %s, IP %s", googleInstance.Name, ip)
				return nil
			}
			return &pkg.Endpoint{Hostname: hostname, DNSZone: dnsZone, IP: ip, ComputeZone: googleInstance.ComputeZone}
		}

	}
	return nil
}
