package main

import (
	"testing"
	"time"
)

var (
	testTags []string
	testFlag = map[string]bool{
		"gauge":    false,
		"count":    false,
		"timing":   false,
		"timeinms": false,
	}
	calledFunctions = map[string]bool{
		"gauge":    false,
		"count":    false,
		"timing":   false,
		"timeinms": false,
	}
)

type fakeClient struct {
	// Namespace to prepend to all statsd calls
	// Namespace string -> not used
	// Tags are global tags to be added to every statsd call
	Tags []string
	// BufferLength is the length of the buffer in commands.
	/*
		bufferLength int
		flushTime    time.Duration
		commands     []string
		buffer       bytes.Buffer
		stop         bool
		sync.Mutex
	*/
}

func (c *fakeClient) Gauge(name string, value float64, tags []string, rate float64) error {
	calledFunctions["gauge"] = true
	return nil
}
func (c *fakeClient) Count(name string, value int64, tags []string, rate float64) error {
	calledFunctions["count"] = true
	return nil
}
func (c *fakeClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	calledFunctions["timing"] = true
	return nil
}
func (c *fakeClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	calledFunctions["timeinms"] = true
	return nil
}

func TestGauge(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["gauge"] = true
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["gauge"] {
		t.Error("Did not send 'gauge' metric.")
	}
}

func TestCount(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["count"] = true
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["count"] {
		t.Error("Did not send 'count' metric.")
	}
}

func TestTiming(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["timing"] = true
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["timing"] {
		t.Error("Did not send 'timing' metric.")
	}
}

func TestTimeInMilliseconds(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["timeinms"] = true
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["timeinms"] {
		t.Error("Did not send 'timeinms' metric.")
	}
}

func TestMultiple(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["count"] = true
	testFlag["gauge"] = true
	err := sendMetrics(client, testFlag, "testMetrics", testTags)
	if err != nil || (!calledFunctions["count"] && !calledFunctions["gauge"]) {
		t.Error("Did not send multiple metrics.")
	}
}

func TestNone(t *testing.T) {
	client := &fakeClient{}
	resetMap(testFlag)
	resetMap(calledFunctions)
	err := sendMetrics(client, testFlag, "testNoMetric", testTags)
	if err != nil {
		t.Error("Error while sending no metrics.")
	}
}

func TestHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	testHostport := "host:port"
	addr := getAddr(testFlag, nil, &testHostport)
	if addr != testHostport {
		t.Error("Did not return hostport.")
	}
}

func TestInvalidHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	testHostport := "hostport"
	addr := getAddr(testFlag, nil, &testHostport)
	if addr != "" {
		t.Error("Did not check for valid hostport flag.")
	}
}

func TestNoAddr(t *testing.T) {
	resetMap(testFlag)
	addr := getAddr(testFlag, nil, nil)
	if addr != "" {
		t.Error("Returned non-empty address with no flags.")
	}
}

func TestGetTags(t *testing.T) {
	testTag := "tag1,tag2,tag3"
	expectedOutput := []string{"tag1", "tag2", "tag3"}
	output := getTags(testTag)
	if len(expectedOutput) != len(output) {
		t.Error("Did not return correct tags array.")
	}
	for i := 0; i < len(output); i++ {
		if expectedOutput[i] != output[i] {
			t.Error("Did not return correct tags array.")
		}
	}
	// for idx, elem := range expectedOutput
}

func resetMap(m map[string]bool) {
	for key := range m {
		m[key] = false
	}
}
