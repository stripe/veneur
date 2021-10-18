package edge

import (
	"fmt"
)

// ListTraceObservers lists the trace observers for an account.
func (e *Edge) ListTraceObservers(accountID int) ([]EdgeTraceObserver, error) {
	resp := traceObserverResponse{}
	vars := map[string]interface{}{
		"accountId": accountID,
	}

	if err := e.client.NerdGraphQuery(listTraceObserversQuery, vars, &resp); err != nil {
		return nil, err
	}

	errors := resp.Actor.Account.Edge.Tracing.TraceObservers.Errors
	if len(errors) > 0 {
		return nil, fmt.Errorf("error listing trace observers: %s", errors[0].Message)
	}

	return resp.Actor.Account.Edge.Tracing.TraceObservers.TraceObservers, nil
}

// CreateTraceObserver creates a trace observer for an account.
func (e *Edge) CreateTraceObserver(accountID int, name string, providerRegion EdgeProviderRegion) (*EdgeTraceObserver, error) {
	resp := createTraceObserverResponse{}
	vars := map[string]interface{}{
		"accountId":            accountID,
		"traceObserverConfigs": []EdgeCreateTraceObserverInput{{name, providerRegion}},
	}

	if err := e.client.NerdGraphQuery(createTraceObserverMutation, vars, &resp); err != nil {
		return nil, err
	}

	errors := resp.EdgeCreateTraceObserver.Responses[0].Errors
	if len(errors) > 0 {
		return nil, fmt.Errorf("error creating trace observer: %s", errors[0].Message)
	}

	return &resp.EdgeCreateTraceObserver.Responses[0].TraceObserver, nil
}

// DeleteTraceObserver deletes a trace observer for an account.
func (e *Edge) DeleteTraceObserver(accountID int, id int) (*EdgeTraceObserver, error) {
	resp := deleteTraceObserversResponse{}

	vars := map[string]interface{}{
		"accountId":            accountID,
		"traceObserverConfigs": []EdgeDeleteTraceObserverInput{{id}},
	}

	if err := e.client.NerdGraphQuery(deleteTraceObserverMutation, vars, &resp); err != nil {
		return nil, err
	}

	errors := resp.EdgeDeleteTraceObservers.Responses[0].Errors
	if len(errors) > 0 {
		return nil, fmt.Errorf("error deleting trace observer: %s", errors[0].Message)
	}

	return &resp.EdgeDeleteTraceObservers.Responses[0].TraceObserver, nil
}

type traceObserverResponse struct {
	Actor struct {
		Account struct {
			Edge struct {
				Tracing struct {
					TraceObservers EdgeTraceObserverResponse
				}
			}
		}
	}
}

const (
	traceObserverSchemaFields = `
		status
		providerRegion
		name
		id
		endpoints {
			https {
				url
				port
				host
			}
			endpointType
			agent {
				port
				host
			}
			status
		}`

	traceObserverErrorSchema = `
		errors {
			type
			message
		}`

	listTraceObserversQuery = `query($accountId: Int!) { actor { account(id: $accountId) { edge { tracing { traceObservers {
			traceObservers { ` +
		traceObserverSchemaFields + `
			} ` +
		traceObserverErrorSchema + `
		} } } } } }`

	createTraceObserverMutation = `
	mutation($traceObserverConfigs: [EdgeCreateTraceObserverInput!]!, $accountId: Int!) {
		edgeCreateTraceObserver(traceObserverConfigs: $traceObserverConfigs, accountId: $accountId) {
			responses {
				traceObserver { ` +
		traceObserverSchemaFields + `
				} ` +
		traceObserverErrorSchema + `
		} } }`

	deleteTraceObserverMutation = `
	mutation($traceObserverConfigs: [EdgeDeleteTraceObserverInput!]!, $accountId: Int!) {
		edgeDeleteTraceObservers(traceObserverConfigs: $traceObserverConfigs, accountId: $accountId) {
			responses {
				traceObserver { ` +
		traceObserverSchemaFields + `
				} ` +
		traceObserverErrorSchema + `
		} } }`
)

type createTraceObserverResponse struct {
	EdgeCreateTraceObserver struct {
		Responses []EdgeCreateTraceObserverResponse
	}
}

type deleteTraceObserversResponse struct {
	EdgeDeleteTraceObservers struct {
		Responses []EdgeDeleteTraceObserverResponse
	}
}
