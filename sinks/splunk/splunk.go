package splunk

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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
	hostname      string
	sendTimeout   time.Duration
	ingestTimeout time.Duration

	workers int

	maxSpanCapacity      int
	hecSubmissionWorkers int
	ingestedSpans        uint32
	droppedSpans         uint32

	ingest       chan *hec.Event
	flushForTime chan *flushRequest
	flushForSize chan *flushRequest

	traceClient *trace.Client
	log         *logrus.Logger
}

const DefaultMaxContentLength = 1000000

// ErrTooManySpans is an error returned when the number of spans
// ingested in a flush interval exceeds the maximum number configured
// for this sink. See the splunk_hec_max_capacity config setting.
var ErrTooManySpans = fmt.Errorf("ingested spans exceed the configured limit.")

// NewSplunkSpanSink constructs a new splunk span sink from the server
// name and token provided, using the local hostname configured for
// veneur. An optional argument, validateServerName is used (if
// non-empty) to instruct go to validate a different hostname than the
// one on the server URL.
func NewSplunkSpanSink(server string, token string, localHostname string, validateServerName string, log *logrus.Logger, ingestTimeout time.Duration, sendTimeout time.Duration, maxSpanCapacity int, workers int) (sinks.SpanSink, error) {
	client := hec.NewClient(server, token).(*hec.Client)
	client.SetMaxRetry(0)
	client.SetMaxContentLength(DefaultMaxContentLength)

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
		flushForTime:    make(chan *flushRequest),
		flushForSize:    make(chan *flushRequest),
		hostname:        localHostname,
		log:             log,
		sendTimeout:     sendTimeout,
		ingestTimeout:   ingestTimeout,
		maxSpanCapacity: maxSpanCapacity,
	}, nil
}

// Name returns this sink's name
func (*splunkSpanSink) Name() string {
	return "splunk"
}

func (sss *splunkSpanSink) Start(cl *trace.Client) error {
	sss.traceClient = cl
	go sss.batchAndSend()

	for i := 0; i < sss.workers; i++ {
		go sss.submitter()
	}

	return nil
}

type flushRequest struct {
	data   *bytes.Buffer
	nSpans int
}

func newFlushRequest() *flushRequest {
	b := &bytes.Buffer{}
	b.Grow(DefaultMaxContentLength)

	return &flushRequest{
		data:   b,
		nSpans: 0,
	}
}

func (fr *flushRequest) addEvent(ev *hec.Event) ([]byte, bool) {
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, true // do not expect JSON encoding errors.
	}
	if ok := fr.addBytes(b); !ok {
		return nil, ok
	}
	fr.nSpans++
	return b, true
}

func (fr *flushRequest) addBytes(b []byte) bool {
	if fr.data.Len()+len(b) >= DefaultMaxContentLength {
		return false
	}
	fr.data.Write(b)
	return true
}

func (sss *splunkSpanSink) submitter() {
	ctx := context.Background()
	for {
		batch := <-sss.flushForSize
		rs := NewBufferReadSeeker(batch.data)
		sss.submitBatch(ctx, rs, batch.nSpans)
	}
}

func (sss *splunkSpanSink) submitBatch(ctx context.Context, batch io.ReadSeeker, nSpans int) {
	samples := &ssf.Samples{}
	defer metrics.Report(sss.traceClient, samples)

	start := time.Now()
	if sss.sendTimeout != 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, sss.sendTimeout)
		defer cancel()
	}
	err := sss.Client.WriteRawWithContext(ctx, batch, nil)
	samples.Add(ssf.Timing("splunk.span_submission_duration_ns", time.Now().Sub(start), time.Nanosecond, map[string]string{}))

	if err != nil {
		samples.Add(ssf.Count("splunk.span_submission_failed_total", float32(nSpans), map[string]string{}))
		if ctx.Err() == nil {
			sss.log.WithError(err).
				WithField("n_spans", nSpans).
				Error("Couldn't flush batch to HEC")
		}
		return
	}
	samples.Add(ssf.Count("splunk.span_submitted_total", float32(nSpans), map[string]string{}))
}

func (sss *splunkSpanSink) batchAndSend() {
	batch := newFlushRequest()
	for {
		success := true
		var leftover []byte
		select {
		case ev := <-sss.ingest:
			leftover, success = batch.addEvent(ev)
		case sss.flushForTime <- batch:
			batch = newFlushRequest()
		}
		if !success || (sss.maxSpanCapacity != 0 && batch.nSpans >= sss.maxSpanCapacity) {
			// we can't ingest more; block the ingestion
			// channel and try to flush what we have:
			select {
			case sss.flushForSize <- batch:
			case sss.flushForTime <- batch:
			}
			batch = newFlushRequest()
			if leftover != nil {
				batch.addBytes(leftover)
			}
		}
	}
}

// Flush takes the batched-up events and sends them to the HEC
// endpoint for ingestion. If set, it uses the send timeout configured
// for the span batch.
func (sss *splunkSpanSink) Flush() {
	batch := <-sss.flushForTime

	// TODO: Ideally, Flush() would get a context of its own:
	ctx := context.Background()

	rs := NewBufferReadSeeker(batch.data)
	sss.submitBatch(ctx, rs, batch.nSpans)

	samples := &ssf.Samples{}
	samples.Add(
		ssf.Count(
			sinks.MetricKeyTotalSpansFlushed,
			float32(atomic.SwapUint32(&sss.ingestedSpans, 0)),
			map[string]string{"sink": sss.Name()}),
		ssf.Count(
			sinks.MetricKeyTotalSpansDropped,
			float32(atomic.SwapUint32(&sss.droppedSpans, 0)),
			map[string]string{"sink": sss.Name()},
		),
	)

	metrics.Report(sss.traceClient, samples)
	return
}

// Ingest takes in a span and batches it up to be sent in the next
// Flush() iteration.
func (sss *splunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	ctx := context.Background()
	if sss.ingestTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, sss.ingestTimeout)
		defer cancel()
	}

	// Only send properly filled-out spans to the HEC:
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

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
	select {
	case sss.ingest <- event:
		atomic.AddUint32(&sss.ingestedSpans, 1)
		return nil
	case <-ctx.Done():
		atomic.AddUint32(&sss.droppedSpans, 1)
		return ErrTooManySpans
	}
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
