// Package nerdgraph provides a programmatic API for interacting with NerdGraph, New Relic One's GraphQL API.
package nerdgraph

import (
	"github.com/newrelic/newrelic-client-go/internal/http"
	"github.com/newrelic/newrelic-client-go/internal/logging"
	"github.com/newrelic/newrelic-client-go/pkg/config"
)

// NerdGraph is used to communicate with the New Relic's GraphQL API, NerdGraph.
type NerdGraph struct {
	client http.Client
	logger logging.Logger
}

// QueryResponse represents the top-level GraphQL response object returned
// from a NerdGraph query request.
type QueryResponse struct {
	Actor          interface{} `json:"actor,omitempty" yaml:"actor,omitempty"`
	Docs           interface{} `json:"docs,omitempty" yaml:"docs,omitempty"`
	RequestContext interface{} `json:"requestContext,omitempty" yaml:"requestContext,omitempty"`
}

// New returns a new GraphQL client for interacting with New Relic's GraphQL API, NerdGraph.
func New(config config.Config) NerdGraph {
	return NerdGraph{
		client: http.NewClient(config),
		logger: config.GetLogger(),
	}
}

// Query facilitates making a NerdGraph request with a raw GraphQL query. Variables may be provided
// in the form of a map. The response's data structure will vary based on the query provided.
func (n *NerdGraph) Query(query string, variables map[string]interface{}) (interface{}, error) {
	respBody := QueryResponse{}

	if err := n.client.NerdGraphQuery(query, variables, &respBody); err != nil {
		return nil, err
	}

	return respBody, nil
}

// AccountReference represents the NerdGraph schema for a New Relic account.
type AccountReference struct {
	ID   int    `json:"id,omitempty" yaml:"id,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}
