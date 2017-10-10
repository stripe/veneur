package localfile

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

type badWriter struct{}

func (w *badWriter) Write(b []byte) (n int, err error) {
	return 0, fmt.Errorf("This writer fails when you try to write!")
}

func TestName(t *testing.T) {
	plugin := Plugin{FilePath: "doesntexist.txt", Logger: logrus.New()}
	assert.Equal(t, plugin.Name(), "localfile")
}

func TestAppendToWriter(t *testing.T) {
	b := &bytes.Buffer{}

	metrics := []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "a.b.c.max",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
			},
			Type: samplers.GaugeMetric,
		},
	}

	err := appendToWriter(b, metrics, "globblestoots", 10)
	assert.NoError(t, err)
	assert.NotEqual(t, b.Len(), 0)
}

func TestHandlesErrorsInAppendToWriter(t *testing.T) {
	b := &badWriter{}

	err := appendToWriter(b, []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "sketchy.metric",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags:      []string{"skepticism:high"},
			Type:      samplers.GaugeMetric,
		},
	}, "globblestoots", 10)

	assert.Error(t, err)
}

func TestWritesToDevNull(t *testing.T) {
	plugin := Plugin{FilePath: "/dev/null", Logger: logrus.New(), hostname: "globblestoots"}
	err := plugin.Flush(context.TODO(), []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "sketchy.metric",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags:      []string{"skepticism:high"},
			Type:      samplers.GaugeMetric,
		},
	})
	assert.NoError(t, err)
}

func TestFailsWritingToInvalidPath(t *testing.T) {
	plugin := Plugin{FilePath: "", Logger: logrus.New(), hostname: "globblestoots"}
	err := plugin.Flush(context.TODO(), []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "sketchy.metric",
			Timestamp: 1476119058,
			Value:     float64(100),
			Tags:      []string{"skepticism:high"},
			Type:      samplers.GaugeMetric,
		},
	})
	assert.Error(t, err)
}
