package consumers

import (
	log "github.com/Sirupsen/logrus"

	"fmt"
	"google.golang.org/api/dns/v1"
	"net/http"
	"strings"
)

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
			log.Warnf("Cannot update some DNS records in zone %s : %v", dnsZoneChange.dnsZone, err)
			return nil
		}
		return fmt.Errorf("Unable to create change for %s/%s: %v", s.project, dnsZoneChange.dnsZone, err)
	}
	return nil
}
