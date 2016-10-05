package veneur

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func testServerImport(t *testing.T, filename string, contentEncoding string) {

	f, err := os.Open(filename)
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", contentEncoding)

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config)
	defer s.Shutdown()

	handler := handleImport(&s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusAccepted, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportCompressed(t *testing.T) {
	// Test that the global veneur instance can handle
	// requests that provide compressed metrics
	testServerImport(t, filepath.Join("fixtures", "import.deflate"), "deflate")
}

func TestServerImportUncompressed(t *testing.T) {
	// Test that the global veneur instance can handle
	// requests that provide uncompressed metrics
	testServerImport(t, filepath.Join("fixtures", "import.uncompressed"), "")
}

func TestServerImportGzip(t *testing.T) {
	// Test that the global veneur instance
	// returns a 400 for gzipped-input

	f, err := os.Open(filepath.Join("fixtures", "import.uncompressed"))
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	_, err = io.Copy(gz, f)
	assert.NoError(t, err)
	gz.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", &data)
	r.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config)
	defer s.Shutdown()

	handler := handleImport(&s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportCompressedInvalid(t *testing.T) {
	// Test that the global veneur instance
	// properly responds to invalid zlib-deflated data

	//TODO(aditya) test that the metrics are properly reported

	f, err := os.Open(filepath.Join("fixtures", "import.uncompressed"))
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", "deflate")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config)
	defer s.Shutdown()

	handler := handleImport(&s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportUncompressedInvalid(t *testing.T) {
	// Test that the global veneur instance
	// properly responds to invalid zlib-deflated data

	//TODO(aditya) test that the metrics are properly reported

	f, err := os.Open(filepath.Join("fixtures", "import.deflate"))
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", "")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config)
	defer s.Shutdown()

	handler := handleImport(&s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Test server returned wrong HTTP response code")
}
