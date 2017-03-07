package consumers

import (
	log "github.com/Sirupsen/logrus"

	"fmt"
	"github.com/everesio/buddy/pkg"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/dns/v1"
	"net/http"
	"strings"
)

var (
	requestZonesTimeSummary        prometheus.Summary
	requestRecordsTimeSummary      *prometheus.SummaryVec
	rrsAdditionsCounter            *prometheus.CounterVec
	rrsDeletionsCounter            *prometheus.CounterVec
	rrsChangeErrorCounter          *prometheus.CounterVec
	rrsChangeAlreadyExistedCounter *prometheus.CounterVec
)

func init() {
	requestZonesTimeSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "get_zones_time",
		Help:      "Time in milliseconds spent on retrieval of the list of DNS zones.",
	})
	prometheus.MustRegister(requestZonesTimeSummary)

	requestRecordsTimeSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "get_records_time",
		Help:      "Time in milliseconds spent on retrieval of the list of resource record sets contained within the specified manged zone.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(requestRecordsTimeSummary)

	rrsAdditionsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "rrs_additions",
		Help:      "Number of resource record set additions.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(rrsAdditionsCounter)

	rrsDeletionsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "rrs_deletions",
		Help:      "Number of resource record set deletions.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(rrsDeletionsCounter)

	rrsChangeErrorCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "rrs_change_errors",
		Help:      "Number of rrs change errors.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(rrsChangeErrorCounter)

	rrsChangeAlreadyExistedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "dns_service",
		Name:      "rrs_already_exists",
		Help:      "Number of rrs change operations rejected due to already_exists error.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(rrsChangeAlreadyExistedCounter)

}

type dnsService struct {
	project string
	service *dns.Service
}

func newDNSService(project string, client *http.Client) (*dnsService, error) {
	service, err := dns.New(client)
	if err != nil {
		return nil, err
	}
	return &dnsService{project: project, service: service}, nil
}

// GetProjectDNSZones provides list of all project DNS managed zones.
// It returns mapping DNSZone to its DNSName
func (s *dnsService) getProjectDNSZones() (map[string]string, error) {
	timer := pkg.NewTimer(prometheus.ObserverFunc(func(v float64) {
		requestZonesTimeSummary.Observe(v)
	}))
	defer timer.ObserveDuration()

	resp, err := s.service.ManagedZones.List(s.project).Do()
	if err != nil {
		return nil, fmt.Errorf("[Cloud DNS] Error getting managed zones: %v", err)
	}
	result := make(map[string]string)
	for _, managedZone := range resp.ManagedZones {
		result[managedZone.Name] = managedZone.DnsName
	}
	return result, nil
}

// getResourceRecordSets retrieves all DNS Resource Record Sets for a give DNS managed zone name
func (s *dnsService) getResourceRecordSets(dnsZone string) ([]*dns.ResourceRecordSet, error) {
	timer := pkg.NewTimer(prometheus.ObserverFunc(func(v float64) {
		requestRecordsTimeSummary.WithLabelValues(dnsZone).Observe(v)
	}))
	defer timer.ObserveDuration()

	pageToken := ""

	resourceRecordSets := make([]*dns.ResourceRecordSet, 0, 16)
	for {
		req := s.service.ResourceRecordSets.List(s.project, dnsZone)
		if pageToken != "" {
			req.PageToken(pageToken)
		}
		resp, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("[Cloud DNS] Error getting DNS resourceRecordSets from zone %s: %v", dnsZone, err)
		}
		for _, r := range resp.Rrsets {
			resourceRecordSets = append(resourceRecordSets, r)
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return resourceRecordSets, nil
}

type dnsZoneChange struct {
	dnsZone string
	change  *dns.Change
}

func (s *dnsService) applyDNSZoneChange(dnsZoneChange *dnsZoneChange) error {
	if len(dnsZoneChange.change.Additions) == 0 && len(dnsZoneChange.change.Deletions) == 0 {
		log.Infof("Didn't submit change (no changes)")
		return nil
	}
	_, err := s.service.Changes.Create(s.project, dnsZoneChange.dnsZone, dnsZoneChange.change).Do()
	if err != nil {
		if strings.Contains(err.Error(), "alreadyExists") {
			rrsChangeAlreadyExistedCounter.WithLabelValues(dnsZoneChange.dnsZone).Inc()
			log.Warnf("Cannot update some DNS records in zone %s : %v", dnsZoneChange.dnsZone, err)
			return nil
		}
		rrsChangeErrorCounter.WithLabelValues(dnsZoneChange.dnsZone).Inc()
		return fmt.Errorf("Unable to create change for %s/%s: %v", s.project, dnsZoneChange.dnsZone, err)
	}
	rrsAdditionsCounter.WithLabelValues(dnsZoneChange.dnsZone).Add(float64(len(dnsZoneChange.change.Additions)))
	rrsDeletionsCounter.WithLabelValues(dnsZoneChange.dnsZone).Add(float64(len(dnsZoneChange.change.Deletions)))

	return nil
}
