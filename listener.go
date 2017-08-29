package veneur

import (
	"github.com/armon/go-metrics"
	"github.com/armon/go-metrics/circonus"
	datadog "github.com/armon/go-metrics/datadog"
	prometheus "github.com/armon/go-metrics/prometheus"
	"github.com/stripe/veneur/samplers"
)

// Listener uses a go-metrics MetricSink interface to syndicate UDPMetric
// metrics to monitoring systems external to veneur
type Listener struct {

	// leading article
	ListenerType string
	MetricChan   chan samplers.UDPMetric
	MetricSink   interface {
		// A Gauge should retain the last value it is set to
		SetGauge(key []string, val float32)

		// Should emit a Key/Value pair for each call
		EmitKey(key []string, val float32)

		// Counters should accumulate values
		IncrCounter(key []string, val float32)

		// Samples are for timing information, where quantiles are used
		AddSample(key []string, val float32)
	}
}

// NewListener creates a new Listener instance
// See https://github.com/armon/go-metrics for supported types
func NewListener(name string, conf Config) (*Listener, error) {

	listener := Listener{
		ListenerType: name,
		MetricChan:   make(chan samplers.UDPMetric),
	}

	switch name {
	case "circonus":
		// https://github.com/circonus-labs/circonus-gometrics/blob/master/OPTIONS.md
		cfg := &circonus.Config{}
		cfg.CheckManager.API.TokenKey = conf.CirconusListenerKey
		cfg.CheckManager.Check.InstanceID = conf.CirconusListenerInstance
		cfg.CheckManager.Check.Tags = conf.CirconusListenerTags

		sink, err := circonus.NewCirconusSink(cfg)
		if err != nil {
			log.WithField("error", err).Error("error creating sink")
			return nil, err
		}
		sink.Start()
		listener.MetricSink = sink

	case "datadog":
		sink, err := datadog.NewDogStatsdSink(conf.DataDogListenerAddr, conf.DataDogListenerHost)
		if err != nil {
			log.WithField("error", err).Error("error creating datadog sink")
			return nil, err
		}
		listener.MetricSink = sink

	case "prometheus":
		sink, err := prometheus.NewPrometheusSink()
		if err != nil {
			log.WithField("error", err).Error("error creating prometheus sink")
			return nil, err
		}
		listener.MetricSink = sink

	case "statsd":
		sink, err := metrics.NewStatsdSink(conf.StatsDListenerAddr)
		if err != nil {
			log.WithField("error", err).Error("error creating datadog sink")
			return nil, err
		}
		listener.MetricSink = sink
	}

	return &listener, nil
}

// Listen reads UDPMetrics from the metric channel and writes them to the go-metrics interface
func (l *Listener) Listen() {

	log.WithField("listener", l).Debug("listener listening")

	for {
		select {
		case m := <-l.MetricChan:
			switch m.Type {
			case "counter":
				l.MetricSink.IncrCounter([]string{m.MetricKey.Name}, float32(m.Value.(float64)))

			case "gauge":
				l.MetricSink.SetGauge([]string{m.MetricKey.Name}, float32(m.Value.(float64)))

			case "timer":
				l.MetricSink.SetGauge([]string{m.MetricKey.Name}, float32(m.Value.(float64)))
			}
		}
	}
}
