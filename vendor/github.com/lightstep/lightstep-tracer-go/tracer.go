package lightstep

import (
	"fmt"
	"reflect"
	"time"

	"golang.org/x/net/context"

	"runtime"
	"sync"

	ot "github.com/opentracing/opentracing-go"
)

var (
	errPreviousReportInFlight = fmt.Errorf("a previous Report is still in flight; aborting Flush()")
	errConnectionWasClosed    = fmt.Errorf("the connection was closed")
	errTracerDisabled         = fmt.Errorf("tracer is disabled; aborting Flush()")
)

// FlushLightStepTracer forces a synchronous Flush.
func FlushLightStepTracer(lsTracer ot.Tracer) error {
	tracer, ok := lsTracer.(Tracer)
	if !ok {
		return fmt.Errorf("Not a LightStep Tracer type: %v", reflect.TypeOf(lsTracer))
	}

	tracer.Flush()
	return nil
}

// GetLightStepAccessToken returns the currently configured AccessToken.
func GetLightStepAccessToken(lsTracer ot.Tracer) (string, error) {
	tracer, ok := lsTracer.(Tracer)
	if !ok {
		return "", fmt.Errorf("Not a LightStep Tracer type: %v", reflect.TypeOf(lsTracer))
	}

	return tracer.Options().AccessToken, nil
}

// CloseTracer synchronously flushes the tracer, then terminates it.
func CloseTracer(tracer ot.Tracer) error {
	lsTracer, ok := tracer.(Tracer)
	if !ok {
		return fmt.Errorf("Not a LightStep Tracer type: %v", reflect.TypeOf(tracer))
	}

	return lsTracer.Close()
}

// Implements the `Tracer` interface. Buffers spans and forwards the to a Lightstep collector.
type tracerImpl struct {
	//////////////////////////////////////////////////////////////
	// IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE IMMUTABLE
	//////////////////////////////////////////////////////////////

	// Note: there may be a desire to update some of these fields
	// at runtime, in which case suitable changes may be needed
	// for variables accessed during Flush.

	reporterID       uint64 // the LightStep tracer guid
	opts             Options
	textPropagator   textMapPropagator
	binaryPropagator lightstepBinaryPropagator

	//////////////////////////////////////////////////////////
	// MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE MUTABLE
	//////////////////////////////////////////////////////////

	// the following fields are modified under `lock`.
	lock sync.Mutex

	// Remote service that will receive reports.
	client       collectorClient
	conn         Connection
	closech      chan struct{}
	reportLoopch chan struct{}

	// Two buffers of data.
	buffer   reportBuffer
	flushing reportBuffer

	// Flush state.
	flushingLock      sync.Mutex
	reportInFlight    bool
	lastReportAttempt time.Time

	// We allow our remote peer to disable this instrumentation at any
	// time, turning all potentially costly runtime operations into
	// no-ops.
	//
	// TODO this should use atomic load/store to test disabled
	// prior to taking the lock, do please.
	disabled bool
}

// NewTracer creates and starts a new Lightstep Tracer.
func NewTracer(opts Options) Tracer {
	err := opts.Initialize()
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}

	attributes := map[string]string{}
	for k, v := range opts.Tags {
		attributes[k] = fmt.Sprint(v)
	}
	// Don't let the GrpcOptions override these values. That would be confusing.
	attributes[TracerPlatformKey] = TracerPlatformValue
	attributes[TracerPlatformVersionKey] = runtime.Version()
	attributes[TracerVersionKey] = TracerVersionValue

	now := time.Now()
	impl := &tracerImpl{
		opts:       opts,
		reporterID: genSeededGUID(),
		buffer:     newSpansBuffer(opts.MaxBufferedSpans),
		flushing:   newSpansBuffer(opts.MaxBufferedSpans),
	}

	impl.buffer.setCurrent(now)

	if opts.UseThrift {
		impl.client = newThriftCollectorClient(opts, impl.reporterID, attributes)
	} else {
		impl.client = newGrpcCollectorClient(opts, impl.reporterID, attributes)
	}

	conn, err := impl.client.ConnectClient()

	if err != nil {
		fmt.Println("Failed to connect to Collector!", err)
		return nil
	}

	impl.conn = conn
	impl.closech = make(chan struct{})
	impl.reportLoopch = make(chan struct{})

	// Important! incase close is called before go routine is kicked off
	closech := impl.closech
	go func() {
		impl.reportLoop(closech)
		close(impl.reportLoopch)
	}()

	return impl
}

func (impl *tracerImpl) Options() Options {
	return impl.opts
}

func (t *tracerImpl) StartSpan(
	operationName string,
	sso ...ot.StartSpanOption,
) ot.Span {
	return newSpan(operationName, t, sso)
}

func (t *tracerImpl) Inject(sc ot.SpanContext, format interface{}, carrier interface{}) error {
	switch format {
	case ot.TextMap, ot.HTTPHeaders:
		return t.textPropagator.Inject(sc, carrier)
	case BinaryCarrier:
		return t.binaryPropagator.Inject(sc, carrier)
	}
	return ot.ErrUnsupportedFormat
}

func (t *tracerImpl) Extract(format interface{}, carrier interface{}) (ot.SpanContext, error) {
	switch format {
	case ot.TextMap, ot.HTTPHeaders:
		return t.textPropagator.Extract(carrier)
	case BinaryCarrier:
		return t.binaryPropagator.Extract(carrier)
	}
	return nil, ot.ErrUnsupportedFormat
}

func (r *tracerImpl) reconnectClient(now time.Time) {
	conn, err := r.client.ConnectClient()
	if err != nil {
		maybeLogInfof("could not reconnect client", r.opts.Verbose)
	} else {
		r.lock.Lock()
		oldConn := r.conn
		r.conn = conn
		r.lock.Unlock()

		oldConn.Close()
		maybeLogInfof("reconnected client connection", r.opts.Verbose)
	}
}

// Close flushes and then terminates the LightStep collector.
func (r *tracerImpl) Close() error {
	r.lock.Lock()
	closech := r.closech
	r.closech = nil
	r.lock.Unlock()

	if closech != nil {
		// notify report loop that we are closing
		close(closech)

		// wait for report loop to finish
		if r.reportLoopch != nil {
			<-r.reportLoopch
		}
	}

	// now its safe to close the connection
	r.lock.Lock()
	conn := r.conn
	r.conn = nil
	r.reportLoopch = nil
	r.lock.Unlock()

	if conn == nil {
		return nil
	}

	return conn.Close()
}

// RecordSpan records a finished Span.
func (r *tracerImpl) RecordSpan(raw RawSpan) {
	r.lock.Lock()
	defer r.lock.Unlock()

	// Early-out for disabled runtimes
	if r.disabled {
		return
	}

	r.buffer.addSpan(raw)

	if r.opts.Recorder != nil {
		r.opts.Recorder.RecordSpan(raw)
	}
}

// Flush sends all buffered data to the collector.
func (r *tracerImpl) Flush() {
	r.flushingLock.Lock()
	defer r.flushingLock.Unlock()

	err := r.preFlush()
	if err != nil {
		maybeLogError(err, r.opts.Verbose)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.opts.ReportTimeout)
	defer cancel()
	resp, err := r.client.Report(ctx, &r.flushing)

	if err != nil {
		maybeLogError(err, r.opts.Verbose)
	} else if len(resp.GetErrors()) > 0 {
		// These should never occur, since this library should understand what
		// makes for valid logs and spans, but just in case, log it anyway.
		for _, err := range resp.GetErrors() {
			maybeLogError(fmt.Errorf("Remote report returned error: %s", err), r.opts.Verbose)
		}
	} else {
		maybeLogInfof("Report: resp=%v, err=%v", r.opts.Verbose, resp, err)
	}

	r.postFlush(resp, err)
}

func (r *tracerImpl) preFlush() error {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.disabled {
		return errTracerDisabled
	}

	if r.conn == nil {
		return errConnectionWasClosed
	}

	now := time.Now()
	r.buffer, r.flushing = r.flushing, r.buffer
	r.reportInFlight = true
	r.flushing.setFlushing(now)
	r.buffer.setCurrent(now)
	r.lastReportAttempt = now
	return nil
}

func (r *tracerImpl) postFlush(resp collectorResponse, err error) {
	var droppedSent int64
	r.lock.Lock()
	defer r.lock.Unlock()
	r.reportInFlight = false
	if err != nil {
		// Restore the records that did not get sent correctly
		r.buffer.mergeFrom(&r.flushing)
	} else {
		droppedSent = r.flushing.droppedSpanCount
		r.flushing.clear()

		if resp.Disable() {
			r.Disable()
		}
	}
	if droppedSent != 0 {
		maybeLogInfof("client reported %d dropped spans", r.opts.Verbose, droppedSent)
	}
}

func (r *tracerImpl) Disable() {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.disabled {
		return
	}

	fmt.Printf("Disabling Runtime instance: %p", r)

	r.buffer.clear()
	r.disabled = true
}

// Every MinReportingPeriod the reporting loop wakes up and checks to see if
// either (a) the Runtime's max reporting period is about to expire (see
// maxReportingPeriod()), (b) the number of buffered log records is
// approaching kMaxBufferedLogs, or if (c) the number of buffered span records
// is approaching kMaxBufferedSpans. If any of those conditions are true,
// pending data is flushed to the remote peer. If not, the reporting loop waits
// until the next cycle. See Runtime.maybeFlush() for details.
//
// This could alternatively be implemented using flush channels and so forth,
// but that would introduce opportunities for client code to block on the
// runtime library, and we want to avoid that at all costs (even dropping data,
// which can certainly happen with high data rates and/or unresponsive remote
// peers).
func (r *tracerImpl) shouldFlushLocked(now time.Time) bool {
	if now.Add(r.opts.MinReportingPeriod).Sub(r.lastReportAttempt) > r.opts.ReportingPeriod {
		// Flush timeout.
		maybeLogInfof("--> timeout", r.opts.Verbose)
		return true
	} else if r.buffer.isHalfFull() {
		// Too many queued span records.
		maybeLogInfof("--> span queue", r.opts.Verbose)
		return true
	}
	return false
}

func (r *tracerImpl) reportLoop(closech chan struct{}) {
	tickerChan := time.Tick(r.opts.MinReportingPeriod)
	for {
		select {
		case <-tickerChan:
			now := time.Now()

			r.lock.Lock()
			disabled := r.disabled
			reconnect := !r.reportInFlight && r.client.ShouldReconnect()
			shouldFlush := r.shouldFlushLocked(now)
			r.lock.Unlock()

			if disabled {
				return
			}
			if shouldFlush {
				r.Flush()
			}
			if reconnect {
				r.reconnectClient(now)
			}
		case <-closech:
			r.Flush()
			return
		}
	}
}
