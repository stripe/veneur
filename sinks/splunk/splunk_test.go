package splunk_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	hec "github.com/fuyufjh/splunk-hec-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/sinks/splunk"
	"github.com/stripe/veneur/ssf"
)

func jsonEndpoint(t *testing.T, ch chan<- hec.Event) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failed := false
		input := hec.Event{}
		j := json.NewDecoder(r.Body)
		j.DisallowUnknownFields()
		err := j.Decode(&input)
		if err != nil {
			t.Errorf("Decoding JSON: %v", err)
			failed = true
		}

		if failed {
			w.WriteHeader(400)
			w.Write([]byte(`{"text": "Error processing event", "code": 90}`))
		} else {
			w.Write([]byte(`{"text":"Success","code":0}`))
			ch <- input
		}
	})
}

func TestSpanIngest(t *testing.T) {
	logger := logrus.StandardLogger()

	ch := make(chan hec.Event, 1)
	ts := httptest.NewServer(jsonEndpoint(t, ch))
	defer ts.Close()
	sink, err := splunk.NewSplunkSpanSink(ts.URL, "00000000-0000-0000-0000-000000000000",
		"test-host", "", logger, time.Duration(0))
	require.NoError(t, err)

	start := time.Unix(100000, 1000000)
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		ParentId:       4,
		Id:             5,
		TraceId:        6,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Service:        "test-srv",
		Name:           "test-span",
		Indicator:      true,
		Error:          true,
		Tags: map[string]string{
			"farts": "mandatory",
		},
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
		},
	}
	err = sink.Ingest(span)
	require.NoError(t, err)
	event := <-ch

	fakeEvent := hec.Event{}
	fakeEvent.SetTime(start)

	assert.Equal(t, *fakeEvent.Time, *event.Time)
	assert.Equal(t, "test-srv", *event.SourceType)
	assert.Equal(t, "test-host", *event.Host)

	spanB, err := json.Marshal(event.Event)
	require.NoError(t, err)
	output := splunk.SerializedSSF{}
	err = json.Unmarshal(spanB, &output)

	assert.Equal(t, span.StartTimestamp, output.StartTimestamp.UnixNano())
	assert.Equal(t, span.EndTimestamp, output.EndTimestamp.UnixNano())
	assert.Equal(t, strconv.FormatInt(span.Id, 10), output.Id)
	assert.Equal(t, strconv.FormatInt(span.ParentId, 10), output.ParentId)
	assert.Equal(t, strconv.FormatInt(span.TraceId, 10), output.TraceId)
	assert.Equal(t, "test-span", output.Name)
	assert.Equal(t, map[string]string{"farts": "mandatory"}, output.Tags)
	assert.Equal(t, true, output.Indicator)
	assert.Equal(t, true, output.Error)
}
