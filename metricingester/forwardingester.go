package metricingester

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

type ForwardingIngester struct {
	discoverer  discoverer
	connections map[string]net.Conn
	dialer      *net.Dialer
}

type discoverer interface {
	Get(name string) (string, error)
}

func (f ForwardingIngester) Ingest(ctx context.Context, m Metric) error {
	// TODO(clin): Consolidate hash/key functions into one "signature" function that
	// encapsulates mixed histo weirdness.
	var hash metricHash
	if m.metricType == mixedHistogram {
		hash = m.MixedHash()
	} else {
		hash = m.Hash()
	}

	hostport, err := f.discoverer.Get(strconv.Itoa(int(hash)))
	if err != nil {
		return errors.WithMessage(
			err, fmt.Sprintf("failed finding host to forward metric to: %v", m),
		)
	}

	conn, err := f.getConn(ctx, hostport)
	if err != nil {
		return errors.WithMessage(
			err, fmt.Sprintf("failed finding host to forward metric to: %v", m),
		)
	}

	_, err = protocol.WriteSSF(conn, &ssf.SSFSpan{Metrics: []*ssf.SSFSample{toSSF(m)}})
	if err != nil {
		return err
	}
	return nil
}

func (f ForwardingIngester) getConn(ctx context.Context, hostport string) (net.Conn, error) {
	conn := f.connections[hostport]
	if conn != nil {
		return conn, nil
	}

	conn, err := f.dialer.DialContext(ctx, "udp", hostport)
	if err != nil {
		return nil, errors.WithMessage(
			err, fmt.Sprintf("failed dialing udp connection to %v", hostport),
		)
	}

	// we don't want cached connections to grow unbounded.
	//
	// we could use an LRU... but this is easy and more than good enough.
	if len(f.connections) > 1024 {
		f.connections = make(map[string]net.Conn, 1024)
	}

	f.connections[hostport] = conn
	return conn, nil
}

func toSSF(m Metric) *ssf.SSFSample {
	tags := tagsToMap(m.tags)
	switch m.metricType {
	case gauge:
		return ssf.Gauge(m.name, float32(m.gaugevalue), tags, ssf.SampleRate(m.samplerate))
	case counter:
		return ssf.Count(m.name, float32(m.countervalue), tags, ssf.SampleRate(m.samplerate))
	case mixedHistogram:
		// the default behavior is mixed histogram currently, so a mixed histo maps
		// to the "HISTOGRAM" ssf type.
		return ssf.Histogram(m.name, float32(m.histovalue), tags, ssf.SampleRate(m.samplerate))
	case histogram:
		h := ssf.Histogram(m.name, float32(m.histovalue), tags, ssf.SampleRate(m.samplerate))
		h.Metric = ssf.SSFSample_GLOBAL_HISTOGRAM
		return h
	case set:
		return ssf.Set(m.name, m.setvalue, tags, ssf.SampleRate(m.samplerate))
	case statusCheck:
		return ssf.Status(m.name, ssf.SSFSample_Status(m.statusValue), tags, ssf.SampleRate(m.samplerate))
	}
}

func tagsToMap(ts []string) map[string]string {
	result := make(map[string]string, len(ts))
	for _, t := range ts {
		splits := strings.Split(t, ":")
		if len(splits) != 2 {
			// for now, we ignore incorrectly formatted tags.
			//
			// moving forward, we should enforce this at a higher level,
			// like when Metrics are created.
			continue
		}
		result[splits[0]] = splits[1]
	}
	return result
}
