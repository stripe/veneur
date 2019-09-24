package main

import (
	"sort"

	"github.com/DataDog/datadog-go/statsd"
)

type inMemoryStat interface {
	Translate(*countCache) statsdStat
}

type statsdStat interface {
	Send(*statsd.Client) error
}

type statID struct {
	Name string
	Tags []string
}

func same(a statID, b statID) bool {
	if a.Name != b.Name {
		return false
	}

	if len(a.Tags) != len(b.Tags) {
		return false
	}

	sort.Strings(a.Tags)
	sort.Strings(b.Tags)

	for i, a := range a.Tags {
		if a != b.Tags[i] {
			return false
		}
	}

	return true
}

func newStatsdCount(name string, tags []string, value int64) statsdCount {
	return statsdCount{statID{name, tags}, value}
}

type statsdCount struct {
	statID
	Value int64
}

func (c statsdCount) Send(client *statsd.Client) error {
	//can skip 0 statsd counters
	if c.Value == 0 {
		return nil
	}

	return client.Count(c.Name, c.Value, c.Tags, 1.0)
}

func newPrometheusCount(name string, tags []string, value int64) prometheusCount {
	return prometheusCount{statID{name, tags}, value}
}

type prometheusCount struct {
	statID
	Value int64
}

func (c prometheusCount) Translate(cache *countCache) statsdStat {
	diffed := c.diff(cache)
	return newStatsdCount(c.Name, c.Tags, diffed)
}

func (c prometheusCount) diff(cache *countCache) int64 {
	cached, has, first := cache.GetAndSwap(c)

	//if this is the first observation cycle the cache has been through
	//then we have _no_ basis for calculations for any metrics dont count
	//the values you see
	if first {
		return 0
	}

	//if we don't have this particular metric, but we do have other it means
	//this metric is new.  you can assume those are part of this sample cycle
	if !has {
		return c.Value
	}

	//if what we have is cached is larger than what we received it means there was a restart
	//on the monitored service,  we know _at least_ the ones it reporting have happened
	//though we may have missed some post our last observation and prior to restart
	if cached.Value > c.Value {
		return c.Value
	}

	//the normal case is we had some last observation cyle, then they grew so we report the diff
	//note that there is an edge case here, that is we saw some, there was a restart and it grew to
	//the value we had before.  in that case we are under counting but we cant determine any difference
	return c.Value - cached.Value
}

func newGauge(name string, tags []string, value float64) gauge {
	return gauge{statID{name, tags}, value}
}

type gauge struct {
	statID
	Value float64
}

func (g gauge) Send(client *statsd.Client) error {
	return client.Gauge(g.Name, g.Value, g.Tags, 1.0)
}

func (g gauge) Translate(_ *countCache) statsdStat {
	return g
}
