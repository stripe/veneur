package distconf

import (
	"sync"
	"sync/atomic"
)

// StrWatch is executed if registered on a Str variable any time the Str contents change
type StrWatch func(str *Str, oldValue string)

type strConf struct {
	Str
	defaultVal string
}

// Str is a string type config inside a Config.
type Str struct {
	watches []StrWatch

	// Lock on watches so updates are atomic
	mutex      sync.Mutex
	currentVal atomic.Value
}

// Update the contents of Str to the new value
func (s *strConf) Update(newValue []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	oldValue := s.currentVal.Load().(string)
	if newValue == nil {
		s.currentVal.Store(s.defaultVal)
	} else {
		s.currentVal.Store(string(newValue))
	}
	if oldValue != s.Get() {
		for _, w := range s.watches {
			w(&s.Str, oldValue)
		}
	}
	return nil
}

// Get the string in this config variable
func (s *Str) Get() string {
	return s.currentVal.Load().(string)
}

func (s *strConf) GenericGet() interface{} {
	return s.Get()
}

// Watch adds a watch for changes to this structure
func (s *Str) Watch(watch StrWatch) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.watches = append(s.watches, watch)
}

func (s *strConf) GenericGetDefault() interface{} {
	return s.defaultVal
}

func (s *strConf) Type() DistType {
	return StrType
}
