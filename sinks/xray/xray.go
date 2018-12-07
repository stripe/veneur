package xray

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math"
	"net"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

var segmentHeader = []byte(`{"format": "json", "version": 1}` + "\n")

const XRayTagNameClientIP = "xray_client_ip"

type XRaySegmentHTTPRequest struct {
	Method        string `json:"method,omitempty"`
	ClientIP      string `json:"client_ip,omitempty"`
	URL           string `json:"url,omitempty"`
	UserAgent     string `json:"user_agent,omitempty"`
	XForwardedFor string `json:"x_forwarded_for,omitempty"`
}

type XRaySegmentHTTPResponse struct {
	Status int `json:"status,omitempty"`
}

type XRaySegmentHTTP struct {
	Request  XRaySegmentHTTPRequest  `json:"request,omitempty"`
	Response XRaySegmentHTTPResponse `json:"response,omitempty"`
}

// XRaySegment is a trace segment for X-Ray as defined by:
// https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html
type XRaySegment struct {
	// The 3-tuple (name, segment type, account ID) uniquely defines
	// an X-Ray service. The Veneur X-Ray sink uses the default type (segment)
	// for all segments, so for a deployment that only uses a single AWS account ID,
	// the name field will uniquely define the service (and be used as the service name)
	Name        string            `json:"name"`
	ID          string            `json:"id"`
	TraceID     string            `json:"trace_id"`
	ParentID    string            `json:"parent_id,omitempty"`
	StartTime   float64           `json:"start_time"`
	EndTime     float64           `json:"end_time"`
	Namespace   string            `json:"namespace"`
	Error       bool              `json:"error"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	HTTP        XRaySegmentHTTP   `json:"http,omitempty"`
}

// XRaySpanSink is a sink for spans to be sent to AWS X-Ray.
type XRaySpanSink struct {
	daemonAddr      string
	traceClient     *trace.Client
	conn            *net.UDPConn
	sampleThreshold uint32
	commonTags      map[string]string
	annotationTags  map[string]struct{}
	log             *logrus.Logger
	spansDropped    int64
	spansHandled    int64
	nameRegex       *regexp.Regexp
}

var _ sinks.SpanSink = &XRaySpanSink{}

// NewXRaySpanSink creates a new instance of a XRaySpanSink.
func NewXRaySpanSink(daemonAddr string, sampleRatePercentage int, commonTags map[string]string, annotationTags []string, log *logrus.Logger) (*XRaySpanSink, error) {

	log.WithFields(logrus.Fields{
		"Address": daemonAddr,
	}).Info("Creating X-Ray client")

	var sampleThreshold uint32
	if sampleRatePercentage < 0 {
		log.WithField("sampleRatePercentage", sampleRatePercentage).Warn("Sample rate < 0 is invalid, defaulting to 0")
		sampleRatePercentage = 0
	}
	if sampleRatePercentage > 100 {
		log.WithField("sampleRatePercentage", sampleRatePercentage).Warn("Sample rate > 100 is invalid, defaulting to 100")
		sampleRatePercentage = 100
	}

	// Set the sample threshold to (sample rate) * (maximum value of uint32), so that
	// we can store it as a uint32 instead of a float64 and compare apples-to-apples
	// with the output of our hashing algorithm.
	sampleThreshold = uint32(sampleRatePercentage * math.MaxUint32 / 100)

	// Build a regex for cleaning names based on valid characters from:
	// https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html#api-segmentdocuments-fields
	reg, err := regexp.Compile(`[^a-zA-Z0-9_\.\:\/\%\&#=+\-\@\s\\]+`)
	if err != nil {
		return nil, err
	}

	annotationTagsMap := map[string]struct{}{}
	for _, key := range annotationTags {
		annotationTagsMap[key] = struct{}{}
	}

	return &XRaySpanSink{
		daemonAddr:      daemonAddr,
		sampleThreshold: sampleThreshold,
		commonTags:      commonTags,
		log:             log,
		nameRegex:       reg,
		annotationTags:  annotationTagsMap,
	}, nil
}

// Start the sink
func (x *XRaySpanSink) Start(cl *trace.Client) error {
	x.traceClient = cl

	xrayDaemon, err := net.ResolveUDPAddr("udp", x.daemonAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, xrayDaemon)
	if err != nil {
		return err
	}
	x.conn = conn

	return nil
}

// Name returns this sink's name.
func (x *XRaySpanSink) Name() string {
	return "xray"
}

// Ingest takes in a span and passed it along to the X-Ray client after
// some sanity checks and improvements are made.
func (x *XRaySpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	sampleCheckValue := []byte(strconv.FormatInt(ssfSpan.TraceId, 10))
	hashKey := crc32.ChecksumIEEE(sampleCheckValue)
	if hashKey > x.sampleThreshold {
		atomic.AddInt64(&x.spansDropped, 1)
		return nil
	}

	metadata := map[string]string{}
	annotations := map[string]string{}
	for k, v := range x.commonTags {
		metadata[k] = v
	}
	for k, v := range ssfSpan.Tags {
		if k != XRayTagNameClientIP {
			metadata[k] = v
		}
		if _, present := x.annotationTags[k]; present {
			annotations[k] = v
		}
	}
	if ssfSpan.Indicator {
		metadata["indicator"] = "true"
		annotations["indicator"] = "true"
	} else {
		metadata["indicator"] = "false"
		annotations["indicator"] = "false"
	}

	name := string(x.nameRegex.ReplaceAll([]byte(ssfSpan.Service), []byte("_")))
	if len(name) > 190 {
		name = name[:190]
	}

	if ssfSpan.Indicator {
		name = name + "-indicator"
	}

	// The fields below are defined here:
	// https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html#api-segmentdocuments-fields
	segment := XRaySegment{
		// ID is a 64-bit hex
		ID: fmt.Sprintf("%016x", ssfSpan.Id),
		// Trace ID is version-startTimeUnixAs8CharHex-traceIdAs24CharHex
		TraceID:     fmt.Sprintf("1-%08x-%024x", ssfSpan.StartTimestamp/1e9, ssfSpan.TraceId),
		Name:        name,
		StartTime:   float64(float64(ssfSpan.StartTimestamp) / float64(time.Second)),
		EndTime:     float64(float64(ssfSpan.EndTimestamp) / float64(time.Second)),
		Annotations: annotations,
		Metadata:    metadata,
		Namespace:   "remote",
		Error:       ssfSpan.Error,
		// Because X-Ray doesn't offer another way to get this data in, we pretend
		// it's HTTP for now. It's likely that as X-Ray and/or Veneur develop this
		// will change.
		HTTP: XRaySegmentHTTP{
			Request: XRaySegmentHTTPRequest{
				URL:      fmt.Sprintf("%s:%s", ssfSpan.Service, ssfSpan.Name),
				ClientIP: ssfSpan.Tags[XRayTagNameClientIP],
			},
		},
	}
	if ssfSpan.ParentId != 0 {
		segment.ParentID = fmt.Sprintf("%016x", ssfSpan.ParentId)
	}
	b, err := json.Marshal(segment)
	if err != nil {
		x.log.WithError(err).Error("Error marshaling segment")
		return err
	}
	// Send the segment
	_, err = x.conn.Write(append(segmentHeader, b...))
	if err != nil {
		x.log.WithError(err).Error("Error sending segment")
		return err
	}

	atomic.AddInt64(&x.spansHandled, 1)
	return nil
}

// Flush doesn't need to do anything, so we emit metrics
// instead.
func (x *XRaySpanSink) Flush() {
	x.log.WithFields(logrus.Fields{
		"flushed_spans": atomic.LoadInt64(&x.spansHandled),
		"dropped_spans": atomic.LoadInt64(&x.spansDropped),
	}).Debug("Checkpointing flushed spans for X-Ray")
	metrics.ReportBatch(x.traceClient, []*ssf.SSFSample{
		ssf.Count(sinks.MetricKeyTotalSpansFlushed, float32(atomic.SwapInt64(&x.spansHandled, 0)), map[string]string{"sink": x.Name()}),
		ssf.Count(sinks.MetricKeyTotalSpansDropped, float32(atomic.SwapInt64(&x.spansDropped, 0)), map[string]string{"sink": x.Name()}),
	})
}
