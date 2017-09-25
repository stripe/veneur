package s3

import (
	"bytes"
	"compress/gzip"
	"context"
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
	Interval int
}

func (p *S3Plugin) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	const Delimiter = '\t'
	const IncludeHeaders = false

	csv, err := EncodeInterMetricsCSV(metrics, Delimiter, IncludeHeaders, p.Hostname, p.Interval)
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

// EncodeInterMetricsCSV returns a reader containing the gzipped CSV representation of the
// InterMetric data, one row per InterMetric.
// the AWS sdk requires seekable input, so we return a ReadSeeker here
func EncodeInterMetricsCSV(metrics []samplers.InterMetric, delimiter rune, includeHeaders bool, hostname string, interval int) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = delimiter

	if includeHeaders {
		// Write the headers first
		headers := [...]string{
			// the order here doesn't actually matter
			// as long as the keys are right
			TsvName:           TsvName.String(),
			TsvTags:           TsvTags.String(),
			TsvMetricType:     TsvMetricType.String(),
			TsvInterval:       TsvInterval.String(),
			TsvVeneurHostname: TsvVeneurHostname.String(),
			TsvValue:          TsvValue.String(),
			TsvTimestamp:      TsvTimestamp.String(),
			TsvPartition:      TsvPartition.String(),
		}

		w.Write(headers[:])
	}

	// TODO avoid edge case at midnight
	partitionDate := time.Now()
	for _, metric := range metrics {
		EncodeInterMetricCSV(metric, w, &partitionDate, hostname, interval)
	}

	w.Flush()
	gzw.Close()
	return bytes.NewReader(b.Bytes()), w.Error()
}
