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

	"github.com/Sirupsen/logrus"
)

// Flush takes the slices of metrics, combines then and marshals them to json
// for posting to Datadog.
func (s *Server) Flush(interval time.Duration, metricLimit int) {
	// number of ddmetrics generated when a histogram flushes
	histogramSize := 3 + len(s.HistogramPercentiles)

	// allocating this long array to count up the sizes is cheaper than appending
	// the []DDMetrics together one at a time
	tempMetrics := make([]WorkerMetrics, 0, len(s.Workers))
	totalLength := 0
	for i, w := range s.Workers {
		s.logger.WithField("worker", i).Debug("Flushing")
		wm := w.Flush()
		tempMetrics = append(tempMetrics, wm)
		totalLength += len(wm.counters) + len(wm.gauges) + len(wm.sets) + ((len(wm.timers) + len(wm.histograms)) * histogramSize)
	}

	finalMetrics := make([]DDMetric, 0, totalLength)
	for _, wm := range tempMetrics {
		for _, c := range wm.counters {
			finalMetrics = append(finalMetrics, c.Flush(interval)...)
		}
		for _, g := range wm.gauges {
			finalMetrics = append(finalMetrics, g.Flush()...)
		}
		for _, h := range wm.histograms {
			finalMetrics = append(finalMetrics, h.Flush(interval, s.HistogramPercentiles)...)
		}
		for _, s := range wm.sets {
			finalMetrics = append(finalMetrics, s.Flush()...)
		}
		for _, t := range wm.timers {
			finalMetrics = append(finalMetrics, t.Flush(interval, s.HistogramPercentiles)...)
		}
	}
	for i := range finalMetrics {
		finalMetrics[i].Hostname = s.Hostname
		finalMetrics[i].Tags = append(finalMetrics[i].Tags, s.Tags...)
	}

	s.statsd.Gauge("flush.post_metrics_total", float64(len(finalMetrics)), nil, 1.0)
	// Check to see if we have anything to do
	if len(finalMetrics) == 0 {
		s.logger.Info("Nothing to flush, skipping.")
		return
	}

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(finalMetrics) - 1) / metricLimit) + 1
	chunkSize := ((len(finalMetrics) - 1) / workers) + 1
	s.logger.WithField("workers", workers).Debug("Worker count chosen")
	s.logger.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")
	var wg sync.WaitGroup
	flushStart := time.Now()
	for i := 0; i < workers; i++ {
		chunk := finalMetrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		wg.Add(1)
		go s.flushPart(chunk, &wg)
	}
	wg.Wait()
	s.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Now().Sub(flushStart).Nanoseconds()), nil, 1.0)

	s.statsd.Count("flush.error_total", 0, nil, 1.0) // make sure this metric is not sparse
	s.logger.WithField("metrics", len(finalMetrics)).Info("Completed flush to Datadog")
}

func (s *Server) flushPart(metricSlice []DDMetric, wg *sync.WaitGroup) {
	defer wg.Done()

	cstart := time.Now()
	var reqBody bytes.Buffer
	compressor := zlib.NewWriter(&reqBody)
	encoder := json.NewEncoder(compressor)
	err := encoder.Encode(map[string][]DDMetric{
		"series": metricSlice,
	})
	if err != nil {
		s.statsd.Count("flush.error_total", 1, []string{"cause:json"}, 1.0)
		s.logger.WithError(err).Error("Error rendering JSON request body")
		return
	}
	// make sure to flush remaining compressed bytes to the buffer
	compressor.Close()
	s.statsd.TimeInMilliseconds(
		"flush.part_duration_ns",
		float64(time.Now().Sub(cstart).Nanoseconds()),
		[]string{"part:marshal"},
		1.0,
	)
	// Len reports the unread length, so we have to record this before it's POSTed
	bodyLength := reqBody.Len()
	s.statsd.Histogram("flush.content_length_bytes", float64(bodyLength), nil, 1.0)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/series?api_key=%s", s.DDHostname, s.DDAPIKey), &reqBody)
	if err != nil {
		s.statsd.Count("flush.error_total", 1, []string{"cause:construct"}, 1.0)
		s.logger.WithError(err).Error("Error constructing POST request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "deflate")

	fstart := time.Now()
	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		s.statsd.Count("flush.error_total", 1, []string{"cause:io"}, 1.0)
		s.logger.WithError(err).Error("Error writing POST request")
		return
	}
	s.statsd.TimeInMilliseconds(
		"flush.part_duration_ns",
		float64(time.Now().Sub(fstart).Nanoseconds()),
		[]string{"part:post"},
		1.0,
	)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// don't bail out if this errors, we'll just log the body as empty
		s.logger.WithError(err).Error("Error reading response body")
	}
	resultFields := logrus.Fields{
		"status":           resp.Status,
		"request_headers":  req.Header,
		"response_headers": resp.Header,
		"request_length":   bodyLength,
		"response":         string(body),
		"total_metrics":    len(metricSlice),
	}

	if resp.StatusCode != http.StatusAccepted {
		s.statsd.Count("flush.error_total", 1, []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		s.logger.WithFields(resultFields).Error("Error POSTing")
		return
	}

	s.logger.WithFields(resultFields).Debug("POSTing JSON")
}
