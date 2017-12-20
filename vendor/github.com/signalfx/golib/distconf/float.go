package distconf

import (
	"math"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/signalfx/golib/errors"
)

// FloatWatch is called on any changes to a register integer config variable
type FloatWatch func(float *Float, oldValue float64)

type floatConf struct {
	Float
	defaultVal float64
}

// Float is an float type config inside a Config.
type Float struct {
	mutex   sync.Mutex
	watches []FloatWatch
	// Lock on update() so watches are called correctly
	// store as uint64 and convert on way in and out for atomicity
	currentVal uint64
}

// Get the float in this config variable
func (c *Float) Get() float64 {
	return math.Float64frombits(atomic.LoadUint64(&c.currentVal))
}

// Update the content of this config variable to newValue.
func (c *floatConf) Update(newValue []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	oldValue := c.Get()
	if newValue == nil {
		atomic.StoreUint64(&c.currentVal, math.Float64bits(c.defaultVal))
	} else {
		newValueFloat, err := strconv.ParseFloat(string(newValue), 64)
		if err != nil {
			return errors.Annotatef(err, "unable to parse float %s", newValue)
		}
		atomic.StoreUint64(&c.currentVal, math.Float64bits(newValueFloat))
	}
	if oldValue != c.Get() {
		for _, w := range c.watches {
			w(&c.Float, oldValue)
		}
	}

	return nil
}

func (c *floatConf) GenericGet() interface{} {
	return c.Get()
}

// Watch for changes to this variable.
func (c *Float) Watch(watch FloatWatch) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.watches = append(c.watches, watch)
}

func (c *floatConf) GenericGetDefault() interface{} {
	return c.defaultVal
}

func (c *floatConf) Type() DistType {
	return FloatType
}
