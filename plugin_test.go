package veneur

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"io"
	"os"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	s3p "github.com/stripe/veneur/plugins/s3"
	s3Mock "github.com/stripe/veneur/plugins/s3/mock"
	"github.com/stripe/veneur/samplers"
)

type dummyPlugin struct {
	logger *logrus.Logger
	statsd *statsd.Client
	flush  func(context.Context, []samplers.InterMetric) error
}

func (dp *dummyPlugin) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	return dp.flush(ctx, metrics)
}

func (dp *dummyPlugin) Name() string {
	return "dummy_plugin"
}

// TestGlobalServerPluginFlush tests that we are able to
// register a dummy plugin on the server, and that when we do,
// flushing on the server causes the plugin to flush
func TestGlobalServerPluginFlush(t *testing.T) {

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues, expectedMetrics := generateMetrics()
	config := globalConfig()
	f := newFixture(t, config, nil, nil)
	defer f.Close()

	dp := &dummyPlugin{logger: log, statsd: f.server.Statsd}

	dp.flush = func(ctx context.Context, metrics []samplers.InterMetric) error {
		assert.Equal(t, len(expectedMetrics), len(metrics))

		firstName := metrics[0].Name
		assert.Equal(t, expectedMetrics[firstName], metrics[0].Value)

		RemoteResponseChan <- struct{}{}
		return nil
	}

	f.server.registerPlugin(dp)

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush(context.Background())
}

// TestLocalFilePluginRegister tests that we are able to register
// a local file as a flush output for Veneur.
func TestLocalFilePluginRegister(t *testing.T) {
	config := globalConfig()
	config.FlushFile = "/dev/null"

	server, err := NewFromConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(server.getPlugins()))
}

// TestGlobalServerS3PluginFlush tests that we are able to
// register the S3 plugin on the server, and that when we do,
// flushing on the server causes the S3 plugin to flush to S3.
// This is the function that actually tests the S3Plugin.Flush()
// method
func TestGlobalServerS3PluginFlush(t *testing.T) {

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues, _ := generateMetrics()

	config := globalConfig()
	f := newFixture(t, config, nil, nil)
	defer f.Close()

	client := &s3Mock.MockS3Client{}
	client.SetPutObject(func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "2016", "10", "14", "1476481302.tsv.gz"))
		assert.NoError(t, err)
		defer f.Close()

		records, err := parseGzipTSV(input.Body)
		assert.NoError(t, err)

		expectedRecords, err := parseGzipTSV(f)
		assert.NoError(t, err)

		assert.Equal(t, len(expectedRecords), len(records))

		assertCSVFieldsMatch(t, expectedRecords, records, []int{0, 1, 2, 3, 4, 6})
		//assert.Equal(t, expectedRecords, records)

		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	})

	s3p := &s3p.S3Plugin{Logger: log, Svc: client, Interval: 10}

	f.server.registerPlugin(s3p)

	plugins := f.server.getPlugins()
	assert.Equal(t, 1, len(plugins))

	for _, value := range metricValues {
		f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.LocalOnly,
		})
	}

	f.server.Flush(context.Background())
}

func parseGzipTSV(r io.Reader) ([][]string, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	cr := csv.NewReader(gzr)
	cr.Comma = '\t'
	return cr.ReadAll()
}

// assertCSVFieldsMatch asserts that all fields of all rows match, but it allows us
// to skip some columns entirely, if we know that they won't match (e.g. timestamps)
func assertCSVFieldsMatch(t *testing.T, expected, actual [][]string, columns []int) {
	if columns == nil {
		columns = make([]int, len(expected[0]))
		for i := 0; i < len(columns); i++ {
			columns[i] = i
		}
	}

	for i, row := range expected {
		assert.Equal(t, len(row), len(actual[i]))
		for _, column := range columns {
			assert.Equal(t, row[column], actual[i][column], "mismatch at column %d", column)
		}
	}
}
