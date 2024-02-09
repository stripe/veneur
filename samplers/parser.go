package samplers

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/fasthash/fnv1a"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/tagging"
	"github.com/stripe/veneur/v14/util/matcher"
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
	Timestamp  int64
	Message    string
	HostName   string
}

var emptyExtendTags = tagging.NewExtendTags([]string{})

// UpdateTags ensures that JoinedTags and Digest are correct, and that any
// extra tags that should be added to everything have been added. It's not
// really the concern of this function which tags are present, but I've added
// `extendTags` as an argument to ensure it doesn't get forgotten, since we
// have so many different functions constructing UDPMetrics in different ways
func (u *UDPMetric) UpdateTags(tags []string, extendTags *tagging.ExtendTags) {
	if extendTags == nil {
		// we don't expect to hit this case, but storing extendTags as a pointer to
		// avoid copying in hot path. this will avoid a nil pointer dereference, and
		// it's sane that if we don't specify any extend tags configuration that we
		// don't add any new tags. the `Extend` method handles sorting and optional
		// defensive copying though, so we want to use the same behavior
		u.Tags = emptyExtendTags.Extend(tags, true)
	} else {
		u.Tags = extendTags.Extend(tags, true)
	}
	h := fnv1a.Init32
	h = fnv1a.AddString32(h, u.Name)
	h = fnv1a.AddString32(h, u.Type)
	u.JoinedTags = strings.Join(u.Tags, ",")
	h = fnv1a.AddString32(h, u.JoinedTags)
	u.Digest = h
}

// MetricScope describes where the metric will be emitted.
type MetricScope int

// ToPB maps the metric scope to a protobuf Scope type.
func (m MetricScope) ToPB() metricpb.Scope {
	switch m {
	case MixedScope:
		return metricpb.Scope_Mixed
	case LocalOnly:
		return metricpb.Scope_Local
	case GlobalOnly:
		return metricpb.Scope_Global
	}
	return 0
}

// ScopeFromPB creates an internal MetricScope type from the protobuf Scope type.
func ScopeFromPB(scope metricpb.Scope) MetricScope {
	switch scope {
	case metricpb.Scope_Global:
		return GlobalOnly
	case metricpb.Scope_Local:
		return LocalOnly
	case metricpb.Scope_Mixed:
		return MixedScope
	}

	return 0
}

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
func NewMetricKeyFromMetric(
	m *metricpb.Metric, ignoredTags []matcher.TagMatcher,
) MetricKey {
	tags := []string{}
tagLoop:
	for _, tag := range m.Tags {
		for _, matcher := range ignoredTags {
			if matcher.Match(tag) {
				continue tagLoop
			}
		}
		tags = append(tags, tag)
	}
	return MetricKey{
		Name:       m.Name,
		Type:       strings.ToLower(m.Type.String()),
		JoinedTags: strings.Join(tags, ","),
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

type Parser struct {
	extendTags *tagging.ExtendTags
}

func NewParser(tags []string) Parser {
	extendTags := tagging.NewExtendTags(tags)

	return Parser{
		extendTags: &extendTags,
	}
}

// ConvertMetrics examines an SSF message, parses and returns a new
// array containing any metrics contained in the message. If any parse
// error occurs in processing any of the metrics, ExtractMetrics
// collects them into the error type InvalidMetrics and returns this
// error alongside any valid metrics that could be parsed.
func (p *Parser) ConvertMetrics(m *ssf.SSFSpan) ([]UDPMetric, error) {
	samples := m.Metrics
	metrics := make([]UDPMetric, 0, len(samples)+1)
	invalid := []*ssf.SSFSample{}

	for _, metricPacket := range samples {
		metric, err := p.ParseMetricSSF(metricPacket)
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
// span. Currently, it converts the span to two timer metrics for the
// duration of the span. One timer (the "indicator") is tagged with the
// span's service and error-ness. The other timer (the "objective") is
// tagged with the span's service, error-ness, and name. The name can
// be overridden with the ssf_objective tag.
func (p *Parser) ConvertIndicatorMetrics(span *ssf.SSFSpan, indicatorTimerName, objectiveTimerName string) (metrics []UDPMetric, err error) {
	if !span.Indicator || !protocol.ValidTrace(span) {
		// No-op if this isn't an indicator span
		return
	}

	end := time.Unix(span.EndTimestamp/1e9, span.EndTimestamp%1e9)
	start := time.Unix(span.StartTimestamp/1e9, span.StartTimestamp%1e9)
	duration := end.Sub(start)

	if indicatorTimerName != "" {
		tags := map[string]string{
			"service": span.Service,
			"error":   "false",
		}
		if span.Error {
			tags["error"] = "true"
		}
		ssfTimer := ssf.Timing(indicatorTimerName, duration, time.Nanosecond, tags)
		ssfTimer.Name = indicatorTimerName // Ensure the name is free from any name prefixes, like "veneur."

		timer, err := p.ParseMetricSSF(ssfTimer)
		if err != nil {
			return metrics, err
		}
		metrics = append(metrics, timer)
	}

	if objectiveTimerName != "" {
		tags := map[string]string{
			"service":          span.Service,
			"objective":        span.Name,
			"error":            "false",
			"veneurglobalonly": "true",
		}
		if span.Tags["ssf_objective"] != "" {
			tags["objective"] = span.Tags["ssf_objective"]
		}
		if span.Error {
			tags["error"] = "true"
		}
		ssfTimer := ssf.Timing(objectiveTimerName, duration, time.Nanosecond, tags)
		ssfTimer.Name = objectiveTimerName // Ensure the name is free from any name prefixes, like "veneur."

		timer, err := p.ParseMetricSSF(ssfTimer)
		if err != nil {
			return metrics, err
		}
		metrics = append(metrics, timer)
	}

	return metrics, nil
}

// ConvertSpanUniquenessMetrics takes a trace span and computes
// uniqueness metrics about it, returning UDPMetrics sampled at
// rate. Currently, the only metric returned is a Set counting the
// unique names per indicator span/service.
func (p *Parser) ConvertSpanUniquenessMetrics(span *ssf.SSFSpan, rate float32) ([]UDPMetric, error) {
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
	metrics := make([]UDPMetric, 0, len(ssfMetrics))
	for _, m := range ssfMetrics {
		udpM, err := p.ParseMetricSSF(m)
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
func (p *Parser) ParseMetricSSF(metric *ssf.SSFSample) (UDPMetric, error) {
	ret := UDPMetric{
		SampleRate: 1.0,
	}

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
	case ssf.SSFSample_STATUS:
		ret.Type = "status"
	default:
		return UDPMetric{}, invalidMetricTypeError
	}

	switch metric.Metric {
	case ssf.SSFSample_SET:
		ret.Value = metric.Message
	case ssf.SSFSample_STATUS:
		ret.Value = metric.Status
	default:
		ret.Value = float64(metric.Value)
	}

	switch metric.Scope {
	case ssf.SSFSample_LOCAL:
		ret.Scope = LocalOnly
	case ssf.SSFSample_GLOBAL:
		ret.Scope = GlobalOnly
	}

	ret.SampleRate = metric.SampleRate

	tempTags := make([]string, 0, len(metric.Tags))
	for key, value := range metric.Tags {
		if key == "veneurlocalonly" {
			ret.Scope = LocalOnly
			continue
		}
		if key == "veneurglobalonly" {
			ret.Scope = GlobalOnly
			continue
		}
		tempTags = append(tempTags, key+":"+value)
	}
	ret.UpdateTags(tempTags, p.extendTags)

	return ret, nil
}

// ParseMetric converts the incoming packet from Datadog DogStatsD
// Datagram format in to a Metric. http://docs.datadoghq.com/guides/dogstatsd/#datagram-format
func (p *Parser) ParseMetric(packet []byte, cb func(*UDPMetric)) error {
	metric := &UDPMetric{
		SampleRate: 1.0,
	}
	typeStart := bytes.IndexByte(packet, '|')
	if typeStart < 0 {
		return errors.New("Invalid metric packet, need at least 1 pipe for type")
	}

	valueStart := bytes.IndexByte(packet[:typeStart], ':')
	if valueStart == -1 {
		return errors.New("Invalid metric packet, need at least 1 colon")
	}
	nameChunk := packet[:valueStart]
	valueChunk := packet[valueStart+1 : typeStart]

	if len(nameChunk) == 0 {
		return errors.New("Invalid metric packet, name cannot be empty")
	}

	metric.Name = string(nameChunk)

	tagsStart := len(packet)
	if idx := bytes.IndexByte(packet[typeStart+1:], '|'); idx > -1 {
		tagsStart = typeStart + 1 + idx
	}
	typeChunk := packet[typeStart+1 : tagsStart]

	if len(typeChunk) == 0 {
		// avoid panicking on malformed packets missing a type
		// (eg "foo:1||")
		return errors.New("Invalid metric packet, metric type not specified")
	}

	// Decide on a type
	switch typeChunk[0] {
	case 'c':
		metric.Type = "counter"
	case 'g':
		metric.Type = "gauge"
	case 'd', 'h': // consider DogStatsD's "distribution" to be a histogram
		metric.Type = "histogram"
	case 'm': // We can ignore the s in "ms"
		metric.Type = "timer"
	case 's':
		metric.Type = "set"
	default:
		return invalidMetricTypeError
	}

	// each of these sections can only appear once in the packet
	foundSampleRate := false
	var tempTags []string
	for tagsStart < len(packet) {
		tagsNext := len(packet)
		idx := bytes.IndexByte(packet[tagsStart+1:], '|')
		if idx > -1 {
			tagsNext = tagsStart + 1 + idx
		}
		chunk := packet[tagsStart+1 : tagsNext]
		tagsStart = tagsNext

		if len(chunk) == 0 {
			// avoid panicking on malformed packets that have too many pipes
			// (eg "foo:1|g|" or "foo:1|c||@0.1")
			return fmt.Errorf("Invalid metric packet, empty string after/between pipes")
		}
		switch chunk[0] {
		case '@':
			if foundSampleRate {
				return errors.New("Invalid metric packet, multiple sample rates specified")
			}
			// sample rate!
			sr := string(chunk[1:])
			sampleRate, err := strconv.ParseFloat(sr, 32)
			if err != nil {
				return fmt.Errorf("Invalid float for sample rate: %s", sr)
			}
			if sampleRate <= 0 || sampleRate > 1 {
				return fmt.Errorf("Sample rate %f must be >0 and <=1", sampleRate)
			}
			metric.SampleRate = float32(sampleRate)
			foundSampleRate = true

		case '#':
			// tags!
			if tempTags != nil {
				return errors.New("Invalid metric packet, multiple tag sections specified")
			}
			// should we be filtering known key tags from here?
			// in order to prevent extremely high cardinality in the global stats?
			// see worker.go line 273
			tempTags = strings.Split(string(chunk[1:]), ",")
			for i, tag := range tempTags {
				// we use this tag as an escape hatch for metrics that always
				// want to be host-local
				if strings.HasPrefix(tag, "veneurlocalonly") {
					// delete the tag from the list
					tempTags = append(tempTags[:i], tempTags[i+1:]...)
					metric.Scope = LocalOnly
					break
				} else if strings.HasPrefix(tag, "veneurglobalonly") {
					// delete the tag from the list
					tempTags = append(tempTags[:i], tempTags[i+1:]...)
					metric.Scope = GlobalOnly
					break
				}
			}
		default:
			return fmt.Errorf("Invalid metric packet, contains unknown section %q", chunk)
		}
	}

	metric.UpdateTags(tempTags, p.extendTags)

	// Now convert the metric's value
	for len(valueChunk) > 0 {
		value := valueChunk
		nextColon := bytes.IndexByte(valueChunk, ':')
		ret := metric
		if nextColon > -1 {
			value = valueChunk[0:nextColon]
			valueChunk = valueChunk[nextColon+1:]
			// make a shallow copy of metric, so that the next iteration of the loop does not overwrite the same value
			metric = &UDPMetric{
				MetricKey: MetricKey{
					Name:       metric.Name,
					Type:       metric.Type,
					JoinedTags: metric.JoinedTags,
				},
				Tags:       metric.Tags,
				SampleRate: metric.SampleRate,
				Scope:      metric.Scope,
				Digest:     metric.Digest,
			}
		} else {
			// indicate that we should terminate after this iteration
			valueChunk = nil
			// don't bother making a shallow copy of `metric` for the next loop iteration; there won't be one
		}

		if ret.Type == "set" {
			ret.Value = string(value)
		} else {
			v, err := strconv.ParseFloat(string(value), 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("Invalid number for metric value: %s", string(value))
			}
			ret.Value = v
		}
		cb(ret)
	}

	return nil
}

// ParseEvent parses a DogStatsD event packet and returns an SSF sample or an
// error on failure. To facilitate the many Datadog-specific values that are
// present in a DogStatsD event but not in an SSF sample, a series of special
// tags are set as defined in protocol/dogstatsd/protocol.go. Any sink that wants
// to consume these events will then need to implement FlushOtherSamples and
// unwind these special tags into whatever is appropriate for that sink.
func (p *Parser) ParseEvent(packet []byte) (*ssf.SSFSample, error) {

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
			mappedTags := tagging.ParseTagSliceToMap(tags)
			// We've already added some tags, so we'll just add these to the ones we've got.
			for k, v := range mappedTags {
				ret.Tags[k] = v
			}
			foundTags = true
		default:
			return nil, errors.New("Invalid event packet, unrecognized metadata section")
		}
	}

	if p.extendTags != nil {
		ret.Tags = p.extendTags.ExtendMap(ret.Tags, true)
	}

	return ret, nil
}

// ParseServiceCheck parses a packet that represents a service status check and
// returns a UDPMetric or an error on failure. The UDPMetric struct has explicit
// fields for each value of a service status check and does not require
// overloading magical tags for conversion.
func (p *Parser) ParseServiceCheck(packet []byte) (*UDPMetric, error) {
	ret := &UDPMetric{
		SampleRate: 1.0,
		Timestamp:  time.Now().Unix(),
		Tags:       []string{},
	}
	ret.Type = "status"

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
		ret.Value = ssf.SSFSample_OK
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'1'}):
		ret.Value = ssf.SSFSample_WARNING
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'2'}):
		ret.Value = ssf.SSFSample_CRITICAL
	case bytes.Equal(pipeSplitter.Chunk(), []byte{'3'}):
		ret.Value = ssf.SSFSample_UNKNOWN
	default:
		return nil, errors.New("Invalid service check packet, must have status of 0, 1, 2, or 3")
	}

	var (
		foundTimestamp bool
		foundHostname  bool
		foundMessage   bool
		foundTags      bool
	)
	var tempTags []string
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
			ret.HostName = string(pipeSplitter.Chunk()[2:])
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
			tempTags = strings.Split(string(pipeSplitter.Chunk()[1:]), ",")
			for i, tag := range tempTags {
				// we use this tag as an escape hatch for metrics that always
				// want to be host-local
				if tag == "veneurlocalonly" {
					// delete the tag from the list
					tempTags = append(tempTags[:i], tempTags[i+1:]...)
					ret.Scope = LocalOnly
					break
				} else if tag == "veneurglobalonly" {
					// delete the tag from the list
					tempTags = append(tempTags[:i], tempTags[i+1:]...)
					ret.Scope = GlobalOnly
					break
				}
			}
			foundTags = true
		default:
			return nil, errors.New("Invalid service check packet, unrecognized metadata section")
		}
	}
	ret.UpdateTags(tempTags, p.extendTags)

	return ret, nil
}
