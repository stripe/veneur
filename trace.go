package veneur

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
}

func (t *Trace) Record(name string, tags []*ssf.SSFTag) {
	recordTrace(t.Start, name, tags, t.SpanId, t.TraceId, t.ParentId, t.Resource)
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
		log.WithField("type", reflect.TypeOf(c.Value(traceKey))).Error("expected *Trace from context")
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

func sendSample(sample *ssf.SSFSample) error {
	server_addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8128")
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

// recordTrace sends a trace to DataDog.
// If the spanId is negative, it will be regenerated.
// If this is the root trace, parentId should be zero.
// resource will be ignored for non-root spans.
func recordTrace(startTime time.Time, name string, tags []*ssf.SSFTag, spanId, traceId, parentId int64, resource string) {
	if spanId < 0 {
		spanId = *proto.Int64(rand.Int63())
	}
	duration := time.Now().Sub(startTime).Nanoseconds()

	sample := &ssf.SSFSample{
		Metric:    ssf.SSFSample_TRACE,
		Timestamp: startTime.UnixNano(),
		Status:    ssf.SSFSample_OK,
		Name:      *proto.String(name),
		Trace: &ssf.SSFTrace{
			TraceId:  traceId,
			Id:       spanId,
			ParentId: parentId,
		},
		Value:      duration,
		SampleRate: *proto.Float32(.10),
		Tags:       []*ssf.SSFTag{},
		Resource:   resource,
		Service:    "veneur",
	}

	err := sendSample(sample)
	if err != nil {
		log.WithError(err).Error("Error submitting sample")
	}
	log.WithFields(logrus.Fields{
		"parent":   parentId,
		"spanId":   spanId,
		"name":     name,
		"resource": resource,
		"traceId":  traceId,
	}).Debug("Recorded trace")
}
