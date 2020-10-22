// Package errors provides error types for specific error scenarios.
package errors

import (
	"fmt"
)

// NewNotFound returns a new instance of NotFound with an optional custom error message.
func NewNotFound(err string) *NotFound {
	e := NotFound{
		err: err,
	}

	return &e
}

// NewNotFoundf returns a new instance of NotFound
// with an optional formatted custom error message.
func NewNotFoundf(format string, args ...interface{}) *NotFound {
	return NewNotFound(fmt.Sprintf(format, args...))
}

// NotFound is returned when the target resource cannot be located.
type NotFound struct {
	err string
}

func (e *NotFound) Error() string {
	if e.err == "" {
		return "resource not found"
	}

	return e.err
}

// NewTimeout returns a new instance of Timeout with an optional custom error message.
func NewTimeout(err string) *Timeout {
	e := Timeout{
		err: err,
	}

	return &e
}

// NewTimeoutf returns a new instance of Timeout
// with an optional formatted custom error message.
func NewTimeoutf(format string, args ...interface{}) *Timeout {
	return NewTimeout(fmt.Sprintf(format, args...))
}

// Timeout is returned when the target resource cannot be located.
type Timeout struct {
	err string
}

func (e *Timeout) Error() string {
	if e.err == "" {
		return "server timeout"
	}

	return e.err
}

// NewUnexpectedStatusCode returns a new instance of UnexpectedStatusCode
// with an optional custom message.
func NewUnexpectedStatusCode(statusCode int, err string) *UnexpectedStatusCode {
	return &UnexpectedStatusCode{
		err:        err,
		statusCode: statusCode,
	}
}

// NewUnexpectedStatusCodef returns a new instance of UnexpectedStatusCode
// with an optional formatted custom message.
func NewUnexpectedStatusCodef(statusCode int, format string, args ...interface{}) *UnexpectedStatusCode {
	return NewUnexpectedStatusCode(statusCode, fmt.Sprintf(format, args...))
}

// UnexpectedStatusCode is returned when an unexpected status code is returned
// from New Relic's APIs.
type UnexpectedStatusCode struct {
	err        string
	statusCode int
}

func (e *UnexpectedStatusCode) Error() string {
	msg := fmt.Sprintf("%d response returned", e.statusCode)

	if e.err != "" {
		msg += fmt.Sprintf(": %s", e.err)
	}

	return msg
}
