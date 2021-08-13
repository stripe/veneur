package attribution

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/axiomhq/hyperloglog"
	"github.com/sirupsen/logrus"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

// S3ClientUninitializedError is an error returned when the S3 client provided to
// AttributionSink is nil
var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

type Timeseries struct {
	Name string
	Tags []string
}

func (ts *Timeseries) ID() string {
	var b strings.Builder
	b.WriteString(ts.Name)
	// TODO: This needs to be order sensitive
	for _, tag := range ts.Tags {
		b.WriteString(tag)
	}
	return b.String()
}

func (ts *Timeseries) GroupID(groupByTag []string) string {
	var b strings.Builder
	b.WriteString(ts.Name)

	// Extract information from tags for grouping purposes, if s.groupByTag is non-empty
	if len(groupByTag) > 0 {
		tagKeyComponents := make([]string, len(groupByTag))
		for _, tag := range ts.Tags {
			for i, tagName := range groupByTag {
				kv := strings.SplitN(tag, fmt.Sprintf("%s:", tagName), 2)
				if len(kv) == 2 {
					tagKeyComponents[i] = kv[1]
				}
			}
		}
		for _, component := range tagKeyComponents {
			b.WriteString(component)
		}
	}

	return b.String()
}

// AttributionSink is a sink for flushing ownership/usage (attribution data) to S3
type AttributionSink struct {
	traceClient         *trace.Client
	log                 *logrus.Logger
	additionalKeyPrefix string
	hostname            string
	s3Svc               s3iface.S3API
	s3Bucket            string
	attributionData     map[string]*hyperloglog.Sketch
	groupByTag          []string
}

// NewAttributionSink creates a new sink to flush ownership/usage (attribution) data to S3
func NewAttributionSink(log *logrus.Logger, additionalKeyPrefix string, hostname string, s3Svc s3iface.S3API, s3Bucket string, groupByTag []string) (*AttributionSink, error) {
	// NOTE: We don't need to account for config.Tags because they do not, generally speaking,
	// increase cardinality on a per-Veneur level. The metric attribution schemas are designed for
	// to account for config.Tags increasing cardinality across a fleet of Veneurs, such that many
	// attribution dumps can be batch aggregated together accurately.
	return &AttributionSink{nil, log, "rmatest1", hostname, s3Svc, s3Bucket, map[string]*hyperloglog.Sketch{}, groupByTag}, nil
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

	// flushStart := time.Now()
	// spanTags := map[string]string{"sink": s.Name()}

	for _, metric := range metrics {
		s.log.Debug(fmt.Sprintf("Checking %s", metric.Name))
		if sinks.IsAcceptableMetric(metric, s) {
			s.recordMetric(metric)
		}
	}

	for k, v := range s.attributionData {
		s.log.Debug(fmt.Sprintf("%s - %d", k, v.Estimate()))
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
	ts := Timeseries{metric.Name, metric.Tags}
	tsID := ts.ID()
	tsGroupID := ts.GroupID(s.groupByTag)

	_, ok := s.attributionData[tsGroupID]
	if !ok {
		s.attributionData[tsGroupID] = hyperloglog.New()
	}
	s.attributionData[tsGroupID].Insert([]byte(tsID))
}

// encodeInterMetricsCSV returns a reader containing the gzipped CSV representation of the
// InterMetric data, one row per InterMetric. The AWS sdk requires seekable input, so we return a ReadSeeker here.
func encodeInterMetricsCSV(metrics []samplers.InterMetric, ts time.Time) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = '\t'

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
		Key:    s3Key(s.additionalKeyPrefix, s.hostname),
		Body:   data,
	}

	_, err := s.s3Svc.PutObject(params)
	return err
}

func s3Key(additionalKeyPrefix, hostname string) *string {
	// NOTE: It would be cool if we could do something like this instead of hardcoding
	// 1h partitions:
    // aws_s3_key_template: "{{ .TimeUnix }}/{{ .SchemaVersion }}/{{ .Hostname }}"
	t := time.Now().UTC()
	exactFlushTs := t.Unix()
	hourPartition := t.Format("2006/01/02/03")
	key := fmt.Sprintf("%s/%s-%d.tsv.gz", hourPartition, hostname, exactFlushTs)
	return aws.String(key)
}

// FlushOtherSamples is a no-op for the time being
// TODO: Implement span attribution
func (s *AttributionSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	return
}
