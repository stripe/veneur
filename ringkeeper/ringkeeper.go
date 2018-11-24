// Package ringkeeper maintains a consistent hash ring of values provided by
// a Discoverer that returns a set of strings.
//
// It's intended for service discovery use cases.
package ringkeeper

import (
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"stathat.com/c/consistent"
)

// RingKeeper maintains a consistent hash ring while occasionally updating
// ring values by calling a supplied "discoverer" function.
type RingKeeper struct {
	ring       *consistent.Consistent
	discoverer func() ([]string, error)

	ticker *time.Ticker
	tC     chan time.Time

	log *logrus.Logger
}

type ringOption func(keeper *RingKeeper)

func OptTicker(tckr chan time.Time) ringOption {
	return func(option *RingKeeper) {
		option.tC = tckr
	}
}

func OptLogger(logger *logrus.Logger) ringOption {
	return func(option *RingKeeper) {
		option.log = logger
	}
}

func NewRingKeeper(discoverer func() ([]string, error), interval time.Duration, opts ...ringOption) RingKeeper {
	r := RingKeeper{
		ring:       consistent.New(),
		discoverer: discoverer,
		ticker:     time.NewTicker(interval),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

func (d RingKeeper) Start() {
	if err := d.refresh(); err != nil {
		d.log.WithError(err).Error("failed first dns request")
	}
	go func() {
		for range d.tC {
			if err := d.refresh(); err != nil {
				d.log.WithError(err).Error("failed dns request")
			}
		}
	}()
}

func (d RingKeeper) Stop() {
	d.ticker.Stop()
	close(d.tC)
}

func (d RingKeeper) Get(name string) (string, error) {
	return d.ring.Get(name)
}

func (d RingKeeper) refresh() error {
	hostports, err := d.discoverer()
	if err != nil {
		return errors.WithMessage(err, "failed performing RingKeeper discovery")
	}
	d.ring.Set(hostports)
}
