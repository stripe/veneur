package attribution

import (
	"compress/gzip"
	"encoding/csv"
	"os"
	"path"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	s3Mock "github.com/stripe/veneur/v14/sinks/attribution/mock"
)

const DefaultServerTimeout = 100 * time.Millisecond

var log = logrus.NewEntry(logrus.New())

// TestS3Post tests that we can post attribution data to S3 without error
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
	f, err := os.Open(path.Join("testdata", "aws", "PutObject", "v2", "2021", "10", "05", "21", "foobox-1234.veneur.org-1633470722.tsv.gz"))
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

		assert.Equal(t, 5, len(records))
		assert.Equal(t, "veneur.trace_client.records_succeeded_total", records[3][0])
		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	})

	sink := &AttributionSink{
		log:   log,
		s3Svc: client,
	}

	err = sink.s3Post(f)
	assert.NoError(t, err)
}

// TestRecordMetric checks that metrics flushed to an AttributionSink are being
// accounted for
func TestRecordMetric(t *testing.T) {
	sink := &AttributionSink{
		log: log,
	}
	attributionData := make(map[string]*TimeseriesGroup)

	dp1 := samplers.InterMetric{
		Name:      "atrtest.1",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val1"},
		Type:      samplers.CounterMetric,
	}
	dp2 := samplers.InterMetric{
		Name:      "atrtest.1",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val2"},
		Type:      samplers.CounterMetric,
	}
	dp3 := samplers.InterMetric{
		Name:      "atrtest.2",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val1"},
		Type:      samplers.CounterMetric,
	}
	sink.recordMetric(attributionData, dp1)
	sink.recordMetric(attributionData, dp2)
	sink.recordMetric(attributionData, dp3)

	tsGroup1, ok := attributionData["atrtest.1"]
	assert.True(t, ok)
	assert.Equal(t, uint64(2), tsGroup1.Sketch.Estimate())
	assert.Equal(t, 2, len(tsGroup1.Digests))
	tsGroup2, ok := attributionData["atrtest.2"]
	assert.True(t, ok)
	assert.Equal(t, uint64(1), tsGroup2.Sketch.Estimate())
	assert.Equal(t, 1, len(tsGroup2.Digests))
}

func TestRecordMetricCommonDimensions(t *testing.T) {
	sink := &AttributionSink{
		log:         log,
		hostnameTag: "host",
		hostname:    "i-1234.west.prod.veneur.org",
		commonDimensions: map[string]string{
			"host_env": "prod",
		},
	}
	attributionData := make(map[string]*TimeseriesGroup)

	dp1 := samplers.InterMetric{
		Name:      "atrtest.1",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val1"},
		Type:      samplers.CounterMetric,
	}
	dp2 := samplers.InterMetric{
		Name:      "atrtest.1",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val1", "host:this-should-be-overridden"},
		Type:      samplers.CounterMetric,
	}
	dp3 := samplers.InterMetric{
		Name:      "atrtest.1",
		Timestamp: 1532009163,
		Value:     1,
		Tags:      []string{"dim1:val1", "host_env:this-should-also-be-overridden"},
		Type:      samplers.CounterMetric,
	}
	sink.recordMetric(attributionData, dp1)
	sink.recordMetric(attributionData, dp2)
	sink.recordMetric(attributionData, dp3)

	tsGroup1, ok := attributionData["atrtest.1"]
	assert.True(t, ok)
	assert.Equal(t, uint64(1), tsGroup1.Sketch.Estimate())
	assert.Equal(t, 1, len(tsGroup1.Digests))
}
