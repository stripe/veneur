package xray

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util"
)

var segmentHeader = []byte(`{"format": "json", "version": 1}` + "\n")

const XRayTagNameClientIP = "xray_client_ip"
const SpanTagNameHttpUrl = "http.url"
const SpanTagNameHttpStatusCode = "http.status_code"
const SpanTagNameHttpMethod = "http.method"

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

type XRaySinkConfig struct {
	Address          string   `yaml:"address"`
	AnnotationTags   []string `yaml:"annotation_tags"`
	SamplePercentage float64  `yaml:"sample_percentage"`
}

// XRaySpanSink is a sink for spans to be sent to AWS X-Ray.
type XRaySpanSink struct {
	daemonAddr      string
	traceClient     *trace.Client
	conn            *net.UDPConn
	sampleThreshold uint32
	commonTags      map[string]string
	annotationTags  map[string]struct{}
	log             *logrus.Entry
	spansDropped    int64
	spansHandled    int64
	name            string
	nameRegex       *regexp.Regexp
}

var _ sinks.SpanSink = &XRaySpanSink{}

// TODO(yeogai): Remove this once the old configuration format has been
// removed.
func MigrateConfig(conf *veneur.Config) {
	if conf.XrayAddress == "" {
		return
	}
	conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
		Kind: "xray",
		Name: "xray",
		Config: XRaySinkConfig{
			Address:          conf.XrayAddress,
			AnnotationTags:   conf.XrayAnnotationTags,
			SamplePercentage: conf.XraySamplePercentage,
		},
	})
}

// ParseConfig decodes the map config for an X-Ray sink into a XRaySinkConfig
// struct.
func ParseConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	xrayConfig := XRaySinkConfig{}
	err := util.DecodeConfig(name, config, &xrayConfig)
	if err != nil {
		return nil, err
	}
	return xrayConfig, nil
}

// Create creates a new instance of a XRaySpanSink.
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	xRaySinkConfig, ok := sinkConfig.(XRaySinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	logger.WithFields(logrus.Fields{
		"Address": xRaySinkConfig.Address,
	}).Info("Creating X-Ray client")

	sampleRatePercentage := xRaySinkConfig.SamplePercentage
	if sampleRatePercentage < 0 {
		logger.WithField("sampleRatePercentage", sampleRatePercentage).Warn("Sample rate < 0 is invalid, defaulting to 0")
		sampleRatePercentage = 0
	}
	if sampleRatePercentage > 100 {
		logger.WithField("sampleRatePercentage", sampleRatePercentage).Warn("Sample rate > 100 is invalid, defaulting to 100")
		sampleRatePercentage = 100
	}

	// Set the sample threshold to (sample rate) * (maximum value of uint32), so that
	// we can store it as a uint32 instead of a float64 and compare apples-to-apples
	// with the output of our hashing algorithm.
	sampleThreshold := uint32(sampleRatePercentage * math.MaxUint32 / 100)

	// Build a regex for cleaning names based on valid characters from:
	// https://docs.aws.amazon.com/xray/latest/devguide/xray-api-segmentdocuments.html#api-segmentdocuments-fields
	reg, err := regexp.Compile(`[^a-zA-Z0-9_\.\:\/\%\&#=+\-\@\s\\]+`)
	if err != nil {
		return nil, err
	}

	annotationTagsMap := map[string]struct{}{}
	for _, tag := range xRaySinkConfig.AnnotationTags {
		key := strings.Split(tag, ":")[0]
		annotationTagsMap[key] = struct{}{}
	}

	return &XRaySpanSink{
		daemonAddr:      xRaySinkConfig.Address,
		sampleThreshold: sampleThreshold,
		commonTags:      server.TagsAsMap,
		log:             logger,
		name:            name,
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
	return x.name
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

	http := XRaySegmentHTTP{
		Request: XRaySegmentHTTPRequest{
			URL:      fmt.Sprintf("%s:%s", ssfSpan.Service, ssfSpan.Name),
			ClientIP: ssfSpan.Tags[XRayTagNameClientIP],
		},
	}

	for k, v := range ssfSpan.Tags {
		switch k {
		case XRayTagNameClientIP:
			continue
		case SpanTagNameHttpUrl:
			http.Request.URL = v
		case SpanTagNameHttpMethod:
			http.Request.Method = v
		case SpanTagNameHttpStatusCode:
			status, err := strconv.Atoi(v)
			if err != nil || status > 599 || status < 100 {
				x.log.WithField("status", v).Warn("Malformed status code")
			} else {
				http.Response.Status = status
			}
		}

		metadata[k] = v
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
		ID:          fmt.Sprintf("%016x", ssfSpan.Id),
		TraceID:     x.CalculateTraceID(ssfSpan),
		Name:        name,
		StartTime:   float64(float64(ssfSpan.StartTimestamp) / float64(time.Second)),
		EndTime:     float64(float64(ssfSpan.EndTimestamp) / float64(time.Second)),
		Annotations: annotations,
		Metadata:    metadata,
		Namespace:   "remote",
		Error:       ssfSpan.Error,
		HTTP:        http,
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
		x.log.WithError(err).Warn("Error sending segment")
		atomic.AddInt64(&x.spansDropped, 1)
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

func (x *XRaySpanSink) CalculateTraceID(ssfSpan *ssf.SSFSpan) string {
	// For multiple segments to be aggregated into a single trace, they should have identical traceID.
	// For this reason, the startTimestamp needs to be the timestamp of the original (i.e. root) request
	// and not the subsequent spans.
	startTimestamp := ssfSpan.RootStartTimestamp / 1e9
	if startTimestamp == 0 {
		// We want to have a stable value here, but want to allow this functionality
		// without requiring the SSF clients to start emitting the new field.
		// Instead, we compute here a psuedo timestamp based on the ~4min interval the span is in.
		// This makes sure the time passed is always is the past and multiple spans of the same
		// trace will have the same AWS X-Ray's TraceID
		temp := ssfSpan.StartTimestamp / 1e9
		// clearing the last byte. MSB is 0 so we don't care signed/unsigned
		startTimestamp = temp & 0xFFFFFFFFFFFF00
	}
	// Trace ID is version-startTimeUnixAs8CharHex-traceIdAs24CharHex
	return fmt.Sprintf("1-%08x-%024x", startTimestamp, ssfSpan.TraceId)
}
