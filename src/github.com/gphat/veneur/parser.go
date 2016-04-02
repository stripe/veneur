package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Metric struct {
	Name  string
	Value int32
	Type  string
	Tags  string
}

// TODO Tags!
func ParseMetric(packet string) (*Metric, error) {
	parts := strings.Split(packet, "|")

	partsLength := len(parts)
	if partsLength < 3 {
		return nil, errors.New("Invalid metric packet, need at least 1 pipe")
	}

	value, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("Invalid int for metric value: %s", parts[1])
	}

	if !checkValidMetricType(parts[2]) {
		return nil, fmt.Errorf("Invalid metric type '%s'", parts[2])
	}

	return &Metric{Name: parts[0], Value: int32(value), Type: parts[2]}, nil
}

func checkValidMetricType(t string) bool {
	switch t {
	case "c", "g", "h", "ms", "s":
		return true
	default:
		return false
	}
}
