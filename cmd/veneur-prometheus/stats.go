package main

import (
	"sort"

	"github.com/DataDog/datadog-go/statsd"
)

type inMemoryStat interface {
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

func newCount(name string, tags []string, value int64) count {
	return count{statID{name, tags}, value}
}

type count struct {
	statID
	Value int64
}

func (c count) Send(client *statsd.Client) error {
	return client.Count(c.Name, c.Value, c.Tags, 1.0)
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
