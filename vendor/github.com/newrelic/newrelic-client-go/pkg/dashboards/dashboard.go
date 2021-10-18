package dashboards

import (
	"fmt"
	"time"
)

// ListDashboardsParams represents a set of filters to be
// used when querying New Relic dashboards.
type ListDashboardsParams struct {
	Category      string     `url:"filter[category],omitempty"`
	CreatedAfter  *time.Time `url:"filter[created_after],omitempty"`
	CreatedBefore *time.Time `url:"filter[created_before],omitempty"`
	Page          int        `url:"page,omitempty"`
	PerPage       int        `url:"per_page,omitempty"`
	Sort          string     `url:"sort,omitempty"`
	Title         string     `url:"filter[title],omitempty"`
	UpdatedAfter  *time.Time `url:"filter[updated_after],omitempty"`
	UpdatedBefore *time.Time `url:"filter[updated_before],omitempty"`
}

// ListDashboards is used to retrieve New Relic dashboards.
func (d *Dashboards) ListDashboards(params *ListDashboardsParams) ([]*Dashboard, error) {
	dashboard := []*Dashboard{}
	nextURL := d.config.Region().RestURL("dashboards.json")

	for nextURL != "" {
		response := dashboardsResponse{}
		resp, err := d.client.Get(nextURL, &params, &response)

		if err != nil {
			return nil, err
		}

		dashboard = append(dashboard, response.Dashboards...)

		paging := d.pager.Parse(resp)
		nextURL = paging.Next
	}

	return dashboard, nil
}

// GetDashboard is used to retrieve a single New Relic dashboard.
func (d *Dashboards) GetDashboard(dashboardID int) (*Dashboard, error) {
	response := dashboardResponse{}
	url := fmt.Sprintf("/dashboards/%d.json", dashboardID)

	_, err := d.client.Get(d.config.Region().RestURL(url), nil, &response)

	if err != nil {
		return nil, err
	}

	return &response.Dashboard, nil
}

// CreateDashboard is used to create a New Relic dashboard.
func (d *Dashboards) CreateDashboard(dashboard Dashboard) (*Dashboard, error) {
	response := dashboardResponse{}
	reqBody := dashboardRequest{
		Dashboard: dashboard,
	}
	_, err := d.client.Post(d.config.Region().RestURL("dashboards.json"), nil, &reqBody, &response)

	if err != nil {
		return nil, err
	}

	return &response.Dashboard, nil
}

// UpdateDashboard is used to update a New Relic dashboard.
func (d *Dashboards) UpdateDashboard(dashboard Dashboard) (*Dashboard, error) {
	response := dashboardResponse{}
	url := fmt.Sprintf("/dashboards/%d.json", dashboard.ID)
	reqBody := dashboardRequest{
		Dashboard: dashboard,
	}

	_, err := d.client.Put(d.config.Region().RestURL(url), nil, &reqBody, &response)

	if err != nil {
		return nil, err
	}

	return &response.Dashboard, nil
}

// DeleteDashboard is used to delete a New Relic dashboard.
func (d *Dashboards) DeleteDashboard(dashboardID int) (*Dashboard, error) {
	response := dashboardResponse{}
	url := fmt.Sprintf("/dashboards/%d.json", dashboardID)

	_, err := d.client.Delete(d.config.Region().RestURL(url), nil, &response)

	if err != nil {
		return nil, err
	}

	return &response.Dashboard, nil
}

type dashboardsResponse struct {
	Dashboards []*Dashboard `json:"dashboards,omitempty"`
}

type dashboardResponse struct {
	Dashboard Dashboard `json:"dashboard,omitempty"`
}

type dashboardRequest struct {
	Dashboard Dashboard `json:"dashboard"`
}
