package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
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
	badCall = false
)

type fakeClient struct {
	Tags []string
}

func (c *fakeClient) Gauge(name string, value float64, tags []string, rate float64) error {
	calledFunctions["gauge"] = true
	if badCall {
		return errors.New("error sending metric")
	}
	return nil
}
func (c *fakeClient) Count(name string, value int64, tags []string, rate float64) error {
	calledFunctions["count"] = true
	if badCall {
		return errors.New("error sending metric")
	}
	return nil
}
func (c *fakeClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	calledFunctions["timing"] = true
	if badCall {
		return errors.New("error sending metric")
	}
	return nil
}
func (c *fakeClient) TimeInMilliseconds(name string, value float64, tags []string, rate float64) error {
	calledFunctions["timeinms"] = true
	if badCall {
		return errors.New("error sending metric")
	}
	return nil
}

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel) // turns off logging in veneur-emit
	os.Exit(m.Run())
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

func TestBadCalls(t *testing.T) {
	client := &fakeClient{}
	badCall = true
	var err error

	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["gauge"] = true
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["gauge"] {
		t.Error("Did not detect error")
	}

	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["timing"] = true
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["timing"] {
		t.Error("Did not detect error")
	}

	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["timeinms"] = true
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["timeinms"] {
		t.Error("Did not detect error")
	}

	resetMap(testFlag)
	resetMap(calledFunctions)
	testFlag["count"] = true
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["count"] {
		t.Error("Did not detect error")
	}
}

func TestHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	testHostport := "host:port"
	addr, err := addr(testFlag, nil, &testHostport)
	if addr != testHostport || err != nil {
		t.Error("Did not return hostport.")
	}
}

func TestInvalidHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	testHostport := "hostport"
	addr, err := addr(testFlag, nil, &testHostport)
	if addr != "" || err == nil {
		t.Error("Did not check for valid hostport flag.")
	}
}

func TestEmptyHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	testHostport := ""
	addr, err := addr(testFlag, nil, &testHostport)
	if addr != "" || err == nil {
		t.Error("Did not check for valid hostport.")
	}
}

func TestNilHostport(t *testing.T) {
	resetMap(testFlag)
	testFlag["hostport"] = true
	addr, err := addr(testFlag, nil, nil)
	if addr != "" || err == nil {
		t.Error("Did not check for valid hostport.")
	}
}

func TestConfig(t *testing.T) {
	resetMap(testFlag)
	fakeConfig := &veneur.Config{}
	fakeConfig.UdpAddress = "testudp"
	testFlag["f"] = true
	addr, err := addr(testFlag, fakeConfig, nil)
	if addr != "testudp" || err != nil {
		t.Error("Did not use config file for hostname and port.")
	}
}

func TestNoAddr(t *testing.T) {
	resetMap(testFlag)
	addr, err := addr(testFlag, nil, nil)
	if addr != "" || err == nil {
		t.Error("Returned non-empty address with no flags.")
	}
}

func TestTags(t *testing.T) {
	testTag := "tag1,tag2,tag3"
	expectedOutput := []string{"tag1", "tag2", "tag3"}
	output := tags(testTag)
	if len(expectedOutput) != len(output) {
		t.Error("Did not return correct tags array.")
	}
	for i := 0; i < len(output); i++ {
		if expectedOutput[i] != output[i] {
			t.Error("Did not return correct tags array.")
		}
	}
}

func TestFlags(t *testing.T) {
	os.Args = append(os.Args, "-name='testname'")
	outputFlags := flags()
	if !outputFlags["name"] {
		t.Error("Did not properly parse flags.")
	}
}

func resetMap(m map[string]bool) {
	for key := range m {
		m[key] = false
	}
}
