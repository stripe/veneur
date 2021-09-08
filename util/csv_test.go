package util_test

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/testhelpers"
	"github.com/stripe/veneur/v14/util"
)

type CSVTestCase struct {
	Name        string
	InterMetric samplers.InterMetric
	Row         io.Reader
}

func CSVTestCases() []CSVTestCase {

	partition := time.Now().UTC().Format("20060102")

	return []CSVTestCase{
		{
			Name: "BasicDDMetric",
			InterMetric: samplers.InterMetric{
				Name:      "a.b.c.max",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{"foo:bar",
					"baz:quz"},
				Type: samplers.GaugeMetric,
			},
			Row: strings.NewReader(fmt.Sprintf(
				"a.b.c.max\t{foo:bar,baz:quz}\tgauge\ttestbox-c3eac9\t10\t2016-10-10 05:04:18\t100\t%s\n", partition)),
		},
		{
			// Test that we are able to handle a missing field (DeviceName)
			Name: "MissingDeviceName",
			InterMetric: samplers.InterMetric{
				Name:      "a.b.c.max",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{"foo:bar",
					"baz:quz"},
				Type: samplers.CounterMetric,
			},
			Row: strings.NewReader(fmt.Sprintf(
				"a.b.c.max\t{foo:bar,baz:quz}\trate\ttestbox-c3eac9\t10\t2016-10-10 05:04:18\t10\t%s\n", partition)),
		},
		{
			// Test that we are able to handle tags which have tab characters in them
			// by quoting the entire field
			// (tags shouldn't do this, but we should handle them properly anyway)
			Name: "TabTag",
			InterMetric: samplers.InterMetric{
				Name:      "a.b.c.count",
				Timestamp: 1476119058,
				Value:     float64(100),
				Tags: []string{"foo:b\tar",
					"baz:quz"},
				Type: samplers.CounterMetric,
			},
			Row: strings.NewReader(fmt.Sprintf(
				"a.b.c.count\t\"{foo:b\tar,baz:quz}\"\trate\ttestbox-c3eac9\t10\t2016-10-10 05:04:18\t10\t%s\n", partition)),
		},
	}
}

func TestEncodeCSV(t *testing.T) {
	testCases := CSVTestCases()

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {

			b := &bytes.Buffer{}

			w := csv.NewWriter(b)
			w.Comma = '\t'

			tm := time.Now()
			err := util.EncodeInterMetricCSV(
				tc.InterMetric, w, &tm, "testbox-c3eac9", 10)
			assert.NoError(t, err)

			// We need to flush or there won't actually be any data there
			w.Flush()
			assert.NoError(t, err)

			assertReadersEqual(t, tc.Row, b)
		})
	}
}

// Helper function for determining that two readers are equal
func assertReadersEqual(t *testing.T, expected io.Reader, actual io.Reader) {

	// If we can seek, ensure that we're starting at the beginning
	for _, reader := range []io.Reader{expected, actual} {
		if readerSeeker, ok := reader.(io.ReadSeeker); ok {
			readerSeeker.Seek(0, io.SeekStart)
		}
	}

	// do the lazy thing for now
	bts, err := ioutil.ReadAll(expected)
	if err != nil {
		t.Fatal(err)
	}

	bts2, err := ioutil.ReadAll(actual)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, string(bts), string(bts2))
}

func TestEncodeDDMetricsCSV(t *testing.T) {
	ExpectedHeader := []string{
		"Name", "Tags", "MetricType", "VeneurHostname", "Interval", "Timestamp",
		"Value", "Partition"}
	const Delimiter = '\t'
	const VeneurHostname = "testbox-c3eac9"

	testCases := CSVTestCases()

	metrics := make([]samplers.InterMetric, len(testCases))
	for i, tc := range testCases {
		metrics[i] = tc.InterMetric
	}

	c, err := util.EncodeInterMetricsCSV(
		metrics, Delimiter, true, VeneurHostname, 10)
	assert.NoError(t, err)
	gzr, err := gzip.NewReader(c)
	assert.NoError(t, err)
	r := csv.NewReader(gzr)
	r.FieldsPerRecord = 8
	r.Comma = Delimiter

	// first line should always contain header information
	header, err := r.Read()
	assert.NoError(t, err)
	assert.Equal(t, ExpectedHeader, header)

	records, err := r.ReadAll()
	assert.NoError(t, err)

	assert.Lenf(
		t, records, len(metrics),
		"Expected %d records and got %d", len(metrics), len(records))
	for i, tc := range testCases {
		record := records[i]
		t.Run(tc.Name, func(t *testing.T) {
			for j, cell := range record {
				if strings.ContainsRune(cell, Delimiter) {
					record[j] = `"` + cell + `"`
				}
			}
			testhelpers.AssertReadersEqual(
				t, testCases[i].Row, strings.NewReader(strings.Join(record, "\t")+"\n"))
		})
	}
}
