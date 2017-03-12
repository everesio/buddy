package consumers

import (
	"github.com/stretchr/testify/assert"
	"google.golang.org/api/dns/v1"
	"testing"
)

type fakeDNSService struct {
	projectDNSZones map[string]string
	managedZoneRRS  map[string][]*dns.ResourceRecordSet
}

func (s *fakeDNSService) getProjectDNSZones() (map[string]string, error) {
	return s.projectDNSZones, nil
}

func (s *fakeDNSService) getResourceRecordSets(dnsZone string) ([]*dns.ResourceRecordSet, error) {
	return s.managedZoneRRS[dnsZone], nil
}

func (s *fakeDNSService) applyDNSZoneChange(dnsZoneChange *dnsZoneChange) error {
	return nil
}

type fakeRecord struct {
	dnsName string
	dnsZone string
	ttl     int64
}

func (f *fakeRecord) aRecord(name string, ips ...string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name:    name + "." + f.dnsName,
		Ttl:     f.ttl,
		Type:    "A",
		Rrdatas: ips,
	}
}
func (f *fakeRecord) txtRecord(name string, labels ...string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name:    name + "." + f.dnsName,
		Ttl:     f.ttl,
		Type:    "TXT",
		Rrdatas: labels,
	}
}
func (f *fakeRecord) recordGroup(name string, ip string, label string) *RecordGroup {
	if label == "" {
		return &RecordGroup{
			DNSName: name + "." + f.dnsName,
			DNSZone: f.dnsZone,
			IPs:     []string{ip},
			TTL:     f.ttl}
	}
	return &RecordGroup{
		DNSName: name + "." + f.dnsName,
		DNSZone: f.dnsZone,
		IPs:     []string{ip},
		TTL:     f.ttl,
		Labels:  []string{label}}
}

func (f *fakeRecord) multiRecordGroup(name string, ips []string, labels []string) *RecordGroup {
	return &RecordGroup{
		DNSName: name + "." + f.dnsName,
		DNSZone: f.dnsZone,
		IPs:     ips,
		TTL:     f.ttl,
		Labels:  labels}
}

func (f *fakeRecord) aAndTxtRecords(name string, ips []string, labels []string) []*dns.ResourceRecordSet {
	return []*dns.ResourceRecordSet{f.aRecord(name, ips...), f.txtRecord(name, labels...)}
}

func concatRecords(elems ...[]*dns.ResourceRecordSet) []*dns.ResourceRecordSet {
	result := make([]*dns.ResourceRecordSet, 0, len(elems)*2)
	for _, elem := range elems {
		for _, rrs := range elem {
			result = append(result, rrs)
		}
	}
	return result
}
func quote(labels ...string) []string {
	rrdatas := make([]string, 0, len(labels))
	for _, label := range labels {
		rrdatas = append(rrdatas, "\""+label+"\"")
	}
	return rrdatas
}

func TestEmptyCurrentRecordGroups(t *testing.T) {
	a := assert.New(t)
	gc := &GoogleConsumer{
		dnsZones: map[string]struct{}{
			"internal-example-com": struct{}{},
		},
		dnsService: &fakeDNSService{
			projectDNSZones: map[string]string{
				"internal-example-com": "internal.example.org.",
				"external-example-com": "external.example.org.",
			},
			managedZoneRRS: map[string][]*dns.ResourceRecordSet{},
		},
	}
	result, err := gc.currentRecordGroups()
	a.Nil(err)
	a.Empty(result)
}

func TestCurrentRecordGroups(t *testing.T) {
	a := assert.New(t)

	fi := &fakeRecord{dnsName: "internal.example.com.", dnsZone: "internal-example-com", ttl: 400}
	fe := &fakeRecord{dnsName: "external.example.com.", dnsZone: "external-example-com", ttl: 500}
	fs := &fakeRecord{dnsName: "services.example.com.", dnsZone: "services-example-com", ttl: 600}

	gc := &GoogleConsumer{
		dnsTTL: 300,
		dnsZones: map[string]struct{}{
			// buddy managed zones
			"internal-example-com": struct{}{},
			"external-example-com": struct{}{},
		},
		multipleIPRecord: true,
		dnsService: &fakeDNSService{
			projectDNSZones: map[string]string{
				"internal-example-com": "internal.example.org.",
				"external-example-com": "external.example.org.",
				"services-example-com": "services.example.org.",
			},
			managedZoneRRS: map[string][]*dns.ResourceRecordSet{
				"internal-example-com": {
					fi.aRecord("instance-1", "10.132.0.1"),
					fi.txtRecord("instance-1", quote("buddy/europe-west1-c/10.132.0.1")...),
					fi.aRecord("instance-2", "10.132.0.2"),
					fi.aRecord("instance-3", "10.132.0.3"),
					fi.txtRecord("instance-3", quote("buddy/europe-west1-d/10.132.0.3")...),
				},
				"external-example-com": {
					fe.aRecord("instance-4", "10.132.0.4"),
					fe.txtRecord("instance-4", quote("buddy/europe-west1-c/10.132.0.4")...),
					fe.aRecord("instance-5", "10.132.0.51", "10.132.0.52"),
					fe.txtRecord("instance-5", quote("buddy/europe-west1-c/10.132.0.51", "buddy/europe-west1-d/10.132.0.52")...),
				},
				// not managed by buddy
				"services-example-com": {
					fs.aRecord("instance-6", "10.132.0.6"),
				},
			},
		},
	}
	result, err := gc.currentRecordGroups()
	a.Nil(err)
	a.NotEmpty(result)
	a.Equal(5, len(result))

	resultMap := make(map[string]*RecordGroup)
	for _, v := range result {
		resultMap[v.DNSName] = v
	}

	a.EqualValues(resultMap["instance-1.internal.example.com."], fi.recordGroup("instance-1", "10.132.0.1", "buddy/europe-west1-c/10.132.0.1"))
	a.EqualValues(resultMap["instance-2.internal.example.com."], fi.recordGroup("instance-2", "10.132.0.2", ""))
	a.EqualValues(resultMap["instance-3.internal.example.com."], fi.recordGroup("instance-3", "10.132.0.3", "buddy/europe-west1-d/10.132.0.3"))
	a.EqualValues(resultMap["instance-4.external.example.com."], fe.recordGroup("instance-4", "10.132.0.4", "buddy/europe-west1-c/10.132.0.4"))

	rg := resultMap["instance-5.external.example.com."]
	a.EqualValues(rg, &RecordGroup{DNSName: "instance-5.external.example.com.", DNSZone: "external-example-com",
		IPs: []string{"10.132.0.51", "10.132.0.52"}, TTL: 500, Labels: []string{"buddy/europe-west1-c/10.132.0.51", "buddy/europe-west1-d/10.132.0.52"}})

}

func TestCalcDNSZoneChanges(t *testing.T) {
	a := assert.New(t)

	fi := &fakeRecord{dnsName: "internal.example.com.", dnsZone: "internal-example-com", ttl: 400}
	rg1 := fi.recordGroup("instance-1", "10.132.0.1", "buddy/europe-west1-c/10.132.0.1")
	ch1 := fi.aAndTxtRecords("instance-1", []string{"10.132.0.1"}, []string{"buddy/europe-west1-c/10.132.0.1"})

	rg1B := fi.recordGroup("instance-1", "10.132.0.10", "buddy/europe-west1-c/10.132.0.10")
	ch1B := fi.aAndTxtRecords("instance-1", []string{"10.132.0.10"}, []string{"buddy/europe-west1-c/10.132.0.10"})

	rg2 := fi.recordGroup("instance-2", "10.132.0.2", "buddy/europe-west1-c/10.132.0.2")
	ch2 := fi.aAndTxtRecords("instance-2", []string{"10.132.0.2"}, []string{"buddy/europe-west1-c/10.132.0.2"})

	testCases := []struct {
		testName             string
		existingRecordGroups []*RecordGroup
		targetRecordGroups   []*RecordGroup
		dnsZoneChange        []*dnsZoneChange
	}{
		{
			"nothing to do / no records",
			[]*RecordGroup{},
			[]*RecordGroup{},
			[]*dnsZoneChange{},
		},
		{
			"nothing to do / same record",
			[]*RecordGroup{rg1},
			[]*RecordGroup{rg1},
			[]*dnsZoneChange{},
		},
		{
			"nothing to do / same records",
			[]*RecordGroup{rg1, rg2},
			[]*RecordGroup{rg2, rg1},
			[]*dnsZoneChange{},
		},
		{
			"nothing to do / multiple IPs are sorted",
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.2", "10.132.0.1"}, []string{})},
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.1", "10.132.0.2"}, []string{})},
			[]*dnsZoneChange{},
		},
		{
			"add record",
			[]*RecordGroup{},
			[]*RecordGroup{rg1},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Additions: ch1,
					},
				},
			},
		},
		{
			"add records",
			[]*RecordGroup{},
			[]*RecordGroup{rg1, rg2},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Additions: ch1,
					},
				},
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Additions: ch2,
					},
				},
			},
		},
		{
			"add next record",
			[]*RecordGroup{rg1},
			[]*RecordGroup{rg1, rg2},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Additions: ch2,
					},
				},
			},
		},
		{
			"delete record",
			[]*RecordGroup{rg1},
			[]*RecordGroup{},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch1,
					},
				},
			},
		},
		{
			"delete records",
			[]*RecordGroup{rg1, rg2},
			[]*RecordGroup{},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch1,
					},
				},
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch2,
					},
				},
			},
		},
		{
			"delete previous record",
			[]*RecordGroup{rg1, rg2},
			[]*RecordGroup{rg2},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch1,
					},
				},
			},
		},
		{
			"modify record",
			[]*RecordGroup{rg1},
			[]*RecordGroup{rg1B},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch1,
						Additions: ch1B,
					},
				},
			},
		},
		{
			"modify record from list",
			[]*RecordGroup{rg1, rg2},
			[]*RecordGroup{rg2, rg1B},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: ch1,
						Additions: ch1B,
					},
				},
			},
		},
		{
			"add IP to record",
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.1"}, []string{"buddy/europe-west1-c/10.132.0.1"})},
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.1", "10.132.0.2"}, []string{"buddy/europe-west1-c/10.132.0.1", "buddy/europe-west1-c/10.132.0.2"})},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: fi.aAndTxtRecords("instance-1", []string{"10.132.0.1"}, []string{"buddy/europe-west1-c/10.132.0.1"}),
						Additions: fi.aAndTxtRecords("instance-1", []string{"10.132.0.1", "10.132.0.2"}, []string{"buddy/europe-west1-c/10.132.0.1", "buddy/europe-west1-c/10.132.0.2"}),
					},
				},
			},
		},
		{
			"delete IP from record",
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.1", "10.132.0.2"}, []string{"buddy/europe-west1-c/10.132.0.1", "buddy/europe-west1-c/10.132.0.2"})},
			[]*RecordGroup{fi.multiRecordGroup("instance-1", []string{"10.132.0.1"}, []string{"buddy/europe-west1-c/10.132.0.1"})},
			[]*dnsZoneChange{
				{dnsZone: "internal-example-com",
					change: &dns.Change{
						Deletions: fi.aAndTxtRecords("instance-1", []string{"10.132.0.1", "10.132.0.2"}, []string{"buddy/europe-west1-c/10.132.0.1", "buddy/europe-west1-c/10.132.0.2"}),
						Additions: fi.aAndTxtRecords("instance-1", []string{"10.132.0.1"}, []string{"buddy/europe-west1-c/10.132.0.1"}),
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			result := calcDNSZoneChanges(tc.existingRecordGroups, tc.targetRecordGroups)
			if !a.EqualValues(tc.dnsZoneChange, result) {
				t.Fail()
			}
		})
	}
}
