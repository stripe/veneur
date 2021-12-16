package splunk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
)

func TestWorkerCount(t *testing.T) {
	counts := map[int]int{
		0:   1,
		1:   1,
		5:   5,
		100: 100,
	}

	for in, out := range counts {
		nWorkers := in
		workerProcs := out
		t.Run(strconv.Itoa(nWorkers), func(t *testing.T) {
			t.Parallel()
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			}))
			defer ts.Close()

			logger := logrus.New()
			logger.SetLevel(logrus.DebugLevel)
			sink, err := Create(&veneur.Server{}, "splunk", logrus.NewEntry(logger),
				veneur.Config{
					Hostname: "test-host",
				},
				SplunkSinkConfig{
					HecAddress:                  ts.URL,
					HecBatchSize:                100,
					HecConnectionLifetimeJitter: 0,
					HecIngestTimeout:            time.Duration(0),
					HecMaxConnectionLifetime:    10 * time.Second,
					HecSendTimeout:              time.Duration(0),
					HecSubmissionWorkers:        nWorkers,
					HecTLSValidateHostname:      "",
					HecToken:                    "00000000-0000-0000-0000-000000000000",
					SpanSampleRate:              10,
				},
				context.Background(),
			)
			sss := sink.(*splunkSpanSink)
			defer sss.Stop()
			require.NoError(t, err)

			// this number should correspond to the input:
			assert.Equal(t, nWorkers, sss.workers)

			err = sss.Start(nil)
			require.NoError(t, err)

			// use the number of channels for synchronization as a proxy
			// for the number of workers:
			assert.Len(t, sss.sync, workerProcs)
		})
	}
}
