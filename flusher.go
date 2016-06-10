package veneur

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	log "github.com/Sirupsen/logrus"
)

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func Flush(postMetrics [][]DDMetric) {
	totalCount := 0
	for _, metrics := range postMetrics {
		totalCount += len(metrics)
	}
	finalMetrics := make([]DDMetric, 0, totalCount)
	for _, metrics := range postMetrics {
		finalMetrics = append(finalMetrics, metrics...)
	}
	for i := range finalMetrics {
		finalMetrics[i].Hostname = Config.Hostname
	}
	// Check to see if we have anything to do
	if totalCount == 0 {
		log.Info("Nothing to flush, skipping.")
		return
	}

	postJSON, err := json.Marshal(map[string][]DDMetric{
		"series": finalMetrics,
	})
	if err != nil {
		Stats.Count("flush.error_total", int64(totalCount), []string{"cause:json"}, 1.0)
		log.WithError(err).Error("Error rendering JSON request body")
		return
	}

	var reqBody bytes.Buffer
	compressor := zlib.NewWriter(&reqBody)
	// bytes.Buffer never errors
	compressor.Write(postJSON)
	// make sure to flush remaining compressed bytes to the buffer
	compressor.Close()

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/series?api_key=%s", Config.APIHostname, Config.Key), &reqBody)
	if err != nil {
		Stats.Count("flush.error_total", int64(totalCount), []string{"cause:construct"}, 1.0)
		log.WithError(err).Error("Error constructing POST request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "deflate")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Stats.Count("flush.error_total", int64(totalCount), []string{"cause:io"}, 1.0)
		log.WithError(err).Error("Error writing POST request")
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// don't bail out if this errors, we'll just log the body as empty
		log.WithError(err).Error("Error reading response body")
	}
	resultFields := log.Fields{
		"status":           resp.Status,
		"request_headers":  req.Header,
		"response_headers": resp.Header,
		"request":          string(postJSON),
		"response":         string(body),
	}

	if resp.StatusCode != http.StatusAccepted {
		Stats.Count("flush.error_total", int64(totalCount), []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		log.WithFields(resultFields).Error("Error POSTing")
		return
	}

	Stats.Count("flush.error_total", 0, nil, 0.1)
	log.WithField("metrics", len(finalMetrics)).Info("Completed flush to Datadog")
	log.WithFields(resultFields).Debug("POSTing JSON")
}
