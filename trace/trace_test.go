package trace

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
)

const ε = .00002

func TestStartTrace(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	const expectedParent int64 = 0
	start := time.Now()
	trace := StartTrace(resource)
	end := time.Now()

	between := end.After(trace.Start) && trace.Start.After(start)

	assert.Equal(t, trace.TraceID, trace.SpanID)
	assert.Equal(t, trace.ParentID, expectedParent)
	assert.Equal(t, trace.Resource, resource)
	assert.True(t, between)
}

func testRecord(t *testing.T, trace *Trace, name string, tags map[string]string) (sample *ssf.SSFSpan, end time.Time) {
	// arbitrary
	const BufferSize = 1087152

	traceAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)
	serverConn, err := net.ListenUDP("udp", traceAddr)
	assert.NoError(t, err)
	defer serverConn.Close()

	err = serverConn.SetReadBuffer(BufferSize)
	assert.NoError(t, err)

	respChan := make(chan []byte)
	kill := make(chan struct{})

	go func() {
		buf := make([]byte, BufferSize)
		n, _, err := serverConn.ReadFrom(buf)
		assert.NoError(t, err)

		buf = buf[:n]
		respChan <- buf
	}()

	go func() {
		<-time.After(5 * time.Second)
		close(kill)
	}()

	client, err := NewClient(fmt.Sprintf("udp://%s", serverConn.LocalAddr().String()))
	require.NoError(t, err)
	defer client.Close()

	sentCh := make(chan error)
	trace.Sent = sentCh
	err = trace.ClientRecord(client, name, tags)
	if assert.NoError(t, err) {
		assert.NoError(t, <-sentCh)
		end = time.Now()

		select {
		case _ = <-kill:
			assert.Fail(t, "timed out waiting for socket read")
		case resp := <-respChan:
			// Because this is marshalled using protobuf,
			// we can't expect the representation to be immutable
			// and cannot test the marshalled payload directly
			sample = &ssf.SSFSpan{}
			err := proto.Unmarshal(resp, sample)
			assert.NoError(t, err)
		}
	}
	return
}

func TestRecord(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	const metricName = "veneur.trace.test"
	const serviceName = "veneur-test"
	Service = serviceName

	trace := StartTrace(resource)
	trace.Status = ssf.SSFSample_CRITICAL
	trace.error = true

	tags := map[string]string{
		"error.msg":   "an error occurred!",
		"error.type":  "type error interface",
		"error.stack": "insert\nlots\nof\nstuff",
		"resource":    resource,
		"name":        metricName,
	}

	sample, end := testRecord(t, trace, metricName, tags)

	timestamp := time.Unix(sample.StartTimestamp/1e9, 0)

	assert.Equal(t, trace.Start.Unix(), timestamp.Unix())

	duration := sample.EndTimestamp - sample.StartTimestamp

	// We don't know the exact duration, but we can assert on the interval
	assert.True(t, duration > 0, "Expected positive trace duration")
	upperBound := end.Sub(trace.Start).Nanoseconds()
	assert.True(t, duration < upperBound, "Expected trace duration (%d) to be less than upper bound %d", duration, upperBound)

	for _, metric := range sample.Metrics {
		assert.InEpsilon(t, metric.SampleRate, 0.1, ε)
	}

	assertTagEquals(t, sample, "resource", resource)
	assertTagEquals(t, sample, "name", metricName)
	assert.Equal(t, true, sample.Error)
	assert.Equal(t, serviceName, sample.Service)
	assert.Equal(t, tags, sample.Tags)
}

func TestRecordManualTime(t *testing.T) {
	trace := StartTrace("test-resource")
	end := time.Now()
	trace.End = end
	sample, _ := testRecord(t, trace, "test-metric", map[string]string{})
	assert.Equal(t, end.UnixNano(), sample.EndTimestamp)
}

func TestAttach(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	ctx := context.Background()

	parent := ctx.Value(traceKey)
	assert.Nil(t, parent, "Expected not to find parent in context before attaching")

	trace := StartTrace(resource)
	ctx2 := trace.Attach(ctx)

	parent = ctx2.Value(traceKey).(*Trace)
	assert.NotNil(t, parent, "Expected not to find parent in context before attaching")
}

func TestSpanFromContext(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	trace := StartTrace(resource)

	ctx := trace.Attach(context.Background())
	child := SpanFromContext(ctx)
	// Test the *grandchild* so that we can ensure that
	// the parent ID is set independently of the trace ID
	ctx = child.Attach(context.Background())
	grandchild := SpanFromContext(ctx)

	assert.Equal(t, child.TraceID, trace.SpanID)
	assert.Equal(t, child.TraceID, trace.TraceID)
	assert.Equal(t, child.ParentID, trace.SpanID)
	assert.Equal(t, grandchild.ParentID, child.SpanID)
	assert.Equal(t, grandchild.TraceID, trace.SpanID)
}

// StartSpanFromContext should create a brand-new root span
// if the context does not contain a span
func TestSpanFromContextNoParent(t *testing.T) {
	const resource = "example"
	ctx := context.Background()

	span, _ := StartSpanFromContext(ctx, resource)

	assert.Equal(t, span.TraceID, span.SpanID)
	assert.Equal(t, int64(0), span.ParentID)
}

func TestStartChildSpan(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	root := StartTrace(resource)
	child := StartChildSpan(root)
	grandchild := StartChildSpan(child)

	assert.Equal(t, resource, child.Resource)
	assert.Equal(t, resource, grandchild.Resource)

	assert.Equal(t, root.SpanID, root.TraceID)
	assert.Equal(t, root.SpanID, child.TraceID)
	assert.Equal(t, root.SpanID, grandchild.TraceID)

	assert.Equal(t, root.SpanID, child.ParentID)
	assert.Equal(t, child.SpanID, grandchild.ParentID)
}

// Test that a Trace is correctly able to generate
// its spanContext representation from the point of view
// of its children
func TestTraceContextAsParent(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	trace := StartTrace(resource)

	ctx := trace.contextAsParent()

	assert.Equal(t, trace.TraceID, ctx.TraceID())
	assert.Equal(t, trace.SpanID, ctx.ParentID())
	assert.Equal(t, trace.Resource, ctx.Resource())
}

type localError struct {
	message string
}

func (le localError) Error() string {
	return le.message
}

func TestError(t *testing.T) {
	const resource = "Robert'); DROP TABLE students;"
	const errorMessage = "some error happened"
	err := localError{errorMessage}

	root := StartTrace(resource)
	root.Error(err)

	assert.Equal(t, root.Status, ssf.SSFSample_CRITICAL)
	assert.Equal(t, len(root.Tags), 3)

	for k, v := range root.Tags {
		switch k {
		case errorMessageTag:
			assert.Equal(t, v, err.Error())
		case errorTypeTag:
			assert.Equal(t, v, "localError")
		case errorStackTag:
			assert.Equal(t, v, err.Error())
		}
	}

}

func TestStripPackageName(t *testing.T) {
	type testCase struct {
		Name     string
		fname    string
		expected string
	}

	cases := []testCase{
		{
			Name:     "Method",
			fname:    "github.com/stripe/veneur.(*Server).Flush",
			expected: "veneur.(*Server).Flush",
		},
		{
			Name:     "NestedPackageMethod",
			fname:    "github.com/stripe/veneur/trace.(*Tracer).StartSpan",
			expected: "trace.(*Tracer).StartSpan",
		},
		{
			// This shouldn't be valid, but we should at least ensure we don't
			// cause a runtime panic if it's passed
			Name:     "TrailingSlash",
			fname:    "github.com/",
			expected: "github.com/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, stripPackageName(tc.fname), tc.expected)
		})
	}
}

func assertTagEquals(t *testing.T, sample *ssf.SSFSpan, name, value string) {
	assert.Equal(t, value, sample.Tags[name])
}

func BenchmarkMarshalSSF(b *testing.B) {
	span := &ssf.SSFSpan{}

	for n := 0; n < b.N; n++ {
		proto.Marshal(span)
	}
}
