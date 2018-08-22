package splunk

import (
	"context"
	"crypto/tls"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	hec "github.com/fuyufjh/splunk-hec-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

type splunkSpanSink struct {
	*hec.Client
	hostname string

	// Total counts of sent and dropped spans, respectively
	sentCount, dropCount uint32

	traceClient *trace.Client
	log         *logrus.Logger
}

// NewSplunkSpanSink constructs a new splunk span sink from the server
// name and token provided, using the local hostname configured for
// veneur. An optional argument, validateServerName is used (if
// non-empty) to instruct go to validate a different hostname than the
// one on the server URL.
func NewSplunkSpanSink(server string, token string, localHostname string, validateServerName string, log *logrus.Logger) (sinks.SpanSink, error) {
	client := hec.NewClient(server, token).(*hec.Client)

	if validateServerName != "" {
		tlsCfg := &tls.Config{}
		tlsCfg.ServerName = validateServerName

		trnsp := &http.Transport{TLSClientConfig: tlsCfg}
		httpC := &http.Client{Transport: trnsp}
		client.SetHTTPClient(httpC)
	}

	return &splunkSpanSink{Client: client, hostname: localHostname}, nil
}

func (sss *splunkSpanSink) Start(cl *trace.Client) error {
	sss.traceClient = cl
	return nil
}

func (sss *splunkSpanSink) Flush() {
	samples := &ssf.Samples{}
	samples.Add(
		ssf.Count(
			sinks.MetricKeyTotalSpansFlushed,
			float32(atomic.SwapUint32(&sss.sentCount, 0)),
			map[string]string{"sink": sss.Name()}),
		ssf.Count(
			sinks.MetricKeyTotalSpansDropped,
			float32(atomic.SwapUint32(&sss.dropCount, 0)),
			map[string]string{"sink": sss.Name()},
		),
	)

	metrics.Report(sss.traceClient, samples)
	return
}

// Name returns this sink's name
func (*splunkSpanSink) Name() string {
	return "splunk"
}

// Ingest takes in a span and passes it to Splunk using the
// HTTP Event Collector
func (sss *splunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	// Only send properly filled-out spans to the HEC:
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	// Fake up a context with a reasonable timeout:
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	return sss.writeSpan(ctx, ssfSpan)
}

// Splunk can't handle int64s, and we don't
// want to round our traceID to the thousands place.
// This is mildly redundant, but oh well
type serializedSSF struct {
	TraceId        string            `json:"trace_id"`
	Id             string            `json:"id"`
	ParentId       string            `json:"parent_id"`
	StartTimestamp time.Time         `json:"start_timestamp"`
	EndTimestamp   time.Time         `json:"end_timestamp"`
	Error          bool              `json:"error"`
	Service        string            `json:"service"`
	Tags           map[string]string `json:"tags"`
	Indicator      bool              `json:"indicator"`
	Name           string            `json:"name"`
}

func (sss *splunkSpanSink) writeSpan(ctx context.Context, ssfSpan *ssf.SSFSpan) error {
	serialized := serializedSSF{
		TraceId:        strconv.FormatInt(ssfSpan.TraceId, 10),
		Id:             strconv.FormatInt(ssfSpan.Id, 10),
		ParentId:       strconv.FormatInt(ssfSpan.ParentId, 10),
		StartTimestamp: time.Unix(0, ssfSpan.StartTimestamp),
		EndTimestamp:   time.Unix(0, ssfSpan.EndTimestamp),
		Error:          ssfSpan.Error,
		Service:        ssfSpan.Service,
		Tags:           ssfSpan.Tags,
		Indicator:      ssfSpan.Indicator,
		Name:           ssfSpan.Name,
	}

	start := time.Unix(int64((time.Duration(ssfSpan.StartTimestamp) / time.Second)), 0)

	event := &hec.Event{
		Event: serialized,
	}
	event.SetTime(time.Unix(0, ssfSpan.StartTimestamp))
	event.SetHost(sss.hostname)
	event.SetSourceType(ssfSpan.Service)

	event.SetTime(start)

	err := sss.WriteEventWithContext(ctx, event)
	if err != nil {
		atomic.AddUint32(&sss.dropCount, 1)

		// TODO: get rid of this, it'll get suuuper chunderous:
		sss.log.WithError(err).Error("Couldn't flush span to HEC")
	} else {
		atomic.AddUint32(&sss.sentCount, 1)
	}
	return err
}
