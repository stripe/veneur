package s3

import (
	"context"
	"errors"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/sirupsen/logrus"

	veneur "github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

type S3SinkConfig struct {
	AccessKeyID     util.StringSecret `yaml:"access_key_id"`
	S3Bucket        string            `yaml:"s3_bucket"`
	Region          string            `yaml:"region"`
	SecretAccessKey util.StringSecret `yaml:"secret_access_key"`
}

type S3Sink struct {
	Hostname string
	Interval int
	Logger   *logrus.Entry
	name     string
	S3Bucket string
	Svc      s3iface.S3API
}

// TODO(arnavdugar): Remove this once the old configuration format has been
// removed.
func MigrateConfig(config *veneur.Config) {
	if config.AwsS3Bucket == "" {
		return
	}
	config.MetricSinks = append(config.MetricSinks, veneur.SinkConfig{
		Kind: "s3",
		Name: "s3",
		Config: S3SinkConfig{
			AccessKeyID:     config.AwsAccessKeyID,
			S3Bucket:        config.AwsS3Bucket,
			Region:          config.AwsRegion,
			SecretAccessKey: config.AwsSecretAccessKey,
		},
	})
}

// ParseConfig decodes the map config for an S3 sink into an S3SinkConfig
// struct.
func ParseConfig(
	name string, config interface{},
) (veneur.MetricSinkConfig, error) {
	s3Config := S3SinkConfig{}
	err := util.DecodeConfig(name, config, &s3Config)
	if err != nil {
		return nil, err
	}
	return s3Config, nil
}

// Create creates a new S3 sink for metrics. This function should match the
// signature of a value in veneur.MetricSinkTypes, and is intended to be passed
// into veneur.NewFromConfig to be called based on the provided configuration.
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	s3Config, ok := sinkConfig.(S3SinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	awsID := s3Config.AccessKeyID
	awsSecret := s3Config.SecretAccessKey
	var sess *session.Session
	var err error
	if len(awsID.Value) > 0 && len(awsSecret.Value) > 0 {
		sess, err = session.NewSession(&aws.Config{
			Region: aws.String(s3Config.Region),
			Credentials: credentials.NewStaticCredentials(
				awsID.Value, awsSecret.Value, ""),
		})
	} else {
		sess, err = session.NewSession(&aws.Config{
			Region: aws.String(s3Config.Region),
		})
	}

	if err != nil {
		logger.Infof("error getting AWS session: %s", err)
		logger.Info("S3 archives are disabled")
		return nil, err
	}

	logger.Info("Successfully created AWS session")
	logger.Info("S3 archives are enabled")
	return &S3Sink{
		Hostname: config.Hostname,
		Logger:   logger,
		name:     name,
		S3Bucket: s3Config.S3Bucket,
		Svc:      s3.New(sess),
	}, nil
}

func (p *S3Sink) Start(traceClient *trace.Client) error {
	return nil
}

func (p *S3Sink) Flush(
	ctx context.Context, metrics []samplers.InterMetric,
) error {
	const Delimiter = '\t'
	const IncludeHeaders = false

	csv, err := util.EncodeInterMetricsCSV(
		metrics, Delimiter, IncludeHeaders, p.Hostname, p.Interval)
	if err != nil {
		p.Logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Could not marshal metrics before posting to s3")
		return err
	}

	err = p.S3Post(p.Hostname, csv, tsvGzFt)
	if err != nil {
		p.Logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Error posting to s3")
		return err
	}

	p.Logger.WithField("metrics", len(metrics)).Info("flushed")
	return nil
}

func (p *S3Sink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	// Do nothing
}

func (p *S3Sink) Name() string {
	return p.name
}

type filetype string

const (
	jsonFt  filetype = "json"
	csvFt            = "csv"
	tsvFt            = "tsv"
	tsvGzFt          = "tsv.gz"
)

var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

func (p *S3Sink) S3Post(hostname string, data io.ReadSeeker, ft filetype) error {
	if p.Svc == nil {
		return S3ClientUninitializedError
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(p.S3Bucket),
		Key:    S3Path(hostname, ft),
		Body:   data,
	}

	_, err := p.Svc.PutObject(params)
	return err
}

func S3Path(hostname string, ft filetype) *string {
	t := time.Now()
	filename := strconv.FormatInt(t.Unix(), 10) + "." + string(ft)
	return aws.String(path.Join(t.Format("2006/01/02"), hostname, filename))
}
