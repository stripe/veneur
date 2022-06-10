package splunk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"math/big"

	"crypto/rand"
	mrand "math/rand"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/util"
)

// TestableSplunkSpanSink provides methods that are useful for testing
// a splunk span sink.
type TestableSplunkSpanSink interface {
	sinks.SpanSink

	// Stop shuts down the sink's submission workers by finishing
	// each worker's last submission HTTP request.
	Stop()

	// Sync instructs all submission workers to finish submitting
	// their current request and start a new one. It returns when
	// the last worker's submission is done.
	Sync()
}

type SplunkSinkConfig struct {
	HecAddress                  string        `yaml:"hec_address"`
	HecBatchSize                int           `yaml:"hec_batch_size"`
	HecConnectionLifetimeJitter time.Duration `yaml:"hec_connection_lifetime_jitter"`
	HecIngestTimeout            time.Duration `yaml:"hec_ingest_timeout"`
	HecMaxConnectionLifetime    time.Duration `yaml:"hec_max_connection_lifetime"`
	HecSendTimeout              time.Duration `yaml:"hec_max_connection_lifetime"`
	HecSubmissionWorkers        int           `yaml:"hec_submission_workers"`
	HecTLSValidateHostname      string        `yaml:"hec_tls_validate_hostname"`
	HecToken                    string        `yaml:"hec_token"`
	SpanSampleRate              int           `yaml:"span_sample_rate"`
}

type splunkSpanSink struct {
	batchSize          int
	connLifetimeJitter time.Duration
	droppedSpans       uint32
	excludedTags       map[string]struct{}
	hec                *hecClient
	hostname           string
	httpClient         *http.Client
	ingest             chan *Event
	ingestedSpans      uint32
	ingestTimeout      time.Duration
	initOnce           sync.Once
	log                *logrus.Entry
	maxConnLifetime    time.Duration
	name               string
	rand               *mrand.Rand
	sendTimeout        time.Duration
	skippedSpans       uint32
	spanSampleRate     int64
	traceClient        *trace.Client
	workers            int

	// these fields are for testing only:

	// sync holds one channel per submission worker.
	sync []chan struct{}

	// synced is marked Done by each submission worker, when the
	// submission has happened.
	synced sync.WaitGroup
}

var _ sinks.SpanSink = &splunkSpanSink{}
var _ TestableSplunkSpanSink = &splunkSpanSink{}

func MigrateConfig(conf *veneur.Config) error {
	if conf.SplunkHecToken == "" && conf.SplunkHecAddress == "" {
		return nil
	}
	if conf.SplunkHecToken == "" || conf.SplunkHecAddress == "" {
		return fmt.Errorf("both hec_address and hec_token must be set")
	}
	conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
		Kind: "signalfx",
		Name: "signalfx",
		Config: SplunkSinkConfig{
			HecAddress:                  conf.SplunkHecAddress,
			HecBatchSize:                conf.SplunkHecBatchSize,
			HecConnectionLifetimeJitter: conf.SplunkHecConnectionLifetimeJitter,
			HecIngestTimeout:            conf.SplunkHecIngestTimeout,
			HecMaxConnectionLifetime:    conf.SplunkHecMaxConnectionLifetime,
			HecSendTimeout:              conf.SplunkHecSendTimeout,
			HecSubmissionWorkers:        conf.SplunkHecSubmissionWorkers,
			HecTLSValidateHostname:      conf.SplunkHecTLSValidateHostname,
			HecToken:                    conf.SplunkHecToken,
			SpanSampleRate:              conf.SplunkSpanSampleRate,
		},
	})
	return nil
}

// ParseConfig decodes the map config for a Splunk sink into a SplunkSinkConfig
// struct.
func ParseConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	signalFxConfig := SplunkSinkConfig{}
	err := util.DecodeConfig(name, config, &signalFxConfig)
	if err != nil {
		return nil, err
	}
	return signalFxConfig, nil
}

// Create constructs a new splunk span sink from the server
// name and token provided, using the local hostname configured for
// veneur. An optional argument, validateServerName is used (if
// non-empty) to instruct go to validate a different hostname than the
// one on the server URL.
// The spanSampleRate is an integer. For any given trace ID, the probability
// that all spans in the trace will be chosen for the sample is 1/spanSampleRate.
// Sampling is performed on the trace ID, so either all spans within a given trace
// will be chosen, or none will.
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	splunkConfig := sinkConfig.(SplunkSinkConfig)

	if splunkConfig.SpanSampleRate < 1 {
		splunkConfig.SpanSampleRate = 1
	}

	client, err := newHecClient(splunkConfig.HecAddress, splunkConfig.HecToken)
	if err != nil {
		return nil, err
	}

	trnsp := &http.Transport{}
	httpC := &http.Client{Transport: trnsp}

	// keep an idle connection in reserve for every worker:
	trnsp.MaxIdleConnsPerHost = splunkConfig.HecSubmissionWorkers

	if splunkConfig.HecTLSValidateHostname != "" {
		tlsCfg := &tls.Config{}
		tlsCfg.ServerName = splunkConfig.HecTLSValidateHostname
		trnsp.TLSClientConfig = tlsCfg
	}
	if splunkConfig.HecSendTimeout > 0 {
		trnsp.ResponseHeaderTimeout = splunkConfig.HecSendTimeout
	}

	seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return nil, err
	}

	return &splunkSpanSink{
		batchSize:          splunkConfig.HecBatchSize,
		connLifetimeJitter: splunkConfig.HecConnectionLifetimeJitter,
		hec:                client,
		hostname:           config.Hostname,
		httpClient:         httpC,
		ingest:             make(chan *Event),
		ingestTimeout:      splunkConfig.HecIngestTimeout,
		log:                logger,
		maxConnLifetime:    splunkConfig.HecMaxConnectionLifetime,
		name:               name,
		rand:               mrand.New(mrand.NewSource(seed.Int64())),
		sendTimeout:        splunkConfig.HecSendTimeout,
		spanSampleRate:     int64(splunkConfig.SpanSampleRate),
		workers:            splunkConfig.HecSubmissionWorkers,
	}, nil
}

// Name returns this sink's name
func (sink *splunkSpanSink) Name() string {
	return sink.name
}

func (sss *splunkSpanSink) Start(cl *trace.Client) error {
	sss.traceClient = cl

	workers := 1
	if sss.workers > 0 {
		workers = sss.workers
	}

	sss.sync = make([]chan struct{}, workers)

	ready := make(chan struct{})
	for i := 0; i < workers; i++ {
		ch := make(chan struct{})
		go sss.submitter(ch, ready)
		sss.sync[i] = ch
	}

	<-ready
	return nil
}

func (sss *splunkSpanSink) Stop() {
	for _, signal := range sss.sync {
		close(signal)
	}
}

func (sss *splunkSpanSink) Sync() {
	sss.synced.Add(len(sss.sync))
	for _, signal := range sss.sync {
		signal <- struct{}{}
	}
	sss.synced.Wait()
}

// submitter runs for the lifetime of the sink and performs batch-wise
// submission to the HEC sink.
func (sss *splunkSpanSink) submitter(sync chan struct{}, ready chan struct{}) {
	ctx := context.Background()
	for {
		exit := sss.submitBatch(ctx, sync, ready)
		if exit {
			return
		}
	}
}

func (sss *splunkSpanSink) batchTimeout() (time.Duration, bool) {
	lifetime := sss.maxConnLifetime
	if sss.connLifetimeJitter > 0 {
		lifetime += time.Duration(sss.rand.Int63n(int64(sss.connLifetimeJitter)))
	}
	if lifetime > 0 {
		return lifetime, true
	}
	return 0, false
}

// setupHTTPRequest sets up and kicks off an HTTP request. It returns
// the elements of it that are necessary in sending a single batch to
// the HEC.
func (sss *splunkSpanSink) setupHTTPRequest(ctx context.Context) (context.CancelFunc, *hecRequest, io.Writer, error) {
	ctx, cancel := context.WithCancel(ctx)
	hecReq := sss.hec.newRequest()
	req, w, err := hecReq.Start(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}

	// At this point, we have a workable HTTP connection;
	// open it in the background:
	go sss.makeHTTPRequest(req, cancel)
	return cancel, hecReq, w, nil
}

func (sss *splunkSpanSink) submitBatch(ctx context.Context, sync chan struct{}, ready chan struct{}) (exit bool) {
	ingested := 0
	timedOut := 0
	httpCancel, hecReq, w, err := sss.setupHTTPRequest(ctx)
	if err != nil {
		sss.log.WithError(err).
			Warn("Could not create HEC request")
		time.Sleep(1 * time.Second)
		return
	}
	defer hecReq.Close()

	// Set the maximum lifetime of the connection:
	lifetime, ok := sss.batchTimeout()
	if ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, lifetime)
		defer cancel()
	}

	sss.initOnce.Do(func() { close(ready) })
	for {
		select {
		case _, ok := <-sync:
			if !ok {
				// sink is shutting down, exit forever:
				httpCancel()
				exit = true
				return
			}
			sss.synced.Done()
			return
		case <-ctx.Done():
			// batch's max lifetime is reached, let's send it:
			return
		case ev := <-sss.ingest:
			err := sss.submitOneEvent(ctx, w, ev)
			if err != nil {
				if err == io.ErrClosedPipe {
					// Our connection went away. Try to re-establish it:
					return
				}
				if err == context.DeadlineExceeded {
					// Couldn't write the event
					// within timeout, keep going:
					timedOut++
					continue
				}
				sss.log.WithError(err).
					WithField("event", ev).
					WithFields(logrus.Fields{
						"ingested":  ingested,
						"timed_out": timedOut,
					}).
					Warn("Could not json-encode HEC event")
				continue
			}
			ingested++

			if ingested >= sss.batchSize {
				// we consumed the batch size's worth, let's send it:
				return
			}
		}
	}
}

// submitOneEvent takes one event and submits it to an HEC HTTP
// connection. It observes the configured splunk_hec_ingest_timeout -
// if the timeout is exceeded, it returns an error. If the timeout is
// 0, it waits forever to submit the event.
func (sss *splunkSpanSink) submitOneEvent(ctx context.Context, w io.Writer, ev *Event) error {
	if sss.sendTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sss.sendTimeout)
		defer cancel()
	}
	encodeErrors := make(chan error)
	enc := json.NewEncoder(w)
	go func() {
		err := enc.Encode(ev)
		select {
		case encodeErrors <- err:
		case <-ctx.Done():
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-encodeErrors:
		return err
	}
}

func (sss *splunkSpanSink) makeHTTPRequest(req *http.Request, cancel func()) {
	samples := &ssf.Samples{}
	defer metrics.Report(sss.traceClient, samples)
	const successMetric = "splunk.hec_submission_success_total"
	const emptyMetric = "splunk.hec_submission_empty_total"
	const failureMetric = "splunk.hec_submission_failed_total"
	const timingMetric = "splunk.span_submission_lifetime_ns"
	start := time.Now()
	defer func() {
		cancel()
		samples.Add(ssf.Timing(timingMetric, time.Since(start),
			time.Nanosecond, map[string]string{}))
	}()

	resp, err := sss.httpClient.Do(req)
	if uerr, ok := err.(*url.Error); ok && uerr.Timeout() {
		// don't report a sentry-able error for timeouts:
		samples.Add(ssf.Count(failureMetric, 1, map[string]string{
			"cause": "submission_timeout",
		}))
		return
	}
	if err != nil {
		samples.Add(ssf.Count(failureMetric, 1, map[string]string{
			"cause": "execution",
		}))
		return
	}

	defer func() {
		_, _ = io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	var cause string
	var statusCode int

	// See
	// http://docs.splunk.com/Documentation/Splunk/7.2.0/Data/TroubleshootHTTPEventCollector#Possible_error_codes
	// for a list of possible status codes / error kinds
	switch resp.StatusCode {
	case http.StatusOK:
		// Everything went well - discard the body so the
		// connection stays alive and early-return (the rest
		// of this function is dedicated to error handling):
		samples.Add(ssf.Count(successMetric, 1, map[string]string{}))
		return
	case http.StatusInternalServerError:
		cause = "internal_server_error"
		statusCode = 8
	case http.StatusServiceUnavailable:
		// This status happens when splunk is out of capacity,
		// no need to report a bug or parse the body for it:
		cause = "service_unavailable"
		statusCode = 9
	default:
		// Something else is wrong, let's parse the body and
		// report a detailed error:
		var parsed Response
		dec := json.NewDecoder(resp.Body)
		err := dec.Decode(&parsed)
		if err != nil {
			entry := sss.log.WithError(err).
				WithFields(logrus.Fields{
					"http_status_code": resp.StatusCode,
					"endpoint":         req.URL.String(),
				})
			if sss.log.Level >= logrus.DebugLevel {
				body, _ := ioutil.ReadAll(dec.Buffered())
				entry = entry.WithField("response_body", string(body))
			}
			entry.Warn("Could not parse response from splunk HEC")

			return
		}
		cause = "error"
		statusCode = parsed.Code
		if statusCode == 5 {
			// "No data": This is peaceful and indicates
			// that no data was sent over a connection
			// that we closed.
			samples.Add(ssf.Count(emptyMetric, 1, map[string]string{}))
			return
		}
		sss.log.WithFields(logrus.Fields{
			"http_status_code":  resp.StatusCode,
			"hec_status_code":   parsed.Code,
			"hec_response_text": parsed.Text,
			"event_number":      parsed.InvalidEventNumber,
		}).Warn("Error response from Splunk HEC. (Splunk restarts may cause transient errors).")
	}
	samples.Add(ssf.Count(failureMetric, 1, map[string]string{
		"cause":       cause,
		"status_code": strconv.Itoa(statusCode),
	}))
}

// Flush takes the batched-up events and sends them to the HEC
// endpoint for ingestion. If set, it uses the send timeout configured
// for the span batch.
func (sss *splunkSpanSink) Flush() {
	// report the sink stats:
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
		ssf.Count(
			sinks.MetricKeyTotalSpansSkipped,
			float32(atomic.SwapUint32(&sss.skippedSpans, 0)),
			map[string]string{"sink": sss.Name()},
		),
	)

	metrics.Report(sss.traceClient, samples)
}

// Ingest takes in a span and batches it up to be sent in the next
// Flush() iteration.
func (sss *splunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	// Only send properly filled-out spans to the HEC:
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	// wouldDrop indicates whether this span would be dropped
	// according to the sample rate. (1/spanSampleRate) spans
	// will be kept. `wouldDrop` is marked on Splunk events
	// below when it's true
	wouldDrop := ssfSpan.TraceId%sss.spanSampleRate != 0

	// indicator spans are never sampled
	if wouldDrop && !ssfSpan.Indicator {
		atomic.AddUint32(&sss.skippedSpans, 1)
		return nil
	}

	// If the span has any of the disallowed tags,
	// the entire span should be skipped
	for k := range sss.excludedTags {
		if _, ok := ssfSpan.Tags[k]; ok {
			return nil
		}
	}

	ctx := context.Background()
	if sss.ingestTimeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, sss.ingestTimeout)
		defer cancel()
	}

	serialized := SerializedSSF{
		TraceId:        strconv.FormatInt(ssfSpan.TraceId, 16),
		Id:             strconv.FormatInt(ssfSpan.Id, 16),
		ParentId:       strconv.FormatInt(ssfSpan.ParentId, 16),
		StartTimestamp: float64(ssfSpan.StartTimestamp) / float64(time.Second),
		EndTimestamp:   float64(ssfSpan.EndTimestamp) / float64(time.Second),
		Duration:       ssfSpan.EndTimestamp - ssfSpan.StartTimestamp,
		Error:          ssfSpan.Error,
		Service:        ssfSpan.Service,
		Tags:           ssfSpan.Tags,
		Indicator:      ssfSpan.Indicator,
		Name:           ssfSpan.Name,
	}

	if wouldDrop {
		// if we would have dropped this span, the trace is marked as "partial"
		// this lets us readily search for indicator spans that have full traces
		// we only mark indicator spans this way
		serialized.Partial = &wouldDrop
	}

	event := &Event{
		Event: serialized,
	}
	event.SetTime(time.Unix(0, ssfSpan.StartTimestamp))
	event.SetHost(sss.hostname)
	event.SetSourceType(ssfSpan.Service)

	event.SetTime(time.Unix(0, ssfSpan.StartTimestamp))
	select {
	case sss.ingest <- event:
		atomic.AddUint32(&sss.ingestedSpans, 1)
	case <-ctx.Done():
		atomic.AddUint32(&sss.droppedSpans, 1)
	}
	return nil
}

// SetExcludedTags sets the excluded tag names. Any spans with the
// provided key (name) will be excluded entirely. Unlike other sinks,
// the Splunk sink will skip the entire span, rather than stripping only
// the single tag, because Splunk restricts purely on volume rather than
// tag cardinality.
func (sss *splunkSpanSink) SetExcludedTags(excludes []string) {

	tagsSet := map[string]struct{}{}
	for _, tag := range excludes {
		tagsSet[tag] = struct{}{}
	}
	sss.excludedTags = tagsSet
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
	Partial        *bool             `json:"partial,omitempty"`
}
