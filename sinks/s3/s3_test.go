package s3

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	s3Mock "github.com/stripe/veneur/v14/sinks/s3/mock"
)

const DefaultServerTimeout = 100 * time.Millisecond

var log = logrus.NewEntry(logrus.New())

const S3TestBucket = "stripe-test-veneur"

// TestS3Post tests that we can correctly post a sequence of
// DDMetrics to S3
func TestS3Post(t *testing.T) {
	const Comma = '\t'
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

	client := &s3Mock.MockS3Client{}
	f, err := os.Open(path.Join("testdata", "aws", "PutObject", "2016", "10", "13", "1476370612.tsv.gz"))
	assert.NoError(t, err)
	defer f.Close()

	client.SetPutObject(func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		// The data should be a gzipped TSV
		gzr, err := gzip.NewReader(input.Body)
		assert.NoError(t, err)
		csvr := csv.NewReader(gzr)
		csvr.Comma = Comma
		records, err := csvr.ReadAll()
		assert.NoError(t, err)

		assert.Equal(t, 6, len(records))
		assert.Equal(t, "a.b.c.max", records[0][0])
		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	})

	s3p := &S3Sink{Logger: log, Svc: client}

	err = s3p.S3Post("testbox", f, tsvFt)
	assert.NoError(t, err)
}

func TestS3Path(t *testing.T) {
	const hostname = "testingbox-9f23c"

	start := time.Now()

	path := S3Path(hostname, jsonFt)

	end := time.Now()

	// We expect paths to follow this format
	// <year>/<month/<day>/<hostname>/<timestamp>.json
	// so we should be able to parse the output with this expectation
	results := strings.Split(*path, "/")
	assert.Equal(t, 5, len(results), "Expected %#v to contain 5 parts", results)

	year, err := strconv.Atoi(results[0])
	assert.NoError(t, err)
	month, err := strconv.Atoi(results[1])
	assert.NoError(t, err)
	day, err := strconv.Atoi(results[2])
	assert.NoError(t, err)

	hostname2 := results[3]
	filename := results[4]
	timestamp, err := strconv.ParseInt(strings.Split(filename, ".")[0], 10, 64)
	assert.NoError(t, err)

	sameYear := year == int(time.Now().Year()) ||
		year == int(start.Year())
	sameMonth := month == int(time.Now().Month()) ||
		month == int(start.Month())
	sameDay := day == int(time.Now().Day()) ||
		day == int(start.Day())

	// we may have started the tests a split-second before midnight
	assert.True(t, sameYear, "Expected year %s and received %s", start.Year(), year)
	assert.True(t, sameMonth, "Expected month %s and received %s", start.Month(), month)
	assert.True(t, sameDay, "Expected day %d and received %s", start.Day(), day)

	assert.Equal(t, hostname, hostname2)
	assert.True(t, start.Unix() <= timestamp && timestamp <= end.Unix())
}

func TestS3PostNoCredentials(t *testing.T) {
	s3p := &S3Sink{Logger: log, Svc: nil}

	f, err := os.Open(path.Join("testdata", "aws", "PutObject", "2016", "10", "07", "1475863542.json"))
	assert.NoError(t, err)
	defer f.Close()

	// this should not panic
	err = s3p.S3Post("testbox", f, jsonFt)
	assert.Equal(t, S3ClientUninitializedError, err)
}

// TestGlobalServerS3PluginFlush tests that we are able to
// register the S3 plugin on the server, and that when we do,
// flushing on the server causes the S3 plugin to flush to S3.
// This is the function that actually tests the S3Plugin.Flush()
// method
func TestFlush(t *testing.T) {
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

	client := &s3Mock.MockS3Client{}
	client.SetPutObject(func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		f, err := os.Open(path.Join("testdata", "aws", "PutObject", "2016", "10", "14", "1476481302.tsv.gz"))
		assert.NoError(t, err)
		defer f.Close()

		records, err := parseGzipTSV(input.Body)
		assert.NoError(t, err)

		expectedRecords, err := parseGzipTSV(f)
		assert.NoError(t, err)

		assert.Equal(t, len(expectedRecords), len(records))

		assertCSVFieldsMatch(t, expectedRecords, records, []int{0, 1, 2, 3, 4, 6})

		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	})

	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Config: veneur.Config{
			Aggregates: []string{"min", "max", "count"},
			Interval:   time.Duration(time.Second),
			MetricSinks: []veneur.SinkConfig{{
				Kind:   "s3",
				Name:   "s3",
				Config: S3SinkConfig{},
			}},
			NumWorkers:   1,
			Percentiles:  []float64{.5, .75, .99},
			StatsAddress: "localhost:8125",
		},
		Logger: logrus.New(),
		MetricSinkTypes: veneur.MetricSinkTypes{
			"s3": {
				Create: func(server *veneur.Server, name string, logger *logrus.Entry,
					config veneur.Config, sinkConfig veneur.MetricSinkConfig,
				) (sinks.MetricSink, error) {
					return &S3Sink{
						Hostname: server.Hostname,
						Interval: 10,
						Logger:   logger,
						name:     name,
						Svc:      client,
					}, nil
				},
				ParseConfig: ParseConfig,
			},
		},
		SpanSinkTypes: veneur.SpanSinkTypes{},
	})
	require.Nil(t, err)
	server.Start()

	metricValues := []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	for _, value := range metricValues {
		server.WorkerSets[0].Workers[0].ProcessMetric(&samplers.UDPMetric{
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

	server.Flush(context.Background(), server.WorkerSets[0])
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

	require.Len(t, actual, len(expected))
	for i, row := range expected {
		require.Len(t, actual[i], len(row))
		for _, column := range columns {
			assert.Equal(t, row[column], actual[i][column], "mismatch at column %d", column)
		}
	}
}
