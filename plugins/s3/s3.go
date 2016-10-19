package s3

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"

	"github.com/stripe/veneur/plugins"
	"github.com/stripe/veneur/samplers"
)

// TODO set log level

var _ plugins.Plugin = &S3Plugin{}

type S3Plugin struct {
	Logger   *logrus.Logger
	Svc      s3iface.S3API
	S3Bucket string
	Hostname string
}

func (p *S3Plugin) Flush(metrics []samplers.DDMetric, hostname string) error {
	const Delimiter = '\t'
	const IncludeHeaders = false

	csv, err := encodeDDMetricsCSV(metrics, Delimiter, IncludeHeaders, p.Hostname)
	if err != nil {
		p.Logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Could not marshal metrics before posting to s3")
		return err
	}

	err = p.S3Post(hostname, csv, tsvGzFt)
	if err != nil {
		p.Logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Error posting to s3")
		return err
	}

	p.Logger.WithField("metrics", len(metrics)).Debug("Completed flush to s3")
	return nil
}

func (p *S3Plugin) Name() string {
	return "s3"
}

type filetype string

const (
	jsonFt  filetype = "json"
	csvFt            = "csv"
	tsvFt            = "tsv"
	tsvGzFt          = "tsv.gz"
)

var S3Bucket = "stripe-veneur"

var S3ClientUninitializedError = errors.New("s3 client has not been initialized")

func (p *S3Plugin) S3Post(hostname string, data io.ReadSeeker, ft filetype) error {
	if p.Svc == nil {
		return S3ClientUninitializedError
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(S3Bucket),
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

// encodeDDMetricsCSV returns a reader containing the gzipped CSV representation of the
// DDMetrics data, one row per DDMetric.
// the AWS sdk requires seekable input, so we return a ReadSeeker here
func encodeDDMetricsCSV(metrics []samplers.DDMetric, delimiter rune, includeHeaders bool, hostname string) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = delimiter

	if includeHeaders {
		// Write the headers first
		headers := [...]string{
			// the order here doesn't actually matter
			// as long as the keys are right
			samplers.TsvName:           samplers.TsvName.String(),
			samplers.TsvTags:           samplers.TsvTags.String(),
			samplers.TsvMetricType:     samplers.TsvMetricType.String(),
			samplers.TsvHostname:       samplers.TsvHostname.String(),
			samplers.TsvDeviceName:     samplers.TsvDeviceName.String(),
			samplers.TsvInterval:       samplers.TsvInterval.String(),
			samplers.TsvVeneurHostname: samplers.TsvVeneurHostname.String(),
			samplers.TsvValue:          samplers.TsvValue.String(),
			samplers.TsvTimestamp:      samplers.TsvTimestamp.String(),
			samplers.TsvPartition:      samplers.TsvPartition.String(),
		}

		w.Write(headers[:])
	}

	// TODO avoid edge case at midnight
	partitionDate := time.Now()
	for _, metric := range metrics {
		metric.EncodeCSV(w, &partitionDate, hostname)
	}

	w.Flush()
	gzw.Close()
	return bytes.NewReader(b.Bytes()), w.Error()
}
