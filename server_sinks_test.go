package veneur

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/sinks/datadog"
	"github.com/stripe/veneur/sinks/lightstep"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func TestFlushTracesBySink(t *testing.T) {
	type TestCase struct {
		Name         string
		ProtobufFile string
		JSONFile     string
	}

	cases := []TestCase{
		{
			Name:         "Success",
			ProtobufFile: filepath.Join("fixtures", "protobuf", "trace.pb"),
			JSONFile:     filepath.Join("fixtures", "tracing_agent", "spans", "trace.pb.json"),
		},
		{
			Name:         "Critical",
			ProtobufFile: filepath.Join("fixtures", "protobuf", "trace_critical.pb"),
			JSONFile:     filepath.Join("fixtures", "tracing_agent", "spans", "trace_critical.pb.json"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			pb, err := os.Open(tc.ProtobufFile)
			assert.NoError(t, err)
			defer pb.Close()

			js, err := os.Open(tc.JSONFile)
			assert.NoError(t, err)
			defer js.Close()

			testFlushTraceDatadog(t, pb, js)
		})
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			pb, err := os.Open(tc.ProtobufFile)
			assert.NoError(t, err)
			defer pb.Close()

			js, err := os.Open(tc.JSONFile)
			assert.NoError(t, err)
			defer js.Close()

			testFlushTraceLightstep(t, pb, js)
		})
	}
}

func testFlushTraceDatadog(t *testing.T, protobuf, jsn io.Reader) {
	var expected [][]DatadogTraceSpan
	err := json.NewDecoder(jsn).Decode(&expected)
	assert.NoError(t, err)

	remoteResponseChan := make(chan [][]DatadogTraceSpan, 1)
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var actual [][]DatadogTraceSpan
		err = json.NewDecoder(r.Body).Decode(&actual)
		assert.NoError(t, err)

		w.WriteHeader(http.StatusAccepted)
		remoteResponseChan <- actual
	}))
	defer remoteServer.Close()

	config := globalConfig()
	config.DatadogAPIKey = "secret"
	config.DatadogTraceAPIAddress = remoteServer.URL

	server := setupVeneurServer(t, config, nil, nil, nil)
	defer server.Shutdown()

	ddSink, err := datadog.NewDatadogSpanSink("http://example.com", 100, server.HTTPClient, logrus.New())

	server.TraceClient = nil
	server.spanSinks = append(server.spanSinks, ddSink)

	packet, err := ioutil.ReadAll(protobuf)
	assert.NoError(t, err)

	server.HandleTracePacket(packet)
	server.Flush(context.Background())

	// wait for remoteServer to process the POST
	select {
	case actual := <-remoteResponseChan:
		assert.Equal(t, expected, actual)
		// all is safe
		break
	case <-time.After(2 * time.Second):
		assert.Fail(t, "Global server did not complete all responses before test terminated!")
	}
}

// testFlushTraceLightstep tests that the Lightstep sink can be initialized correctly
// and that the flushSpansLightstep function executes without error.
// We can't actually test the functionality end-to-end because the lightstep
// implementation doesn't expose itself for mocking.
func testFlushTraceLightstep(t *testing.T, protobuf, jsn io.Reader) {
	config := globalConfig()

	// this can be anything as long as it's not empty
	config.LightstepAccessToken = "secret"
	server := setupVeneurServer(t, config, nil, nil, nil)
	defer server.Shutdown()

	//collector string, reconnectPeriod string, maximumSpans int, numClients int, accessToken string
	lsSink, err := lightstep.NewLightStepSpanSink("example.com", "5m", 10000, 1, "secret", log)
	server.spanSinks = append(server.spanSinks, lsSink)

	packet, err := ioutil.ReadAll(protobuf)
	assert.NoError(t, err)

	server.HandleTracePacket(packet)

	assert.NoError(t, err)
	server.Flush(context.Background())
}

// This test lives here because is tests the server's behavior when making a
// datadog metric sink
func TestNewDatadogMetricSinkConfig(t *testing.T) {
	// test the variables that have been renamed
	config := Config{
		DatadogAPIKey:          "apikey",
		DatadogAPIHostname:     "http://api",
		DatadogTraceAPIAddress: "http://trace",
		DatadogSpanBufferSize:  32,
		SsfListenAddresses:     []string{"udp://127.0.0.1:99"},

		// required or NewFromConfig fails
		Interval:     "10s",
		StatsAddress: "localhost:62251",
	}
	server, err := NewFromConfig(logrus.New(), config)

	if err != nil {
		t.Fatal(err)
	}
	sink := server.metricSinks[0].(*datadog.DatadogMetricSink)
	assert.Equal(t, "datadog", sink.Name())
	// Verify that the values got set	assert.Equal(t, "apikey", sink.APIKey)
	assert.Equal(t, "http://api", sink.DDHostname)
}

type sadSpanSink struct {
	t    *testing.T
	done chan struct{}
	once sync.Once
}

func newSadSpanSink(t *testing.T, done chan struct{}) *sadSpanSink {
	return &sadSpanSink{
		t:    t,
		done: done,
		once: sync.Once{},
	}
}

func (*sadSpanSink) Start(*trace.Client) error {
	return nil
}

func (*sadSpanSink) Name() string {
	return "sad_span_sink"
}

func (*sadSpanSink) Ingest(*ssf.SSFSpan) error {
	return nil
}

func (sss *sadSpanSink) Flush(ctx context.Context) {
	defer sss.once.Do(func() { close(sss.done) })
	select {
	case <-ctx.Done():
		sss.t.Log("Correctly timed out")
	case <-time.After(2 * time.Second):
		sss.t.Fatal("Did not time-constrain trace flush")
		close(sss.done)
	}
}

func TestSpanFlushTimeouts(t *testing.T) {
	cfg := localConfig()
	cfg.IndicatorSpanTimerName = "indicator.span.timer"
	cfg.SpanFlushTimeout = "1ns"

	// We use the forwarding fixture, but we're only really using the server:
	done := make(chan struct{})
	srv := setupVeneurServer(t, cfg, nil, nil, newSadSpanSink(t, done))

	// The server is responsible for creating its own context with
	// an appropriate short timeout. A failure to do so should
	// result in a test timeout & panic:
	srv.Flush(context.TODO())
	<-done
}
