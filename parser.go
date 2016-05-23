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

// Metric is a representation of the sample provided by a client.
type Metric struct {
	Name       string
	Digest     uint32
	Value      interface{}
	SampleRate float32
	Type       string
	Tags       []string
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
	h := fnv.New32()

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
			return nil, fmt.Errorf("Invalid integer for metric value: %s", parts[1])
		}
		ret.Value = v
	}

	for i := 2; i < len(data); i++ {
		if len(data[i]) == 0 {
			// avoid panicking on malformed packets that have too many pipes
			// (eg "foo:1|g|" or "foo:1|c||@0.1")
			return nil, errors.New("Invalid metric packet, empty string after/between pipes")
		}
		switch data[i][0] {
		case '@':
			// sample rate!
			sr := string(data[i][1:])
			sampleRate, err := strconv.ParseFloat(sr, 32)
			if err != nil {
				return nil, fmt.Errorf("Invalid float for sample rate: %s", sr)
			}
			if ret.Type == "gauge" || ret.Type == "set" {
				return nil, fmt.Errorf("Invalid metric packet, %s cannot have a sample rate", ret.Type)
			}
			ret.SampleRate = float32(sampleRate)

		case '#':
			// tags!
			tags := strings.Split(string(data[i][1:]), ",")
			sort.Strings(tags)
			h.Write([]byte(strings.Join(tags, ",")))
			ret.Tags = tags
		}
	}
	if len(Config.Tags) > 0 {
		ret.Tags = append(ret.Tags, Config.Tags...)
	}

	ret.Digest = h.Sum32()

	return ret, nil
}
