package veneur

import (
	"context"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/sinks/lightstep"
	"github.com/stripe/veneur/v14/sinks/prometheus"
	"github.com/stripe/veneur/v14/util"
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
			ProtobufFile: filepath.Join("testdata", "protobuf", "trace.pb"),
			JSONFile:     filepath.Join("testdata", "tracing_agent", "spans", "trace.pb.json"),
		},
		{
			Name:         "Critical",
			ProtobufFile: filepath.Join("testdata", "protobuf", "trace_critical.pb"),
			JSONFile:     filepath.Join("testdata", "tracing_agent", "spans", "trace_critical.pb.json"),
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

			testFlushTraceLightstep(t, pb, js)
		})
	}
}

// TODO(jcrpaquin): Replace testFlushTraceDatadog with testing of main.go init code

// testFlushTraceLightstep tests that the Lightstep sink can be initialized correctly
// and that the flushSpansLightstep function executes without error.
// We can't actually test the functionality end-to-end because the lightstep
// implementation doesn't expose itself for mocking.
func testFlushTraceLightstep(t *testing.T, protobuf, jsn io.Reader) {
	config := globalConfig()

	// this can be anything as long as it's not empty
	config.LightstepAccessToken = util.StringSecret{Value: "secret"}
	server := setupVeneurServer(t, config, nil, nil, nil, nil)
	defer server.Shutdown()

	//collector string, reconnectPeriod string, maximumSpans int, numClients int, accessToken string
	lsSink, err := lightstep.NewLightStepSpanSink("example.com", "5m", 10000, 1, "secret", log)
	server.spanSinks = append(server.spanSinks, lsSink)

	packet, err := ioutil.ReadAll(protobuf)
	assert.NoError(t, err)

	server.HandleTracePacket(packet, SSF_UNIX)

	assert.NoError(t, err)
	server.Flush(context.Background())
}

func TestNewPrometheusMetricSinkConfig(t *testing.T) {
	config := Config{
		PrometheusRepeaterAddress: "localhost:9125",
		PrometheusNetworkType:     "tcp",

		// Required or NewFromConfig fails.
		Interval:     time.Duration(10 * time.Second),
		StatsAddress: "localhost:62251",
	}

	server, err := NewFromConfig(ServerConfig{
		Logger: logrus.New(),
		Config: config,
	})
	assert.NoError(t, err)

	sink := server.metricSinks[0].(*prometheus.StatsdRepeater)
	assert.Equal(t, "prometheus", sink.Name())
}
