package pkg

import "time"

type Observer interface {
	Observe(float64)
}

type ObserverFunc func(float64)

func (f ObserverFunc) Observe(value float64) {
	f(value)
}

type Timer struct {
	begin    time.Time
	observer Observer
}

func NewTimer(o Observer) *Timer {
	return &Timer{
		begin:    time.Now(),
		observer: o,
	}
}

// Same as prometheus.Timer, but provides duration in millisecond
func (t *Timer) ObserveDuration() {
	if t.observer != nil {
		t.observer.Observe(float64(time.Since(t.begin) / time.Millisecond))
	}
}
