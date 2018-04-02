package samplers

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/veneur/protocol/dogstatsd"
	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/ssf"
)

var invalidMetricTypeError = errors.New("Invalid type for metric")

// UDPMetric is a representation of the sample provided by a client. The tag list
// should be deterministically ordered.
type UDPMetric struct {
	MetricKey
	Digest     uint32
	Value      interface{}
	SampleRate float32
	Tags       []string
	Scope      MetricScope
}

type MetricScope int

const (
	MixedScope MetricScope = iota
	LocalOnly
	GlobalOnly
)

// MetricKey is a struct used to key the metrics into the worker's map. All fields must be comparable types.
type MetricKey struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	JoinedTags string `json:"tagstring"` // tags in deterministic order, joined with commas
}

// NewMetricKeyFromMetric initializes a MetricKey from the protobuf-compatible
// metricpb.Metric
func NewMetricKeyFromMetric(m *metricpb.Metric) MetricKey {
	return MetricKey{
		Name:       m.Name,
		Type:       strings.ToLower(m.Type.String()),
		JoinedTags: strings.Join(m.Tags, ","),
	}
}

// ToString returns a string representation of this MetricKey
func (m MetricKey) String() string {
	var buff bytes.Buffer
	buff.WriteString(m.Name)
	buff.WriteString(m.Type)
	buff.WriteString(m.JoinedTags)
	return buff.String()
}

// ConvertMetrics examines an SSF message, parses and returns a new
// array containing any metrics contained in the message. If any parse
// error occurs in processing any of the metrics, ExtractMetrics
// collects them into the error type InvalidMetrics and returns this
// error alongside any valid metrics that could be parsed.
func ConvertMetrics(m *ssf.SSFSpan) ([]UDPMetric, error) {
	samples := m.Metrics
	metrics := make([]UDPMetric, 0, len(samples)+1)
	invalid := []*ssf.SSFSample{}

	for _, metricPacket := range samples {
		metric, err := ParseMetricSSF(metricPacket)
		if err != nil || !ValidMetric(metric) {
			invalid = append(invalid, metricPacket)
			continue
		}
		metrics = append(metrics, metric)
	}
	if len(invalid) != 0 {
		return metrics, &invalidMetrics{invalid}
	}
	return metrics, nil
}

// ConvertIndicatorMetrics takes a span that may be an "indicator"
// span and returns metrics that can be determined from that
// span. Currently, it converts the span to a timer metric for the
// duration of the span.
func ConvertIndicatorMetrics(span *ssf.SSFSpan, timerName string) (metrics []UDPMetric, err error) {
	if !span.Indicator || timerName == "" {
		// No-op if this isn't an indicator span
		return
	}

	end := time.Unix(span.EndTimestamp/1e9, span.EndTimestamp%1e9)
	start := time.Unix(span.StartTimestamp/1e9, span.StartTimestamp%1e9)
	tags := map[string]string{
		"span_name": span.Name,
		"service":   span.Service,
		"error":     "false",
	}
	if span.Error {
		tags["error"] = "true"
	}
	ssfTimer := ssf.Timing(timerName, end.Sub(start), time.Nanosecond, tags)
	timer, err := ParseMetricSSF(ssfTimer)
	if err != nil {
		return metrics, err
	}
	metrics = append(metrics, timer)
	return metrics, nil
}

// ConvertSpanUniquenessMetrics takes a trace span and computes
// uniqueness metrics about it, returning UDPMetrics sampled at
// rate. Currently, the only metric returned is a Set counting the
// unique names per indicator span/service.
func ConvertSpanUniquenessMetrics(span *ssf.SSFSpan, rate float32) ([]UDPMetric, error) {
	if span.Service == "" {
		return []UDPMetric{}, nil
	}
	ssfMetrics := []*ssf.SSFSample{}
	ssfMetrics = append(ssfMetrics,
		ssf.RandomlySample(rate,
			ssf.Set("ssf.names_unique", span.Name, map[string]string{
				"indicator": strconv.FormatBool(span.Indicator),
				"service":   span.Service,
				"root_span": strconv.FormatBool(span.Id == span.TraceId),
			}))...)
	metrics := []UDPMetric{}
	for _, m := range ssfMetrics {
		udpM, err := ParseMetricSSF(m)
		if err != nil {
			return []UDPMetric{}, err
		}
		metrics = append(metrics, udpM)
	}
	return metrics, nil
}

// ValidMetric takes in an SSF sample and determines if it is valid or not.
func ValidMetric(sample UDPMetric) bool {
	ret := true
	ret = ret && sample.Name != ""
	ret = ret && sample.Value != nil
	return ret
}

// InvalidMetrics is an error type returned if any metric could not be parsed.
type InvalidMetrics interface {
	error

	// Samples returns any samples that couldn't be parsed or validated.
	Samples() []*ssf.SSFSample
}

type invalidMetrics struct {
	samples []*ssf.SSFSample
}

func (err *invalidMetrics) Error() string {
	return fmt.Sprintf("parse errors on %d metrics", len(err.samples))
}

func (err *invalidMetrics) Samples() []*ssf.SSFSample {
	return err.samples
}

// ParseMetricSSF converts an incoming SSF packet to a Metric.
func ParseMetricSSF(metric *ssf.SSFSample) (UDPMetric, error) {
	ret := UDPMetric{
		SampleRate: 1.0,
	}
	h := fnv.New32a()
	h.Write([]byte(metric.Name))
	ret.Name = metric.Name
	switch metric.Metric {
	case ssf.SSFSample_COUNTER:
		ret.Type = "counter"
	case ssf.SSFSample_GAUGE:
		ret.Type = "gauge"
	case ssf.SSFSample_HISTOGRAM:
		ret.Type = "histogram"
	case ssf.SSFSample_SET:
		ret.Type = "set"
	default:
		return UDPMetric{}, invalidMetricTypeError
	}
	h.Write([]byte(ret.Type))
	if metric.Metric == ssf.SSFSample_SET {
		ret.Value = metric.Message
	} else {
		ret.Value = float64(metric.Value)
	}
	ret.SampleRate = metric.SampleRate
	tempTags := make([]string, 0)
	for key, value := range metric.Tags {
		if key == "veneurlocalonly" {
			ret.Scope = LocalOnly
			continue
		}
		if key == "veneurglobalonly" {
			ret.Scope = GlobalOnly
			continue
		}
		tempTags = append(tempTags, fmt.Sprintf("%s:%s", key, value))
	}
	sort.Strings(tempTags)
	ret.Tags = tempTags
	ret.JoinedTags = strings.Join(tempTags, ",")
	h.Write([]byte(ret.JoinedTags))
	ret.Digest = h.Sum32()
	return ret, nil
}

// ParseMetric converts the incoming packet from Datadog DogStatsD
// Datagram format in to a Metric. http://docs.datadoghq.com/guides/dogstatsd/#datagram-format
func ParseMetric(packet []byte) (*UDPMetric, error) {
	ret := &UDPMetric{
		SampleRate: 1.0,
	}
	pipeSplitter := NewSplitBytes(packet, '|')
	pipeSplitter.Next() // first split always succeeds, since there are at least zero pipes

	startingColon := bytes.IndexByte(pipeSplitter.Chunk(), ':')
	if startingColon == -1 {
		return nil, errors.New("Invalid metric packet, need at least 1 colon")
	}
	nameChunk := pipeSplitter.Chunk()[:startingColon]
	valueChunk := pipeSplitter.Chunk()[startingColon+1:]
	if len(nameChunk) == 0 {
		return nil, errors.New("Invalid metric packet, name cannot be empty")
	}

	if !pipeSplitter.Next() {
		return nil, errors.New("Invalid metric packet, need at least 1 pipe for type")
	}
	typeChunk := pipeSplitter.Chunk()
	if len(typeChunk) == 0 {
		// avoid panicking on malformed packets missing a type
		// (eg "foo:1||")
		return nil, errors.New("Invalid metric packet, metric type not specified")
	}

	h := fnv.New32a()
	h.Write(nameChunk)
	ret.Name = string(nameChunk)

	// Decide on a type
	switch typeChunk[0] {
	case 'c':
		ret.Type = "counter"
	case 'g':
		ret.Type = "gauge"
	case 'h':
		ret.Type = "histogram"
	case 'm': // We can ignore the s in "ms"
		ret.Type = "timer"
	case 's':
		ret.Type = "set"
	default:
		return nil, invalidMetricTypeError
	}
	// Add the type to the digest
	h.Write([]byte(ret.Type))

	// Now convert the metric's value
	if ret.Type == "set" {
		ret.Value = string(valueChunk)
	} else {
		v, err := strconv.ParseFloat(string(valueChunk), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("Invalid number for metric value: %s", valueChunk)
		}
		ret.Value = v
	}

	// each of these sections can only appear once in the packet
	foundSampleRate := false
	for pipeSplitter.Next() {
		if len(pipeSplitter.Chunk()) == 0 {
			// avoid panicking on malformed packets that have too many pipes
			// (eg "foo:1|g|" or "foo:1|c||@0.1")
			return nil, errors.New("Invalid metric packet, empty string after/between pipes")
		}
		switch pipeSplitter.Chunk()[0] {
		case '@':
			if foundSampleRate {
				return nil, errors.New("Invalid metric packet, multiple sample rates specified")
			}
			// sample rate!
			sr := string(pipeSplitter.Chunk()[1:])
			sampleRate, err := strconv.ParseFloat(sr, 32)
			if err != nil {
				return nil, fmt.Errorf("Invalid float for sample rate: %s", sr)
			}
			if sampleRate <= 0 || sampleRate > 1 {
				return nil, fmt.Errorf("Sample rate %f must be >0 and <=1", sampleRate)
			}
			ret.SampleRate = float32(sampleRate)
			foundSampleRate = true

		case '#':
			// tags!
			if ret.Tags != nil {
				return nil, errors.New("Invalid metric packet, multiple tag sections specified")
			}
			tags := strings.Split(string(pipeSplitter.Chunk()[1:]), ",")
			sort.Strings(tags)
			for i, tag := range tags {
				// we use this tag as an escape hatch for metrics that always
				// want to be host-local
				if tag == "veneurlocalonly" {
					// delete the tag from the list
					tags = append(tags[:i], tags[i+1:]...)
					ret.Scope = LocalOnly
					break
				} else if tag == "veneurglobalonly" {
					// delete the tag from the list
					tags = append(tags[:i], tags[i+1:]...)
					ret.Scope = GlobalOnly
					break
				}
			}
			ret.Tags = tags
			// we specifically need the sorted version here so that hashing over
			// tags behaves deterministically
			ret.JoinedTags = strings.Join(tags, ",")
			h.Write([]byte(ret.JoinedTags))

		default:
			return nil, fmt.Errorf("Invalid metric packet, contains unknown section %q", pipeSplitter.Chunk())
		}
	}

	ret.Digest = h.Sum32()

	return ret, nil
}

// ParseEvent parses a DogStatsD event packet and returns an SSF sample or an
// error on failure. To facilitate the many Datadog-specific values that are
// present in a DogStatsD event but not in an SSF sample, a series of special
// tags are set as defined in protocol/dogstatsd/protocol.go. Any sink that wants
// to consume these events will then need to implement FlushOtherSamples and
// unwind these special tags into whatever is appropriate for that sink.
func ParseEvent(packet []byte) (*ssf.SSFSample, error) {

	ret := &ssf.SSFSample{
		Timestamp: time.Now().Unix(),
		Tags:      map[string]string{dogstatsd.EventIdentifierKey: ""},
	}

	pipeSplitter := NewSplitBytes(packet, '|')
	pipeSplitter.Next()

	startingColon := bytes.IndexByte(pipeSplitter.Chunk(), ':')
	if startingColon == -1 {
		return nil, errors.New("Invalid event packet, need at least 1 colon")
	}

	lengthsChunk := pipeSplitter.Chunk()[:startingColon]
	// the second half of the condition will never panic, because the first half
	// guarantees that it has a nonzero length
	if !bytes.HasPrefix(lengthsChunk, []byte{'_', 'e', '{'}) || lengthsChunk[len(lengthsChunk)-1] != '}' {
		return nil, errors.New("Invalid event packet, must have _e{} wrapper around length section")
	}
	// discard the _e{} wrapper
	lengthsChunk = lengthsChunk[3 : len(lengthsChunk)-1]

	lengthComma := bytes.IndexByte(lengthsChunk, ',')
	if lengthComma == -1 {
		return nil, errors.New("Invalid event packet, length section requires comma divider")
	}

	titleExpectedLength, err := strconv.Atoi(string(lengthsChunk[:lengthComma]))
	if err != nil {
		return nil, fmt.Errorf("Invalid event packet, title length is not an integer: %s", err)
	}
	if titleExpectedLength <= 0 {
		return nil, errors.New("Invalid event packet, title length must be positive")
	}

	textExpectedLength, err := strconv.Atoi(string(lengthsChunk[lengthComma+1:]))
	if err != nil {
		return nil, fmt.Errorf("Invalid event packet, text length is not an integer: %s", err)
	}
	if textExpectedLength <= 0 {
		return nil, errors.New("Invalid event packet, text length must be positive")
	}

	titleChunk := pipeSplitter.Chunk()[startingColon+1:]
	if len(titleChunk) != titleExpectedLength {
		return nil, errors.New("Invalid event packet, actual title length did not match encoded length")
	}
	ret.Name = string(titleChunk)

	if !pipeSplitter.Next() {
		return nil, errors.New("Invalid event packet, must have at least 1 pipe for text")
	}
	textChunk := pipeSplitter.Chunk()
	if len(textChunk) != textExpectedLength {
		return nil, errors.New("Invalid event packet, actual text length did not match encoded length")
	}
	ret.Message = strings.Replace(string(textChunk), "\\n", "\n", -1)

	var (
		foundTimestamp   bool
		foundHostname    bool
		foundAggregation bool
		foundPriority    bool
		foundSource      bool
		foundAlert       bool
		foundTags        bool
	)
	for pipeSplitter.Next() {
		if len(pipeSplitter.Chunk()) == 0 {
			return nil, errors.New("Invalid event packet, empty string after/between pipes")
		}

		switch {
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'d', ':'}):
			if foundTimestamp {
				return nil, errors.New("Invalid event packet, multiple date sections")
			}
			unixTimestamp, err := strconv.ParseInt(string(pipeSplitter.Chunk()[2:]), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Invalid event packet, could not parse date as unix timestamp: %s", err)
			}
			ret.Timestamp = unixTimestamp
			foundTimestamp = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'h', ':'}):
			if foundHostname {
				return nil, errors.New("Invalid event packet, multiple hostname sections")
			}
			ret.Tags[dogstatsd.EventHostnameTagKey] = string(pipeSplitter.Chunk()[2:])
			foundHostname = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'k', ':'}):
			if foundAggregation == true {
				return nil, errors.New("Invalid event packet, multiple aggregation key sections")
			}
			ret.Tags[dogstatsd.EventAggregationKeyTagKey] = string(pipeSplitter.Chunk()[2:])
			foundAggregation = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'p', ':'}):
			if foundPriority == true {
				return nil, errors.New("Invalid event packet, multiple priority sections")
			}
			ret.Tags[dogstatsd.EventPriorityTagKey] = string(pipeSplitter.Chunk()[2:])
			if ret.Tags[dogstatsd.EventPriorityTagKey] != "normal" && ret.Tags[dogstatsd.EventPriorityTagKey] != "low" {
				return nil, errors.New("Invalid event packet, priority must be normal or low")
			}
			foundPriority = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'s', ':'}):
			if foundSource == true {
				return nil, errors.New("Invalid event packet, multiple source sections")
			}
			ret.Tags[dogstatsd.EventSourceTypeTagKey] = string(pipeSplitter.Chunk()[2:])
			foundSource = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'t', ':'}):
			if foundAlert == true {
				return nil, errors.New("Invalid event packet, multiple alert sections")
			}
			aType := string(pipeSplitter.Chunk()[2:])
			if aType != "error" &&
				aType != "warning" &&
				aType != "info" &&
				aType != "success" {
				return nil, errors.New("Invalid event packet, alert level must be error, warning, info or success")
			}
			ret.Tags[dogstatsd.EventAlertTypeTagKey] = aType
			foundAlert = true
		case pipeSplitter.Chunk()[0] == '#':
			if foundTags == true {
				return nil, errors.New("Invalid event packet, multiple tag sections")
			}
			tags := strings.Split(string(pipeSplitter.Chunk()[1:]), ",")
			mappedTags := ParseTagSliceToMap(tags)
			// We've already added some tags, so we'll just add these to the ones we've got.
			for k, v := range mappedTags {
				ret.Tags[k] = v
			}
			foundTags = true
		default:
			return nil, errors.New("Invalid event packet, unrecognized metadata section")
		}
	}

	return ret, nil
}

// ParseServiceCheck parses a packet that represents a Datadog service check and
// returns an SSFSample or an error on failure. To facilitate the many Datadog
// -specific values that are present in a DogStatsD service check but not in an
// SSF sample, a series of special tags are set as defined in
// protocol/dogstatsd/protocol.go. Any sink that wants to consume these service
// checks will then need to implement FlushOtherSamples and unwind these special
// tags into whatever is appropriate for that sink.
func ParseServiceCheck(packet []byte) (*ssf.SSFSample, error) {

	ret := &ssf.SSFSample{
		Timestamp: time.Now().Unix(),
		Tags:      map[string]string{dogstatsd.CheckIdentifierKey: ""},
	}

	pipeSplitter := NewSplitBytes(packet, '|')
	pipeSplitter.Next()

	if !bytes.Equal(pipeSplitter.Chunk(), []byte{'_', 's', 'c'}) {
		return nil, errors.New("Invalid service check packet, no _sc prefix")
	}

	if !pipeSplitter.Next() {
		return nil, errors.New("Invalid service check packet, need name section")
	}
	if len(pipeSplitter.Chunk()) == 0 {
		return nil, errors.New("Invalid service check packet, empty name")
	}
	ret.Name = string(pipeSplitter.Chunk())

	if !pipeSplitter.Next() {
		return nil, errors.New("Invalid service check packet, need status section")
	}
	switch {
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'0'}):
		ret.Status = ssf.SSFSample_OK
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'1'}):
		ret.Status = ssf.SSFSample_WARNING
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'2'}):
		ret.Status = ssf.SSFSample_CRITICAL
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'3'}):
		ret.Status = ssf.SSFSample_UNKNOWN
	default:
		return nil, errors.New("Invalid service check packet, must have status of 0, 1, 2, or 3")
	}

	var (
		foundTimestamp bool
		foundHostname  bool
		foundMessage   bool
		foundTags      bool
	)
	for pipeSplitter.Next() {
		if len(pipeSplitter.Chunk()) == 0 {
			return nil, errors.New("Invalid service packet packet, empty string after/between pipes")
		}
		if foundMessage {
			return nil, errors.New("Invalid service check packet, message must be the last metadata section")
		}

		switch {
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'d', ':'}):
			if foundTimestamp || foundMessage {
				return nil, errors.New("Invalid service check packet, multiple date sections")
			}
			unixTimestamp, err := strconv.ParseInt(string(pipeSplitter.Chunk()[2:]), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Invalid service check packet, could not parse date as unix timestamp: %s", err)
			}
			ret.Timestamp = unixTimestamp
			foundTimestamp = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'h', ':'}):
			if foundHostname || foundMessage {
				return nil, errors.New("Invalid service check packet, multiple hostname sections")
			}
			ret.Tags[dogstatsd.CheckHostnameTagKey] = string(pipeSplitter.Chunk()[2:])
			foundHostname = true
		case bytes.HasPrefix(pipeSplitter.Chunk(), []byte{'m', ':'}):
			// this section must come last, so its flag also gets checked by
			// the other cases
			if foundMessage {
				return nil, errors.New("Invalid service check packet, multiple message sections")
			}
			ret.Message = strings.Replace(string(pipeSplitter.Chunk()[2:]), "\\n", "\n", -1)
			foundMessage = true
		case pipeSplitter.Chunk()[0] == '#':
			if foundTags == true {
				return nil, errors.New("Invalid service chack packet, multiple tag sections")
			}
			tags := strings.Split(string(pipeSplitter.Chunk()[1:]), ",")
			mappedTags := ParseTagSliceToMap(tags)
			// We've already added some tags, so we'll just add these to the ones we've got.
			for k, v := range mappedTags {
				ret.Tags[k] = v
			}
			foundTags = true
		default:
			return nil, errors.New("Invalid service check packet, unrecognized metadata section")
		}
	}

	return ret, nil
}

// ParseTagSliceToMap handles splitting a slice of string tags on `:` and
// creating a map from the parts.
func ParseTagSliceToMap(tags []string) map[string]string {
	mappedTags := make(map[string]string)
	for _, tag := range tags {
		splt := strings.SplitN(tag, ":", 2)
		if len(splt) < 2 {
			mappedTags[splt[0]] = ""
		} else {
			mappedTags[splt[0]] = splt[1]
		}
	}
	return mappedTags
}
