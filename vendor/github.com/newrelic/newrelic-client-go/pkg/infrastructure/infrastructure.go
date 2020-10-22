// Package infrastructure provides metadata about the underlying Infrastructure API.
package infrastructure

import (
	"github.com/newrelic/newrelic-client-go/internal/http"
)

// ErrorResponse represents an error response from New Relic Infrastructure.
type ErrorResponse struct {
	Errors  []*ErrorDetail `json:"errors,omitempty"`
	Message string         `json:"description,omitempty"`
}

// ErrorDetail represents the details of an error response from New Relic Infrastructure.
type ErrorDetail struct {
	Status string `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Error surfaces an error message from the Infrastructure error response.
func (e *ErrorResponse) Error() string {
	if e.Message != "" {
		return e.Message
	}

	if len(e.Errors) > 0 && e.Errors[0].Detail != "" {
		return e.Errors[0].Detail
	}

	return ""
}

// New creates a new instance of ErrorResponse.
func (e *ErrorResponse) New() http.ErrorResponse {
	return &ErrorResponse{}
}

func (e *ErrorResponse) IsNotFound() bool {
	return false
}

func (e *ErrorResponse) IsTimeout() bool {
	return false
}
