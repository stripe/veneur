package lightstep_test

import (
	"context"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sinks/lightstep"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

func TestFlushTracesBySinkSuccess(t *testing.T) {
	protobufFile := filepath.Join("../../testdata/protobuf/trace.pb")
	testFlushTracesBySink(t, protobufFile)
}

func TestFlushTracesBySinkCritical(t *testing.T) {
	protobufFile := filepath.Join("../../testdata/protobuf/trace_critical.pb")
	testFlushTracesBySink(t, protobufFile)
}

func testFlushTracesBySink(t *testing.T, protobufFile string) {
	protobufData, err := os.Open(protobufFile)
	require.NoError(t, err)
	defer protobufData.Close()

	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Logger: logrus.New(),
		Config: veneur.Config{
			Debug:    true,
			Hostname: "localhost",

			// Use a shorter interval for tests
			Interval:            50 * time.Millisecond,
			ReadBufferSizeBytes: 2097152,

			// Use only one reader, so that we can run tests on platforms which do not
			// support SO_REUSEPORT
			NumReaders: 1,

			// Currently this points nowhere, which is intentional. We don't need
			// internal metrics for the tests, and they make testing more complicated.
			StatsAddress: "localhost:8125",

			// Don't use the default port 8128: Veneur sends its own traces there,
			// causing failures
			SsfListenAddresses: []util.Url{{
				Value: &url.URL{
					Scheme: "udp",
					Host:   "127.0.0.1:0",
				},
			}},
			TraceMaxLengthBytes: 4096,

			SpanSinks: []veneur.SinkConfig{{
				Kind: "lightstep",
				Name: "lightstep",
				Config: lightstep.LightStepSpanSinkConfig{
					AccessToken: util.StringSecret{
						Value: "secret",
					},
					CollectorHost: util.Url{
						Value: &url.URL{
							Scheme: "http",
							Host:   "example.com",
						},
					},
				},
			}},
		},
		SpanSinkTypes: veneur.SpanSinkTypes{
			"lightstep": {
				Create:      lightstep.CreateSpanSink,
				ParseConfig: lightstep.ParseSpanConfig,
			},
		},
	})
	assert.NoError(t, err)

	trace.NeutralizeClient(server.TraceClient)
	server.TraceClient = nil

	server.Start()
	defer server.Shutdown()

	packet, err := ioutil.ReadAll(protobufData)
	assert.NoError(t, err)

	server.HandleTracePacket(packet, veneur.SSF_UNIX)

	assert.NoError(t, err)
	server.Flush(context.Background())
}
