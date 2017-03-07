package producers

import (
	log "github.com/Sirupsen/logrus"

	"fmt"
	"github.com/everesio/buddy/pkg"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/api/compute/v1"
	"net/http"
)

var (
	requestInstancesTime *prometheus.SummaryVec
)

func init() {
	requestInstancesTime = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "buddy",
		Subsystem: "compute_service",
		Name:      "get_instances_time",
		Help:      "Time in milliseconds spent on retrieval of the list of instances contained within the specified compute zone.",
	},
		[]string{"compute_zone"},
	)
	prometheus.MustRegister(requestInstancesTime)
}

type googleInstance struct {
	// googleInstance name. It must be 1-63 characters long, comply with RFC1035
	// and match the regular expression [a-z]([-a-z0-9]*[a-z0-9])?
	Name string `json:"name"`
	// An IPv4 internal network address
	InternalIP string `json:"internalIP"`
	// An external IP address associated with this instance
	ExternalIP string `json:"externalIP,omitempty"`
	// Metadata key/value pairs
	Metadata map[string]string `json:"metadata,omitempty"`
	// Tags
	Tags map[string]struct{} `json:"tags,omitempty"`
	// Name of google compute zone
	ComputeZone string `json:"zone"`
}

type computeEngineService struct {
	project string
	service *compute.Service
}

func newComputeEngineService(project string, client *http.Client) (*computeEngineService, error) {
	service, err := compute.New(client)
	if err != nil {
		return nil, err
	}
	return &computeEngineService{project: project, service: service}, nil
}

func (svc *computeEngineService) getInstances(zone string) ([]googleInstance, error) {
	timer := pkg.NewTimer(prometheus.ObserverFunc(func(v float64) {
		requestInstancesTime.WithLabelValues(zone).Observe(v)
	}))
	defer timer.ObserveDuration()

	instances := make([]googleInstance, 0, 16)

	pageToken := ""
	for {
		req := svc.service.Instances.List(svc.project, zone)
		if pageToken != "" {
			req.PageToken(pageToken)
		}
		computeInstanceList, err := req.Do()
		if err != nil {
			return nil, fmt.Errorf("[Compute Engine] Unable to retrieve list of instances: %v", err)
		}
		for _, computeInstance := range computeInstanceList.Items {
			instance, err := fromComputeInstance(computeInstance)
			// computeInstance container zone URL
			instance.ComputeZone = zone
			if err != nil {
				log.Warnln(err)
				continue
			}
			instances = append(instances, *instance)
		}
		if computeInstanceList.NextPageToken == "" {
			break
		}
		pageToken = computeInstanceList.NextPageToken
	}
	return instances, nil
}

func fromComputeInstance(computeInstance *compute.Instance) (*googleInstance, error) {
	instance := &googleInstance{Name: computeInstance.Name, Metadata: make(map[string]string), Tags: map[string]struct{}{}}
	for _, md := range computeInstance.Metadata.Items {
		instance.Metadata[md.Key] = *md.Value
	}

	for _, tag := range computeInstance.Tags.Items {
		instance.Tags[tag] = struct{}{}
	}
	if len(computeInstance.NetworkInterfaces) != 1 {
		return nil, fmt.Errorf("[Compute Engine] Skip instance '%s'. googleInstance must have one internal IP", computeInstance.Name)
	}
	networkInterface := computeInstance.NetworkInterfaces[0]
	instance.InternalIP = networkInterface.NetworkIP

	if len(networkInterface.AccessConfigs) == 1 {
		instance.ExternalIP = networkInterface.AccessConfigs[0].NatIP
	} else if len(networkInterface.AccessConfigs) > 1 {
		return nil, fmt.Errorf("[Compute Engine] Skip instance '%s'. Multiple external IPs are not supported", computeInstance.Name)
	}
	return instance, nil
}

// GetZones retrieves zone names for a given region
func (svc *computeEngineService) getZones(region string) ([]string, error) {
	computeRegion, err := svc.service.Regions.Get(svc.project, region).Do()
	if err != nil {
		return nil, fmt.Errorf("[Compute Engine] Unable to retrieve region: %v", err)
	}
	zonesURLs := make(map[string]struct{})
	for _, computeZoneURL := range computeRegion.Zones {
		zonesURLs[computeZoneURL] = struct{}{}
	}
	computeZones, err := svc.service.Zones.List(svc.project).Do()
	if err != nil {
		return nil, fmt.Errorf("[Compute Engine] Unable to retrieve zones: %v", err)
	}
	zones := make([]string, 0, len(zonesURLs))
	for _, computeZone := range computeZones.Items {
		if _, ok := zonesURLs[computeZone.SelfLink]; ok {
			zones = append(zones, computeZone.Name)
		}
	}
	return zones, nil
}

// GetRegion retrieves the zone name names for a given zone
func (svc *computeEngineService) getRegion(zone string) (string, error) {
	req := svc.service.Zones.Get(svc.project, zone)
	computeZone, err := req.Do()
	if err != nil {
		return "", fmt.Errorf("[Compute Engine] Unable to retrieve zone: %v", err)
	}
	computeRegions, err := svc.service.Regions.List(svc.project).Do()
	if err != nil {
		return "", fmt.Errorf("[Compute Engine] Unable to retrieve regions: %v", err)
	}
	for _, computeRegion := range computeRegions.Items {
		for _, computeZoneURL := range computeRegion.Zones {
			if computeZoneURL == computeZone.SelfLink {
				return computeRegion.Name, nil
			}
		}
	}
	return "", fmt.Errorf("[Compute Engine] Internal error. Region for zone %s was not found", zone)
}
