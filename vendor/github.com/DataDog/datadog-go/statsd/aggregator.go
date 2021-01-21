package statsd

import (
	"strings"
	"sync"
	"time"
)

type (
	countsMap map[string]*countMetric
	gaugesMap map[string]*gaugeMetric
	setsMap   map[string]*setMetric
)

type aggregator struct {
	client *Client

	counts  countsMap
	countsM sync.RWMutex

	gauges  gaugesMap
	gaugesM sync.RWMutex

	sets  setsMap
	setsM sync.RWMutex

	closed chan struct{}
	exited chan struct{}
}

func newAggregator(c *Client) *aggregator {
	return &aggregator{
		client: c,
		counts: countsMap{},
		gauges: gaugesMap{},
		sets:   setsMap{},
		closed: make(chan struct{}),
		exited: make(chan struct{}),
	}
}

func (a *aggregator) start(flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				a.sendMetrics()
			case <-a.closed:
				close(a.exited)
				return
			}
		}
	}()
}

func (a *aggregator) sendMetrics() {
	for _, m := range a.flushMetrics() {
		a.client.send(m)
	}
}

func (a *aggregator) stop() {
	close(a.closed)
	<-a.exited
	a.sendMetrics()
}

func (a *aggregator) flushMetrics() []metric {
	metrics := []metric{}

	// We reset the values to avoid sending 'zero' values for metrics not
	// sampled during this flush interval

	a.setsM.Lock()
	sets := a.sets
	a.sets = setsMap{}
	a.setsM.Unlock()

	for _, s := range sets {
		metrics = append(metrics, s.flushUnsafe()...)
	}

	a.gaugesM.Lock()
	gauges := a.gauges
	a.gauges = gaugesMap{}
	a.gaugesM.Unlock()

	for _, g := range gauges {
		metrics = append(metrics, g.flushUnsafe())
	}

	a.countsM.RLock()
	counts := a.counts
	a.counts = countsMap{}
	a.countsM.RUnlock()

	for _, c := range counts {
		metrics = append(metrics, c.flushUnsafe())
	}

	return metrics
}

func getContext(name string, tags []string) string {
	return name + ":" + strings.Join(tags, ",")
}

func (a *aggregator) count(name string, value int64, tags []string, rate float64) error {
	context := getContext(name, tags)
	a.countsM.RLock()
	if count, found := a.counts[context]; found {
		count.sample(value)
		a.countsM.RUnlock()
		return nil
	}
	a.countsM.RUnlock()

	a.countsM.Lock()
	a.counts[context] = newCountMetric(name, value, tags, rate)
	a.countsM.Unlock()
	return nil
}

func (a *aggregator) gauge(name string, value float64, tags []string, rate float64) error {
	context := getContext(name, tags)
	a.gaugesM.RLock()
	if gauge, found := a.gauges[context]; found {
		gauge.sample(value)
		a.gaugesM.RUnlock()
		return nil
	}
	a.gaugesM.RUnlock()

	gauge := newGaugeMetric(name, value, tags, rate)

	a.gaugesM.Lock()
	a.gauges[context] = gauge
	a.gaugesM.Unlock()
	return nil
}

func (a *aggregator) set(name string, value string, tags []string, rate float64) error {
	context := getContext(name, tags)
	a.setsM.RLock()
	if set, found := a.sets[context]; found {
		set.sample(value)
		a.setsM.RUnlock()
		return nil
	}
	a.setsM.RUnlock()

	a.setsM.Lock()
	a.sets[context] = newSetMetric(name, value, tags, rate)
	a.setsM.Unlock()
	return nil
}
