package attribution

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
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

// AttributionSinkConfig represents the YAML options that can be used to
// configure an instance of AttributionSink
type AttributionSinkConfig struct {
	AWSAccessKeyID     util.StringSecret `yaml:"aws_access_key_id"`
	AWSRegion          string            `yaml:"aws_region"`
	AWSSecretAccessKey util.StringSecret `yaml:"aws_secret_access_key"`
	OwnerKey           string            `yaml:"owner_key"`
	S3Bucket           string            `yaml:"s3_bucket"`
	S3KeyPrefix        string            `yaml:"s3_key_prefix"`
	VeneurInstanceID   string            `yaml:"veneur_instance_id"`
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
	traceClient     *trace.Client
	log             *logrus.Entry
	s3KeyPrefix     string
	hostname        string
	s3Svc           s3iface.S3API
	s3Bucket        string
	attributionData map[string]*Timeseries
	ownerKey        string
}

// S3ClientUninitializedError is an error returned when the S3 client provided to
// AttributionSink is nil
var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

// Timeseries represents an attributable unit of metrics data, complete with
// owner, metadata corresponding to the metrics data grouping, and cardinality
// tallying
type Timeseries struct {
	// TODO: This shouldn't be a full InterMetric, it should just be the
	// relevant fields we pull from an InterMetric
	Metric samplers.InterMetric
	Owner  string
	Sketch *hyperloglog.Sketch
}

// Create returns a pointer to an AttributionSink
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

	return &AttributionSink{
		traceClient:     nil,
		log:             logger,
		s3KeyPrefix:     attributionSinkConfig.S3KeyPrefix,
		hostname:        attributionSinkConfig.VeneurInstanceID,
		s3Svc:           s3.New(sess),
		s3Bucket:        attributionSinkConfig.S3Bucket,
		attributionData: map[string]*Timeseries{},
		ownerKey:        attributionSinkConfig.OwnerKey,
	}, nil
}

// Start starts the AttributionSink
func (s *AttributionSink) Start(traceClient *trace.Client) error {
	s.traceClient = traceClient
	return nil
}

// Name returns the name of the sink
func (s *AttributionSink) Name() string {
	return "attribution"
}

// Flush tallies together metrics, then makes a PUT request to the AWS API to
// persist attribution data to S3.
func (s *AttributionSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)

	flushStart := time.Now()

	for _, metric := range metrics {
		s.log.Debug(fmt.Sprintf("Checking %s", metric.Name))
		if sinks.IsAcceptableMetric(metric, s) {
			s.recordMetric(metric)
		}
	}

	if s.log.Level >= logrus.DebugLevel {
		for k, v := range s.attributionData {
			s.log.Debug(fmt.Sprintf("%s - %d", k, v.Sketch.Estimate()))
		}
	}

	csv, err := encodeAttributionDataCSV(s.attributionData)
	if err != nil {
		s.log.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Could not marshal metrics before posting to s3")
		return err
	}

	err = s.s3Post(csv)
	if err != nil {
		s.log.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Error posting to s3")
		return err
	}

	s.log.WithField("metrics", len(metrics)).Debug("Completed flush to s3")
	spanTags := map[string]string{"sink": s.Name()}
	span.Add(ssf.Timing(sinks.MetricKeyMetricFlushDuration, time.Since(flushStart), time.Nanosecond, spanTags))
	return nil
}

// timeseriesID takes in InterMetric and derives an identifier that can be used
// to tell if multiple InterMetrics correspond to the same Timeseries, for
// tallying purposes
func timeseriesID(metric samplers.InterMetric) string {
	var b strings.Builder
	b.WriteString(metric.Name)

	// Workers have some notion of timeseries uniqueness, but one that isn't
	// quite rigorous enough to perform timeseries cardinality computation
	// accurately:
	// https://github.com/stripe/veneur/blob/531ffab0dc9bc105a69fa9106023ddc36b49591e/samplers/parser.go#L38-L60
	// DogStatsD tags can be ordered arbitrarily, but (*UDPMetric) UpdateTags
	// does not take into account that (foo:val, bar:val) and
	// (bar:val, foo:val) are semantically identical. However, it's a more
	// involved refactor to make Tags order agnostic, so in the meantime we
	// have this hack so we can accurately tally timeseries:
	tagsToWrite := metric.Tags // make a copy
	sort.Strings(tagsToWrite)

	for _, tag := range tagsToWrite {
		b.WriteString(tag)
	}
	return b.String()
}

// timeseriesGroupIDAndOwner takes an InterMetric and the name of a tag that
// can be used to derive a Timeseries owner, and returns an identifier that
// ultimately corresponds to a TSV row and tally
//
// If an ownerKey is not specified, timeseriesGroupIDAndOwner returns the name
// of the metric and a blank owner
func timeseriesGroupIDAndOwner(metric samplers.InterMetric, ownerKey string) (groupID, owner string) {
	var b strings.Builder
	b.WriteString(metric.Name)

	// Find the tag matching ownerKey and add its value to the Group ID
	for _, tag := range metric.Tags {
		tagSplit := strings.SplitN(tag, fmt.Sprintf("%s:", tag), 2)
		if len(tagSplit) == 2 && tagSplit[0] == ownerKey {
			owner = tagSplit[1]
			b.WriteString(owner)
		}
	}

	groupID = b.String()
	return
}

// recordMetric tallies an InterMetric, creating a Timeseries in the process
func (s *AttributionSink) recordMetric(metric samplers.InterMetric) {
	tsGroupID, owner := timeseriesGroupIDAndOwner(metric, s.ownerKey)
	ts, ok := s.attributionData[tsGroupID]
	if !ok {
		ts = &Timeseries{metric, owner, hyperloglog.New()}
		s.attributionData[tsGroupID] = ts
	}
	tsID := timeseriesID(metric)
	ts.Sketch.Insert([]byte(tsID))
}

// encodeInterMetricsCSV returns a reader containing the gzipped CSV
// representation of Timeseries data, one row per Timeseries. The AWS SDK
// requires seekable input, so we return a ReadSeeker here
func encodeAttributionDataCSV(attributionData map[string]*Timeseries) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = '\t'

	for _, metric := range attributionData {
		encodeInterMetricCSV(metric, w)
	}

	w.Flush()
	err := gzw.Close()
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b.Bytes()), w.Error()
}

// s3Post takes in an io.ReadSeeker created in Flush, and persists the data in
// question to S3
func (s *AttributionSink) s3Post(data io.ReadSeeker) error {
	if s.s3Svc == nil {
		return S3ClientUninitializedError
	}
	key := s3Key(s.s3KeyPrefix, s.hostname)
	params := &s3.PutObjectInput{
		Bucket: aws.String(s.s3Bucket),
		Key:    aws.String(key),
		Body:   data,
	}

	s.log.WithFields(logrus.Fields{
		"bucket": s.s3Bucket,
		"key":    key,
	}).Debug("Posting to S3")

	_, err := s.s3Svc.PutObject(params)
	return err
}

// s3Key generates the full key attribution data is to be stored at in S3.
//
// For the time being, the key format here is hardcoded assuming 1h wide
// partitions. In the future, it may be useful to expand to other partition
// widths, e.g.:
// aws_s3_key_template: "{{ .TimeUnix }}/{{ .SchemaVersion }}/{{ .Hostname }}"
// Note that the example setting here is hypothetical and does NOT yet exist.
//
// AWS documentation makes reference to a baseline S3 PUT rate of 3.5k req/s
// per S3 prefix for a given application:
// https://docs.aws.amazon.com/AmazonS3/latest/userguide/optimizing-performance.html
// It is unclear whether this rate limit works on a per-bucket, or
// per-application-per-bucket level. Should it be per-bucket, encoding a unique
// Veneur instance ID per instance of the attribution sink (such as a hostname
// or Kubernetes pod ID) will help a fleet of Veneurs avoid hitting this limit.
func s3Key(s3KeyPrefix, hostname string) string {
	t := time.Now().UTC()
	exactFlushTs := t.Unix()
	hourPartition := t.Format("2006/01/02/03")

	prefix := ""
	if s3KeyPrefix != "" {
		prefix = fmt.Sprintf("%s/", s3KeyPrefix)
	}

	key := fmt.Sprintf("%s%s/%s-%d.tsv.gz", prefix, hourPartition, hostname, exactFlushTs)
	return key
}

// FlushOtherSamples is a no-op for the time being, so span attribution does
// not yet exist
func (s *AttributionSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	return
}
