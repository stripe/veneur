package xray

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/ssf"
)

func TestConstructor(t *testing.T) {
	logger := logrus.StandardLogger()

	sink, err := NewXRaySpanSink("127.0.0.1:2000", 100, map[string]string{"foo": "bar"}, nil, logger)
	assert.NoError(t, err)
	assert.Equal(t, "xray", sink.Name())
	assert.Equal(t, "bar", sink.commonTags["foo"])
	assert.Equal(t, "127.0.0.1:2000", sink.daemonAddr)
}

func TestIngestSpans(t *testing.T) {

	// Load up a fixture to compare the output to what we get over UDP
	reader, err := os.Open(filepath.Join("testdata", "xray_segment.json"))
	assert.NoError(t, err)
	defer reader.Close()
	fixtureSegment, err := ioutil.ReadAll(reader)
	assert.NoError(t, err)

	// Don't use a port so we get one auto-assigned
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	sock, _ := net.ListenUDP("udp", udpAddr)
	defer sock.Close()
	// Grab the port we got assigned so we can use it.
	port := sock.LocalAddr().(*net.UDPAddr).Port

	segments := make(chan string)

	buf := make([]byte, 1024)
	go func() {
		for {
			n, _, serr := sock.ReadFromUDP(buf)
			segments <- string(buf[0:n])
			if serr != nil {
				assert.NoError(t, serr)
			}
		}
	}()

	sink, err := NewXRaySpanSink(fmt.Sprintf("127.0.0.1:%d", port), 100, map[string]string{"foo": "bar"}, []string{"baz", "mind"}, logrus.New())
	assert.NoError(t, err)
	err = sink.Start(nil)
	assert.NoError(t, err)

	// Because xray uses the timestamp as part of the trace id, this must remain
	// fixed for the fixture comparison to work!
	start := time.Unix(1518279577, 0)
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        4601851300195147788,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz":      "qux",
			"mind":     "crystal",
			"feelings": "magenta",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	err = sink.Ingest(testSpan)
	assert.NoError(t, err)

	select {
	case seg := <-segments:
		assert.Equal(t, strings.TrimSpace(string(fixtureSegment)), seg)
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Did not receive segment from xray ingest")
	}

	assert.Equal(t, int64(1), sink.spansHandled)
	sink.Flush()
	assert.Equal(t, int64(0), sink.spansHandled)
}

func TestIngestSpansRootStartTimestamp(t *testing.T) {

	// Load up a fixture to compare the output to what we get over UDP
	reader, err := os.Open(filepath.Join("testdata", "xray_segment_root_start.json"))
	assert.NoError(t, err)
	defer reader.Close()
	fixtureSegment, err := ioutil.ReadAll(reader)
	assert.NoError(t, err)

	// Don't use a port so we get one auto-assigned
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	sock, _ := net.ListenUDP("udp", udpAddr)
	defer sock.Close()
	// Grab the port we got assigned so we can use it.
	port := sock.LocalAddr().(*net.UDPAddr).Port

	segments := make(chan string)

	buf := make([]byte, 1024)
	go func() {
		for {
			n, _, serr := sock.ReadFromUDP(buf)
			segments <- string(buf[0:n])
			if serr != nil {
				assert.NoError(t, serr)
			}
		}
	}()

	sink, err := NewXRaySpanSink(fmt.Sprintf("127.0.0.1:%d", port), 100, map[string]string{"foo": "bar"}, []string{"baz", "mind"}, logrus.New())
	assert.NoError(t, err)
	err = sink.Start(nil)
	assert.NoError(t, err)

	// Because xray uses the timestamp as part of the trace id, this must remain
	// fixed for the fixture comparison to work!
	start := time.Unix(1518279577, 0)
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        4601851300195147788,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz":      "qux",
			"mind":     "crystal",
			"feelings": "magenta",
		},
		Indicator:          false,
		Name:               "farting farty farts",
		RootStartTimestamp: int64(1e9),
	}
	err = sink.Ingest(testSpan)
	assert.NoError(t, err)

	select {
	case seg := <-segments:
		assert.Equal(t, strings.TrimSpace(string(fixtureSegment)), seg)
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Did not receive segment from xray ingest")
	}

	assert.Equal(t, int64(1), sink.spansHandled)
	sink.Flush()
	assert.Equal(t, int64(0), sink.spansHandled)
}

func TestIngestSpansHttpHeadersInTags(t *testing.T) {

	// Load up a fixture to compare the output to what we get over UDP
	reader, err := os.Open(filepath.Join("testdata", "xray_segment_root_start_http_fields.json"))
	assert.NoError(t, err)
	defer reader.Close()
	fixtureSegment, err := ioutil.ReadAll(reader)
	assert.NoError(t, err)

	// Don't use a port so we get one auto-assigned
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	sock, _ := net.ListenUDP("udp", udpAddr)
	defer sock.Close()
	// Grab the port we got assigned so we can use it.
	port := sock.LocalAddr().(*net.UDPAddr).Port

	segments := make(chan string)

	buf := make([]byte, 1024)
	go func() {
		for {
			n, _, serr := sock.ReadFromUDP(buf)
			segments <- string(buf[0:n])
			if serr != nil {
				assert.NoError(t, serr)
			}
		}
	}()

	sink, err := NewXRaySpanSink(fmt.Sprintf("127.0.0.1:%d", port), 100, map[string]string{"foo": "bar"}, []string{"baz", "mind"}, logrus.New())
	assert.NoError(t, err)
	err = sink.Start(nil)
	assert.NoError(t, err)

	// Because xray uses the timestamp as part of the trace id, this must remain
	// fixed for the fixture comparison to work!
	start := time.Unix(1518279577, 0)
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        4601851300195147788,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz":              "qux",
			"mind":             "crystal",
			"feelings":         "magenta",
			"http.url":         "https://domain.name/path1/path2",
			"http.method":      "POST",
			"http.status_code": "200",
		},
		Indicator:          false,
		Name:               "farting farty farts",
		RootStartTimestamp: int64(1e9),
	}
	err = sink.Ingest(testSpan)
	assert.NoError(t, err)

	select {
	case seg := <-segments:
		assert.Equal(t, strings.TrimSpace(string(fixtureSegment)), seg)
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Did not receive segment from xray ingest")
	}

	assert.Equal(t, int64(1), sink.spansHandled)
	sink.Flush()
	assert.Equal(t, int64(0), sink.spansHandled)
}

func TestSampleSpans(t *testing.T) {

	// Load up a fixture to compare the output to what we get over UDP
	reader, err := os.Open(filepath.Join("testdata", "xray_segment.json"))
	assert.NoError(t, err)
	defer reader.Close()
	fixtureSegment, err := ioutil.ReadAll(reader)
	assert.NoError(t, err)

	// Don't use a port so we get one auto-assigned
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	sock, _ := net.ListenUDP("udp", udpAddr)
	defer sock.Close()
	// Grab the port we got assigned so we can use it.
	port := sock.LocalAddr().(*net.UDPAddr).Port

	segments := make(chan string)

	buf := make([]byte, 1024)
	go func() {
		for {
			n, _, serr := sock.ReadFromUDP(buf)
			segments <- string(buf[0:n])
			if serr != nil {
				assert.NoError(t, serr)
			}
		}
	}()

	sink, err := NewXRaySpanSink(fmt.Sprintf("127.0.0.1:%d", port), 50, map[string]string{"foo": "bar"}, []string{"baz", "mind"}, logrus.New())
	assert.NoError(t, err)
	err = sink.Start(nil)
	assert.NoError(t, err)

	// Because xray uses the timestamp as part of the trace id, this must remain
	// fixed for the fixture comparison to work!
	start := time.Unix(1518279577, 0)
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        548547320537590250, // This one will NOT be sampled!
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	assert.NoError(t, sink.Ingest(testSpan))

	testSpan2 := &ssf.SSFSpan{
		TraceId:        548547320537590250, // This one will NOT be sampled!
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	assert.NoError(t, sink.Ingest(testSpan2))

	testSpan3 := &ssf.SSFSpan{
		TraceId:        4601851300195147788, // This one will be sampled!
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz":      "qux",
			"mind":     "crystal",
			"feelings": "magenta",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	assert.NoError(t, sink.Ingest(testSpan3))

	select {
	case seg := <-segments:
		assert.Equal(t, strings.TrimSpace(string(fixtureSegment)), seg)
	case <-time.After(1 * time.Second):
		assert.Fail(t, "Did not receive segment from xray ingest")
	}

	assert.Equal(t, int64(1), sink.spansHandled)
	sink.Flush()
	assert.Equal(t, int64(0), sink.spansHandled)
}

func TestCalculateTraceID(t *testing.T) {
	sink, err := NewXRaySpanSink(fmt.Sprintf("127.0.0.1:12345"), 50, map[string]string{"foo": "bar"}, []string{"baz", "mind"}, logrus.New())
	assert.NoError(t, err)

	start := time.Unix(1518279577, 0)
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        4601851300195147788,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags:           map[string]string{},
		Indicator:      false,
		Name:           "farting farty farts",
	}

	// TraceID in hex: 000000003fdd0f60394d200c
	// Span starttime in hex - 5A7F1B99
	// -> middle timestamp should be 5A7F200C
	assert.Equal(t, sink.CalculateTraceID(testSpan), "1-5a7f1b00-000000003fdd0f60394d200c")

	testSpan = &ssf.SSFSpan{
		TraceId:            4601851300195147788,
		ParentId:           1,
		Id:                 2,
		StartTimestamp:     int64(start.UnixNano()),
		EndTimestamp:       int64(end.UnixNano()),
		Error:              false,
		Service:            "farts-srv",
		Tags:               map[string]string{},
		Indicator:          false,
		Name:               "farting farty farts",
		RootStartTimestamp: int64(1e9),
	}

	assert.Equal(t, sink.CalculateTraceID(testSpan), "1-00000001-000000003fdd0f60394d200c")

}
