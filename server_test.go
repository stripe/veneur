package veneur

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/tdigest"
)

const ε = .00002

const DefaultFlushInterval = 50 * time.Millisecond
const DefaultServerTimeout = 100 * time.Millisecond

var DebugMode bool

func TestMain(m *testing.M) {
	flag.Parse()
	DebugMode = flag.Lookup("test.v").Value.(flag.Getter).Get().(bool)
	os.Exit(m.Run())
}

// On the CI server, we can't be guaranteed that the port will be
// released immediately after the server is shut down. Instead, use
// a unique port for each test. As long as we don't have an insane number
// of integration tests, we should be fine.
var HttpAddrPort = 8127

// set up a boilerplate local config for later use
func localConfig() Config {
	return generateConfig("http://localhost")
}

// set up a boilerplate global config for later use
func globalConfig() Config {
	return generateConfig("")
}

// generateConfig is not called config to avoid
// accidental variable shadowing
func generateConfig(forwardAddr string) Config {
	port := HttpAddrPort
	HttpAddrPort++

	return Config{
		APIHostname: "http://localhost",
		Debug:       DebugMode,
		Hostname:    "localhost",

		// Use a shorter interval for tests
		Interval:            DefaultFlushInterval,
		Key:                 "",
		MetricMaxLength:     4096,
		Percentiles:         []float64{.5, .75, .99},
		ReadBufferSizeBytes: 2097152,
		UDPAddr:             "localhost:8126",
		HTTPAddr:            fmt.Sprintf("localhost:%d", port),
		ForwardAddr:         forwardAddr,
		NumWorkers:          96,

		// Use only one reader, so that we can run tests
		// on platforms which do not support SO_REUSEPORT
		NumReaders: 1,

		// Currently this points nowhere, which is intentional.
		// We don't need internal metrics for the tests, and they make testing
		// more complicated.
		StatsAddr:  "localhost:8125",
		Tags:       []string{},
		SentryDSN:  "",
		FlushLimit: 1024,
	}
}

// assertMetrics checks that all expected metrics are present
// and have the correct value
func assertMetrics(t *testing.T, metrics DDMetricsRequest, expectedMetrics map[string]float64) {
	// it doesn't count as accidentally quadratic if it's intentional
	for metricName, expectedValue := range expectedMetrics {
		assertMetric(t, metrics, metricName, expectedValue)
	}
}

func assertMetric(t *testing.T, metrics DDMetricsRequest, metricName string, value float64) {
	defer func() {
		if r := recover(); r != nil {
			assert.Fail(t, "error extracting metrics", r)
		}
	}()
	for _, metric := range metrics.Series {
		if metric.Name == metricName {
			assert.Equal(t, int(value+.5), int(metric.Value[0][1]+.5), "Incorrect value for metric %s", metricName)
			return
		}
	}
	assert.Fail(t, "did not find expected metric", metricName)
}

// setupVeneurServer creates a local server from the specified config
// and starts listening for requests. It returns the server for inspection.
func setupVeneurServer(t *testing.T, config Config) Server {
	server, err := NewFromConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	packetPool := &sync.Pool{
		New: func() interface{} {
			return make([]byte, config.MetricMaxLength)
		},
	}

	for i := 0; i < config.NumReaders; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					assert.Fail(t, "reader panicked while reading from socket", err)
				}
			}()
			server.ReadSocket(packetPool, config.NumReaders != 1)
		}()
	}

	go server.HTTPServe()
	return server
}

// DDMetricsRequest represents the body of the POST request
// for sending metrics data to Datadog
// Eventually we'll want to define this symmetrically.
type DDMetricsRequest struct {
	Series []DDMetric
}

// TestLocalServerUnaggregatedMetrics tests the behavior of
// the veneur client when operating without a global veneur
// instance (ie, when sending data directly to the remote server)
func TestLocalServerUnaggregatedMetrics(t *testing.T) {
	// Since we are asserting inside a separate goroutine, we should
	// always assert that we get to the end of the goroutine.
	// In the future, if the key call (in this case, server.Flush)
	// is made asynchronous, we will know if we are ever encountering
	// a race condition in which our assertions are simply not executing.
	// (Failures after a test has finished executing will actually cause
	// the test suite to fail, as long as it is still running, but this is
	// a failsafe).

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues := []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	expectedMetrics := map[string]float64{
		"a.b.c.max": 100,
		"a.b.c.min": 1,

		// Count is normalized by second
		// so 5 values/50ms = 100 values/s
		"a.b.c.count": float64(len(metricValues)) * float64(time.Second) / float64(DefaultFlushInterval),

		// tdigest approximation causes this to be off by 1
		"a.b.c.50percentile": 6,
		"a.b.c.75percentile": 42,
		"a.b.c.99percentile": 98,
	}

	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var ddmetrics DDMetricsRequest
		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, 6, len(ddmetrics.Series), "incorrect number of elements in the flushed series on the remote server")
		assertMetrics(t, ddmetrics, expectedMetrics)
		w.WriteHeader(http.StatusAccepted)
		RemoteResponseChan <- struct{}{}
	}))

	config := localConfig()
	config.APIHostname = remoteServer.URL

	server := setupVeneurServer(t, config)
	defer server.Shutdown()

	for _, value := range metricValues {
		server.Workers[0].ProcessMetric(&UDPMetric{
			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		})
	}
	server.Flush(config.Interval, config.FlushLimit)
}

func TestGlobalServerFlush(t *testing.T) {

	// Same as in TestLocalServerUnaggregatedMetrics
	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	metricValues := []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	expectedMetrics := map[string]float64{
		"a.b.c.max": 100,
		"a.b.c.min": 1,

		// Count is normalized by second
		// so 5 values/50ms = 100 values/s
		"a.b.c.count": float64(len(metricValues)) * float64(time.Second) / float64(DefaultFlushInterval),

		// tdigest approximation causes this to be off by 1
		"a.b.c.50percentile": 6,
		"a.b.c.75percentile": 42,
		"a.b.c.99percentile": 98,
	}

	config := globalConfig()

	// Set up a remote server (the API that we're sending the data to)
	// (e.g. Datadog)
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// test that we are actually running as the server that we think we are
		// since it's easy to mess this up when refactoring the test fixtures
		apiUrl, err := url.Parse(config.APIHostname)
		assert.NoError(t, err)
		assert.Equal(t, apiUrl.Host, r.Host)

		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var ddmetrics DDMetricsRequest

		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(expectedMetrics), len(ddmetrics.Series), "incorrect number of elements in the flushed series on the remote server")
		assertMetrics(t, ddmetrics, expectedMetrics)

		RemoteResponseChan <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))

	config.APIHostname = remoteServer.URL
	config.NumWorkers = 1

	server := setupVeneurServer(t, config)
	defer server.Shutdown()

	for _, value := range metricValues {
		server.Workers[0].ProcessMetric(&UDPMetric{
			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		})
	}

	server.Flush(config.Interval, config.FlushLimit)
}

func TestLocalServerMixedMetrics(t *testing.T) {

	// Same as in TestLocalServerUnaggregatedMetrics
	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	// Unlike flushing to to the remote server, flushing forward
	// (ie, flushing to global veneur) is asynchronous.
	// This means it is quite likely that the function will return before
	// the global server has actually responded, unless we introduce a small timeout
	GlobalResponseChan := make(chan struct{})
	defer func() {
		select {
		case <-GlobalResponseChan:
			// all is safe
			return
		case <-time.After(DefaultServerTimeout):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	// The exact gob stream that we will receive might differ, so we can't
	// test against the bytestream directly. But the two streams should unmarshal
	// to t-digests that have the same key properties, so we can test
	// those.
	const ExpectedGobStream = "\r\xff\x87\x02\x01\x02\xff\x88\x00\x01\xff\x84\x00\x007\xff\x83\x03\x01\x01\bCentroid\x01\xff\x84\x00\x01\x03\x01\x04Mean\x01\b\x00\x01\x06Weight\x01\b\x00\x01\aSamples\x01\xff\x86\x00\x00\x00\x17\xff\x85\x02\x01\x01\t[]float64\x01\xff\x86\x00\x01\b\x00\x00/\xff\x88\x00\x05\x01\xfe\xf0?\x01\xfe\xf0?\x00\x01@\x01\xfe\xf0?\x00\x01\xfe\x1c@\x01\xfe\xf0?\x00\x01\xfe @\x01\xfe\xf0?\x00\x01\xfeY@\x01\xfe\xf0?\x00\x05\b\x00\xfeY@\x05\b\x00\xfe\xf0?\x05\b\x00\xfeY@"

	var HistogramValues = []float64{1.0, 2.0, 7.0, 8.0, 100.0}

	// Number of events observed (in 50ms interval)
	var HistogramCountRaw = len(HistogramValues)

	// Normalize to events/second
	// Explicitly convert to int to avoid confusing Stringer behavior
	var HistogramCountNormalized = float64(HistogramCountRaw) * float64(time.Second) / float64(DefaultFlushInterval)

	// Number of events observed
	const CounterNumEvents = 40

	expectedMetrics := map[string]float64{
		// 40 events/50ms = 800 events/s
		"x.y.z":     CounterNumEvents * float64(time.Second) / float64(DefaultFlushInterval),
		"a.b.c.max": 100,
		"a.b.c.min": 1,

		// Count is normalized by second
		// so 5 values/50ms = 100 values/s
		"a.b.c.count": float64(HistogramCountNormalized),
	}

	// Set up a remote server (the API that we're sending the data to)
	// (e.g. Datadog)
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var ddmetrics DDMetricsRequest

		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(expectedMetrics), len(ddmetrics.Series), "incorrect number of elements in the flushed series on the remote server")
		assertMetrics(t, ddmetrics, expectedMetrics)

		RemoteResponseChan <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))

	// This represents the global veneur instance, which receives request from
	// the local veneur instances, aggregates the data, and sends it to the remote API
	// (e.g. Datadog)
	globalVeneur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, r.URL.Path, "/import", "Global veneur should receive request on /import path")

		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		type requestItem struct {
			Name      string      `json:"name"`
			Tags      interface{} `json:"tags"`
			Tagstring string      `json:"tagstring"`
			Type      string      `json:"type"`
			Value     []byte      `json:"value"`
		}

		var metrics []requestItem

		err = json.NewDecoder(zr).Decode(&metrics)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, 1, len(metrics), "incorrect number of elements in the flushed series")

		tdExpected := tdigest.NewMerging(100, false)
		err = tdExpected.GobDecode([]byte(ExpectedGobStream))
		assert.NoError(t, err, "Should not have encountered error in decoding expected gob stream")

		td := tdigest.NewMerging(100, false)
		err = td.GobDecode(metrics[0].Value)
		assert.NoError(t, err, "Should not have encountered error in decoding gob stream")

		assert.Equal(t, expectedMetrics["a.b.c.min"], td.Min(), "Minimum value is incorrect")
		assert.Equal(t, expectedMetrics["a.b.c.max"], td.Max(), "Maximum value is incorrect")

		// The remote server receives the raw count, *not* the normalized count
		assert.InEpsilon(t, HistogramCountRaw, td.Count(), ε)
		assert.Equal(t, tdExpected, td, "Underlying tdigest structure is incorrect")

		GlobalResponseChan <- struct{}{}
		w.WriteHeader(http.StatusAccepted)
	}))

	config := localConfig()
	config.APIHostname = remoteServer.URL
	config.ForwardAddr = globalVeneur.URL
	config.NumWorkers = 1

	server := setupVeneurServer(t, config)
	defer server.Shutdown()

	// Create non-local metrics that should be passed to the global veneur instance
	for _, value := range HistogramValues {
		server.Workers[0].ProcessMetric(&UDPMetric{
			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      value,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  false,
		})
	}

	// Create local-only metrics that should be passed directly to the remote API
	for i := 0; i < CounterNumEvents; i++ {
		server.Workers[0].ProcessMetric(&UDPMetric{
			MetricKey: MetricKey{
				Name: "x.y.z",
				Type: "counter",
			},
			Value:      1.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		})
	}

	server.Flush(config.Interval, config.FlushLimit)
}

func TestSplitBytes(t *testing.T) {
	rand.Seed(time.Now().Unix())
	buf := make([]byte, 1000)

	for i := 0; i < 1000; i++ {
		// we construct a string of random length which is approximately 1/3rd A
		// and the other 2/3rds B
		buf = buf[:rand.Intn(cap(buf))]
		for i := range buf {
			if rand.Intn(3) == 0 {
				buf[i] = 'A'
			} else {
				buf[i] = 'B'
			}
		}
		checkBufferSplit(t, buf)
		buf = buf[:cap(buf)]
	}

	// also test pathological cases that the fuzz is unlikely to find
	checkBufferSplit(t, nil)
	checkBufferSplit(t, []byte{})
}

func checkBufferSplit(t *testing.T, buf []byte) {
	var testSplit [][]byte
	sb := NewSplitBytes(buf, 'A')
	for sb.Next() {
		testSplit = append(testSplit, sb.Chunk())
	}

	// now compare our split to the "real" implementation of split
	assert.EqualValues(t, bytes.Split(buf, []byte{'A'}), testSplit, "should have split %s correctly", buf)
}
