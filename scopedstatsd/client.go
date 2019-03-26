package scopedstatsd

import (
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stripe/veneur/ssf"
)

// StatsdClient represents the statsd client functions that veneur's
// SSF sinks call (so as not to write-amplify by reporting its own
// metrics).
type Client interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Incr(name string, tags []string, rate float64) error
	Histogram(name string, value float64, tags []string, rate float64) error
	TimeInMilliseconds(name string, value float64, tags []string, rate float64) error
	Timing(name string, value time.Duration, tags []string, rate float64) error
}

// Ensure takes a statsd client and wraps it in such a way that it is
// safe to store in a struct if it should be nil. Otherwise returns
// the Client unchanged.
func Ensure(cl Client) Client {
	if cl == nil {
		return &ScopedClient{}
	}
	return cl
}

// MetricScopes holds the scopes that are configured for each metric
// type.
type MetricScopes struct {
	Gauge     ssf.SSFSample_Scope
	Count     ssf.SSFSample_Scope
	Histogram ssf.SSFSample_Scope
}

type ScopedClient struct {
	client *statsd.Client

	addTags []string
	scopes  MetricScopes
}

var _ Client = &ScopedClient{}

func addScopeTag(tags []string, scope ssf.SSFSample_Scope) []string {
	if scope == ssf.SSFSample_DEFAULT {
		return tags
	}
	switch scope {
	case ssf.SSFSample_LOCAL:
		return append(tags, "veneurlocalonly:true")
	case ssf.SSFSample_GLOBAL:
		return append(tags, "veneurglobalonly:true")
	}
	return tags
}

func (s *ScopedClient) Gauge(name string, value float64, tags []string, rate float64) error {
	if s == nil {
		return nil
	}
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Gauge)
	return s.client.Gauge(name, value, tags, rate)
}

func (s *ScopedClient) Count(name string, value int64, tags []string, rate float64) error {
	if s == nil {
		return nil
	}
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Count)
	return s.client.Count(name, value, tags, rate)
}

func (s *ScopedClient) Incr(name string, tags []string, rate float64) error {
	if s == nil {
		return nil
	}

	return s.Count(name, 1, tags, rate)
}

func (s *ScopedClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	if s == nil {
		return nil
	}
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.TimeInMilliseconds(name, value, tags, rate)
}

func (s *ScopedClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	if s == nil {
		return nil
	}
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.Timing(name, value, tags, rate)
}

func (s *ScopedClient) Histogram(name string, value float64, tags []string, rate float64) error {
	if s == nil {
		return nil
	}
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.Histogram(name, value, tags, rate)
}

func NewClient(inner *statsd.Client, addTags []string, scopes MetricScopes) *ScopedClient {
	return &ScopedClient{
		client:  inner,
		addTags: addTags,
		scopes:  scopes,
	}
}
