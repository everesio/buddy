package producers

import (
	"fmt"
	"github.com/everesio/buddy/pkg"
)

// Producer provides endpoints which should be synchronized
type Producer interface {

	// compute zones which are managed
	ComputeZones() []string

	// all endpoints in the from managed compute zones
	Endpoints() ([]*pkg.Endpoint, error)
}

// New creates a new producer
func New(name string) (Producer, error) {
	switch name {
	case "google":
		return NewGoogleProducer()
	}
	return nil, fmt.Errorf("Unknown producer '%s'", name)
}
