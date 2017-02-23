package pkg

// Endpoint is used to pass data from the producer to the consumer.
type Endpoint struct {

	// name name in the zone
	Hostname string `json:"hostname"`

	// Google Cloud DNS zone name to be used by the consumer for the record.
	DNSZone string `json:"dnsZone"`

	// IPv4 address.
	IP string `json:"ip"`

	// Compute engine zone
	ComputeZone string `json:"computeZone"`
}
