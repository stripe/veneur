package veneur

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSortableJSONMetrics(t *testing.T) {
	testList := []JSONMetric{
		JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
		JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
		JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
		JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
	}

	sortable := newSortableJSONMetrics(testList, 96)
	assert.EqualValues(t, []uint32{0x4f, 0x3a, 0x2, 0x3c}, sortable.workerIndices, "should have hashed correctly")

	sort.Sort(sortable)
	assert.EqualValues(t, []JSONMetric{
		JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
		JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
		JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
		JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
	}, testList, "should have sorted the metrics by hashes")
}

func TestSortableJSONMetricHashing(t *testing.T) {
	packet, err := ParseMetric([]byte("foo:1|h|#bar"))
	assert.NoError(t, err, "should have parsed test packet")

	testList := []JSONMetric{
		JSONMetric{
			MetricKey: packet.MetricKey,
			Tags:      packet.Tags,
		},
	}

	sortable := newSortableJSONMetrics(testList, 96)
	assert.Equal(t, 1, sortable.Len(), "should have exactly 1 metric")
	assert.Equal(t, packet.Digest%96, sortable.workerIndices[0], "should have had the same hash")
}

func TestIteratingByWorker(t *testing.T) {
	testList := []JSONMetric{
		JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
		JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
		JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
		JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
		JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
		JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
		JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
		JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
	}

	var testChunks [][]JSONMetric
	iter := newJSONMetricsByWorker(testList, 96)
	for iter.Next() {
		nextChunk, workerIndex := iter.Chunk()
		testChunks = append(testChunks, nextChunk)

		for i := iter.currentStart; i < iter.nextStart; i++ {
			assert.Equal(t, workerIndex, int(iter.sjm.workerIndices[i]), "mismatched worker index for %#v", iter.sjm.metrics[i])
		}
	}

	assert.EqualValues(t, [][]JSONMetric{
		[]JSONMetric{
			JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
			JSONMetric{MetricKey: MetricKey{Name: "baz", Type: "counter"}},
		},
		[]JSONMetric{
			JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
			JSONMetric{MetricKey: MetricKey{Name: "bar", Type: "set"}},
		},
		[]JSONMetric{
			JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
			JSONMetric{MetricKey: MetricKey{Name: "qux", Type: "gauge"}},
		},
		[]JSONMetric{
			JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
			JSONMetric{MetricKey: MetricKey{Name: "foo", Type: "histogram"}},
		},
	}, testChunks, "should have sorted the metrics by hashes")
}
