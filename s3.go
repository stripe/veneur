package veneur

import (
	"errors"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

const S3Bucket = "stripe-test-veneur"

// TODO(aditya) config-ify this
const DefaultAWSRegion = "us-west-2"
const AwsProfile = "veneur-s3-test"

var svc s3iface.S3API

var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

func s3Post(hostname string, data io.ReadSeeker) error {
	if svc == nil {
		return S3ClientUninitializedError
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(S3Bucket),
		Key:    s3Path(hostname),
		Body:   data,
	}

	_, err := svc.PutObject(params)
	return err
}

func s3Path(hostname string) *string {
	t := time.Now()
	filename := strconv.FormatInt(t.Unix(), 10) + ".json"
	return aws.String(path.Join(t.Format("2006/01/02"), hostname, filename))
}
