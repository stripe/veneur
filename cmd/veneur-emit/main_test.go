package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur"
)

var (
	testTags []string
	testFlag = map[string]bool{
		"gauge":  false,
		"count":  false,
		"timing": false,
	}
	calledFunctions = map[string]bool{
		"gauge":  false,
		"count":  false,
		"timing": false,
	}
	badCall     = false
	dataWritten []byte
)

type fakeClient struct {
	Tags []string
}

type fakeValue struct {
	value string
}

func (v *fakeValue) String() string {
	return v.value
}

func (v *fakeValue) Set(s string) error {
	v.value = s
	return nil
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

type fakeConn struct{}

func (c *fakeConn) Write(data []byte) (int, error) {
	dataWritten = data
	return len(data), nil
}

type badConn struct{}

func (c *badConn) Write(data []byte) (int, error) {
	return 0, errors.New("bad write")
}

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel) // turns off logging in veneur-emit
	os.Exit(m.Run())
}

func TestGauge(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = &fakeValue{value: "3"}
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["gauge"] {
		t.Error("Did not send 'gauge' metric.")
	}
}

func TestCount(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["count"] = &fakeValue{value: "3"}
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["count"] {
		t.Error("Did not send 'count' metric.")
	}
}

func TestTiming(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["timing"] = &fakeValue{value: "3ns"}
	err := sendMetrics(client, testFlag, "testMetric", testTags)
	if err != nil || !calledFunctions["timing"] {
		t.Error("Did not send 'timing' metric.")
	}
}

func TestMultiple(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = &fakeValue{value: "3"}
	testFlag["count"] = &fakeValue{value: "2"}
	err := sendMetrics(client, testFlag, "testMetrics", testTags)
	if err != nil || (!calledFunctions["count"] && !calledFunctions["gauge"]) {
		t.Error("Did not send multiple metrics.")
	}
}

func TestNone(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	err := sendMetrics(client, testFlag, "testNoMetric", testTags)
	if err != nil {
		t.Error("Error while sending no metrics.")
	}
}

func TestBadCalls(t *testing.T) {
	client := &fakeClient{}
	badCall = true
	var err error
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = &fakeValue{value: "3"}
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["gauge"] {
		t.Error("Did not detect error")
	}

	resetMap(calledFunctions)
	testFlag = make(map[string]flag.Value)
	testFlag["timing"] = &fakeValue{value: "3ns"}
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["timing"] {
		t.Error("Did not detect error")
	}

	resetMap(calledFunctions)
	testFlag = make(map[string]flag.Value)
	testFlag["count"] = &fakeValue{value: "3"}
	err = sendMetrics(client, testFlag, "testBadMetric", testTags)
	if err == nil || err.Error() != "error sending metric" || !calledFunctions["count"] {
		t.Error("Did not detect error")
	}
}

func TestHostport(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testHostport := "host:port"
	testFlag["hostport"] = &fakeValue{value: testHostport}
	addr, err := addr(testFlag, nil, &testHostport)
	if addr != testHostport || err != nil {
		t.Error("Did not return hostport.")
	}
}

func TestNilHostport(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	addr, err := addr(testFlag, nil, nil)
	if addr != "" || err == nil {
		t.Error("Did not check for valid hostport.")
	}
}

func TestConfig(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	fakeConfig := &veneur.Config{}
	fakeConfig.UdpAddress = "testudp"
	testFlag["f"] = &fakeValue{value: "/pay/conf/veneur.yaml"}
	addr, err := addr(testFlag, fakeConfig, nil)
	if addr != "testudp" || err != nil {
		t.Error("Did not use config file for hostname and port.")
	}
}

func TestNoAddr(t *testing.T) {
	testFlag := make(map[string]flag.Value)
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
	if outputFlags["name"] == nil {
		t.Error("Did not properly parse flags.")
	}
}

func TestBareMetrics(t *testing.T) {
	metric := bareMetric("test_name", "tag1:value1,tag2:value2")
	if metric.Name != "test_name" {
		t.Error("Bare metric does not have correct name.")
	}
	testTag1 := metric.Tags["tag1"]
	testTag2 := metric.Tags["tag2"]
	if testTag1 != "value1" || testTag2 != "value2" {
		t.Error("Bare metric does not have correct tags.")
	}
	if metric.Value != 0 {
		t.Error("Bare metric is not bare.")
	}
}

func TestCreateMetrics(t *testing.T) {
	testFlag := make(map[string]flag.Value)

	testFlag["gauge"] = &fakeValue{value: "3.14"}
	testFlag["count"] = &fakeValue{value: "2"}
	span, _ := createMetrics(testFlag, "test.metric", "tag1:value1")
	if len(span.Metrics) != 2 {
		t.Error("Not reporting right number of metrics.")
	}
	for _, metric := range span.Metrics {
		if metric.Name != "test.metric" {
			t.Error("Metric name not correct.")
		}
		if metric.Tags["tag1"] != "value1" {
			t.Error("Metric tags not correct.")
		}
	}
}

func TestSendSpan(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	dataWritten = []byte{}
	conn := &fakeConn{}
	span, _ := createMetrics(testFlag, "test.metric", "tag1:value1")
	sendSpan(conn, span)
	orig, _ := proto.Marshal(span)
	if !bytes.Equal(orig, dataWritten) {
		t.Error("Did not send correct data.")
	}
}

func TestBadSendSpan(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	dataWritten = []byte{}
	conn := &badConn{}
	span, _ := createMetrics(testFlag, "test.metric", "tag1:value1")
	err := sendSpan(conn, span)
	assert.NotNil(t, err, "Did not return err!")
}

func TestBuildEventPacketError(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	pkt, err := buildEventPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error.")
}

func resetMap(m map[string]bool) {
	for key := range m {
		m[key] = false
	}
}
