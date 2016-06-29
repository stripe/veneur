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
type Metric struct {
	MetricKey
	Digest     uint32
	Value      interface{}
	SampleRate float32
	Tags       []string
}

// a struct used to key the metrics into the worker's map - must only contain
// comparable types
type MetricKey struct {
	Name       string
	Type       string
	JoinedTags string // tags in deterministic order, joined with commas
}

// ParseMetric converts the incoming packet from Datadog DogStatsD
// Datagram format in to a Metric. http://docs.datadoghq.com/guides/dogstatsd/#datagram-format
func ParseMetric(packet []byte) (*Metric, error) {
	ret := &Metric{
		SampleRate: 1.0,
	}
	parts := bytes.SplitN(packet, []byte(":"), 2)
	if len(parts) < 2 {
		return nil, errors.New("Invalid metric packet, need at least 1 colon")
	}

	// Create a new hasher for accumulating the data we read
	h := fnv.New32a()

	// Add the name to the digest
	h.Write(parts[0])
	ret.Name = string(parts[0])

	// Use pipes as the delimiter now
	data := bytes.Split(parts[1], []byte("|"))
	if len(data) < 2 {
		return nil, errors.New("Invalid metric packet, need at least 1 pipe for type")
	}
	if len(data[1]) == 0 {
		// avoid panicking on malformed packets missing a type
		// (eg "foo:1||")
		return nil, errors.New("Invalid metric packet, metric type not specified")
	}
	// Decide on a type
	switch data[1][0] {
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
	h.Write(data[1])

	// Now convert it
	if ret.Type == "set" {
		ret.Value = string(data[0])
	} else {
		v, err := strconv.ParseFloat(string(data[0]), 64)
		if err != nil {
			return nil, fmt.Errorf("Invalid number for metric value: %s", parts[1])
		}
		ret.Value = v
	}

	// each of these sections can only appear once in the packet
	foundSampleRate := false
	foundTags := false
	for i := 2; i < len(data); i++ {
		if len(data[i]) == 0 {
			// avoid panicking on malformed packets that have too many pipes
			// (eg "foo:1|g|" or "foo:1|c||@0.1")
			return nil, errors.New("Invalid metric packet, empty string after/between pipes")
		}
		switch data[i][0] {
		case '@':
			if foundSampleRate {
				return nil, errors.New("Invalid metric packet, multiple sample rates specified")
			}
			// sample rate!
			sr := string(data[i][1:])
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
			if foundTags {
				return nil, errors.New("Invalid metric packet, multiple tag sections specified")
			}
			tags := strings.Split(string(data[i][1:]), ",")
			sort.Strings(tags)
			ret.Tags = tags
			// we specifically need the sorted version here so that hashing over
			// tags behaves deterministically
			ret.JoinedTags = strings.Join(tags, ",")
			h.Write([]byte(ret.JoinedTags))
			foundTags = true

		default:
			return nil, fmt.Errorf("Invalid metric packet, contains unknown section %q", data[i])
		}
	}

	ret.Digest = h.Sum32()

	return ret, nil
}
