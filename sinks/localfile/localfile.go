package localfile

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

type LocalFileSinkConfig struct {
	Delimiter util.Rune `yaml:"delimiter"`
	FlushFile string    `yaml:"flush_file"`
}

// LocalFileSink is the LocalFile plugin that we'll use in Veneur
type LocalFileSink struct {
	Delimiter  rune
	FilePath   string
	FileSystem FileSystem
	hostname   string
	Logger     *logrus.Entry
	name       string
}

type FileSystem interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}

type File interface {
	Close() error
	Write(p []byte) (n int, err error)
}

type osFS struct{}

func (osFS) OpenFile(
	name string, flag int, perm os.FileMode,
) (File, error) {
	return os.OpenFile(name, flag, perm)
}

// TODO(arnavdugar): Remove this once the old configuration format has been
// removed.
func MigrateConfig(config *veneur.Config) {
	if config.FlushFile == "" {
		return
	}
	config.MetricSinks = append(config.MetricSinks, veneur.SinkConfig{
		Kind: "localfile",
		Name: "localfile",
		Config: LocalFileSinkConfig{
			Delimiter: '\t',
			FlushFile: config.FlushFile,
		},
	})
}

// ParseConfig decodes the map config for an local file sink into a
// LocalFileSinkConfig struct.
func ParseConfig(
	name string, config interface{},
) (veneur.MetricSinkConfig, error) {
	localFileConfig := LocalFileSinkConfig{}
	err := util.DecodeConfig(name, config, &localFileConfig)
	if err != nil {
		return nil, err
	}
	return localFileConfig, nil
}

// Create creates a new local file sink for metrics. This function should match
// the signature of a value in veneur.MetricSinkTypes, and is intended to be
// passed into veneur.NewFromConfig to be called based on the provided
// configuration.
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	localFileConfig, ok := sinkConfig.(LocalFileSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	return NewLocalFileSink(
		localFileConfig, osFS{}, server.Hostname, logger, name,
	), nil
}

func NewLocalFileSink(
	config LocalFileSinkConfig, filesystem FileSystem, hostname string,
	logger *logrus.Entry, name string,
) *LocalFileSink {
	return &LocalFileSink{
		Delimiter:  rune(config.Delimiter),
		FilePath:   config.FlushFile,
		FileSystem: filesystem,
		hostname:   hostname,
		Logger:     logger,
		name:       name,
	}
}

func (sink *LocalFileSink) Start(traceClient *trace.Client) error {
	return nil
}

// Flush the metrics from the LocalFilePlugin
func (sink *LocalFileSink) Flush(
	ctx context.Context, metrics []samplers.InterMetric,
) error {
	file, err := sink.FileSystem.OpenFile(
		sink.FilePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		return fmt.Errorf("couldn't open %s for appending: %s", sink.FilePath, err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	csvWriter := csv.NewWriter(gzipWriter)
	csvWriter.Comma = sink.Delimiter

	partitionDate := time.Now()
	for _, metric := range metrics {
		util.EncodeInterMetricCSV(
			metric, csvWriter, &partitionDate, sink.hostname, 0)
	}
	csvWriter.Flush()
	gzipWriter.Close()
	werr := csvWriter.Error()
	if werr != nil {
		return werr
	}
	return nil
}

// Name is the name of the LocalFilePlugin, i.e., "localfile"
func (sink *LocalFileSink) Name() string {
	return sink.name
}

func (sink *LocalFileSink) FlushOtherSamples(
	ctx context.Context, samples []ssf.SSFSample,
) {
	// Do nothing
}
