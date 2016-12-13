package veneur

import (
	"math/rand"
	"net"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/ssf"
)

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
		"parent": parentId,
		"spanId": spanId,
	}).Debug("Recorded trace")
}
