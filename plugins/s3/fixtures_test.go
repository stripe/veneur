package s3

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/stripe/veneur/samplers"
)

type CSVTestCase struct {
	Name     string
	DDMetric samplers.DDMetric
	Row      io.Reader
}

func CSVTestCases() []CSVTestCase {

	partition := time.Now().Format("20060102")

	return []CSVTestCase{
		{
			Name: "BasicDDMetric",
			DDMetric: samplers.DDMetric{
				Name: "a.b.c.max",
				Value: [1][2]float64{[2]float64{1476119058,
					100}},
				Tags: []string{"foo:bar",
					"baz:quz"},
				MetricType: "gauge",
				Hostname:   "globalstats",
				DeviceName: "food",
				Interval:   0,
			},
			Row: strings.NewReader(fmt.Sprintf("a.b.c.max\t{foo:bar,baz:quz}\tgauge\tglobalstats\ttestbox-c3eac9\tfood\t0\t2016-10-10 05:04:18\t100\t%s\n", partition)),
		},
		{
			// Test that we are able to handle a missing field (DeviceName)
			Name: "MissingDeviceName",
			DDMetric: samplers.DDMetric{
				Name: "a.b.c.max",
				Value: [1][2]float64{[2]float64{1476119058,
					100}},
				Tags: []string{"foo:bar",
					"baz:quz"},
				MetricType: "rate",
				Hostname:   "localhost",
				DeviceName: "",
				Interval:   10,
			},
			Row: strings.NewReader(fmt.Sprintf("a.b.c.max\t{foo:bar,baz:quz}\trate\tlocalhost\ttestbox-c3eac9\t\t10\t2016-10-10 05:04:18\t100\t%s\n", partition)),
		},
		{
			// Test that we are able to handle tags which have tab characters in them
			// by quoting the entire field
			// (tags shouldn't do this, but we should handle them properly anyway)
			Name: "TabTag",
			DDMetric: samplers.DDMetric{
				Name: "a.b.c.max",
				Value: [1][2]float64{[2]float64{1476119058,
					100}},
				Tags: []string{"foo:b\tar",
					"baz:quz"},
				MetricType: "rate",
				Hostname:   "localhost",
				DeviceName: "eniac",
				Interval:   10,
			},
			Row: strings.NewReader(fmt.Sprintf("a.b.c.max\t\"{foo:b\tar,baz:quz}\"\trate\tlocalhost\ttestbox-c3eac9\teniac\t10\t2016-10-10 05:04:18\t100\t%s\n", partition)),
		},
	}
}
