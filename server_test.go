package veneur

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// set up a boilerplate local config for later use
func localConfig() Config {
	return Config{
		APIHostname: "http://localhost",
		Debug:       false,
		Hostname:    "localhost",

		// Use a shorter interval for tests
		Interval:            50 * time.Millisecond,
		Key:                 "",
		MetricMaxLength:     4096,
		Percentiles:         []float64{.5, .75, .99},
		ReadBufferSizeBytes: 2097152,
		UDPAddr:             "localhost:8126",
		HTTPAddr:            "localhost:8127",
		ForwardAddr:         "http://localhost",
		NumWorkers:          96,

		// Use only one reader, so that we can run tests
		// on platforms which do not support SO_REUSEPORT
		NumReaders: 1,

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
			assert.Equal(t, int(value+.5), int(metric.Value[0][1]+.5), metricName)
			return
		}
	}
	assert.Fail(t, "did not find expected metric", metricName)
}

// setupLocalServer creates a local server from the specified config
// and starts listening for requests. It returns the server for inspection.
func setupLocalServer(t *testing.T, config Config) Server {
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
type DDMetricsRequest struct {
	Series []DDMetric
}

func TestLocalServer(t *testing.T) {
	var ddmetrics DDMetricsRequest

	globalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}
		expectedMetrics := map[string]float64{
			"a.b.c.max": 100,
			"a.b.c.min": 1,

			// Count is normalized by second
			// so 5 values/50ms = 100 values/s
			"a.b.c.count": 100,

			// tdigest approximation causes this to be off by 1
			"a.b.c.50percentile": 6,
			"a.b.c.75percentile": 42,
			"a.b.c.99percentile": 98,
		}
		assert.Equal(t, 6, len(ddmetrics.Series), "incorrect number of elements in the flushed series")
		assertMetrics(t, ddmetrics, expectedMetrics)
		w.WriteHeader(http.StatusAccepted)
	}))
	config := localConfig()
	config.APIHostname = globalServer.URL

	server := setupLocalServer(t, config)

	metrics := []UDPMetric{
		{
			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      1.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		},
		{

			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      2.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		},
		{

			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      7.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		},
		{

			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      8.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		},
		{

			MetricKey: MetricKey{
				Name: "a.b.c",
				Type: "histogram",
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			LocalOnly:  true,
		},
	}

	for _, metric := range metrics {
		server.Workers[0].ProcessMetric(&metric)
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
