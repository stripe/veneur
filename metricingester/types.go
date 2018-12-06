package metricingester

import (
	"fmt"
	"sort"
	"strings"

	"github.com/segmentio/fasthash/fnv1a"
	"github.com/stripe/veneur/samplers"
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
	mixedHistogram
	set
	statusCheck
)

type metricHash uint32

// Metric represents a single metric sample.
type Metric struct {
	// metadata
	name       string
	samplerate float32
	tags       []string
	hostname   string

	// type
	metricType metricType

	// values
	gaugevalue    float64
	countervalue  int64
	setvalue      string
	histovalue    float64
	statusMessage string
	statusValue   float64
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

// MixedKey is the key used to identify mixed histograms, which need to be aggregated
// without consideration of host.
func (m Metric) MixedKey() metricKey {
	return metricKey{
		name: m.name,
		tags: strings.Join(m.tags, ","),
	}
}

func (m Metric) MixedHash() metricHash {
	return metricHash(hash(m.name, "", m.tags))
}

func createMetric(m Metric) Metric {
	sort.Sort(sort.StringSlice(m.tags))
	return m
}

// NewCounter constructs a Metric that represents whole number values that accumulate.
//
// tags are assumed to be inC sorted order.
func NewCounter(name string, v int64, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:         name,
		countervalue: v,
		tags:         tags,
		samplerate:   samplerate,
		metricType:   counter,
		hostname:     hostname,
	})
}

// NewGauge constructs a Metric that represents arbitrary floating point readings.
//
// tags are assumed to be inC sorted order.
func NewGauge(name string, v float64, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:       name,
		gaugevalue: v,
		tags:       tags,
		samplerate: samplerate,
		metricType: gauge,
		hostname:   hostname,
	})
}

// NewStatusCheck constructs a metric that represents a status check.
func NewStatusCheck(name string, value float64, message string, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:          name,
		statusMessage: message,
		statusValue:   value,
		tags:          tags,
		samplerate:    samplerate,
		metricType:    statusCheck,
		hostname:      hostname,
	})
}

// NewHisto constructs a metric that represents a histogram value.
func NewHisto(name string, v float64, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:       name,
		histovalue: v,
		tags:       tags,
		samplerate: samplerate,
		metricType: histogram,
		hostname:   hostname,
	})
}

// NewMixedHisto constructs a metric that represents a mixed histogram value.
func NewMixedHisto(name string, v float64, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:       name,
		histovalue: v,
		tags:       tags,
		samplerate: samplerate,
		metricType: mixedHistogram,
		hostname:   hostname,
	})
}

// NewSet constructs a metric that represents a set value.
func NewSet(name string, v string, tags []string, samplerate float32, hostname string) Metric {
	return createMetric(Metric{
		name:       name,
		setvalue:   v,
		tags:       tags,
		samplerate: samplerate,
		metricType: set,
		hostname:   hostname,
	})
}

/***********
 * DIGESTS *
 ***********/

type digestType int

const (
	histoDigest digestType = iota + 1
	mixedHistoDigest
	mixedGlobalHistoDigest
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

	flushMixed bool
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

// MixedKey is the key used to identify mixed histograms, which need to be aggregated
// without consideration of host.
func (d Digest) MixedKey() metricKey {
	return metricKey{
		name: d.name,
		tags: strings.Join(d.tags, ","),
	}
}

func (d Digest) Hash() metricHash {
	return metricHash(hash(d.name, d.hostname, d.tags))
}

// MixedHash is the hash used to shard mixed histograms, which need to be aggregated
// without consideration of host.
func (d Digest) MixedHash() metricHash {
	return metricHash(hash(d.name, "", d.tags))
}

//
// tags are assumed to be inC sorted order.
func NewHistogramDigest(
	name string,
	digest *metricpb.HistogramValue,
	tags []string,
	hostname string,
) Digest {
	// TODO(clin): Ensure tags are always sorted.
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
	flushMixed bool,
) Digest {
	sort.Sort(sort.StringSlice(tags))
	return Digest{
		name:        name,
		tags:        tags,
		digestType:  mixedHistoDigest,
		hostname:    hostname,
		histodigest: digest,
		flushMixed:  flushMixed,
	}
}

func NewMixedGlobalHistogramDigest(
	name string,
	digest *metricpb.HistogramValue,
	tags []string,
	hostname string,
) Digest {
	sort.Sort(sort.StringSlice(tags))
	return Digest{
		name:        name,
		tags:        tags,
		digestType:  mixedGlobalHistoDigest,
		hostname:    hostname,
		histodigest: digest,
	}
}

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
	mixedHistograms map[metricKey]samplers.MixedHisto
	sets            map[metricKey]*samplers.Set
	statusChecks    map[metricKey]*samplers.StatusCheck

	mixedHosts map[string]struct{}
}

func newSamplerEnvelope() samplerEnvelope {
	return samplerEnvelope{
		counters:        make(map[metricKey]*samplers.Counter),
		gauges:          make(map[metricKey]*samplers.Gauge),
		histograms:      make(map[metricKey]*samplers.Histo),
		mixedHistograms: make(map[metricKey]samplers.MixedHisto),
		sets:            make(map[metricKey]*samplers.Set),
		statusChecks:    make(map[metricKey]*samplers.StatusCheck),

		mixedHosts: make(map[string]struct{}),
	}
}

/***********
 * MAPPING *
 ***********/

func ToMetric(u samplers.UDPMetric) (Metric, error) {
	var f64 float64
	var s string
	switch v := u.Value.(type) {
	case float64:
		f64 = v
	case string:
		s = v
	}

	switch u.Type {
	case "counter":
		return NewCounter(u.Name, int64(f64), u.Tags, u.SampleRate, u.HostName), nil
	case "gauge":
		return NewGauge(u.Name, f64, u.Tags, u.SampleRate, u.HostName), nil
	case "histogram":
		return NewHisto(u.Name, f64, u.Tags, u.SampleRate, u.HostName), nil
	case "timer":
		return NewHisto(u.Name, f64, u.Tags, u.SampleRate, u.HostName), nil
	case "set":
		return NewSet(u.Name, s, u.Tags, u.SampleRate, u.HostName), nil
	case "status":
		return NewStatusCheck(u.Name, f64, u.Message, u.Tags, u.SampleRate, u.HostName), nil
	}
	return Metric{}, fmt.Errorf("invalid UDPMetric type: %+v", u)
}
