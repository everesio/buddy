package consumers

import (
	"errors"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/everesio/buddy/pkg"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/dns/v1"
	"reflect"
	"sort"
	"strings"
)

var (
	// buddy/<compute-zone>
	computeZonePrefix string
	// buddy/<compute-zone>/IPv4
	labelPrefix string

	additionsCounter     *prometheus.CounterVec
	deletionsCounter     *prometheus.CounterVec
	modificationsCounter *prometheus.CounterVec
)

func init() {
	buddyPrefix := pkg.GoogleConfig.BuddyLabelPrefix
	if buddyPrefix == "" {
		buddyPrefix = pkg.DefaultBuddyLabelPrefix
	}
	computeZonePrefix = buddyPrefix + "/%s"
	labelPrefix = computeZonePrefix + "/%s"

	additionsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "google_consumer",
		Name:      "rrs_additions",
		Help:      "Number of calculated resource record set additions.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(additionsCounter)

	deletionsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "google_consumer",
		Name:      "rrs_deletions",
		Help:      "Number of calculated resource record set deletions.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(deletionsCounter)

	modificationsCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "buddy",
		Subsystem: "google_consumer",
		Name:      "rrs_modifications",
		Help:      "Number of calculated resource record set modifications.",
	},
		[]string{"dns_zone"},
	)
	prometheus.MustRegister(modificationsCounter)
}

// GoogleConsumer synchronizes google cloud DNS
type GoogleConsumer struct {
	dnsTTL           int64
	dnsZones         map[string]struct{}
	multipleIPRecord bool
	dnsService       dnsService
}

// NewGoogleConsumer creates a new GoogleConsumer
func NewGoogleConsumer() (*GoogleConsumer, error) {
	if pkg.GoogleConfig.Project == "" {
		return nil, errors.New("Please provide --google-project")
	}
	client, err := google.DefaultClient(context.Background(), dns.NdevClouddnsReadwriteScope)
	if err != nil {
		return nil, fmt.Errorf("[Cloud DNS] Unable to create google oauth2 http client %v", err)
	}
	dnsService, err := newCloudDNSService(pkg.GoogleConfig.Project, client)
	if err != nil {
		return nil, fmt.Errorf("[Cloud DNS] Unable to create cloud dns service: %v", err)
	}

	allDNSZones, err := dnsService.getProjectDNSZones()
	if err != nil {
		return nil, err
	}

	dnsZones := getZonesToManage()
	if len(dnsZones) == 0 {
		return nil, errors.New("Please provide --dns-zones")
	}

	for dnsZone := range dnsZones {
		if _, ok := allDNSZones[dnsZone]; !ok {
			return nil, fmt.Errorf("[Cloud DNS] Configured DNS zone '%s' is not a managed zone. Managed zones %v", dnsZone, allDNSZones)
		}
	}
	dnsTTL := pkg.GoogleConfig.DNSTTL
	if dnsTTL < 0 {
		dnsTTL = 300
	}

	log.Printf("[Cloud DNS] Google consumer: project %s, dns zones %v", pkg.GoogleConfig.Project, reflect.ValueOf(dnsZones).MapKeys())
	return &GoogleConsumer{dnsTTL: dnsTTL, dnsZones: dnsZones, multipleIPRecord: pkg.GoogleConfig.MultipleIPRecord, dnsService: dnsService}, nil
}

func getZonesToManage() map[string]struct{} {
	result := make(map[string]struct{})

	if pkg.GoogleConfig.ExternalIPDNSZone != "" {
		result[pkg.GoogleConfig.ExternalIPDNSZone] = struct{}{}
	}
	if pkg.GoogleConfig.InternalIPDNSZone != "" {
		result[pkg.GoogleConfig.InternalIPDNSZone] = struct{}{}
	}
	for _, zone := range strings.Split(pkg.GoogleConfig.DNSZones, ",") {
		if zone != "" {
			result[zone] = struct{}{}
		}
	}
	return result
}

func (gc *GoogleConsumer) Sync(computeZones []string, endpoints []*pkg.Endpoint) error {
	return gc.SyncOne(computeZones, endpoints)
}

// Sync synchronizes provided endpoints with Cloud DNS
func (gc *GoogleConsumer) SyncOne(computeZones []string, endpoints []*pkg.Endpoint) error {
	dnsZoneChanges, err := gc.getDNSZoneChanges(computeZones, endpoints)
	if err != nil {
		return err
	}
	for _, v := range dnsZoneChanges {
		err = gc.dnsService.applyDNSZoneChange(v)
		if err != nil {
			return fmt.Errorf("Error applying change for %s: %v", v.dnsZone, err)
		}
	}
	return nil
}

func (gc *GoogleConsumer) SyncBulk(computeZones []string, endpoints []*pkg.Endpoint) error {

	dnsZoneChanges, err := gc.getDNSZoneChanges(computeZones, endpoints)
	if err != nil {
		return err
	}
	zoneChanges := make(map[string]*dnsZoneChange)
	for _, v := range dnsZoneChanges {
		zoneChange, exists := zoneChanges[v.dnsZone]
		if !exists {
			zoneChange = new(dnsZoneChange)
			zoneChanges[v.dnsZone] = zoneChange
		}
		zoneChange.change.Additions = append(zoneChange.change.Additions, v.change.Additions...)
		zoneChange.change.Deletions = append(zoneChange.change.Deletions, v.change.Deletions...)
	}

	for dnsZone, change := range zoneChanges {
		err = gc.dnsService.applyDNSZoneChange(change)
		if err != nil {
			return fmt.Errorf("Error applying change for %s: %v", dnsZone, err)
		}
	}
	return nil
}

func (gc *GoogleConsumer) endpointsRecordGroups(computeZones []string, endpoints []*pkg.Endpoint) ([]*RecordGroup, error) {
	managedZones, err := gc.dnsService.getProjectDNSZones()
	if err != nil {
		return nil, err
	}

	computeZonesMap := make(map[string]struct{})
	for _, v := range computeZones {
		computeZonesMap[v] = struct{}{}
	}

	recordGroups := map[string]*RecordGroup{}
	for _, endpoint := range endpoints {
		if endpoint.Hostname == "" || endpoint.ComputeZone == "" || endpoint.DNSZone == "" || endpoint.IP == "" {
			log.Warningf("[Cloud DNS] Skip invalid endpoint: %v", endpoint)
			continue
		}
		if _, computeZoneOk := computeZonesMap[endpoint.ComputeZone]; computeZoneOk {
			if zoneDNSName, zoneDNSNameOk := managedZones[endpoint.DNSZone]; zoneDNSNameOk {
				dnsName := strings.Trim(endpoint.Hostname, ".") + "." + strings.Trim(zoneDNSName, ".") + "."

				recordGroup, exists := recordGroups[dnsName]
				if !exists {
					recordGroup = &RecordGroup{
						DNSName: dnsName,
						DNSZone: endpoint.DNSZone,
						IPs:     make([]string, 0, 1),
						TTL:     gc.dnsTTL,
						Labels:  []string{},
					}
				}
				recordGroup.IPs = append(recordGroup.IPs, endpoint.IP)
				recordGroup.Labels = append(recordGroup.Labels, fmt.Sprintf(labelPrefix, endpoint.ComputeZone, endpoint.IP))
				recordGroups[dnsName] = recordGroup

			}
		}
	}

	if !gc.multipleIPRecord {
		recordGroups = removeMultipleIPRecord(recordGroups)
	}

	result := make([]*RecordGroup, 0, len(recordGroups))
	for _, v := range recordGroups {
		result = append(result, v)
	}
	return result, nil
}

func removeMultipleIPRecord(recordGroups map[string]*RecordGroup) map[string]*RecordGroup {
	result := map[string]*RecordGroup{}
	for dnsName, recordGroup := range recordGroups {
		if len(recordGroup.IPs) > 1 {
			log.Warningf("[Cloud DNS] Skip multiple IP record for %s: %v", dnsName, recordGroup.IPs)
			continue
		}
		result[dnsName] = recordGroup
	}
	return result
}

func (gc *GoogleConsumer) getDNSZoneChanges(computeZones []string, endpoints []*pkg.Endpoint) ([]*dnsZoneChange, error) {
	currentRecordGroups, err := gc.currentRecordGroups()
	if err != nil {
		return nil, err
	}
	ownRecordGroups := filterOwnRecordGroups(currentRecordGroups, computeZones)
	targetRecordGroups, err := gc.endpointsRecordGroups(computeZones, endpoints)
	if err != nil {
		return nil, err
	}
	return calcDNSZoneChanges(ownRecordGroups, targetRecordGroups), nil
}

func calcDNSZoneChanges(existingRecordGroups []*RecordGroup, targetRecordGroups []*RecordGroup) []*dnsZoneChange {

	existingMap := make(map[string]*RecordGroup)
	for _, v := range existingRecordGroups {
		existingMap[v.DNSName] = v
	}
	targetMap := make(map[string]*RecordGroup)
	for _, v := range targetRecordGroups {
		targetMap[v.DNSName] = v
	}
	log.Debugln("Current record groups:")
	printRecordGroups(existingRecordGroups)

	log.Debugln("Target record groups:")
	printRecordGroups(targetRecordGroups)

	dnsZoneChanges := make([]*dnsZoneChange, 0)
	for _, existingRecordGroup := range existingRecordGroups {
		targetRecordGroup, exists := targetMap[existingRecordGroup.DNSName]
		if !exists {
			change := new(dns.Change)
			rrs := toResourceRecordSet(existingRecordGroup)
			change.Deletions = append(change.Deletions, rrs...)
			dnsZoneChange := &dnsZoneChange{dnsZone: existingRecordGroup.DNSZone, change: change}
			dnsZoneChanges = append(dnsZoneChanges, dnsZoneChange)

			deletionsCounter.WithLabelValues(existingRecordGroup.DNSZone).Inc()
			log.Infof("[Cloud DNS]: Change deletion: %s / %v", existingRecordGroup.DNSName, existingRecordGroup.IPs)

		} else {
			existingIPs := sortedCopy(existingRecordGroup.IPs)
			targetIPs := sortedCopy(targetRecordGroup.IPs)
			if !stringArrayEquals(existingIPs, targetIPs) {
				change := new(dns.Change)
				change.Deletions = append(change.Deletions, toResourceRecordSet(existingRecordGroup)...)
				change.Additions = append(change.Additions, toResourceRecordSet(targetRecordGroup)...)
				dnsZoneChange := &dnsZoneChange{dnsZone: existingRecordGroup.DNSZone, change: change}
				dnsZoneChanges = append(dnsZoneChanges, dnsZoneChange)

				modificationsCounter.WithLabelValues(targetRecordGroup.DNSZone).Inc()
				log.Infof("[Cloud DNS]: Change modification: %s / %v -> %v", existingRecordGroup.DNSName, existingRecordGroup.IPs, targetRecordGroup.IPs)
			}
		}
	}
	for _, targetRecordGroup := range targetRecordGroups {
		_, exists := existingMap[targetRecordGroup.DNSName]
		if !exists {
			change := new(dns.Change)
			change.Additions = append(change.Additions, toResourceRecordSet(targetRecordGroup)...)
			dnsZoneChange := &dnsZoneChange{dnsZone: targetRecordGroup.DNSZone, change: change}
			dnsZoneChanges = append(dnsZoneChanges, dnsZoneChange)

			additionsCounter.WithLabelValues(targetRecordGroup.DNSZone).Inc()
			log.Infof("[Cloud DNS]: Change addition: %s / %v", targetRecordGroup.DNSName, targetRecordGroup.IPs)
		}
	}
	return dnsZoneChanges
}

// Records current records managed by buddy
func (gc *GoogleConsumer) Records(computeZones []string) (interface{}, error) {
	currentRecordGroups, err := gc.currentRecordGroups()
	if err != nil {
		return nil, err
	}

	recordGroups := make([]*RecordGroup, 0, 16)
	for _, v := range filterOwnRecordGroups(currentRecordGroups, computeZones) {
		recordGroups = append(recordGroups, v)
	}
	return recordGroups, nil

}

func filterOwnRecordGroups(recordGroups []*RecordGroup, computeZones []string) []*RecordGroup {
	computeZonesPrefixes := make(map[string]struct{})
	for _, computeZone := range computeZones {
		computeZonesPrefixes[fmt.Sprintf(computeZonePrefix, computeZone)] = struct{}{}
	}
	ownRecordGroups := make([]*RecordGroup, 0, len(recordGroups))
	for _, record := range recordGroups {
		skip := false
	labelLoop:
		for _, label := range record.Labels {
			for computeZonesPrefix := range computeZonesPrefixes {
				if strings.HasPrefix(label, computeZonesPrefix) {
					continue labelLoop
				}
			}
			skip = true
		}
		if len(record.Labels) == 0 || skip {
			log.Debugf("[Cloud DNS] Skip not owned record %v", record)
			continue
		}
		ownRecordGroups = append(ownRecordGroups, record)

	}
	return ownRecordGroups
}

func toResourceRecordSet(recordGroup *RecordGroup) []*dns.ResourceRecordSet {
	return []*dns.ResourceRecordSet{
		{
			Name:    recordGroup.DNSName,
			Rrdatas: recordGroup.IPs,
			Ttl:     recordGroup.TTL,
			Type:    "A",
		},
		{
			Name:    recordGroup.DNSName,
			Rrdatas: recordGroup.Labels,
			Ttl:     recordGroup.TTL,
			Type:    "TXT",
		},
	}
}

func (gc *GoogleConsumer) currentRecordGroups() ([]*RecordGroup, error) {
	records := make(map[string]*RecordGroup)
	for dnsZone := range gc.dnsZones {
		resourceRecordSets, err := gc.dnsService.getResourceRecordSets(dnsZone)
		if err != nil {
			return nil, err
		}
		for _, r := range resourceRecordSets {
			if r.Type == "A" || r.Type == "TXT" {
				record, exists := records[r.Name]
				if !exists {
					record = &RecordGroup{DNSName: r.Name, DNSZone: dnsZone}
				}
				switch r.Type {
				case "A":
					record.IPs = r.Rrdatas
					record.TTL = r.Ttl
				case "TXT":
					record.Labels = trimLabels(r.Rrdatas)
				}
				records[r.Name] = record
			}
		}
	}
	result := make([]*RecordGroup, 0, len(records))
	for _, v := range records {
		result = append(result, v)
	}
	return result, nil
}

func printRecordGroups(recordGroup []*RecordGroup) {
	for _, v := range recordGroup {
		log.Debugln(" ", v.DNSZone, v.DNSName, v.IPs, v.Labels, v.TTL)
	}
}

func sortedCopy(source []string) []string {
	if len(source) == 0 {
		return source
	}
	result := make([]string, len(source), len(source))
	copy(result, source)
	sort.Strings(result)
	return result
}

func stringArrayEquals(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func trimLabels(labels []string) []string {
	if len(labels) == 0 {
		return labels
	}
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		result = append(result, strings.Trim(label, `"`))
	}
	return result

}

// RecordGroup contains data from A and TXT record for the DNS name
type RecordGroup struct {
	DNSName string   `json:"dnsName,omitempty"`
	DNSZone string   `json:"dnsZone,omitempty"`
	IPs     []string `json:"ips,omitempty"`
	TTL     int64    `json:"ttl,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}
