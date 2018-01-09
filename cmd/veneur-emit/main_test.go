package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
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

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.WarnLevel) // turns off logging in veneur-emit
	os.Exit(m.Run())
}

func TestTimeCommand(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		command := []string{"true"}
		st, start, ended, err := timeCommand(&ssf.SSFSpan{}, command)

		assert.NoError(t, err, "timeCommand had an error")
		assert.NotZero(t, start)
		assert.NotZero(t, ended)
		assert.Zero(t, st)
	})

	t.Run("badCall", func(t *testing.T) {
		command := []string{"false"}
		st, _, _, err := timeCommand(&ssf.SSFSpan{}, command)
		assert.Error(t, err, "timeCommand did not throw error.")
		assert.NotZero(t, st)
	})
}

func TestGauge(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("3")
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 1)
	assert.Equal(t, ssf.SSFSample_GAUGE, span.Metrics[0].Metric)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
	assert.Equal(t, float32(3.0), span.Metrics[0].Value)
}

func TestCount(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["count"] = newValue("3")
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 1)
	assert.Equal(t, ssf.SSFSample_COUNTER, span.Metrics[0].Metric)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
	assert.Equal(t, float32(3.0), span.Metrics[0].Value)
}

func TestTiming(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["timing"] = newValue("3ms")
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 1)
	assert.Equal(t, ssf.SSFSample_HISTOGRAM, span.Metrics[0].Metric)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
	assert.Equal(t, "ms", span.Metrics[0].Unit)
	assert.Equal(t, float32(3.0), span.Metrics[0].Value)
}

func TestMultiple(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("3")
	testFlag["count"] = newValue("2")
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 2)
	assert.Equal(t, "testMetric", span.Metrics[1].Name)
	assert.Equal(t, "testMetric", span.Metrics[0].Name)
}

func TestNone(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "testMetric", "")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 0)
}

func TestBadCalls(t *testing.T) {
	var err error
	testFlag := make(map[string]flag.Value)
	testFlag["gauge"] = newValue("notanumber")
	span := &ssf.SSFSpan{}
	_, err = createMetric(span, testFlag, "testBadMetric", "")
	assert.Error(t, err)

	testFlag = make(map[string]flag.Value)
	testFlag["timing"] = newValue("40megayears")
	_, err = createMetric(span, testFlag, "testBadMetric", "")
	assert.Error(t, err)

	testFlag = make(map[string]flag.Value)
	testFlag["count"] = newValue("four")
	_, err = createMetric(span, testFlag, "testBadMetric", "")
	assert.Error(t, err)
}

func TestHostport(t *testing.T) {
	testHostport := "127.0.0.1:8200"
	addr, netAddr, err := destination(&testHostport, false)
	assert.NoError(t, err)
	assert.Equal(t, "udp://127.0.0.1:8200", addr)
	assert.Equal(t, "127.0.0.1:8200", netAddr.String())
	assert.Equal(t, "udp", netAddr.Network())
}

func TestHostportAsURL(t *testing.T) {
	testHostport := "tcp://127.0.0.1:8200"
	addr, netAddr, err := destination(&testHostport, false)
	assert.NoError(t, err)
	assert.Equal(t, "tcp://127.0.0.1:8200", addr)
	assert.Equal(t, "tcp", netAddr.Network())
	assert.Equal(t, "127.0.0.1:8200", netAddr.String())
}

func TestNilHostport(t *testing.T) {
	addr, netAddr, err := destination(nil, false)
	assert.Empty(t, addr)
	assert.Nil(t, netAddr)
	assert.Error(t, err)
}

func TestTags(t *testing.T) {
	testTag := "tag1,tag2,tag3,,tag4:value"
	expectedOutput := map[string]string{"tag1": "", "tag2": "", "tag3": "", "tag4": "value"}
	output := ssfTags(testTag)
	assert.Equal(t, expectedOutput, output)
}

func TestFlags(t *testing.T) {
	os.Args = append(os.Args, "-name='testname'")
	outputFlags := flags()
	if outputFlags["name"] == nil {
		t.Error("Did not properly parse flags.")
	}
}

func TestCreateMetrics(t *testing.T) {
	testFlag := make(map[string]flag.Value)

	testFlag["gauge"] = newValue("3.14")
	testFlag["count"] = newValue("2")
	span := &ssf.SSFSpan{}
	_, err := createMetric(span, testFlag, "test.metric", "tag1:value1")
	assert.NoError(t, err)
	assert.Len(t, span.Metrics, 2)
	for _, metric := range span.Metrics {
		assert.Equal(t, "test.metric", metric.Name)
		assert.Equal(t, "value1", metric.Tags["tag1"])
	}
}

type testBackend struct {
	t      *testing.T
	ch     chan *ssf.SSFSpan
	errors chan error
}

func (tb *testBackend) Close() error {
	tb.t.Logf("Closing backend")
	close(tb.ch)
	close(tb.errors)
	return nil
}

func (tb *testBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	tb.t.Logf("Sending span")
	tb.ch <- span
	return <-tb.errors
}

func (tb *testBackend) FlushSync(ctx context.Context) error {
	return nil
}

func TestSetupSpanWithTracing(t *testing.T) {
	span, err := setupSpan(proto.Int64(1), proto.Int64(2), "oink", "hi:there")
	if assert.NoError(t, err) {
		assert.NotZero(t, span.Id)
		assert.Equal(t, int64(1), span.TraceId)
		assert.Equal(t, int64(2), span.ParentId)
		assert.Equal(t, "oink", span.Name)
		assert.Equal(t, 1, len(span.Tags))
	}
}

func TestSetupSpanWithoutTracing(t *testing.T) {
	span, err := setupSpan(nil, nil, "oink", "hi:there")
	if assert.NoError(t, err) {
		assert.Zero(t, span.Id)
		assert.Zero(t, span.TraceId)
		assert.Zero(t, span.ParentId)
		assert.Equal(t, "", span.Name)
		assert.Equal(t, 0, len(span.Tags))
	}
}

func TestSendSpan(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	dataWritten = []byte{}
	span := &ssf.SSFSpan{}
	_, _ = createMetric(span, testFlag, "test.metric", "tag1:value1")

	ch := make(chan *ssf.SSFSpan, 1)
	errors := make(chan error, 1)
	be := &testBackend{t: t, ch: ch, errors: errors}
	defer be.Close()
	cl, err := trace.NewBackendClient(be)
	require.NoError(t, err)

	be.errors <- nil
	err = sendSSF(cl, span)
	assert.NoError(t, err)

	spanOut := <-ch
	assert.Equal(t, span, spanOut)
}

func TestBadSendSpan(t *testing.T) {
	testFlag := make(map[string]flag.Value)
	dataWritten = []byte{}

	span := &ssf.SSFSpan{}
	_, _ = createMetric(span, testFlag, "test.metric", "tag1:value1")

	ch := make(chan *ssf.SSFSpan, 1)
	errors := make(chan error, 1)
	be := &testBackend{t: t, ch: ch, errors: errors}
	defer be.Close()
	cl, err := trace.NewBackendClient(be)
	require.NoError(t, err)

	errors <- fmt.Errorf("a potential error in sending")
	err = sendSSF(cl, span)
	assert.Error(t, err)
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
