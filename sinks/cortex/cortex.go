package cortex

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

const (
	// TIMEOUT is the time in seconds before a remote-write request fails
	TIMEOUT = 0
	// MAX_CONNS is the number of simultaneous connections we allow to one host
	MAX_CONNS = 100
)

// CortexMetricSink writes metrics to one or more configured Cortex instances
// using the prometheus remote-write API. For specifications, see
// https://github.com/prometheus/compliance/tree/main/remote_write
type CortexMetricSink struct {
	URL    string
	client *http.Client
}

// NewCortexMetricSink creates and returns a new instance of the sink
func NewCortexMetricSink(URL string) (*CortexMetricSink, error) {
	return &CortexMetricSink{URL: URL}, nil
}

// Name returns the string cortex
func (s *CortexMetricSink) Name() string {
	return "cortex"
}

// Start sets up the HTTP client for writing to Cortex
func (s *CortexMetricSink) Start(*trace.Client) error {
	// Default concurrent connections is 2
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = MAX_CONNS
	t.MaxConnsPerHost = MAX_CONNS
	t.MaxIdleConnsPerHost = MAX_CONNS

	s.client = &http.Client{
		Timeout:   time.Duration(TIMEOUT * time.Second),
		Transport: t,
	}
	return nil
}

// Flush sends a batch of metrics to the configured remote-write endpoint
func (s *CortexMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	wr := makeWriteRequest(metrics)
	data, err := proto.Marshal(wr)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	encoded := snappy.Encode(nil, data)
	buf.Write(encoded)

	req, err := http.NewRequest("POST", s.URL, &buf)
	if err != nil {
		return err
	}

	// This set of headers is prescribed by the remote-write standard
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", "veneur/cortex")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	r, err := s.client.Do(req)
	// failing to close the body can result in resource leak
	defer r.Body.Close()

	return err
}

func (s *CortexMetricSink) FlushOtherSamples(context.Context, []ssf.SSFSample) {
	return
}

// makeWriteRequest converts a list of samples from a flush into a single
// prometheus remote-write compatible protobuf object
func makeWriteRequest(metrics []samplers.InterMetric) *prompb.WriteRequest {
	ts := make([]*prompb.TimeSeries, len(metrics))
	for i, metric := range metrics {
		ts[i] = metricToTimeSeries(metric)
	}

	return &prompb.WriteRequest{
		Timeseries: ts,
	}
}

// metricToTimeSeries converts a sample to a prometheus timeseries.
// This is not a 1:1 conversion! We constrain metric and label names to the
// legal set of characters (see https://prometheus.io/docs/practices/naming)
// and we drop tags which are not in "key:value" format
// (see https://prometheus.io/docs/concepts/data_model/)
func metricToTimeSeries(metric samplers.InterMetric) *prompb.TimeSeries {
	var ts prompb.TimeSeries
	ts.Labels = []*prompb.Label{
		&prompb.Label{"__name__", sanitise(metric.Name)},
	}
	for _, tag := range metric.Tags {
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) < 2 {
			continue // drop illegal tag
		}
		ts.Labels = append(ts.Labels, &prompb.Label{sanitise(kv[0]), kv[1]})
	}

	// Prom format has the ability to carry batched samples, in this instance we
	// send a single sample per write. Probably worth exploring this as an area
	// for optimisation if we find the write path becomes contended
	ts.Samples = []prompb.Sample{
		prompb.Sample{metric.Value, metric.Timestamp},
	}

	return &ts
}

// sanitise replaces all characters which are not in the set [a-zA-Z0-9_:]
// with underscores, and additionally prefixes the whole with an underscore if
// the first character is numeric
func sanitise(input string) string {
	output := make([]rune, len(input))
	prefix := false

	// this relatively verbose byte comparison is ~4x quicker than using a single
	// compiled regexp.ReplaceAll, and we'll be calling it a lot
	for i, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			output[i] = r
		case r >= 'A' && r <= 'Z':
			output[i] = r
		case r >= '0' && r <= '9':
			if i == 0 {
				prefix = true
			}
			output[i] = r
		case r == '_':
			output[i] = r
		case r == ':':
			output[i] = r
		default:
			output[i] = '_'
		}
	}
	if prefix {
		return "_" + string(output)
	}
	return string(output)
}
