package influxdb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur/plugins"
	"github.com/stripe/veneur/samplers"
)

var _ plugins.Plugin = &InfluxDBPlugin{}

// A helper type that we use to allow a `Len()` call
// on an io.Reader
type lengther interface {
	Len() int
}

// InfluxDBPlugin is a plugin for emitting metrics to InfluxDB.
type InfluxDBPlugin struct {
	Logger     *logrus.Logger
	InfluxURL  string
	HTTPClient *http.Client
	Statsd     *statsd.Client
}

// NewInfluxDBPlugin creates a new Influx Plugin.
func NewInfluxDBPlugin(logger *logrus.Logger, addr string, consistency string, db string, client *http.Client, stats *statsd.Client) *InfluxDBPlugin {
	plugin := &InfluxDBPlugin{
		Logger:     logger,
		HTTPClient: client,
		Statsd:     stats,
	}

	inurl, err := url.Parse(addr)
	if err != nil {
		logger.Fatalf("Error parsing URL for InfluxDB: %q", err)
	}

	// Construct a path we will be using later.
	inurl.Path = "/write"
	q := inurl.Query()
	q.Set("db", db)
	q.Set("precision", "s")
	inurl.RawQuery = q.Encode()
	plugin.InfluxURL = inurl.String()

	return plugin
}

// Flush sends a slice of metrics to InfluxDB
func (p *InfluxDBPlugin) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	p.Statsd.Gauge("flush.post_metrics_total", float64(len(metrics)), nil, 1.0)
	// Check to see if we have anything to do
	if len(metrics) == 0 {
		p.Logger.Info("Nothing to flush, skipping.")
		return nil
	}

	buff := bytes.Buffer{}
	colons := regexp.MustCompile(":")
	for _, metric := range metrics {
		tags := strings.Join(metric.Tags, ",")
		// This is messy and we shouldn't have to do it this way, but since Veneur treats tags as arbitrary strings
		// rather than name value pairs, we have to do this ugly conversion
		cleanTags := colons.ReplaceAllLiteralString(tags, "=")
		buff.WriteString(
			fmt.Sprintf("%s,%s value=%f %d\n", metric.Name, cleanTags, metric.Value, metric.Timestamp),
		)
	}

	p.postHelper(p.InfluxURL, &buff)

	return nil
}

// Name returns the name of the plugin.
func (p *InfluxDBPlugin) Name() string {
	return "influxdb"
}

// Common for POSTing to an endpoint, that consumes JSON.
func (p *InfluxDBPlugin) postHelper(endpoint string, bodyBuffer io.Reader) error {

	// attach this field to all the logs we generate
	innerLogger := p.Logger.WithField("action", "influxdb_post")

	// Len reports the unread length, so we have to record this before the
	// http client consumes it
	if lenReader, ok := bodyBuffer.(lengther); ok {
		bodyLength := lenReader.Len()
		p.Statsd.Histogram("influxdb_post.content_length_bytes", float64(bodyLength), nil, 1.0)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bodyBuffer)
	if err != nil {
		p.Statsd.Count("influxdb_post.error_total", 1, []string{"cause:construct"}, 1.0)
		innerLogger.WithError(err).Error("Could not construct request")
		return err
	}

	// we only make http requests at flush time, so keepalive is not a big win
	req.Close = true

	requestStart := time.Now()
	resp, err := p.HTTPClient.Do(req)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			// if the error has the url in it, then retrieve the inner error
			// and ditch the url (which might contain secrets)
			err = urlErr.Err
		}
		p.Statsd.Count("influxdb_post.error_total", 1, []string{"cause:io"}, 1.0)
		innerLogger.WithError(err).Error("Could not execute request")
		return err
	}
	p.Statsd.TimeInMilliseconds("influxdb_post.duration_ns", float64(time.Now().Sub(requestStart).Nanoseconds()), []string{"part:post"}, 1.0)
	defer resp.Body.Close()

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// this error is not fatal, since we only need the body for reporting
		// purposes
		p.Statsd.Count("influxdb_post.error_total", 1, []string{"cause:readresponse"}, 1.0)
		innerLogger.WithError(err).Error("Could not read response body")
	}
	resultLogger := innerLogger.WithFields(logrus.Fields{
		"request_headers":  req.Header,
		"status":           resp.Status,
		"response_headers": resp.Header,
		"response":         string(responseBody),
	})

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		p.Statsd.Count("influxdb_post.error_total", 1, []string{fmt.Sprintf("cause:%d", resp.StatusCode)}, 1.0)
		resultLogger.Error("Could not POST")
		return err
	}

	// make sure the error metric isn't sparse
	p.Statsd.Count("influxdb_post.error_total", 0, nil, 1.0)
	resultLogger.Debug("POSTed successfully")
	return nil
}
