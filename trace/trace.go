package trace

import (
	"context"
	"math/rand"
	"net"
	"reflect"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/ssf"
)

func init() {
	rand.Seed(time.Now().Unix())
}

const traceKey = "trace"

// this should be set exactly once, at startup
var Service = ""

const localVeneurAddress = "127.0.0.1:8128"

type Trace struct {
	// the ID for the root span
	// which is also the ID for the trace itself
	TraceId int64

	// For the root span, this will be equal
	// to the TraceId
	SpanId int64

	// For the root span, this will be <= 0
	ParentId int64

	// The Resource should be the same for all spans in the same trace
	Resource string

	Start time.Time

	// If non-zero, the trace will be treated
	// as an error
	Status ssf.SSFSample_Status
}

// Record sends a trace to the (local) veneur instance,
// which will pass it on to the tracing agent running on the
// global veneur instance.
func (t *Trace) Record(name string, tags []*ssf.SSFTag) error {
	duration := time.Now().Sub(t.Start).Nanoseconds()

	sample := &ssf.SSFSample{
		Metric:    ssf.SSFSample_TRACE,
		Timestamp: t.Start.UnixNano(),
		Status:    t.Status,
		Name:      *proto.String(name),
		Trace: &ssf.SSFTrace{
			TraceId:  t.TraceId,
			Id:       t.SpanId,
			ParentId: t.ParentId,
		},
		Value:      duration,
		SampleRate: *proto.Float32(.10),
		Tags:       tags,
		Resource:   t.Resource,
		Service:    Service,
	}

	err := sendSample(sample)
	if err != nil {
		logrus.WithError(err).Error("Error submitting sample")
	}
	return err
}

// Attach attaches the current trace to the context
// and returns a copy of the context with that trace
// stored under the key "trace".
func (t *Trace) Attach(c context.Context) context.Context {
	return context.WithValue(c, traceKey, t)
}

// SpanFromContext is used to create a child span
// when the parent trace is in the context
func SpanFromContext(c context.Context) *Trace {
	parent, ok := c.Value(traceKey).(*Trace)
	if !ok {
		logrus.WithField("type", reflect.TypeOf(c.Value(traceKey))).Error("expected *Trace from context")
	}

	spanId := proto.Int64(rand.Int63())
	span := &Trace{
		TraceId:  parent.TraceId,
		SpanId:   *spanId,
		ParentId: parent.SpanId,
		Resource: parent.Resource,
		Start:    time.Now(),
	}

	return span
}

// StartTrace is called by to create the root-level span
// for a trace
func StartTrace(resource string) *Trace {
	traceId := proto.Int64(rand.Int63())

	t := &Trace{
		TraceId:  *traceId,
		SpanId:   *traceId,
		ParentId: 0,
		Resource: resource,
	}

	t.Start = time.Now()
	return t
}

// sendSample marshals the sample using protobuf and sends it
// over UDP to the local veneur instance
func sendSample(sample *ssf.SSFSample) error {
	server_addr, err := net.ResolveUDPAddr("udp", localVeneurAddress)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, server_addr)
	if err != nil {
		return err
	}

	defer conn.Close()

	data, err := proto.Marshal(sample)
	if err != nil {
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	return nil
}
