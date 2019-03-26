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

// MetricScopes holds the scopes that are configured for each metric
// type.
type MetricScopes struct {
	Gauge     ssf.SSFSample_Scope
	Count     ssf.SSFSample_Scope
	Histogram ssf.SSFSample_Scope
}

type scopedStatsd struct {
	client *statsd.Client

	addTags []string
	scopes  MetricScopes
}

var _ Client = &scopedStatsd{}

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

func (s *scopedStatsd) Gauge(name string, value float64, tags []string, rate float64) error {
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Gauge)
	return s.client.Gauge(name, value, tags, rate)
}

func (s *scopedStatsd) Count(name string, value int64, tags []string, rate float64) error {
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Count)
	return s.client.Count(name, value, tags, rate)
}

func (s *scopedStatsd) Incr(name string, tags []string, rate float64) error {
	return s.client.Count(name, 1, tags, rate)
}

func (s *scopedStatsd) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.TimeInMilliseconds(name, value, tags, rate)
}

func (s *scopedStatsd) Timing(name string, value time.Duration, tags []string, rate float64) error {
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.Timing(name, value, tags, rate)
}

func (s *scopedStatsd) Histogram(name string, value float64, tags []string, rate float64) error {
	tags = append(tags, s.addTags...)
	tags = addScopeTag(tags, s.scopes.Histogram)
	return s.client.Histogram(name, value, tags, rate)
}

func NewClient(inner *statsd.Client, addTags []string, scopes MetricScopes) Client {
	return &scopedStatsd{
		client:  inner,
		addTags: addTags,
		scopes:  scopes,
	}
}
