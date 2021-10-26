package veneur

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stripe/veneur/v14/routing"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
)

func TestSortableJSONMetrics(t *testing.T) {
	testList := []samplers.JSONMetric{
		{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
		{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
	}

	sortable := newSortableJSONMetrics(testList)
	sort.Sort(sortable)
	assert.EqualValues(t, []samplers.JSONMetric{
		{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
		{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
	}, testList, "should have sorted the metrics by hashes")
}

func TestIteratingByWorkerSet(t *testing.T) {
	testList := []samplers.JSONMetric{
		{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
		{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
		{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
		{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
	}

	workerSet := WorkerSet{
		Workers: []*Worker{
			{}, {}, {},
		},
		ComputationRoutingConfig: &routing.ComputationRoutingConfig{
			MatcherConfigs: []routing.MatcherConfig{
				{
					Name: routing.NameMatcher{
						Match: func(s string) bool {
							return true
						},
					},
				},
			},
			WorkerCount: 3,
		},
	}

	sortable := newSortableJSONMetrics(testList)
	sort.Sort(sortable)
	var testChunks [][]samplers.JSONMetric
	iter := newJSONMetricsByWorkerSet(sortable, workerSet)
	testWorkerIndices := []int{}
	for iter.Next() {
		nextChunk, workerIndex := iter.Chunk()
		testChunks = append(testChunks, nextChunk)
		testWorkerIndices = append(testWorkerIndices, workerIndex)
	}
	assert.EqualValues(t, [][]samplers.JSONMetric{
		[]samplers.JSONMetric{
			{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
			{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		},
		[]samplers.JSONMetric{
			{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
			{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		},
		[]samplers.JSONMetric{
			{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
			{MetricKey: samplers.MetricKey{Name: "bar", Type: "set"}},
		},
		[]samplers.JSONMetric{
			{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
			{MetricKey: samplers.MetricKey{Name: "qux", Type: "gauge"}},
		},
	}, testChunks, "should have arranged metrics that hashed equally contiguously")
	assert.EqualValues(t, []int{0, 1, 2, 0}, testWorkerIndices, "should have distributed chunks to workers round-robin")
}

func TestJSONMetricsComputationRoutingFiltering(t *testing.T) {
	testList := []samplers.JSONMetric{
		{MetricKey: samplers.MetricKey{Name: "foo", Type: "histogram"}},
		{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
	}

	workerSet := WorkerSet{
		Workers: []*Worker{
			{}, {}, {},
		},
		ComputationRoutingConfig: &routing.ComputationRoutingConfig{
			MatcherConfigs: []routing.MatcherConfig{
				{
					Name: routing.NameMatcher{
						Match: func(s string) bool {
							return s != "foo"
						},
					},
				},
			},
			WorkerCount: 3,
		},
	}

	sortable := newSortableJSONMetrics(testList)
	sort.Sort(sortable)
	var testChunks [][]samplers.JSONMetric
	iter := newJSONMetricsByWorkerSet(sortable, workerSet)
	testWorkerIndices := []int{}
	for iter.Next() {
		nextChunk, workerIndex := iter.Chunk()
		testChunks = append(testChunks, nextChunk)
		testWorkerIndices = append(testWorkerIndices, workerIndex)
	}
	assert.EqualValues(t, [][]samplers.JSONMetric{
		[]samplers.JSONMetric{
			{MetricKey: samplers.MetricKey{Name: "baz", Type: "counter"}},
		},
	}, testChunks, "should have skipped foo")
	assert.EqualValues(t, []int{0}, testWorkerIndices, "first chunk should go to worker 0")
}

func testServerImport(t *testing.T, filename string, contentEncoding string) {

	f, err := os.Open(filename)
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", contentEncoding)

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	handler := handleImport(s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusAccepted, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportCompressed(t *testing.T) {
	// Test that the global veneur instance can handle
	// requests that provide compressed metrics
	testServerImport(t, filepath.Join("testdata", "import.deflate"), "deflate")
}

func TestServerImportUncompressed(t *testing.T) {
	// Test that the global veneur instance can handle
	// requests that provide uncompressed metrics
	testServerImport(t, filepath.Join("testdata", "import.uncompressed"), "")
}

func TestServerImportGzip(t *testing.T) {
	// Test that the global veneur instance
	// returns a 400 for gzipped-input

	f, err := os.Open(filepath.Join("testdata", "import.uncompressed"))
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
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	handler := handleImport(s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportCompressedInvalid(t *testing.T) {
	// Test that the global veneur instance
	// properly responds to invalid zlib-deflated data

	//TODO(aditya) test that the metrics are properly reported

	f, err := os.Open(filepath.Join("testdata", "import.uncompressed"))
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", "deflate")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	handler := handleImport(s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Test server returned wrong HTTP response code")
}

func TestServerImportUncompressedInvalid(t *testing.T) {
	// Test that the global veneur instance
	// properly responds to invalid zlib-deflated data

	//TODO(aditya) test that the metrics are properly reported

	f, err := os.Open(filepath.Join("testdata", "import.deflate"))
	assert.NoError(t, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", "")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	handler := handleImport(s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Test server returned wrong HTTP response code")
}

// TestServerImportEmptyError tests that the global
// veneur instance returns an error
// if it receives what amounts to an empty struct,
// because that's usually the sign of an error
func TestServerImportEmptyError(t *testing.T) {

	// explicitly use the wrong type here
	data := []struct {
		Bad string
	}{
		{"Foo"},
		{"Bar"},
	}
	testServerImportHelper(t, data)
}

// TestServerImportEmptyError tests that
// the global veneur instance returns an error
// if it receives what amounts to a slice of empty structs
// because that's usually the sign of an error
func TestServerImportEmptyStructError(t *testing.T) {

	// explicitly use the wrong type here
	data := []struct {
		Bad string
	}{
		{"Foo"},
		{"Bar"},
	}
	testServerImportHelper(t, data)
}

// TestServerImportEmptyError tests that
// the global veneur instance returns an error
// if it receives an empty list, because the client
// should never be sending an empty list.
func TestServerImportEmptyListError(t *testing.T) {
	data := []samplers.JSONMetric{}
	testServerImportHelper(t, data)
}

func TestGeneralHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Healthcheck did not succeed")
}

func TestOkTraceHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck/tracing", nil)

	config := localConfig()
	// We must enable tracing, as it's disabled by default, by turning on one
	// of the tracing sinks.
	config.LightstepAccessToken = util.StringSecret{Value: "farts"}
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Trace healthcheck did not succeed")
}

func TestNoTracingConfiguredTraceHealthCheck(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthcheck/tracing", nil)

	config := localConfig()

	config.SsfListenAddresses = []string{}
	server, _ := NewFromConfig(ServerConfig{
		Logger: logrus.New(),
		Config: config,
	})
	server.Start()
	defer server.Shutdown()

	w := httptest.NewRecorder()

	handler := server.Handler()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code, "Trace healthcheck reports tracing is enabled")
}

func TestBuildDate(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/builddate", nil)

	config := localConfig()
	config.SsfListenAddresses = []string{}
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	bts, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err, "error reading /builddate")

	assert.Equal(t, string(bts), BUILD_DATE, "received invalid build date")

	// we can't always check this against the current time
	// because that would break local tests when run with `go test`
	if BUILD_DATE != defaultLinkValue {
		date, err := strconv.ParseInt(string(bts), 10, 64)
		assert.NoError(t, err, "error parsing date %s", string(bts))

		dt := time.Unix(date, 0)
		duration := time.Since(dt)
		if duration > 60*time.Minute {
			assert.Fail(t, fmt.Sprintf("either date %s is invalid, or our builds are taking more than an hour", dt.Format(time.RFC822)))
		}
	}
}

func TestVersion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/version", nil)

	config := localConfig()
	config.SsfListenAddresses = []string{}
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	w := httptest.NewRecorder()

	handler := s.Handler()
	handler.ServeHTTP(w, r)

	bts, err := ioutil.ReadAll(w.Body)
	assert.NoError(t, err, "error reading /version")

	assert.Equal(t, string(bts), VERSION, "received invalid version")
}

func testServerImportHelper(t *testing.T, data interface{}) {
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(data)
	assert.NoError(t, err)

	r := httptest.NewRequest(http.MethodPost, "/import", &b)
	r.Header.Set("Content-Encoding", "")

	w := httptest.NewRecorder()

	config := localConfig()
	s := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer s.Shutdown()

	handler := handleImport(s)
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Test server returned wrong HTTP response code")
}

func BenchmarkNewSortableJSONMetrics(b *testing.B) {
	filename := filepath.Join("testdata", "import.deflate")
	contentEncoding := "deflate"

	f, err := os.Open(filename)
	assert.NoError(b, err, "Error reading response fixture")
	defer f.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", f)
	r.Header.Set("Content-Encoding", contentEncoding)

	w := httptest.NewRecorder()

	_, jsonMetrics, err := unmarshalMetricsFromHTTP(context.Background(), trace.DefaultClient, w, r)
	assert.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		newSortableJSONMetrics(jsonMetrics)
	}
}
