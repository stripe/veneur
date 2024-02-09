package http

import (
	"encoding/json"
	"strings"
)

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data interface{} `json:"data"`
}

// GraphQLError represents a single error.
type GraphQLError struct {
	Message            string                      `json:"message"`
	DownstreamResponse []GraphQLDownstreamResponse `json:"downstreamResponse,omitempty"`
}

// GraphQLDownstreamResponse represents an error's downstream response.
type GraphQLDownstreamResponse struct {
	Extensions struct {
		Code             string `json:"code,omitempty"`
		ValidationErrors []struct {
			Name   string `json:"name,omitempty"`
			Reason string `json:"reason,omitempty"`
		} `json:"validationErrors,omitempty"`
	} `json:"extensions,omitempty"`
	Message string `json:"message,omitempty"`
}

// GraphQLErrorResponse represents a default error response body.
type GraphQLErrorResponse struct {
	Errors []GraphQLError `json:"errors"`
}

func (r *GraphQLErrorResponse) Error() string {
	if len(r.Errors) > 0 {
		messages := []string{}
		for _, e := range r.Errors {
			f, _ := json.Marshal(e.DownstreamResponse)

			messages = append(messages, e.Message)
			messages = append(messages, string(f))
		}
		return strings.Join(messages, ", ")
	}

	return ""
}

// IsNotFound determines if the error is due to a missing resource.
func (r *GraphQLErrorResponse) IsNotFound() bool {
	return false
}

// New creates a new instance of GraphQLErrorRepsonse.
func (r *GraphQLErrorResponse) New() ErrorResponse {
	return &GraphQLErrorResponse{}
}
