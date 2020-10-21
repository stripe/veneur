package apm

import (
	"github.com/newrelic/newrelic-client-go/pkg/errors"
)

// Label represents a New Relic label.
type Label struct {
	Key      string     `json:"key,omitempty"`
	Category string     `json:"category,omitempty"`
	Name     string     `json:"name,omitempty"`
	Links    LabelLinks `json:"links,omitempty"`
}

// LabelLinks represents external references on the Label.
type LabelLinks struct {
	Applications []int `json:"applications"`
	Servers      []int `json:"servers"`
}

// ListLabels returns the labels within an account.
func (a *APM) ListLabels() ([]*Label, error) {
	labels := []*Label{}
	nextURL := a.config.Region().RestURL("labels.json")

	for nextURL != "" {
		response := labelsResponse{}
		resp, err := a.client.Get(nextURL, nil, &response)

		if err != nil {
			return nil, err
		}

		labels = append(labels, response.Labels...)

		paging := a.pager.Parse(resp)
		nextURL = paging.Next
	}

	return labels, nil
}

// GetLabel gets a label by key. A label's key
// is a string hash formatted as <Category>:<Name>.
func (a *APM) GetLabel(key string) (*Label, error) {
	labels, err := a.ListLabels()

	if err != nil {
		return nil, err
	}

	for _, label := range labels {
		if label.Key == key {
			return label, nil
		}
	}

	return nil, errors.NewNotFoundf("no label found with key %s", key)
}

// CreateLabel creates a new label within an account.
func (a *APM) CreateLabel(label Label) (*Label, error) {
	reqBody := labelRequestBody{
		Label: label,
	}
	resp := labelResponse{}

	// The API currently uses a PUT request for label creation
	_, err := a.client.Put(a.config.Region().RestURL("labels.json"), nil, &reqBody, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.Label, nil
}

// DeleteLabel deletes a label by key. A label's key
// is a string hash formatted as <Category>:<Name>.
func (a *APM) DeleteLabel(key string) (*Label, error) {
	resp := labelResponse{}

	_, err := a.client.Delete(a.config.Region().RestURL("labels", key+".json"), nil, &resp)

	if err != nil {
		return nil, err
	}

	return &resp.Label, nil
}

type labelsResponse struct {
	Labels []*Label `json:"labels,omitempty"`
}

type labelResponse struct {
	Label Label `json:"label,omitempty"`
}

type labelRequestBody struct {
	Label Label `json:"label,omitempty"`
}
