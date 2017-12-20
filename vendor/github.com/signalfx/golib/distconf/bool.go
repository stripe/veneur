package distconf

import (
	"strconv"
	"sync"
	"sync/atomic"
)

// BoolWatch is executed if registered on a Bool variable any time the Bool contents change
type BoolWatch func(str *Bool, oldValue bool)

type boolConf struct {
	Bool
	defaultVal int32
}

// Bool is a Boolean type config inside a Config.  It uses strconv.ParseBool to parse the conf
// contents as either true for false
type Bool struct {
	watches []BoolWatch

	// Lock on watches so updates are atomic
	mutex      sync.Mutex
	currentVal int32
}

// Update the contents of Bool to the new value
func (s *boolConf) Update(newValue []byte) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	oldValue := s.Get()
	if newValue == nil {
		atomic.StoreInt32(&s.currentVal, s.defaultVal)
	} else {
		newValueStr := string(newValue)
		if parsedBool, err := strconv.ParseBool(newValueStr); err != nil {
			atomic.StoreInt32(&s.currentVal, s.defaultVal)
		} else if parsedBool {
			atomic.StoreInt32(&s.currentVal, 1)
		} else {
			atomic.StoreInt32(&s.currentVal, 0)
		}
	}
	if oldValue != s.Get() {
		for _, w := range s.watches {
			w(&s.Bool, oldValue)
		}
	}
	return nil
}

// Get the boolean in this config variable
func (s *Bool) Get() bool {
	return atomic.LoadInt32(&s.currentVal) != 0
}

func (s *boolConf) GenericGet() interface{} {
	return s.Get()
}

// Watch adds a watch for changes to this structure
func (s *Bool) Watch(watch BoolWatch) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.watches = append(s.watches, watch)
}

func (s *boolConf) GenericGetDefault() interface{} {
	return s.defaultVal
}

func (s *boolConf) Type() DistType {
	return BoolType
}
