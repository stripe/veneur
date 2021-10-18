package synthetics

import (
	"fmt"
	"strings"
)

// MonitorLabel represents a single label for a New Relic Synthetics monitor.
type MonitorLabel struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Href  string `json:"href"`
}

// GetMonitorLabels is used to retrieve all labels for a given Synthetics monitor.
//
// Deprecated: Use entities.ListTags instead.
// https://discuss.newrelic.com/t/end-of-life-notice-synthetics-labels-and-synthetics-apm-group-by-tag/103781
func (s *Synthetics) GetMonitorLabels(monitorID string) ([]*MonitorLabel, error) {
	url := fmt.Sprintf("/v4/monitors/%s/labels", monitorID)

	resp := getMonitorLabelsResponse{}

	_, err := s.client.Get(s.config.Region().SyntheticsURL(url), nil, &resp)
	if err != nil {
		return []*MonitorLabel{}, err
	}

	return resp.Labels, nil
}

// AddMonitorLabel is used to add a label to a given monitor.
//
// Deprecated: Use entities.AddTags instead.
// https://discuss.newrelic.com/t/end-of-life-notice-synthetics-labels-and-synthetics-apm-group-by-tag/103781
func (s *Synthetics) AddMonitorLabel(monitorID, labelKey, labelValue string) error {
	url := fmt.Sprintf("/v4/monitors/%s/labels", monitorID)

	data := fmt.Sprintf("%s:%s", strings.Title(labelKey), strings.Title(labelValue))

	// We pass []byte of data do avoid JSON encoding due to the Syntheics API's lack of
	// support for JSON on this call.  The values must be POSTed as bare key:value word string.
	_, err := s.client.Post(s.config.Region().SyntheticsURL(url), nil, []byte(data), nil)
	if err != nil {
		return err
	}

	return nil
}

// DeleteMonitorLabel deletes a key:value label from the given Syntheics monitor.
//
// Deprecated: Use entities.DeleteTags instead.
// https://discuss.newrelic.com/t/end-of-life-notice-synthetics-labels-and-synthetics-apm-group-by-tag/103781
func (s *Synthetics) DeleteMonitorLabel(monitorID, labelKey, labelValue string) error {
	url := fmt.Sprintf("/v4/monitors/%s/labels/%s:%s", monitorID, strings.Title(labelKey), strings.Title(labelValue))

	_, err := s.client.Delete(s.config.Region().SyntheticsURL(url), nil, nil)
	if err != nil {
		return err
	}

	return nil
}

type getMonitorLabelsResponse struct {
	Labels []*MonitorLabel `json:"labels"`
	Count  int             `json:"count"`
}
