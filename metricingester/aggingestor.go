package metricingester

import "time"

type AggregatingIngestor struct {
	workers  []aggWorker
	flusher  func(samplerEnvelope) error
	interval time.Duration
	quit     chan struct{}
}

// TODO(clin): This needs to take ctx.
func (a AggregatingIngestor) Ingest(m Metric) error {
	workerid := m.Hash() % metricHash(len(a.workers))
	a.workers[workerid].Ingest(m)
	return nil
}

func (a AggregatingIngestor) Merge(d Digest) error {
	workerid := d.Hash() % metricHash(len(a.workers))
	a.workers[workerid].Merge(d)
	return nil
}

func (a AggregatingIngestor) Start() {
	go func() {
		ticker := time.NewTicker(a.interval)
		for {
			select {
			case <-ticker.C:
				a.flush()
			case <-a.quit:
				return
			}
		}
	}()
}

func (a AggregatingIngestor) Stop() {
	close(a.quit)
}

func (a AggregatingIngestor) flush() {
	for _, w := range a.workers {
		go a.flusher(w.Flush())
	}
}
