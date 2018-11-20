package metricingester

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/stripe/veneur/trace"

	"github.com/stripe/veneur/samplers"
)

type AggregatingIngestor struct {
	workers []aggWorker
	flusher flusher
	ticker  *time.Ticker
	tickerC <-chan time.Time
	quit    chan struct{}
	logger  *logrus.Logger
	tc      *trace.Client
}

type flusher interface {
	Flush(ctx context.Context, envelope samplerEnvelope)
}

type ingesterOption func(AggregatingIngestor) AggregatingIngestor

// Override the ticker channel that triggers flushing. Useful for testing.
func OptFlushChan(tckr <-chan time.Time) ingesterOption {
	return func(option AggregatingIngestor) AggregatingIngestor {
		option.tickerC = tckr
		return option
	}
}

func OptLogger(logger *logrus.Logger) ingesterOption {
	return func(option AggregatingIngestor) AggregatingIngestor {
		option.logger = logger
		return option
	}
}

func OptTraceClient(tc *trace.Client) ingesterOption {
	return func(option AggregatingIngestor) AggregatingIngestor {
		option.tc = tc
		return option
	}
}

// NewFlushingIngester creates an ingester that flushes metrics to the specified sinks.
func NewFlushingIngester(
	workers int,
	interval time.Duration,
	sinks []Sink,
	percentiles []float64,
	aggregates samplers.Aggregate,
	options ...ingesterOption,
) AggregatingIngestor {
	var aggW []aggWorker
	for i := 0; i < workers; i++ {
		aggW = append(aggW, newAggWorker())
	}

	t := time.NewTicker(interval)
	ing := AggregatingIngestor{
		workers: aggW,
		ticker:  t,
		tickerC: t.C,
		quit:    make(chan struct{}),
		logger:  logrus.StandardLogger(),
		tc:      trace.DefaultClient,
	}
	for _, opt := range options {
		ing = opt(ing)
	}

	flusher := sinkFlusher{
		sinks:       sinks,
		percentiles: percentiles,
		aggregates:  samplers.HistogramAggregates{aggregates, 4},
		log:         ing.logger,
		tc:          ing.tc,
	}
	ing.flusher = flusher
	return ing
}

// TODO(clin): This needs to take ctx.
func (a AggregatingIngestor) Ingest(m Metric) error {
	workerid := m.Hash() % metricHash(len(a.workers))
	a.workers[workerid].Ingest(m)
	return nil
}

func (a AggregatingIngestor) Merge(d Digest) error {
	var workerid metricHash
	if d.digestType == mixedHistoDigest || d.digestType == mixedGlobalHistoDigest {
		workerid = d.MixedHash() % metricHash(len(a.workers))
	} else {
		workerid = d.Hash() % metricHash(len(a.workers))
	}
	a.workers[workerid].Merge(d)
	return nil
}

func (a AggregatingIngestor) Start() {
	for _, w := range a.workers {
		w.Start()
	}

	go func() {
		for {
			select {
			case <-a.tickerC:
				a.flush()
			case <-a.quit:
				return
			}
		}
	}()
}

func (a AggregatingIngestor) Stop() {
	// nb: tickers must be explicitly stopped to be GCed.
	a.ticker.Stop()
	close(a.quit)
	for _, w := range a.workers {
		w.Stop()
	}
}

func (a AggregatingIngestor) flush() {
	for _, w := range a.workers {
		go func(worker aggWorker) {
			span, ctx := trace.StartSpanFromContext(context.Background(), "flush")
			defer span.Finish()
			a.flusher.Flush(ctx, worker.Flush())
		}(w)
	}
}
