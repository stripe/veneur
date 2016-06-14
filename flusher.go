package veneur

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
)

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func Flush(postMetrics [][]DDMetric, metricLimit int) {
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

	Stats.Gauge("flush.post_metrics_total", float64(totalCount), nil, 1.0)
	// Check to see if we have anything to do
	if totalCount == 0 {
		log.Info("Nothing to flush, skipping.")
		return
	}

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((totalCount - 1) / metricLimit) + 1
	chunkSize := ((totalCount - 1) / workers) + 1
	log.WithField("workers", workers).Info("Worker count chosen")
	log.WithField("chunkSize", chunkSize).Info("Chunk size chosen")
	var wg sync.WaitGroup
	flushStart := time.Now()
	for i := 0; i < workers; i++ {
		chunk := finalMetrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		wg.Add(1)
		go flushPart(chunk, &wg)
	}
	wg.Wait()
	Stats.TimeInMilliseconds("flush.total_duration_ns", float64(time.Now().Sub(flushStart).Nanoseconds()), nil, 1.0)

	Stats.Count("flush.error_total", 0, nil, 0.1) // make sure this metric is not sparse
	log.WithField("metrics", totalCount).Info("Completed flush to Datadog")
}

func flushPart(metricSlice []DDMetric, wg *sync.WaitGroup) {
	defer wg.Done()

	cstart := time.Now()
	var reqBody bytes.Buffer
	compressor := zlib.NewWriter(&reqBody)
	encoder := json.NewEncoder(compressor)
	err := encoder.Encode(map[string][]DDMetric{
		"series": metricSlice,
	})
	if err != nil {
		Stats.Count("flush.error_total", int64(len(metricSlice)), []string{"cause:json"}, 1.0)
		log.WithError(err).Error("Error rendering JSON request body")
		return
	}
	// make sure to flush remaining compressed bytes to the buffer
	compressor.Close()
	Stats.TimeInMilliseconds(
		"flush.part_duration_ns",
		float64(time.Now().Sub(cstart).Nanoseconds()),
		[]string{"part:marshal"},
		1.0,
	)
	// Len reports the unread length, so we have to record this before it's POSTed
	bodyLength := reqBody.Len()
	Stats.Gauge("flush.content_length_bytes", float64(bodyLength), nil, 1.0)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/series?api_key=%s", Config.APIHostname, Config.Key), &reqBody)
	if err != nil {
		Stats.Count("flush.error_total", int64(len(metricSlice)), []string{"cause:construct"}, 1.0)
		log.WithError(err).Error("Error constructing POST request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "deflate")

	fstart := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Stats.Count("flush.error_total", int64(len(metricSlice)), []string{"cause:io"}, 1.0)
		log.WithError(err).Error("Error writing POST request")
		return
	}
	Stats.TimeInMilliseconds(
		"flush.part_duration_ns",
		float64(time.Now().Sub(fstart).Nanoseconds()),
		[]string{"part:post"},
		1.0,
	)
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
		"request_length":   bodyLength,
		"response":         string(body),
		"total_metrics":    len(metricSlice),
	}

	if resp.StatusCode != http.StatusAccepted {
		Stats.Count("flush.error_total", int64(len(metricSlice)), []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		log.WithFields(resultFields).Error("Error POSTing")
		return
	}

	log.WithFields(resultFields).Debug("POSTing JSON")
}
