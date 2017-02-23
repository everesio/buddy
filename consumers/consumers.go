package consumers

import (
	"fmt"
	"github.com/everesio/buddy/pkg"
)

// Consumer consumer provided endpoints
type Consumer interface {
	Sync(computeZones []string, endpoints []*pkg.Endpoint) error
	Records(computeZones []string) (interface{}, error)
}

// New creates A new producer
func New(name string) (Consumer, error) {
	switch name {
	case "google":
		return NewGoogleConsumer()
	}
	return nil, fmt.Errorf("Unknown consumer '%s'", name)
}
