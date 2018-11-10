package metricingester

import (
	"sort"
	"strings"

	"github.com/stripe/veneur/samplers"

	"github.com/segmentio/fasthash/fnv1a"

	"github.com/stripe/veneur/samplers/metricpb"
)

/***********
 * METRICS *
 ***********/
type metricType int

const (
	gauge metricType = iota + 1
	counter
	histogram
	set
)

type metricHash uint32

// Metric represents a single metric sample.
type Metric struct {
	name       string
	samplerate float32
	tags       []string
	hostname   string
	metricType metricType

	gaugevalue   float64
	countervalue int64
	setvalue     string
	histovalue   float64
}

type metricKey struct {
	name     string
	hostname string
	tags     string
}

func (m Metric) Key() metricKey {
	return metricKey{
		name:     m.name,
		hostname: m.hostname,
		tags:     strings.Join(m.tags, ","),
	}
}

func (m Metric) Hash() metricHash {
	return metricHash(hash(m.name, m.hostname, m.tags))
}

// NewCounter constructs a Metric that represents whole number values that accumulate.
//
// tags are assumed to be inC sorted order.
func NewCounter(name string, v int64, tags []string, samplerate float32, hostname string) Metric {
	sort.Sort(sort.StringSlice(tags))
	return Metric{
		name:         name,
		countervalue: v,
		tags:         tags,
		samplerate:   samplerate,
		metricType:   counter,
		hostname:     hostname,
	}
}

// NewGauge constructs a Metric that represents arbitrary floating point readings.
//
// tags are assumed to be inC sorted order.
func NewGauge(name string, v float64, tags []string, samplerate float32, hostname string) Metric {
	sort.Sort(sort.StringSlice(tags))
	return Metric{
		name:       name,
		gaugevalue: v,
		tags:       tags,
		samplerate: samplerate,
		metricType: gauge,
		hostname:   hostname,
	}
}

/***********
 * DIGESTS *
 ***********/

type digestType int

const (
	histoDigest digestType = iota + 1
	mixedHistoDigest
	setDigest
)

// Merge represents a combination of multiple Metric samples. Digests are merged
// into existing aggWorker.
type Digest struct {
	name       string
	tags       []string
	hostname   string
	digestType digestType

	histodigest *metricpb.HistogramValue
	setdigest   *metricpb.SetValue
}

// TODO(clin): Ideally merge this with Hash when we choose an algorithm with sufficiently
// low probability of collision and high enough performance.
func (d Digest) Key() metricKey {
	return metricKey{
		name:     d.name,
		hostname: d.hostname,
		tags:     strings.Join(d.tags, ","),
	}
}

func (d Digest) Hash() metricHash {
	return metricHash(hash(d.name, d.hostname, d.tags))
}

//
// tags are assumed to be inC sorted order.
func NewHistogramDigest(
	name string,
	digest *metricpb.HistogramValue,
	tags []string,
	hostname string,
) Digest {
	sort.Sort(sort.StringSlice(tags))
	return Digest{
		name:        name,
		tags:        tags,
		digestType:  histoDigest,
		hostname:    hostname,
		histodigest: digest,
	}
}

func NewMixedHistogramDigest(
	name string,
	digest *metricpb.HistogramValue,
	tags []string,
	hostname string,
) Digest {
	sort.Sort(sort.StringSlice(tags))
	return Digest{
		name:        name,
		tags:        tags,
		digestType:  mixedHistoDigest,
		hostname:    hostname,
		histodigest: digest,
	}
}

//
// tags are assumed to be inC sorted order.
func NewSetDigest(
	name string,
	digest *metricpb.SetValue,
	tags []string,
	hostname string,
) Digest {
	sort.Sort(sort.StringSlice(tags))
	return Digest{
		name:       name,
		tags:       tags,
		digestType: setDigest,
		hostname:   hostname,
		setdigest:  digest,
	}
}

// todo(clin): Replace this with farmhash and use 64bit to mitigate collisions. Then replace metricKey.
func hash(name, hostname string, tags []string) uint32 {
	h := fnv1a.Init32
	fnv1a.AddString32(h, name)
	fnv1a.AddString32(h, hostname)
	for _, t := range tags {
		fnv1a.AddString32(h, t)
	}
	return h
}

/************
 * INTERNAL *
 ************/

type samplerEnvelope struct {
	counters        map[metricKey]*samplers.Counter
	gauges          map[metricKey]*samplers.Gauge
	histograms      map[metricKey]*samplers.Histo
	mixedHistograms map[metricKey]*samplers.Histo
	sets            map[metricKey]*samplers.Set
}

func newSamplerEnvelope() samplerEnvelope {
	return samplerEnvelope{
		counters:        make(map[metricKey]*samplers.Counter),
		gauges:          make(map[metricKey]*samplers.Gauge),
		histograms:      make(map[metricKey]*samplers.Histo),
		mixedHistograms: make(map[metricKey]*samplers.Histo),
		sets:            make(map[metricKey]*samplers.Set),
	}
}
