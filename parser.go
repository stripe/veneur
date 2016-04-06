package main

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
	Value      int32
	SampleRate float32
	Type       string
	Tags       []string
}

// ParseMetric converts the incoming packet from Datadog DogStatsD
// Datagram format in to a Metric. http://docs.datadoghq.com/guides/dogstatsd/#datagram-format
func ParseMetric(packet []byte) (*Metric, error) {
	parts := bytes.SplitN(packet, []byte(":"), 2)

	var metricName string
	var metricValue int64
	var metricType string
	var metricTags []string
	var metricSampleRate float64
	if len(parts) < 2 {
		return nil, errors.New("Invalid metric packet, need at least 1 colon")
	}

	// Create a new hasher for accumulating the data we read
	h := fnv.New32()

	// Add the name to the digest
	h.Write(parts[0])
	metricName = string(parts[0])

	// Use pipes as the delimiter now
	data := bytes.Split(parts[1], []byte("|"))
	if len(data) < 2 {
		return nil, errors.New("Invalid metric packet, need at least 1 pipe for type")
	}
	// Now convert it
	v, err := strconv.ParseInt(string(data[0]), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("Invalid integer for metric value: %s", parts[1])
	}
	metricValue = v

	// Decide on a type
	switch data[1][0] {
	case 'c':
		metricType = "counter"
	case 'g':
		metricType = "gauge"
	case 'h':
		metricType = "histogram"
	case 'm': // We can ignore the s in "ms"
		metricType = "timer"
	case 's':
		metricType = "set"
	default:
		return nil, errors.New("Invalid type for metric")
	}
	// Add the type to the digest
	h.Write(data[1])

	for i := 2; i < len(data); i++ {
		switch data[i][0] {
		case '@':
			// sample rate!
			sr := string(data[i][1:])
			sampleRate, err := strconv.ParseFloat(sr, 32)
			if err != nil {
				return nil, fmt.Errorf("Invalid float for sample rate: %s", sr)
			}
			metricSampleRate = sampleRate

		case '#':
			// tags!
			tags := strings.Split(string(data[i][1:]), ",")
			sort.Strings(tags)
			h.Write([]byte(strings.Join(tags, ",")))
			metricTags = tags
		}
	}

	digest := h.Sum32()

	return &Metric{Name: metricName, Digest: digest, Value: int32(metricValue), Type: metricType, SampleRate: float32(metricSampleRate), Tags: metricTags}, nil
}
