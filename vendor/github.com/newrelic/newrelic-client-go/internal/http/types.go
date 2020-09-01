package http

import (
	"errors"
)

var (
	// ErrNotFound is returned when the resource was not found in New Relic.
	ErrNotFound = errors.New("newrelic: Resource not found")
)
