package veneur

import (
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, logrus.New())

	m := UDPMetric{
		MetricKey: MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Value:      1.0,
		Digest:     12345,
		SampleRate: 1.0,
	}
	w.ProcessMetric(&m)

	wm := w.Flush()
	assert.Len(t, wm.counters, 1, "Number of flushed metrics")

	nometrics := w.Flush()
	assert.Len(t, nometrics.counters, 0, "Should flush no metrics")
}

func TestWorkerImportSet(t *testing.T) {
	w := NewWorker(1, nil, logrus.New())
	testset := NewSet("a.b.c", nil)
	testset.Sample("foo", 1.0)
	testset.Sample("bar", 1.0)

	jsonMetric, err := testset.Export()
	assert.NoError(t, err, "should have exported successfully")

	w.ImportMetric(jsonMetric)

	wm := w.Flush()
	assert.Len(t, wm.sets, 1, "number of flushed sets")
}
