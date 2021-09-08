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
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/axiomhq/hyperloglog"
	"github.com/sirupsen/logrus"

	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

type AttributionSinkConfig struct {
	VeneurInstanceID   string            `yaml:"veneur_instance_id"`
	AWSAccessKeyID     util.StringSecret `yaml:"aws_access_key_id"`
	AWSRegion          string            `yaml:"aws_region"`
	AWSSecretAccessKey util.StringSecret `yaml:"aws_secret_access_key"`
	S3Bucket           string            `yaml:"s3_bucket"`
	S3KeyPrefix        string            `yaml:"s3_key_prefix"`
}

// ParseConfig decodes the map config for an S3 sink into an S3SinkConfig
// struct.
func ParseConfig(config interface{}) (veneur.MetricSinkConfig, error) {
	attributionSinkConfig := AttributionSinkConfig{}
	err := util.DecodeConfig(config, &attributionSinkConfig)
	if err != nil {
		return nil, err
	}
	return attributionSinkConfig, nil
}

// AttributionSink is a sink for flushing ownership/usage (attribution data) to S3
type AttributionSink struct {
	traceClient         *trace.Client
	log                 *logrus.Entry
	additionalKeyPrefix string
	hostname            string
	s3Svc               s3iface.S3API
	s3Bucket            string
	attributionData     map[string]*hyperloglog.Sketch
}

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
	// TODO: Check whether tags are ordered past samplers.go
	for _, tag := range ts.Tags {
		b.WriteString(tag)
	}
	return b.String()
}

func (ts *Timeseries) GroupID() string {
	var b strings.Builder
	b.WriteString(ts.Name)
	return b.String()
}

func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	attributionSinkConfig, ok := sinkConfig.(AttributionSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	awsID := attributionSinkConfig.AWSAccessKeyID
	awsSecret := attributionSinkConfig.AWSSecretAccessKey
	var sess *session.Session
	var err error
	if len(awsID.Value) > 0 && len(awsSecret.Value) > 0 {
		sess, err = session.NewSession(&aws.Config{
			Region: aws.String(attributionSinkConfig.AWSRegion),
			Credentials: credentials.NewStaticCredentials(
				awsID.Value, awsSecret.Value, ""),
		})
	} else {
		sess, err = session.NewSession(&aws.Config{
			Region: aws.String(attributionSinkConfig.AWSRegion),
		})
	}

	if err != nil {
		logger.Infof("error getting AWS session: %s", err)
		logger.Info("S3 archives are disabled")
		return nil, err
	}

	logger.Info("Successfully created AWS session")
	logger.Info("S3 attribution sink is enabled")

	groupings := make([]string, 1)
	groupings = append(groupings, "host_contact")

	return &AttributionSink{
		traceClient:         nil,
		log:                 logger,
		additionalKeyPrefix: "rmatest1",
		hostname:            "sirenbox-1234.northwest.qa.stripe.io",
		s3Svc:               s3.New(sess),
		s3Bucket:            attributionSinkConfig.S3Bucket,
		attributionData:     map[string]*hyperloglog.Sketch{},
	}, nil
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

	// TODO: Don't even iterate if debug level is not Debug
	for k, v := range s.attributionData {
		s.log.Debug(fmt.Sprintf("%s - %d", k, v.Estimate()))
	}

	// csv, err := encodeAttributionDataCSV(s.attributionData, flushStart)
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

	s.log.WithField("metrics", len(metrics)).Debug("Completed flush to s3")
	// span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, spanTags))
	return nil
}

// recordMetric tallies an InterMetric
func (s *AttributionSink) recordMetric(metric samplers.InterMetric) {
	ts := Timeseries{metric.Name, metric.Tags}
	tsID := ts.ID()
	tsGroupID := ts.GroupID()

	_, ok := s.attributionData[tsGroupID]
	if !ok {
		s.attributionData[tsGroupID] = hyperloglog.New()
	}
	s.attributionData[tsGroupID].Insert([]byte(tsID))
}

// encodeInterMetricsCSV returns a reader containing the gzipped CSV representation of the
// InterMetric data, one row per InterMetric. The AWS sdk requires seekable input, so we return a ReadSeeker here.
func encodeAttributionDataCSV(metrics []samplers.InterMetric, ts time.Time) (io.ReadSeeker, error) {
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
