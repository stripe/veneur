package metricingester

import (
	"context"
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
)

func NewForwardingIngester(connecter Connecter) ForwardingIngester {
	return ForwardingIngester{
		connecter: connecter,
	}
}

type ForwardingIngester struct {
	connecter Connecter
}

type Connecter interface {
	// Conn attempts to retrieve a connection for this hash
	Conn(ctx context.Context, hash string) (net.Conn, error)
	// Return a connection in the event of error.
	//
	// Some errors require throwing away the connection.
	Return(net.Conn, error)
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

	conn, err := f.connecter.Conn(ctx, strconv.Itoa(int(hash)))
	if err != nil {
		return errors.WithMessage(
			err, "failed retrieving connection to forward metrics to",
		)
	}

	_, err = protocol.WriteSSF(
		conn,
		&ssf.SSFSpan{Metrics: []*ssf.SSFSample{toSSF(m)}},
	)
	f.connecter.Return(conn, err)
	return nil
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
	return nil
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
