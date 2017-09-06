package veneur

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/trace"
)

type metricSink interface {
	Name() string
	Flush(context.Context, []samplers.InterMetric) error
}

type datadogMetricSink struct {
	HTTPClient      *http.Client
	hostname        string
	apiKey          string
	flushMaxPerBody int
	statsd          *statsd.Client
	tags            []string
}

// DDMetric is a data structure that represents the JSON that Datadog
// wants when posting to the API
type DDMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float64 `json:"points"`
	Tags       []string      `json:"tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host,omitempty"`
	DeviceName string        `json:"device_name,omitempty"`
	Interval   int32         `json:"interval,omitempty"`
}

// NewDatadogMetricSink creates a new Datadog sink for trace spans.
func NewDatadogMetricSink(config *Config, httpClient *http.Client, stats *statsd.Client) (*datadogMetricSink, error) {
	return &datadogMetricSink{
		HTTPClient: httpClient,
		statsd:     stats,
	}, nil
}

// Name returns the name of this sink.
func (dd *datadogMetricSink) Name() string {
	return "datadog"
}

func (dd *datadogMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.Finish()

	// break the metrics into chunks of approximately equal size, such that
	// each chunk is less than the limit
	// we compute the chunks using rounding-up integer division
	workers := ((len(metrics) - 1) / dd.flushMaxPerBody) + 1
	chunkSize := ((len(metrics) - 1) / workers) + 1
	log.WithField("workers", workers).Debug("Worker count chosen")
	log.WithField("chunkSize", chunkSize).Debug("Chunk size chosen")
	var wg sync.WaitGroup
	flushStart := time.Now()
	for i := 0; i < workers; i++ {
		chunk := metrics[i*chunkSize:]
		if i < workers-1 {
			// trim to chunk size unless this is the last one
			chunk = chunk[:chunkSize]
		}
		wg.Add(1)
		go dd.flushPart(span.Attach(ctx), chunk, &wg)
	}
	wg.Wait()
	dd.statsd.TimeInMilliseconds("flush.total_duration_ns", float64(time.Since(flushStart).Nanoseconds()), []string{"part:post"}, 1.0)

	log.WithField("metrics", len(metrics)).Info("Completed flush to Datadog")
	return nil
}

func (dd *datadogMetricSink) finalizeMetrics(metrics []samplers.InterMetric) []DDMetric {
	ddMetrics := make([]DDMetric, len(metrics))

	for _, m := range metrics {
		ddMetric := DDMetric{
			Name: m.Name,
			Value: [1][2]float64{
				[2]float64{
					float64(m.Timestamp), m.Value,
				},
			},
			Tags:       dd.tags,
			MetricType: m.MetricType,
		}

		// Let's look for "magic tags" that override metric fields host and device.
		for _, tag := range m.Tags {
			// This overrides hostname
			if strings.HasPrefix(tag, "host:") {
				// Override the hostname with the tag, trimming off the prefix.
				ddMetric.Hostname = tag[5:]
			} else if strings.HasPrefix(tag, "device:") {
				// Same as above, but device this time
				ddMetric.DeviceName = tag[7:]
			} else {
				// Add it, no reason to exclude it.
				ddMetric.Tags = append(ddMetric.Tags, tag)
			}
		}
		if ddMetric.Hostname == "" {
			// No magic tag, set the hostname
			ddMetric.Hostname = dd.hostname
		}
	}

	return ddMetrics
}

func (dd *datadogMetricSink) flushPart(ctx context.Context, metricSlice []samplers.InterMetric, wg *sync.WaitGroup) {
	defer wg.Done()
	dd.postHelper(ctx, dd.HTTPClient, dd.statsd, fmt.Sprintf("%s/api/v1/series?api_key=%s", dd.hostname, dd.apiKey), map[string][]samplers.InterMetric{
		"series": metricSlice,
	}, "flush", true)
}

// shared code for POSTing to an endpoint, that consumes JSON, that is zlib-
// compressed, that returns 202 on success, that has a small response
// action is a string used for statsd metric names and log messages emitted from
// this function - probably a static string for each callsite
// you can disable compression with compress=false for endpoints that don't
// support it
func (dd *datadogMetricSink) postHelper(ctx context.Context, httpClient *http.Client, stats *statsd.Client, endpoint string, bodyObject interface{}, action string, compress bool) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	span.SetTag("action", action)
	defer span.Finish()

	// attach this field to all the logs we generate
	innerLogger := log.WithField("action", action)

	marshalStart := time.Now()
	var (
		bodyBuffer bytes.Buffer
		encoder    *json.Encoder
		compressor *zlib.Writer
	)
	if compress {
		compressor = zlib.NewWriter(&bodyBuffer)
		encoder = json.NewEncoder(compressor)
	} else {
		encoder = json.NewEncoder(&bodyBuffer)
	}
	if err := encoder.Encode(bodyObject); err != nil {
		stats.Count(action+".error_total", 1, []string{"cause:json"}, 1.0)
		innerLogger.WithError(err).Error("Could not render JSON")
		return err
	}
	if compress {
		// don't forget to flush leftover compressed bytes to the buffer
		if err := compressor.Close(); err != nil {
			stats.Count(action+".error_total", 1, []string{"cause:compress"}, 1.0)
			innerLogger.WithError(err).Error("Could not finalize compression")
			return err
		}
	}
	stats.TimeInMilliseconds(action+".duration_ns", float64(time.Since(marshalStart).Nanoseconds()), []string{"part:json"}, 1.0)

	// Len reports the unread length, so we have to record this before the
	// http client consumes it
	bodyLength := bodyBuffer.Len()
	stats.Histogram(action+".content_length_bytes", float64(bodyLength), nil, 1.0)

	req, err := http.NewRequest(http.MethodPost, endpoint, &bodyBuffer)

	if err != nil {
		stats.Count(action+".error_total", 1, []string{"cause:construct"}, 1.0)
		innerLogger.WithError(err).Error("Could not construct request")
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if compress {
		req.Header.Set("Content-Encoding", "deflate")
	}
	// we only make http requests at flush time, so keepalive is not a big win
	req.Close = true

	err = tracer.InjectRequest(span.Trace, req)
	if err != nil {
		stats.Count("veneur.opentracing.flush.inject.errors", 1, nil, 1.0)
		innerLogger.WithError(err).Error("Error injecting header")
	}

	requestStart := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			// if the error has the url in it, then retrieve the inner error
			// and ditch the url (which might contain secrets)
			err = urlErr.Err
		}
		stats.Count(action+".error_total", 1, []string{"cause:io"}, 1.0)
		innerLogger.WithError(err).Error("Could not execute request")
		return err
	}
	stats.TimeInMilliseconds(action+".duration_ns", float64(time.Since(requestStart).Nanoseconds()), []string{"part:post"}, 1.0)
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// this error is not fatal, since we only need the body for reporting
		// purposes
		stats.Count(action+".error_total", 1, []string{"cause:readresponse"}, 1.0)
		innerLogger.WithError(err).Error("Could not read response body")
	}
	resultLogger := innerLogger.WithFields(logrus.Fields{
		"endpoint":         endpoint,
		"request_length":   bodyLength,
		"request_headers":  req.Header,
		"status":           resp.Status,
		"response_headers": resp.Header,
		"response":         string(responseBody),
	})

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		stats.Count(action+".error_total", 1, []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		resultLogger.Error("Could not POST")
		return err
	}

	// make sure the error metric isn't sparse
	stats.Count(action+".error_total", 0, nil, 1.0)
	resultLogger.Debug("POSTed successfully")
	return nil
}
