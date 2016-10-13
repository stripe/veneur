package veneur

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"errors"
	"io"
	"path"
	"strconv"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

var _ plugin = &S3Plugin{}

type S3Plugin struct {
	logger   *logrus.Logger
	statsd   *statsd.Client
	svc      s3iface.S3API
	s3Bucket string
	hostname string
}

func (p *S3Plugin) Flush(metrics []DDMetric, hostname string) error {
	const Delimiter = '\t'
	const IncludeHeaders = false

	start := time.Now()
	csv, err := encodeDDMetricsCSV(metrics, Delimiter, IncludeHeaders, p.hostname)
	if err != nil {
		p.logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Could not marshal metrics before posting to s3")
		return err
	}

	err = p.s3Post(hostname, csv, tsvGzFt)
	if err != nil {
		p.logger.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"metrics":       len(metrics),
		}).Error("Error posting to s3")
		return err
	}

	p.statsd.TimeInMilliseconds("flush.plugins.s3.total_duration_ns", float64(time.Now().Sub(start).Nanoseconds()), []string{"part:post"}, 1.0)
	log.WithField("metrics", len(metrics)).Debug("Completed flush to s3")
	p.statsd.Gauge("flush.plugins.s3.post_metrics_total", float64(len(metrics)), nil, 1.0)
	return nil
}

func (p *S3Plugin) Name() string {
	return "s3"
}

func (p *S3Plugin) Initialize(statsd *statsd.Client, logger *logrus.Logger) {
	p.statsd = statsd
	p.logger = logger
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

func (p *S3Plugin) s3Post(hostname string, data io.ReadSeeker, ft filetype) error {
	if p.svc == nil {
		return S3ClientUninitializedError
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(S3Bucket),
		Key:    s3Path(hostname, ft),
		Body:   data,
	}

	_, err := p.svc.PutObject(params)
	return err
}

func s3Path(hostname string, ft filetype) *string {
	t := time.Now()
	filename := strconv.FormatInt(t.Unix(), 10) + "." + string(ft)
	return aws.String(path.Join(t.Format("2006/01/02"), hostname, filename))
}

// encodeDDMetricsCSV returns a reader containing the gzipped CSV representation of the
// DDMetrics data, one row per DDMetric.
// the AWS sdk requires seekable input, so we return a ReadSeeker here
func encodeDDMetricsCSV(metrics []DDMetric, delimiter rune, includeHeaders bool, hostname string) (io.ReadSeeker, error) {
	b := &bytes.Buffer{}
	gzw := gzip.NewWriter(b)
	w := csv.NewWriter(gzw)
	w.Comma = delimiter

	if includeHeaders {
		// Write the headers first
		headers := [...]string{
			// the order here doesn't actually matter
			// as long as the keys are right
			tsvName:           tsvName.String(),
			tsvTags:           tsvTags.String(),
			tsvMetricType:     tsvMetricType.String(),
			tsvHostname:       tsvHostname.String(),
			tsvDeviceName:     tsvDeviceName.String(),
			tsvInterval:       tsvInterval.String(),
			tsvVeneurHostname: tsvVeneurHostname.String(),
			tsvValue:          tsvValue.String(),
			tsvTimestamp:      tsvTimestamp.String(),
			tsvPartition:      tsvPartition.String(),
		}

		w.Write(headers[:])
	}

	// TODO avoid edge case at midnight
	partitionDate := time.Now()
	for _, metric := range metrics {
		metric.encodeCSV(w, &partitionDate, hostname)
	}

	w.Flush()
	gzw.Close()
	return bytes.NewReader(b.Bytes()), w.Error()
}
