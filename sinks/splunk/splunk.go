package splunk

import (
	"context"
	"crypto/tls"
	"fmt"
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
	hostname        string
	sendTimeout     time.Duration
	maxSpanCapacity int
	ingestedSpans   uint32

	ingest chan *hec.Event
	flush  chan []*hec.Event

	traceClient *trace.Client
	log         *logrus.Logger
}

// ErrTooManySpans is an error returned when the number of spans
// ingested in a flush interval exceeds the maximum number configured
// for this sink. See the splunk_hec_max_capacity config setting.
var ErrTooManySpans = fmt.Errorf("Ingested spans exceed the configured limit.")

// NewSplunkSpanSink constructs a new splunk span sink from the server
// name and token provided, using the local hostname configured for
// veneur. An optional argument, validateServerName is used (if
// non-empty) to instruct go to validate a different hostname than the
// one on the server URL.
func NewSplunkSpanSink(server string, token string, localHostname string, validateServerName string, log *logrus.Logger, sendTimeout time.Duration, maxSpanCapacity int) (sinks.SpanSink, error) {
	client := hec.NewClient(server, token).(*hec.Client)

	if validateServerName != "" {
		tlsCfg := &tls.Config{}
		tlsCfg.ServerName = validateServerName

		trnsp := &http.Transport{TLSClientConfig: tlsCfg}
		httpC := &http.Client{Transport: trnsp}
		client.SetHTTPClient(httpC)
	}

	return &splunkSpanSink{
		Client:          client,
		ingest:          make(chan *hec.Event),
		flush:           make(chan []*hec.Event),
		hostname:        localHostname,
		log:             log,
		sendTimeout:     sendTimeout,
		maxSpanCapacity: maxSpanCapacity,
	}, nil
}

func (sss *splunkSpanSink) Start(cl *trace.Client) error {
	sss.traceClient = cl
	go sss.batchAndSend()

	return nil
}

func (sss *splunkSpanSink) batchAndSend() {
	batch := make([]*hec.Event, 0, sss.maxSpanCapacity)
	for {
		select {
		case ev := <-sss.ingest:
			batch = append(batch, ev)
		case sss.flush <- batch:
			// We could flush the batch - get us a new one.
			batch = make([]*hec.Event, 0, sss.maxSpanCapacity)
		}

	}
}

// Flush takes the batched-up events and sends them to the HEC
// endpoint for ingestion. If set, it uses the send timeout configured
// for the span batch.
func (sss *splunkSpanSink) Flush() {
	flushed := 0
	dropped := 0
	batch := <-sss.flush
	atomic.StoreUint32(&sss.ingestedSpans, 0)

	// TODO: Ideally, Flush() would get a context of its own:
	ctx := context.Background()
	if sss.sendTimeout != 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, sss.sendTimeout)
		defer cancel()
	}
	err := sss.Client.WriteBatchWithContext(ctx, batch)
	if err != nil {
		dropped += len(batch)
		if ctx.Err() == nil {
			sss.log.WithError(err).
				WithField("n_spans", len(batch)).
				Error("Couldn't flush batch to HEC")
		}
	} else {
		flushed += len(batch)
	}

	samples := &ssf.Samples{}
	samples.Add(
		ssf.Count(
			sinks.MetricKeyTotalSpansFlushed,
			float32(flushed),
			map[string]string{"sink": sss.Name()}),
		ssf.Count(
			sinks.MetricKeyTotalSpansDropped,
			float32(dropped),
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

// Ingest takes in a span and batches it up to be sent in the next
// Flush() iteration.
func (sss *splunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	// Only send properly filled-out spans to the HEC:
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}
	if sss.maxSpanCapacity != 0 && atomic.LoadUint32(&sss.ingestedSpans) >= uint32(sss.maxSpanCapacity) {
		return ErrTooManySpans
	}
	atomic.AddUint32(&sss.ingestedSpans, 1)
	serialized := SerializedSSF{
		TraceId:        strconv.FormatInt(ssfSpan.TraceId, 10),
		Id:             strconv.FormatInt(ssfSpan.Id, 10),
		ParentId:       strconv.FormatInt(ssfSpan.ParentId, 10),
		StartTimestamp: float64(ssfSpan.StartTimestamp) / float64(time.Second),
		EndTimestamp:   float64(ssfSpan.EndTimestamp) / float64(time.Second),
		Duration:       ssfSpan.EndTimestamp - ssfSpan.StartTimestamp,
		Error:          ssfSpan.Error,
		Service:        ssfSpan.Service,
		Tags:           ssfSpan.Tags,
		Indicator:      ssfSpan.Indicator,
		Name:           ssfSpan.Name,
	}

	event := &hec.Event{
		Event: serialized,
	}
	event.SetTime(time.Unix(0, ssfSpan.StartTimestamp))
	event.SetHost(sss.hostname)
	event.SetSourceType(ssfSpan.Service)

	event.SetTime(time.Unix(0, ssfSpan.StartTimestamp))
	sss.ingest <- event
	return nil
}

// SerializedSSF holds a set of fields in a format that Splunk can
// handle (it can't handle int64s, and we don't want to round our
// traceID to the thousands place).  This is mildly redundant, but oh
// well.
type SerializedSSF struct {
	TraceId        string            `json:"trace_id"`
	Id             string            `json:"id"`
	ParentId       string            `json:"parent_id"`
	StartTimestamp float64           `json:"start_timestamp"`
	EndTimestamp   float64           `json:"end_timestamp"`
	Duration       int64             `json:"duration_ns"`
	Error          bool              `json:"error"`
	Service        string            `json:"service"`
	Tags           map[string]string `json:"tags"`
	Indicator      bool              `json:"indicator"`
	Name           string            `json:"name"`
}
