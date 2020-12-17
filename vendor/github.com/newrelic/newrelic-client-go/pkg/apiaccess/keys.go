package apiaccess

import (
	"errors"
	"fmt"

	"github.com/newrelic/newrelic-client-go/internal/http"
)

type APIKey struct {
	APIAccessKey

	AccountID  *int                   `json:"accountId,omitempty"`
	IngestType APIAccessIngestKeyType `json:"ingestType,omitempty"`
	UserID     *int                   `json:"userId,omitempty"`
}

// CreateAPIAccessKeys create keys. You can create keys for multiple accounts at once.
func (a *APIAccess) CreateAPIAccessKeys(keys APIAccessCreateInput) ([]APIKey, error) {
	vars := map[string]interface{}{
		"keys": keys,
	}

	resp := apiAccessKeyCreateResponse{}

	if err := a.client.NerdGraphQuery(apiAccessKeyCreateKeys, vars, &resp); err != nil {
		return nil, err
	}

	if len(resp.APIAccessCreateKeys.Errors) > 0 {
		return nil, errors.New(formatAPIAccessKeyErrors(resp.APIAccessCreateKeys.Errors))
	}

	return resp.APIAccessCreateKeys.CreatedKeys, nil
}

// GetAPIAccessKey returns a single API access key.
func (a *APIAccess) GetAPIAccessKey(keyID string, keyType APIAccessKeyType) (*APIKey, error) {
	vars := map[string]interface{}{
		"id":      keyID,
		"keyType": keyType,
	}

	resp := apiAccessKeyGetResponse{}

	if err := a.client.NerdGraphQuery(apiAccessKeyGetKey, vars, &resp); err != nil {
		return nil, err
	}

	if resp.Errors != nil {
		return nil, errors.New(resp.Error())
	}

	return &resp.Actor.APIAccess.Key, nil
}

// SearchAPIAccessKeys returns the relevant keys based on search criteria. Returns keys are scoped to the current user.
func (a *APIAccess) SearchAPIAccessKeys(params APIAccessKeySearchQuery) ([]APIKey, error) {
	vars := map[string]interface{}{
		"query": params,
	}

	resp := apiAccessKeySearchResponse{}

	if err := a.client.NerdGraphQuery(apiAccessKeySearch, vars, &resp); err != nil {
		return nil, err
	}

	if resp.Errors != nil {
		return nil, errors.New(resp.Error())
	}

	return resp.Actor.APIAccess.KeySearch.Keys, nil
}

// UpdateAPIAccessKeys updates keys. You can update keys for multiple accounts at once.
func (a *APIAccess) UpdateAPIAccessKeys(keys APIAccessUpdateInput) ([]APIKey, error) {
	vars := map[string]interface{}{
		"keys": keys,
	}

	resp := apiAccessKeyUpdateResponse{}

	if err := a.client.NerdGraphQuery(apiAccessKeyUpdateKeys, vars, &resp); err != nil {
		return nil, err
	}

	if len(resp.APIAccessUpdateKeys.Errors) > 0 {
		return nil, errors.New(formatAPIAccessKeyErrors(resp.APIAccessUpdateKeys.Errors))
	}

	return resp.APIAccessUpdateKeys.UpdatedKeys, nil
}

// DeleteAPIAccessKey deletes one or more keys.
func (a *APIAccess) DeleteAPIAccessKey(keys APIAccessDeleteInput) ([]APIAccessDeletedKey, error) {
	vars := map[string]interface{}{
		"keys": keys,
	}

	resp := apiAccessKeyDeleteResponse{}

	if err := a.client.NerdGraphQuery(apiAccessKeyDeleteKeys, vars, &resp); err != nil {
		return nil, err
	}

	if len(resp.APIAccessDeleteKeys.Errors) > 0 {
		return nil, errors.New(formatAPIAccessKeyErrors(resp.APIAccessDeleteKeys.Errors))
	}

	return resp.APIAccessDeleteKeys.DeletedKeys, nil
}

func formatAPIAccessKeyErrors(errs []APIAccessKeyError) string {
	errorString := ""
	for _, e := range errs {
		errorString += fmt.Sprintf("%v: %v\n", e.Type, e.Message)
	}
	return errorString
}

// apiAccessKeyCreateResponse represents the JSON response returned from creating key(s).
type apiAccessKeyCreateResponse struct {
	APIAccessCreateKeys struct {
		CreatedKeys []APIKey            `json:"createdKeys"`
		Errors      []APIAccessKeyError `json:"errors"`
	} `json:"apiAccessCreateKeys"`
}

// apiAccessKeyUpdateResponse represents the JSON response returned from updating key(s).
type apiAccessKeyUpdateResponse struct {
	APIAccessUpdateKeys struct {
		UpdatedKeys []APIKey            `json:"updatedKeys"`
		Errors      []APIAccessKeyError `json:"errors"`
	} `json:"apiAccessUpdateKeys"`
}

// apiAccessKeyGetResponse represents the JSON response returned from getting an access key.
type apiAccessKeyGetResponse struct {
	Actor struct {
		APIAccess struct {
			Key APIKey `json:"key,omitempty"`
		} `json:"apiAccess"`
	} `json:"actor"`
	http.GraphQLErrorResponse
}

type apiAccessKeySearchResponse struct {
	Actor struct {
		APIAccess struct {
			KeySearch struct {
				Keys []APIKey `json:"keys"`
			} `json:"keySearch,omitempty"`
		} `json:"apiAccess"`
	} `json:"actor"`
	http.GraphQLErrorResponse
}

// apiAccessKeyDeleteResponse represents the JSON response returned from creating key(s).
type apiAccessKeyDeleteResponse struct {
	APIAccessDeleteKeys APIAccessDeleteKeyResponse `json:"apiAccessDeleteKeys"`
}

const (
	graphqlAPIAccessKeyBaseFields = `
		id
		key
		name
		notes
		type
		... on ApiAccessIngestKey {
			id
			name
			accountId
			ingestType
			key
			notes
			type
		}
		... on ApiAccessUserKey {
			id
			name
			accountId
			key
			notes
			type
			userId
		}
		... on ApiAccessKey {
			id
			name
			key
			notes
			type
		}`

	graphqlAPIAccessCreateKeyFields = `createdKeys {` + graphqlAPIAccessKeyBaseFields + `}`

	graphqlAPIAccessUpdatedKeyFields = `updatedKeys {` + graphqlAPIAccessKeyBaseFields + `}`

	graphqlAPIAccessKeyErrorFields = `errors {
		  message
		  type
		... on ApiAccessIngestKeyError {
			id
			ingestErrorType: errorType
			accountId
			ingestType
			message
			type
		  }
		... on ApiAccessKeyError {
			message
			type
		  }
		... on ApiAccessUserKeyError {
			id
			accountId
			userErrorType: errorType
			message
			type
			userId
		  }
		}
	`

	apiAccessKeyCreateKeys = `mutation($keys: ApiAccessCreateInput!) {
			apiAccessCreateKeys(keys: $keys) {` + graphqlAPIAccessCreateKeyFields + graphqlAPIAccessKeyErrorFields + `
		}}`

	apiAccessKeyGetKey = `query($id: ID!, $keyType: ApiAccessKeyType!) {
		actor {
			apiAccess {
				key(id: $id, keyType: $keyType) {` + graphqlAPIAccessKeyBaseFields + `}}}}`

	apiAccessKeySearch = `query($query: ApiAccessKeySearchQuery!) {
		actor {
			apiAccess {
				keySearch(query: $query) {
					keys {` + graphqlAPIAccessKeyBaseFields + `}
				}}}}`

	apiAccessKeyUpdateKeys = `mutation($keys: ApiAccessUpdateInput!) {
			apiAccessUpdateKeys(keys: $keys) {` + graphqlAPIAccessUpdatedKeyFields + graphqlAPIAccessKeyErrorFields + `
		}}`

	apiAccessKeyDeleteKeys = `mutation($keys: ApiAccessDeleteInput!) {
			apiAccessDeleteKeys(keys: $keys) {
				deletedKeys {
					id
				}` + graphqlAPIAccessKeyErrorFields + `}}`
)
