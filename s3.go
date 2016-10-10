package veneur

import (
	"errors"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

const S3Bucket = "stripe-veneur"

// TODO(aditya) config-ify this
const DefaultAWSRegion = "us-west-2"
const AwsProfile = "veneur-s3-test"

var svc s3iface.S3API

var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

// credentials will be pull from environment variables
// AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY

func init() {
	sess := session.New(&aws.Config{
		Region: aws.String(DefaultAWSRegion),
	})

	_, err := sess.Config.Credentials.Get()
	if err == nil {
		svc = s3.New(sess)
	}
}

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
