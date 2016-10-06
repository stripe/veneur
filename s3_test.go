package veneur

import (
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/stretchr/testify/assert"
)

const S3TestBucket = "stripe-test-veneur"

type mockS3Client struct {
	s3iface.S3API
	putObject func(*s3.PutObjectInput) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return m.putObject(input)
}

// TestS3Post tests that we can correctly post a sequence of
// DDMetrics to S3
func TestS3Post(t *testing.T) {
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

	client := &mockS3Client{}
	f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "foo", "bar.req"))
	assert.NoError(t, err)
	defer f.Close()

	client.putObject = func(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
		var data []DDMetric
		assert.NoError(t, err)
		json.NewDecoder(input.Body).Decode(&data)
		assert.Equal(t, 6, len(data))
		assert.Equal(t, "a.b.c.max", data[0].Name)
		RemoteResponseChan <- struct{}{}
		return &s3.PutObjectOutput{ETag: aws.String("912ec803b2ce49e4a541068d495ab570")}, nil
	}

	svc = client

	err = s3Post("testbox", f)
	assert.NoError(t, err)
}

func TestS3Path(t *testing.T) {
	const hostname = "testingbox"

	start := time.Now()

	path := s3Path(hostname)

	tm, err := time.Parse("2006/01/02/testingbox", *path)
	assert.NoError(t, err)
	assert.Equal(t, tm.Year(), time.Now().Year())
	assert.Equal(t, tm.Month(), time.Now().Month())

	// we may have started the tests a split-second before midnight
	sameDay := tm.Day() == time.Now().Day() ||
		tm.Day() == start.Day()
	assert.True(t, sameDay)
}

func TestS3PostNoCredentials(t *testing.T) {
	svc = nil

	f, err := os.Open(path.Join("fixtures", "aws", "PutObject", "foo", "bar.req"))
	assert.NoError(t, err)
	defer f.Close()

	// this should not panic
	err = s3Post("testbox", f)
	assert.Equal(t, S3ClientUninitializedError, err)
}
