package localfile

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur/plugins"
	"github.com/stripe/veneur/plugins/s3"
	"github.com/stripe/veneur/samplers"
)

var _ plugins.Plugin = &Plugin{}

// Plugin is the LocalFile plugin that we'll use in Veneur
type Plugin struct {
	FilePath string
	Logger   *logrus.Logger
	hostname string
	interval int
}

// Delimiter defines what kind of delimiter we'll use in the CSV format -- in this case, we want TSV
const Delimiter = '\t'

// Flush the metrics from the LocalFilePlugin
func (p *Plugin) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	f, err := os.OpenFile(p.FilePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, os.ModePerm)
	defer f.Close()

	if err != nil {
		return fmt.Errorf("couldn't open %s for appending: %s", p.FilePath, err)
	}
	appendToWriter(f, metrics, p.hostname, p.interval)
	return nil
}

func appendToWriter(appender io.Writer, metrics []samplers.InterMetric, hostname string, interval int) error {
	gzW := gzip.NewWriter(appender)
	csvW := csv.NewWriter(gzW)
	csvW.Comma = Delimiter

	partitionDate := time.Now()
	for _, metric := range metrics {
		s3.EncodeInterMetricCSV(metric, csvW, &partitionDate, hostname, interval)
	}
	csvW.Flush()
	gzW.Close()
	return csvW.Error()
}

// Name is the name of the LocalFilePlugin, i.e., "localfile"
func (p *Plugin) Name() string {
	return "localfile"
}
