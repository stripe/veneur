package localfile_test

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks/localfile"
	"github.com/stripe/veneur/v14/util"
)

var metrics = []samplers.InterMetric{{
	Name:      "a.b.c.max",
	Timestamp: 1476119058,
	Value:     float64(100),
	Tags: []string{
		"foo:bar",
		"baz:quz",
	},
	Type: samplers.GaugeMetric,
}}

type fakeFileSystem struct {
	files map[string]fakeFile
	t     *testing.T
}

type fakeFile struct {
	Builder *strings.Builder
}

func (fs fakeFileSystem) OpenFile(
	name string, flag int, perm os.FileMode,
) (localfile.File, error) {
	assert.Equal(fs.t, os.O_RDWR|os.O_APPEND|os.O_CREATE, flag)
	file := fakeFile{
		&strings.Builder{},
	}
	fs.files[name] = file
	return file, nil
}

func (file fakeFile) Close() error {
	return nil
}

func (file fakeFile) Write(value []byte) (int, error) {
	return file.Builder.Write(value)
}

func TestName(t *testing.T) {
	sink := localfile.NewLocalFileSink(
		localfile.LocalFileSinkConfig{}, fakeFileSystem{}, "hostname",
		logrus.NewEntry(logrus.New()), "sink-name")
	assert.Equal(t, sink.Name(), "sink-name")
}

func TestFlush(t *testing.T) {
	filesystem := fakeFileSystem{
		files: map[string]fakeFile{},
		t:     t,
	}
	sink := localfile.NewLocalFileSink(
		localfile.LocalFileSinkConfig{
			Delimiter: ',',
			FlushFile: "flush-file.tsv",
		}, filesystem, "hostname", logrus.NewEntry(logrus.New()), "sink-name")

	_, err := sink.Flush(context.Background(), metrics)
	require.NoError(t, err)

	resultFile, ok := filesystem.files["flush-file.tsv"]
	require.True(t, ok)
	resultGzip := resultFile.Builder.String()
	gzipReader, err := gzip.NewReader(strings.NewReader(resultGzip))
	require.Nil(t, err)
	result, err := csv.NewReader(gzipReader).ReadAll()
	require.Nil(t, err)
	assert.Equal(t, [][]string{{
		"a.b.c.max", "{foo:bar,baz:quz}", "gauge", "hostname", "0",
		"2016-10-10 05:04:18", "100",
		time.Now().UTC().Format(util.PartitionDateFormat),
	}}, result)
}

type fakeFileSystemWriteError struct{}

type fakeFileWriteError struct{}

func (fs fakeFileSystemWriteError) OpenFile(
	name string, flag int, perm os.FileMode,
) (localfile.File, error) {
	return fakeFileWriteError{}, nil
}

func (file fakeFileWriteError) Close() error {
	return nil
}

func (file fakeFileWriteError) Write(value []byte) (int, error) {
	return 0, fmt.Errorf("this writer fails when you try to write")
}

func TestFlushWriteError(t *testing.T) {
	sink := localfile.NewLocalFileSink(
		localfile.LocalFileSinkConfig{
			Delimiter: ',',
			FlushFile: "flush-file.tsv",
		}, fakeFileSystemWriteError{}, "hostname", logrus.NewEntry(logrus.New()),
		"sink-name")

	_, err := sink.Flush(context.Background(), metrics)
	require.Error(t, err)
}

func TestFlushToDevNull(t *testing.T) {
	sink, err := localfile.Create(&veneur.Server{
		Hostname: "hostname",
	}, "sink-name",
		logrus.NewEntry(logrus.New()), veneur.Config{},
		localfile.LocalFileSinkConfig{
			FlushFile: "/dev/null",
		})
	require.Nil(t, err)
	_, err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)
}

func TestFlushToInvalidPath(t *testing.T) {
	sink, err := localfile.Create(&veneur.Server{
		Hostname: "hostname",
	}, "sink-name",
		logrus.NewEntry(logrus.New()), veneur.Config{},
		localfile.LocalFileSinkConfig{
			FlushFile: "",
		})
	require.Nil(t, err)
	_, err = sink.Flush(context.Background(), metrics)
	assert.Error(t, err)
}
