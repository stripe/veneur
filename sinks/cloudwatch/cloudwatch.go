package cloudwatch

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/sirupsen/logrus"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

const (
	DefaultCloudwatchStandardUnitTagName = "cloudwatch_standard_unit"
	DefaultRemoteTimeout                 = time.Duration(30 * time.Second)
)

type CloudwatchMetricSinkConfig struct {
	AWSRegion                     string             `yaml:"aws_region"`
	AWSDisableRetries             bool               `yaml:"aws_disable_retries"`
	CloudwatchEndpoint            string             `yaml:"cloudwatch_endpoint"`
	CloudwatchNamespace           string             `yaml:"cloudwatch_namespace"`
	CloudwatchStandardUnitTagName types.StandardUnit `yaml:"cloudwatch_standard_unit_tag_name"`
	RemoteTimeout                 time.Duration      `yaml:"remote_timeout"`
	StripTags                     []string           `yaml:"strip_tags"`
}

type cloudwatchMetricSink struct {
	logger              *logrus.Entry
	remoteTimeout       time.Duration
	client              *cloudwatch.Client
	endpoint            string
	region              string
	namespace           string
	standardUnitTagName types.StandardUnit
	disableRetries      bool
	stripTags           []string
}

var _ sinks.MetricSink = (*cloudwatchMetricSink)(nil)

func NewCloudwatchMetricSink(
	endpoint, namespace, region string, standardUnitTagName types.StandardUnit,
	remoteTimeout time.Duration, disableRetries bool, stripTags []string, logger *logrus.Entry,
) *cloudwatchMetricSink {
	return &cloudwatchMetricSink{
		endpoint:            endpoint,
		namespace:           namespace,
		region:              region,
		standardUnitTagName: standardUnitTagName,
		logger:              logger,
		remoteTimeout:       remoteTimeout,
		disableRetries:      disableRetries,
		stripTags:           stripTags,
	}
}

func ParseConfig(name string, config interface{}) (veneur.MetricSinkConfig, error) {
	cloudwatchConfig := CloudwatchMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &cloudwatchConfig)
	if err != nil {
		return nil, err
	}
	if cloudwatchConfig.CloudwatchStandardUnitTagName == "" {
		cloudwatchConfig.CloudwatchStandardUnitTagName = DefaultCloudwatchStandardUnitTagName
	}
	if cloudwatchConfig.RemoteTimeout == 0 {
		cloudwatchConfig.RemoteTimeout = DefaultRemoteTimeout
	}
	return cloudwatchConfig, nil
}

func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	conf, ok := sinkConfig.(CloudwatchMetricSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	return NewCloudwatchMetricSink(
		conf.CloudwatchEndpoint,
		conf.CloudwatchNamespace,
		conf.AWSRegion,
		conf.CloudwatchStandardUnitTagName,
		conf.RemoteTimeout,
		conf.AWSDisableRetries,
		conf.StripTags,
		logger,
	), nil
}

func (s *cloudwatchMetricSink) Name() string {
	return "cloudwatch"
}

func (s *cloudwatchMetricSink) Start(*trace.Client) error {
	opts := cloudwatch.Options{}
	if s.endpoint != "" {
		opts.EndpointResolver = cloudwatch.EndpointResolverFromURL(s.endpoint)
	}
	if s.region != "" {
		opts.Region = s.region
	}
	if s.remoteTimeout != 0 {
		opts.HTTPClient = &http.Client{
			Timeout: s.remoteTimeout,
		}
	}
	if s.disableRetries {
		opts.Retryer = aws.NopRetryer{}
	}
	s.client = cloudwatch.New(opts)
	return nil
}

func (s *cloudwatchMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	if len(metrics) == 0 {
		return nil
	}

	metricData := make([]types.MetricDatum, len(metrics))
	for i, metric := range metrics {
		dimensions := make([]types.Dimension, 0, len(metric.Tags))
		standardUnit := types.StandardUnitNone
		for _, tag := range metric.Tags {
			kv := strings.SplitN(tag, ":", 2)

			if len(kv) < 2 {
				continue // drop illegal tag
			}
			skip := false
			if kv[0] == string(s.standardUnitTagName) {
				standardUnit = types.StandardUnit(kv[1])
				skip = true
			}
			for _, tag := range s.stripTags {
				if kv[0] == tag {
					skip = true
				}
			}
			if skip {
				continue
			}

			dimensions = append(dimensions, types.Dimension{
				Name:  aws.String(kv[0]),
				Value: aws.String(kv[1]),
			})
		}
		metricData[i] = types.MetricDatum{
			MetricName: aws.String(metric.Name),
			Unit:       standardUnit,
			Value:      aws.Float64(metric.Value),
			Dimensions: dimensions,
		}
	}
	input := &cloudwatch.PutMetricDataInput{
		Namespace:  aws.String(s.namespace),
		MetricData: metricData,
	}
	_, err := s.client.PutMetricData(ctx, input)
	if err != nil {
		return err
	}
	return nil
}

func (s *cloudwatchMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	// unimplemented
}
