package distconf

import (
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/logkey"
	"sync"
	"sync/atomic"
	"time"
)

// DurationWatch is executed if registered on a Duration variable any time the contents change
type DurationWatch func(duration *Duration, oldValue time.Duration)

type durationConf struct {
	Duration
	defaultVal time.Duration
	logger     log.Logger
}

// Duration is a duration type config inside a Config.
type Duration struct {
	watches []DurationWatch

	// Lock on watches so updates are atomic
	mutex      sync.Mutex
	currentVal int64
}

// Get the string in this config variable
func (s *Duration) Get() time.Duration {
	return time.Duration(atomic.LoadInt64(&s.currentVal))
}

// Update the contents of Duration to the new value
func (s *durationConf) Update(newValue []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	oldValue := s.Get()
	if newValue == nil {
		atomic.StoreInt64(&s.currentVal, int64(s.defaultVal))
	} else {
		newValDuration, err := time.ParseDuration(string(newValue))
		if err != nil {
			s.logger.Log(log.Err, err, logkey.DistconfNewVal, string(newValue), "Invalid duration string")
			atomic.StoreInt64(&s.currentVal, int64(s.defaultVal))
		} else {
			atomic.StoreInt64(&s.currentVal, int64(newValDuration))
		}
	}
	if oldValue != s.Get() {
		for _, w := range s.watches {
			w(&s.Duration, oldValue)
		}
	}

	return nil
}

// Watch adds a watch for changes to this structure
func (s *Duration) Watch(watch DurationWatch) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.watches = append(s.watches, watch)
}

func (s *durationConf) GenericGet() interface{} {
	return s.Get()
}

func (s *durationConf) GenericGetDefault() interface{} {
	return s.defaultVal.String()
}

func (s *durationConf) Type() DistType {
	return DurationType
}
