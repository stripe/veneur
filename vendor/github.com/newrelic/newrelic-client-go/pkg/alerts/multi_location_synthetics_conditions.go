package alerts

import (
	"fmt"
	"strconv"

	"github.com/newrelic/newrelic-client-go/pkg/errors"
)

// MultiLocationSyntheticsCondition represents a location-based failure condition.
//
// ViolationTimeLimitSeconds must be one of 3600, 7200, 14400, 28800, 43200, 86400.
type MultiLocationSyntheticsCondition struct {
	ID                        int                                    `json:"id,omitempty"`
	Name                      string                                 `json:"name,omitempty"`
	Enabled                   bool                                   `json:"enabled"`
	RunbookURL                string                                 `json:"runbook_url,omitempty"`
	Entities                  []string                               `json:"entities,omitempty"`
	Terms                     []MultiLocationSyntheticsConditionTerm `json:"terms,omitempty"`
	ViolationTimeLimitSeconds int                                    `json:"violation_time_limit_seconds,omitempty"`
}

// MultiLocationSyntheticsConditionTerm represents a single term for a location-based failure condition.
//
// Priority must be "warning" or "critical".
// Threshold must be greater than zero.
type MultiLocationSyntheticsConditionTerm struct {
	Priority  string `json:"priority,omitempty"`
	Threshold int    `json:"threshold,omitempty"`
}

// ListMultiLocationSyntheticsConditions returns alert conditions for a specified policy.
func (a *Alerts) ListMultiLocationSyntheticsConditions(policyID int) ([]*MultiLocationSyntheticsCondition, error) {
	response := multiLocationSyntheticsConditionListResponse{}
	multiLocationSyntheticsConditions := []*MultiLocationSyntheticsCondition{}
	queryParams := listMultiLocationSyntheticsConditionsParams{
		PolicyID: policyID,
	}

	nextURL := a.config.Region().RestURL("/alerts_location_failure_conditions/policies/", strconv.Itoa(policyID)+".json")

	for nextURL != "" {
		resp, err := a.client.Get(nextURL, &queryParams, &response)

		if err != nil {
			return nil, err
		}

		multiLocationSyntheticsConditions = append(multiLocationSyntheticsConditions, response.MultiLocationSyntheticsConditions...)

		paging := a.pager.Parse(resp)
		nextURL = paging.Next
	}

	return multiLocationSyntheticsConditions, nil
}

// GetMultiLocationSyntheticsCondition retrieves a specific Synthetics alert condition.
func (a *Alerts) GetMultiLocationSyntheticsCondition(policyID int, conditionID int) (*MultiLocationSyntheticsCondition, error) {
	conditions, err := a.ListMultiLocationSyntheticsConditions(policyID)

	if err != nil {
		return nil, err
	}

	for _, c := range conditions {
		if c.ID == conditionID {
			return c, nil
		}
	}

	return nil, errors.NewNotFoundf("no condition found for policy %d and condition ID %d", policyID, conditionID)
}

// CreateMultiLocationSyntheticsCondition creates an alert condition for a specified policy.
func (a *Alerts) CreateMultiLocationSyntheticsCondition(condition MultiLocationSyntheticsCondition, policyID int) (*MultiLocationSyntheticsCondition, error) {
	reqBody := multiLocationSyntheticsConditionRequestBody{
		MultiLocationSyntheticsCondition: condition,
	}
	resp := multiLocationSyntheticsConditionCreateResponse{}

	url := fmt.Sprintf("/alerts_location_failure_conditions/policies/%d.json", policyID)
	_, err := a.client.Post(a.config.Region().RestURL(url), nil, &reqBody, &resp)
	if err != nil {
		return nil, err
	}

	return &resp.MultiLocationSyntheticsCondition, nil
}

// UpdateMultiLocationSyntheticsCondition updates an alert condition.
func (a *Alerts) UpdateMultiLocationSyntheticsCondition(condition MultiLocationSyntheticsCondition) (*MultiLocationSyntheticsCondition, error) {
	reqBody := multiLocationSyntheticsConditionRequestBody{
		MultiLocationSyntheticsCondition: condition,
	}
	resp := multiLocationSyntheticsConditionCreateResponse{}

	url := fmt.Sprintf("/alerts_location_failure_conditions/%d.json", condition.ID)
	_, err := a.client.Put(a.config.Region().RestURL(url), nil, &reqBody, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.MultiLocationSyntheticsCondition, nil
}

// DeleteMultiLocationSyntheticsCondition delete an alert condition.
func (a *Alerts) DeleteMultiLocationSyntheticsCondition(conditionID int) (*MultiLocationSyntheticsCondition, error) {
	resp := multiLocationSyntheticsConditionCreateResponse{}
	url := fmt.Sprintf("/alerts_conditions/%d.json", conditionID)

	_, err := a.client.Delete(a.config.Region().RestURL(url), nil, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.MultiLocationSyntheticsCondition, nil
}

type listMultiLocationSyntheticsConditionsParams struct {
	PolicyID int `url:"policy_id,omitempty"`
}

type multiLocationSyntheticsConditionListResponse struct {
	MultiLocationSyntheticsConditions []*MultiLocationSyntheticsCondition `json:"location_failure_conditions,omitempty"`
}

type multiLocationSyntheticsConditionCreateResponse struct {
	MultiLocationSyntheticsCondition MultiLocationSyntheticsCondition `json:"location_failure_condition,omitempty"`
}

type multiLocationSyntheticsConditionRequestBody struct {
	MultiLocationSyntheticsCondition MultiLocationSyntheticsCondition `json:"location_failure_condition,omitempty"`
}
