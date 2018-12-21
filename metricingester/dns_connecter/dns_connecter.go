package dns_connecter

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/stripe/veneur/metricingester"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"stathat.com/c/consistent"
)

var _ metricingester.Connecter = StripeDNSUDP{}

// StripeDNSUDP is a connecter designed for Stripe's SRV record based DNS discovery and UDP usage.
type StripeDNSUDP struct {
	service    string
	ticker     *time.Ticker
	tC         <-chan time.Time
	dialer     *net.Dialer
	discoverer func() ([]string, error)
	ring       *consistent.Consistent

	log *logrus.Logger
}

type option func(dnsudp *StripeDNSUDP)

func OptTickerChan(ch <-chan time.Time) option {
	return func(dnsudp *StripeDNSUDP) {
		dnsudp.tC = ch
	}
}

func OptDiscoverer(d func() ([]string, error)) option {
	return func(dnsudp *StripeDNSUDP) {
		dnsudp.discoverer = d
	}
}

func NewStripeDNSUDP(interval time.Duration, service string, opts ...option) StripeDNSUDP {
	tckr := time.NewTicker(interval)
	s := StripeDNSUDP{
		service:    service,
		ring:       consistent.New(),
		discoverer: func() ([]string, error) { return dnssrv(service) },
		dialer:     &net.Dialer{},
		ticker:     tckr,
		tC:         tckr.C,
	}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

func (d StripeDNSUDP) Start() {
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

func (d StripeDNSUDP) Stop() {
	d.ticker.Stop()
}

func (d StripeDNSUDP) refresh() error {
	hostports, err := d.discoverer()
	if err != nil {
		return errors.WithMessage(err, "failed performing RingKeeper discovery")
	}
	d.ring.Set(hostports)
	return nil
}

// Conn attempts to retrieve a connection for this hash
func (d StripeDNSUDP) Conn(ctx context.Context, hash string) (net.Conn, error) {
	url, err := d.ring.Get(hash)
	if err != nil {
		return nil, errors.WithMessage(err, "error finding url")
	}
	return d.dialer.DialContext(ctx, "udp", url)
}

// Error is a noop because UDP connections are always reset anyway!
func (d StripeDNSUDP) Return(net.Conn, error) {
}

// dnssrv uses a DNS SRV request to discover services.
func dnssrv(service string) ([]string, error) {
	_, addrs, err := net.LookupSRV("", "", service)
	if err != nil {
		return nil, errors.WithMessage(err, "failed performing dnssrv discovery")
	}

	var hostports []string
	for _, addr := range addrs {
		hostports = append(
			hostports,
			addr.Target[:len(addr.Target)-1]+":"+strconv.Itoa(int(addr.Port)),
		)
	}
	return hostports, nil
}
