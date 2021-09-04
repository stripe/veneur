package s3Mock

import (
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

type MockS3Client struct {
	s3iface.S3API
	putObject func(*s3.PutObjectInput) (*s3.PutObjectOutput, error)
}

// SetPutObject sets the function that acts as the PutObject handler
func (m *MockS3Client) SetPutObject(f func(*s3.PutObjectInput) (*s3.PutObjectOutput, error)) {
	m.putObject = f
}

func (m *MockS3Client) PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return m.putObject(input)
}
