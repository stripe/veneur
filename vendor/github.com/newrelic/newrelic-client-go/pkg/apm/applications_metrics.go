package apm

import (
	"strconv"
	"time"
)

// MetricNamesParams are the request parameters for the /metrics.json endpoint.
type MetricNamesParams struct {
	Name string `url:"name,omitempty"`
}

// MetricDataParams are the request parameters for the /metrics/data.json endpoint.
type MetricDataParams struct {
	Names     []string   `url:"names[],omitempty"`
	Values    []string   `url:"values[],omitempty"`
	From      *time.Time `url:"from,omitempty"`
	To        *time.Time `url:"to,omitempty"`
	Period    int        `url:"period,omitempty"`
	Summarize bool       `url:"summarize,omitempty"`
	Raw       bool       `url:"raw,omitempty"`
}

// MetricName is the name of a metric, and the names of the values that can be retrieved.
type MetricName struct {
	Name   string   `json:"name,omitempty"`
	Values []string `json:"values,omitempty"`
}

// MetricData is the series of time windows and the data therein, for a given metric name.
type MetricData struct {
	Name       string            `json:"name,omitempty"`
	Timeslices []MetricTimeslice `json:"timeslices,omitempty"`
}

// MetricTimeslice is a single window of time for a given metric, with the associated metric data.
type MetricTimeslice struct {
	From   *time.Time            `json:"from"`
	To     *time.Time            `json:"to"`
	Values MetricTimesliceValues `json:"values"`
}

//MetricTimesliceValues is the collection of metric values for a single time slice.
type MetricTimesliceValues struct {
	AsPercentage           float64 `json:"as_percentage,omitempty"`
	AverageTime            float64 `json:"average_time,omitempty"`
	CallsPerMinute         float64 `json:"calls_per_minute,omitempty"`
	MaxValue               float64 `json:"max_value,omitempty"`
	TotalCallTimePerMinute float64 `json:"total_call_time_per_minute,omitempty"`
	Utilization            float64 `json:"utilization,omitempty"`
}

// GetMetricNames is used to retrieve a list of known metrics and their value names for the given resource.
//
// https://rpm.newrelic.com/api/explore/applications/metric_names
func (a *APM) GetMetricNames(applicationID int, params MetricNamesParams) ([]*MetricName, error) {
	nextURL := a.config.Region().RestURL("applications", strconv.Itoa(applicationID), "metrics.json")
	response := metricNamesResponse{}
	metrics := []*MetricName{}

	for nextURL != "" {
		resp, err := a.client.Get(nextURL, &params, &response)

		if err != nil {
			return nil, err
		}

		metrics = append(metrics, response.Metrics...)

		paging := a.pager.Parse(resp)
		nextURL = paging.Next
	}

	return metrics, nil
}

// GetMetricData is used to retrieve a list of values for each of the requested metrics.
//
// https://rpm.newrelic.com/api/explore/applications/metric_data
func (a *APM) GetMetricData(applicationID int, params MetricDataParams) ([]*MetricData, error) {
	nextURL := a.config.Region().RestURL("applications", strconv.Itoa(applicationID), "/metrics/data.json")

	response := metricDataResponse{}
	data := []*MetricData{}

	for nextURL != "" {
		resp, err := a.client.Get(nextURL, &params, &response)

		if err != nil {
			return nil, err
		}

		data = append(data, response.MetricData.Metrics...)

		paging := a.pager.Parse(resp)
		nextURL = paging.Next
	}

	return data, nil
}

type metricNamesResponse struct {
	Metrics []*MetricName
}

type metricDataResponse struct {
	MetricData struct {
		From            *time.Time    `json:"from"`
		To              *time.Time    `json:"to"`
		MetricsNotFound []string      `json:"metrics_not_found"`
		MetricsFound    []string      `json:"metrics_found"`
		Metrics         []*MetricData `json:"metrics"`
	} `json:"metric_data"`
}
