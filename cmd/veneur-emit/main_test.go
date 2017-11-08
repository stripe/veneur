package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/ssf"
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

func newValue(s string) *fakeValue {
	return &fakeValue{value: s}
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

func TestTimeCommand(t *testing.T) {
	client := &fakeClient{}
	resetMap(calledFunctions)
	testFlag := make(map[string]flag.Value)
	testFlag["command"] = newValue("/bin/true")
	testFlag["name"] = newValue("test.timing")
	command := []string{"echo", "hello"}
	name := "test.timing"

	t.Run("basic", func(t *testing.T) {
		st, _, _, err := timeCommand(command)

		assert.NoError(t, err, "timeCommand had an error")
		assert.True(t, calledFunctions["timing"], "timeCommand did not call Timing")
		assert.Zero(t, st)
	})

	t.Run("badCall", func(t *testing.T) {
		badCall = true
		defer func() {
			badCall = false
		}()
		st, _, _, err := timeCommand(command)
		assert.Error(t, err, "timeCommand did not throw error.")
		assert.Zero(t, st)
	})

	t.Run("badCall", func(t *testing.T) {
		command = []string{"/bin/false"}
		st, _, _, err := timeCommand(command)
		assert.NotZero(t, st)
		assert.NoError(t, err)
	})
}

func TestGauge(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("3")
	span, _, err := createMetric(testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 1)
	assert.Equal(t, ssf.SSFSample_GAUGE, span.Metrics[0].Metric)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
	assert.Equal(t, 3.0, span.Metrics[0].Value)
}

func TestCount(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["count"] = newValue("3")
	span, _, err := createMetric(testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 1)
	assert.Equal(t, ssf.SSFSample_COUNTER, span.Metrics[0].Metric)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
	assert.Equal(t, 3.0, span.Metrics[0].Value)
}

func TestTiming(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["timing"] = newValue("3ns")
	span, _, err := createMetric(testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 0)
	assert.Equal(t, "testMetric", span.Name)
	assert.Equal(t, 3*time.Nanosecond, span.StartTimestamp)
	assert.Equal(t, 3*time.Nanosecond, span.EndTimestamp)
}

func TestMultiple(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("3")
	testFlag["count"] = newValue("2")
	span, _, err := createMetric(testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 2)
	assert.Equal(t, "testMetric", span.Metrics[1].Name)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
}

func TestNone(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	span, _, err := createMetric(testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 0)
}

func TestBadCalls(t *testing.T) {
	var err error
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("notanumber")
	_, _, err = createMetric(testFlag, "testBadMetric", "")
	assert.Error(t, err)

	testFlag = make(map[string]flag.Value)
	testFlag["timing"] = newValue("40megayears")
	_, _, err = createMetric(testFlag, "testBadMetric", "")
	assert.Error(t, err)

	testFlag = make(map[string]flag.Value)
	testFlag["count"] = newValue("four")
	_, _, err = createMetric(testFlag, "testBadMetric", "")
	assert.Error(t, err)
}

func TestHostport(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testHostport := "127.0.0.1:8200"
	testFlag["hostport"] = newValue(testHostport)
	addr, network, err := destination(testFlag, &testHostport, false)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8200", addr)
	assert.Equal(t, "udp", network)
}

func TestHostportAsURL(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testHostport := "tcp://127.0.0.1:8200"
	testFlag["hostport"] = newValue(testHostport)
	addr, network, err := destination(testFlag, &testHostport, false)
	assert.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8200", addr)
	assert.Equal(t, "tcp", network)
}

func TestNilHostport(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	addr, network, err := destination(testFlag, nil, false)
	if addr != "" || network != "udp" || err == nil {
		t.Error("Did not check for valid hostport.")
	}
}

func TestNoAddr(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	addr, network, err := destination(testFlag, nil, false)
	if addr != "" || network != "udp" || err == nil {
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

	testFlag["gauge"] = newValue("3.14")
	testFlag["count"] = newValue("2")
	span, _, err := createMetric(testFlag, "test.metric", "tag1:value1")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 2)
	for _, metric := range span.Metrics {
		assert.Equal(t, "test.metric", metric.Name)
		assert.Equal(t, "value1", metric.Tags["tag1"])
	}
}

func TestSendSpan(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	dataWritten = []byte{}
	conn := &fakeConn{}
	span, _, _ := createMetric(testFlag, "test.metric", "tag1:value1")
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
	span, _, _ := createMetric(testFlag, "test.metric", "tag1:value1")
	err := sendSpan(conn, span)
	assert.NotNil(t, err, "Did not return err!")
}

func TestBuildEventPacketError(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	_, err := buildEventPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error.")

	testFlag["e_title"] = newValue("myBadTitle")
	_, err = buildEventPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error. (title, no text)")

	testFlag = make(map[string]flag.Value)
	testFlag["e_text"] = newValue("myBadText")
	_, err = buildEventPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error. (text, no title)")
}

func TestBuildEventPacket(t *testing.T) {
	myTitle := "myTitle"
	myText := "myText"

	testFlag := make(map[string]flag.Value)

	t.Run("simple", func(t *testing.T) {
		testFlag["e_title"] = newValue(myTitle)
		testFlag["e_text"] = newValue(myText)

		pkt, err := buildEventPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		expected := fmt.Sprintf("_e{%d,%d}:%s|%s", len(myTitle), len(myText), myTitle, myText)
		assert.Equal(t, pkt.String(), expected, "Bad event packet.")
	})

	t.Run("basic", func(t *testing.T) {
		testFlag["e_title"] = newValue("An exception occurred")
		testFlag["e_text"] = newValue("Cannot parse CSV file from 10.0.0.17")
		testFlag["e_alert_type"] = newValue("warning")
		tags := "tag1:value1"
		testFlag["e_event_tags"] = newValue(tags)

		pkt, err := buildEventPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		suffix := fmt.Sprintf("#%s", tags)
		assert.True(t, strings.HasSuffix(pkt.String(), suffix), "Tags not at end of event packet.")

		assert.True(t, strings.HasPrefix(pkt.String(), "_e"))
	})

	t.Run("newline", func(t *testing.T) {
		testFlag["e_title"] = newValue("An exception occurred")
		testFlag["e_text"] = newValue("Cannot parse JSON request:\n{'foo': 'bar'}")
		testFlag["e_priority"] = newValue("low")
		testFlag["e_event_tags"] = newValue("err_type:bad_request")

		pkt, err := buildEventPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		assert.Contains(t, pkt.String(), "\\n", "Did not parse newline.")
	})

	t.Run("all", func(t *testing.T) {
		testFlag["e_title"] = newValue("An exception occurred")
		testFlag["e_text"] = newValue("Cannot parse CSV file from 10.0.0.17")
		testFlag["e_time"] = newValue("1501002564")
		testFlag["e_hostname"] = newValue("tester.host")
		testFlag["e_aggr_key"] = newValue("testEvents")
		testFlag["e_priority"] = newValue("low")
		testFlag["e_source_type"] = newValue("tester")
		testFlag["e_alert_type"] = newValue("warning")
		testFlag["e_event_tags"] = newValue("err_type:bad")

		pkt, err := buildEventPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		data := pkt.String()
		assert.True(t, strings.HasPrefix(data, "_e"), "Missing _e at beginning.")
		assert.Contains(t, data, "|d:1501002564", "Missing timestamp.")
		assert.Contains(t, data, "|h:tester.host", "Missing hostname.")
		assert.Contains(t, data, "|k:testEvents", "Missing aggregation key.")
		assert.Contains(t, data, "|p:low", "Missing priority.")
		assert.Contains(t, data, "|s:tester", "Missing source type.")
		assert.Contains(t, data, "|t:warning", "Missing alert type.")
		assert.True(t, strings.HasSuffix(data, "|#err_type:bad"), "Missing tags at end of packet.")
	})
}

func TestBuilldSCPacketError(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	_, err := buildSCPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error.")

	testFlag["sc_name"] = newValue("myBadName")
	_, err = buildSCPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error. (title, no text)")

	testFlag = make(map[string]flag.Value)
	testFlag["sc_status"] = newValue("myBadStatus")
	_, err = buildSCPacket(testFlag)
	assert.NotNil(t, err, "Did not catch error. (text, no title)")
}

func TestBuilldSCPacket(t *testing.T) {
	myName := "myName"
	myStatus := "1" // Corresponds to WARNING

	testFlag := make(map[string]flag.Value)

	t.Run("simple", func(t *testing.T) {
		testFlag["sc_name"] = newValue(myName)
		testFlag["sc_status"] = newValue(myStatus)

		pkt, err := buildSCPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		expected := fmt.Sprintf("_sc|%s|%s", myName, myStatus)
		assert.Equal(t, pkt.String(), expected, "Bad service check packet.")
	})

	t.Run("basic", func(t *testing.T) {
		msg := "Redis connection timed out after 10s"
		testFlag["sc_name"] = newValue("Redis connection")
		testFlag["sc_status"] = newValue("2")
		testFlag["sc_tags"] = newValue("redis_instance:10.0.0.16:6379")
		testFlag["sc_msg"] = newValue(msg)

		pkt, err := buildSCPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		expected := "_sc|Redis connection|2|#redis_instance:10.0.0.16:6379|m:Redis connection timed out after 10s"
		assert.Equal(t, expected, pkt.String(), "Bad service check packet.")

		assert.True(t, strings.HasSuffix(pkt.String(), msg), "Message not at the end of the packet.")
	})

	t.Run("all", func(t *testing.T) {
		msg := "Redis connection timed out after 10s"
		testFlag["sc_name"] = newValue("Redis connection")
		testFlag["sc_status"] = newValue("2")
		testFlag["sc_time"] = newValue("1501002564")
		testFlag["sc_hostname"] = newValue("tester.host")
		testFlag["sc_tags"] = newValue("redis_instance:10.0.0.16:6379")
		testFlag["sc_msg"] = newValue(msg)

		pkt, err := buildSCPacket(testFlag)
		assert.Nil(t, err, "Returned non-nil error.")

		data := pkt.String()
		assert.True(t, strings.HasPrefix(data, "_sc"), "Missing _sc at beginning.")
		assert.Contains(t, data, "|d:1501002564", "Missing timestamp.")
		assert.Contains(t, data, "|h:tester.host", "Missing hostname.")
		assert.Contains(t, data, "|#redis_instance:10.0.0.16:6379", "Missing service check tags.")
		suffix := fmt.Sprintf("|m:%s", msg)
		assert.True(t, strings.HasSuffix(data, suffix), "Missing message at end of packet.")
	})
}

func resetMap(m map[string]bool) {
	for key := range m {
		m[key] = false
	}
}
