package pkg

import (
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	DefaultBuddyLabelPrefix = "buddy"
)

// GoogleConfig provides configuration of google producer and consumer
var GoogleConfig struct {
	Project           string
	Zone              string
	Region            string
	ExternalIPDNSZone string
	InternalIPDNSZone string
	DNSTTL            int64
	// additional zones not configured by ExternalIPDNSZone and InternalIPDNSZone
	DNSZones         string
	MultipleIPRecord bool
	BuddyLabelPrefix string
}

func init() {
	kingpin.Flag("google-project", "Project ID that manages the zone").StringVar(&GoogleConfig.Project)
	kingpin.Flag("google-zone", "Name of the google compute zone to manage").StringVar(&GoogleConfig.Zone)
	kingpin.Flag("google-region", "Name of the google compute region to manage").StringVar(&GoogleConfig.Region)
	kingpin.Flag("external-ip-dns-zone", "Default DNS managed zone name for external IPs").StringVar(&GoogleConfig.ExternalIPDNSZone)
	kingpin.Flag("internal-ip-dns-zone", "Default DNS managed zone name for internal IPs").StringVar(&GoogleConfig.InternalIPDNSZone)
	kingpin.Flag("dns-ttl", "TTL in seconds for managed DNS resource records").Default("300").Int64Var(&GoogleConfig.DNSTTL)
	kingpin.Flag("dns-zones", "Comma separated names of DNS managed zones").StringVar(&GoogleConfig.DNSZones)
	kingpin.Flag("multiple-ip-record", "Allow multiple IP addresses in A record").Default("true").BoolVar(&GoogleConfig.MultipleIPRecord)
	kingpin.Flag("buddy-label-prefix", "Prefix used in TXT records").Default(DefaultBuddyLabelPrefix).StringVar(&GoogleConfig.BuddyLabelPrefix)
}
