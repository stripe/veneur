package distconf

import (
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/signalfx/golib/errors"
)

// IntWatch is called on any changes to a register integer config variable
type IntWatch func(str *Int, oldValue int64)

type intConf struct {
	Int
	defaultVal int64
}

// Int is an integer type config inside a Config.
type Int struct {
	mutex   sync.Mutex
	watches []IntWatch
	// Lock on update() so watches are called correctly
	currentVal int64
}

// Get the integer in this config variable
func (c *Int) Get() int64 {
	return atomic.LoadInt64(&c.currentVal)
}

// Update the content of this config variable to newValue.
func (c *intConf) Update(newValue []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	oldValue := c.Get()
	if newValue == nil {
		atomic.StoreInt64(&c.currentVal, c.defaultVal)
	} else {
		newValueInt, err := strconv.ParseInt(string(newValue), 10, 64)
		if err != nil {
			return errors.Annotatef(err, "Unparsable float %s", string(newValue))
		}
		atomic.StoreInt64(&c.currentVal, newValueInt)
	}
	if oldValue != c.Get() {
		for _, w := range c.watches {
			w(&c.Int, oldValue)
		}
	}

	return nil
}

func (c *intConf) GenericGet() interface{} {
	return c.Get()
}

// Watch for changes to this variable.
func (c *Int) Watch(watch IntWatch) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.watches = append(c.watches, watch)
}

func (c *intConf) GenericGetDefault() interface{} {
	return c.defaultVal
}

func (c *intConf) Type() DistType {
	return IntType
}
