package veneur

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
)

// Metric is a representation of the sample provided by a client. The tag list
// should be deterministically ordered.
type UDPMetric struct {
	MetricKey
	Digest     uint32
	Value      interface{}
	SampleRate float32
	Tags       []string
	LocalOnly  bool
}

// a struct used to key the metrics into the worker's map - must only contain
// comparable types
type MetricKey struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	JoinedTags string `json:"tagstring"` // tags in deterministic order, joined with commas
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
		return nil, errors.New("Invalid type for metric")
	}
	// Add the type to the digest
	h.Write([]byte(ret.Type))

	// Now convert the metric's value
	if ret.Type == "set" {
		ret.Value = string(valueChunk)
	} else {
		v, err := strconv.ParseFloat(string(valueChunk), 64)
		if err != nil {
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
			if ret.Type == "gauge" || ret.Type == "set" {
				return nil, fmt.Errorf("Invalid metric packet, %s cannot have a sample rate", ret.Type)
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
					ret.LocalOnly = true
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
