package attribution

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

// S3ClientUninitializedError is an error returned when the S3 client provided to
// AttributionSink is nil
var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

type TimeseriesKey []string

type Timeseries struct {
	Name string
	Tags []string
}

func (ts *Timeseries) Serialize() (serialized []byte) {
	serialized := make([]byte, len(ts.Name))
	serialized = append(serialized, []byte(ts.Name))
	for _, tag := range ts.Tags {
		serialized = append(serialized, tag)
	}
}

// AttributionSink is a sink for flushing ownership/usage (attribution data) to S3
type AttributionSink struct {
	traceClient     *trace.Client
	log             *logrus.Logger
	hostname        string
	s3Svc           s3iface.S3API
	s3Bucket        string
	attributionData map[TimeseriesKey]*hyperloglog.Sketch
	groupByTag      []string
}

// TimeseriesSet is used to determine approx. how many timeseries corresponding to a
// grouping of metric attributes (names, ownership tags) are being flushed in one batch
// type TimeseriesSet struct {
// 	// Group is typically at least a metric name, but can include other metadata
// 	Group []byte
// 	// Hll keeps track of approximate unique MTS corrsesponding to a Groop
// 	Hll   *hyperloglog.Sketch
// }

// NewAttributionSink creates a new sink to flush ownership/usage (attribution) data to S3
func NewAttributionSink(log *logrus.Logger, hostname string, s3Svc s3iface.S3API, s3Bucket string, groupByTag []string) (*AttributionSink, error) {
	// NOTE: We don't need to account for config.Tags because they do not, generally speaking,
	// increase cardinality on a per-Veneur level. The metric attribution schemas are designed for
	// to account for config.Tags increasing cardinality across a fleet of Veneurs, such that many
	// attribution dumps can be batch aggregated together accurately.
	return &AttributionSink{nil, log, hostname, s3Svc, s3Bucket, groupByTag}, nil
}

// Start starts the AttributionSink
func (s *AttributionSink) Start(traceClient *trace.Client) error {
	s.traceClient = traceClient
	return nil
}

func (s *AttributionSink) Name() string {
	return "attribution"
}

func (s *AttributionSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)

	flushStart := time.Now()
	spanTags := map[string]string{"sink": s.Name()}

	for _, metric := range metrics {
		if sinks.IsAcceptableMetric(metric, s) {
			s.recordMetric(metric)
		}
	}

	s.log.WithField("metrics", len(metrics)).Debug("Completed flush to s3")

	// csv, err := encodeInterMetricsCSV(metrics, flushStart)
	// if err != nil {
	// 	s.log.WithFields(logrus.Fields{
	// 		logrus.ErrorKey: err,
	// 		"metrics":       len(metrics),
	// 	}).Error("Could not marshal metrics before posting to s3")
	// 	return err
	// }

	// err = s.s3Post(csv)
	// if err != nil {
	// 	s.log.WithFields(logrus.Fields{
	// 		logrus.ErrorKey: err,
	// 		"metrics":       len(metrics),
	// 	}).Error("Error posting to s3")
	// 	return err
	// }

	// s.log.WithField("metrics", len(metrics)).Debug("Completed flush to s3")
	// span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, spanTags))
	return nil
}

// recordMetric tallies an InterMetric
func (s *AttributionSink) recordMetric(metric samplers.InterMetric) {
	key := make([]string, len(s.groupByTag) + 1)
	key = append(key, metric.Name)

	// Extract information from tags for grouping purposes, if s.groupByTag is non-empty
	if len(s.groupByTag) > 0 {
		tagKeyComponents := make([]string, len(s.groupByTag))
		for _, tag := range metric.Tags {
			for i, tagName := range s.groupByTag {
				kv := strings.SplitN(tag, fmt.Sprintf("%s:", tagName), 2)
				key := kv[0]
				if len(kv) == 2 {
					tagKeyComponents[i] = kv[1]
				}
			}
		}
		key = append(key, tagKeyComponents)
	}

	timeseries := Timeseries{metric.Name, metric.Tags}
	serialized := timeseries.Serialize()

	hll, ok := s.attributionData[key]
	if ok {
		s.attributionData[key].Insert(serialized)
	} else {
		s.attributionData[key] = hyperloglog.New()
	}
}

// encodeInterMetricsCSV returns a reader containing the gzipped CSV representation of the
// InterMetric data, one row per InterMetric. The AWS sdk requires seekable input, so we return a ReadSeeker here.
func encodeInterMetricsCSV(metrics []samplers.InterMetric, ts time.Time) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = '\t'

	attributionReport := compileAttributionReport(metrics)

	for _, metric := range metrics {
		encodeInterMetricCSV(metric, w, ts)
	}

	w.Flush()
	err := gzw.Close()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b.Bytes()), w.Error()
}

func (s *AttributionSink) s3Post(data io.ReadSeeker) error {
	if s.s3Svc == nil {
		return S3ClientUninitializedError
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    s3Key(s.hostname),
		Body:   data,
	}

	_, err := s.s3Svc.PutObject(params)
	return err
}

func s3Key(hostname string) *string {
	// NOTE: It would be cool if we could do something like this instead of hardcoding
	// 1h partitions:
    // aws_s3_key_template: "{{ .TimeUnix }}/{{ .SchemaVersion }}/{{ .Hostname }}"
	t := time.Now().UTC()
	exactFlushTs := t.Unix()
	hourPartition := t.Format("2006/01/02/03")
	key := fmt.Sprintf("%s/%s-%s.tsv.gz", hourPartition, hostname, exactFlushTs)
	return aws.String(key)
}

// FlushOtherSamples is a no-op for the time being
// TODO: Implement span attribution
func (s *AttributionSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	return
}
