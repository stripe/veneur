package workloads

import (
	"github.com/newrelic/newrelic-client-go/internal/serialization"
	"github.com/newrelic/newrelic-client-go/pkg/errors"
	"github.com/newrelic/newrelic-client-go/pkg/nerdgraph"
)

// Workload represents a New Relic One workload.
type Workload struct {
	Account             nerdgraph.AccountReference `json:"account,omitempty"`
	CreatedAt           serialization.EpochTime    `json:"createdAt,omitempty"`
	CreatedBy           UserReference              `json:"createdBy,omitempty"`
	Entities            []EntityRef                `json:"entities,omitempty"`
	EntitySearchQueries []EntitySearchQuery        `json:"entitySearchQueries,omitempty"`
	EntitySearchQuery   string                     `json:"entitySearchQuery,omitempty"`
	GUID                string                     `json:"guid,omitempty"`
	ID                  int                        `json:"id,omitempty"`
	Name                string                     `json:"name,omitempty"`
	Permalink           string                     `json:"permalink,omitempty"`
	ScopeAccounts       ScopeAccounts              `json:"scopeAccounts,omitempty"`
	UpdatedAt           *serialization.EpochTime   `json:"updatedAt,omitempty"`
}

// EntityRef represents an entity referenced by this workload.
type EntityRef struct {
	GUID string `json:"guid,omitempty"`
}

// EntitySearchQuery represents an entity search used by this workload.
type EntitySearchQuery struct {
	CreatedAt serialization.EpochTime  `json:"createdAt,omitempty"`
	CreatedBy UserReference            `json:"createdBy,omitempty"`
	ID        int                      `json:"id,omitempty"`
	Query     string                   `json:"query,omitempty"`
	UpdatedAt *serialization.EpochTime `json:"updatedAt,omitempty"`
}

// UserReference represents a user referenced by this workload's search query.
type UserReference struct {
	Email    string `json:"email,omitempty"`
	Gravatar string `json:"gravatar,omitempty"`
	ID       int    `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ScopeAccounts represents the accounts used to scope this workload.
type ScopeAccounts struct {
	AccountIDs []int `json:"accountIds,omitempty"`
}

// CreateInput represents the input parameters used for creating or updating a workload.
type CreateInput struct {
	EntityGUIDs         []string                 `json:"entityGuids,omitempty"`
	EntitySearchQueries []EntitySearchQueryInput `json:"entitySearchQueries,omitempty"`
	Name                string                   `json:"name,omitempty"`
	ScopeAccountsInput  *ScopeAccountsInput      `json:"scopeAccounts,omitempty"`
}

// EntitySearchQueryInput represents an entity search query for creating or updating a workload.
type EntitySearchQueryInput struct {
	Query string `json:"query,omitempty"`
}

// UpdateCollectionEntitySearchQueryInput represents an entity search query for creating or updating a workload.
type UpdateCollectionEntitySearchQueryInput struct {
	ID int `json:"id,omitempty"`
	EntitySearchQueryInput
}

// ScopeAccountsInput is the input object containing accounts that will be used to get entities from.
type ScopeAccountsInput struct {
	AccountIDs []int `json:"accountIds,omitempty"`
}

// DuplicateInput represents the input object used to identify the workload to be duplicated.
type DuplicateInput struct {
	Name string `json:"name,omitempty"`
}

// UpdateInput represents the input object used to identify the workload to be updated and its new changes.
type UpdateInput struct {
	EntityGUIDs         []string                 `json:"entityGuids,omitempty"`
	EntitySearchQueries []EntitySearchQueryInput `json:"entitySearchQueries,omitempty"`
	Name                string                   `json:"name,omitempty"`
	ScopeAccountsInput  *ScopeAccountsInput      `json:"scopeAccounts,omitempty"`
}

// ListWorkloads retrieves a set of New Relic One workloads by their account ID.
func (e *Workloads) ListWorkloads(accountID int) ([]*Workload, error) {
	resp := workloadsResponse{}
	vars := map[string]interface{}{
		"accountId": accountID,
	}

	if err := e.client.NerdGraphQuery(listWorkloadsQuery, vars, &resp); err != nil {
		return nil, err
	}

	if len(resp.Actor.Account.Workload.Collections) == 0 {
		return nil, errors.NewNotFound("")
	}

	return resp.Actor.Account.Workload.Collections, nil
}

// GetWorkload retrieves a New Relic One workload by its GUID.
func (e *Workloads) GetWorkload(accountID int, workloadGUID string) (*Workload, error) {
	resp := workloadResponse{}
	vars := map[string]interface{}{
		"accountId": accountID,
		"guid":      workloadGUID,
	}

	if err := e.client.NerdGraphQuery(getWorkloadQuery, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.Actor.Account.Workload.Collection, nil
}

// CreateWorkload creates a New Relic One workload.
func (e *Workloads) CreateWorkload(accountID int, workload CreateInput) (*Workload, error) {
	resp := workloadCreateResponse{}
	vars := map[string]interface{}{
		"accountId": accountID,
		"workload":  workload,
	}

	if err := e.client.NerdGraphQuery(createWorkloadMutation, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.WorkloadCreate, nil
}

// DeleteWorkload deletes a New Relic One workload.
func (e *Workloads) DeleteWorkload(guid string) (*Workload, error) {
	resp := workloadDeleteResponse{}
	vars := map[string]interface{}{
		"guid": guid,
	}

	if err := e.client.NerdGraphQuery(deleteWorkloadMutation, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.WorkloadDelete, nil
}

// DuplicateWorkload duplicates a New Relic One workload.
func (e *Workloads) DuplicateWorkload(accountID int, sourceGUID string, workload *DuplicateInput) (*Workload, error) {
	resp := workloadDuplicateResponse{}
	vars := map[string]interface{}{
		"accountId":  accountID,
		"sourceGuid": sourceGUID,
		"workload":   workload,
	}

	if err := e.client.NerdGraphQuery(duplicateWorkloadMutation, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.WorkloadDuplicate, nil
}

// UpdateWorkload updates a New Relic One workload.
func (e *Workloads) UpdateWorkload(guid string, workload UpdateInput) (*Workload, error) {
	resp := workloadUpdateResponse{}
	vars := map[string]interface{}{
		"guid":     guid,
		"workload": workload,
	}

	if err := e.client.NerdGraphQuery(updateWorkloadMutation, vars, &resp); err != nil {
		return nil, err
	}

	return &resp.WorkloadUpdate, nil
}

const (
	// graphqlWorkloadStructFields is the set of fields that we want returned on workload queries,
	// and should map back directly to the Workload struct
	graphqlWorkloadStructFields = `
			account {
				id
				name
			}
			createdBy {
				email
				gravatar
				id
				name
			}
			createdAt
			entities {
				guid
			}
			entitySearchQueries {
				createdAt
				createdBy {
					email
					gravatar
					id
					name
				}
				id
				query
				updatedAt
			}
			entitySearchQuery
			guid
			id
			permalink
			name
			scopeAccounts {
				accountIds
			}
			updatedAt
`

	getWorkloadQuery = `query($guid: EntityGuid!, $accountId: Int!) { actor { account(id: $accountId) { workload { collection(guid: $guid) {` +
		graphqlWorkloadStructFields +
		` } } } } }`

	listWorkloadsQuery = `query($accountId: Int!) { actor { account(id: $accountId) { workload { collections {` +
		graphqlWorkloadStructFields +
		` } } } } }`

	createWorkloadMutation = `
		mutation($accountId: Int!, $workload: WorkloadCreateInput!) {
			workloadCreate(accountId: $accountId, workload: $workload) {` +
		graphqlWorkloadStructFields +
		` } }`

	deleteWorkloadMutation = `
		mutation($guid: EntityGuid!) {
			workloadDelete(guid: $guid) {` +
		graphqlWorkloadStructFields +
		` } }`

	duplicateWorkloadMutation = `
		mutation($accountId: Int!, $sourceGuid: EntityGuid!, $workload: WorkloadDuplicateInput) {
			workloadDuplicate(accountId: $accountId, sourceGuid: $sourceGuid, workload: $workload) {` +
		graphqlWorkloadStructFields +
		` } }`

	updateWorkloadMutation = `
		mutation($guid: EntityGuid!, $workload: WorkloadUpdateInput!) {
			workloadUpdate(guid: $guid, workload: $workload) {` +
		graphqlWorkloadStructFields +
		` } }`
)

type workloadsResponse struct {
	Actor struct {
		Account struct {
			Workload struct {
				Collections []*Workload
			}
		}
	}
}

type workloadResponse struct {
	Actor struct {
		Account struct {
			Workload struct {
				Collection Workload
			}
		}
	}
}

type workloadCreateResponse struct {
	WorkloadCreate Workload
}

type workloadDeleteResponse struct {
	WorkloadDelete Workload
}

type workloadDuplicateResponse struct {
	WorkloadDuplicate Workload
}

type workloadUpdateResponse struct {
	WorkloadUpdate Workload
}
