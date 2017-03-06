package consumers

import (
	"github.com/everesio/buddy/pkg"
	"sync"
)

type SyncedConsumer struct {
	sync.Mutex
	Consumer
}

func NewSynced(name string) (Consumer, error) {
	consumer, err := New(name)
	if err != nil {
		return nil, err
	}
	return &SyncedConsumer{Consumer: consumer}, nil
}

func (s *SyncedConsumer) Sync(computeZones []string, endpoints []*pkg.Endpoint) error {
	s.Lock()
	defer s.Unlock()
	return s.Consumer.Sync(computeZones, endpoints)
}

func (s *SyncedConsumer) Records(computeZones []string) (interface{}, error) {
	return s.Consumer.Records(computeZones)
}
