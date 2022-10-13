package veneur

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/samplers"
)

type DroppedMetrics interface {
	Start(time.Duration)
	Stop()
	Track(reason string, metric samplers.InterMetric)
}

type DroppedMetricsOpts struct {
	format string
}

type droppedMetric struct {
	metrics map[string]map[string]int64
	logger  *logrus.Entry
	cancel  chan bool
	format  string
}

func (m *droppedMetric) Stop() {
	m.cancel <- true
}

func (m *droppedMetric) Start(d time.Duration) {
	t := time.NewTicker(d)
	go func() {
		for {
			select {
			case <-m.cancel:
				break
			case <-t.C:
				metrics := m.metrics
				for reason, dm := range metrics {
					fields := logrus.Fields{"reason": reason}
					for k, v := range dm {
						fields[k] = v
					}

					m.logger.Logger.WithFields(fields).Warnf(m.format, "dropped metrics")
				}
			}
		}
	}()
}

func (m *droppedMetric) Track(reason string, metric samplers.InterMetric) {
	if m.metrics[reason] == nil {
		m.metrics[reason] = map[string]int64{}
	}

	m.metrics[reason][metric.Name] += 1
}

func NewDroppedMetricsTracker(logger *logrus.Entry, opts ...DroppedMetricsOpts) DroppedMetrics {
	format := "%s"
	if len(opts) > 0 && opts[0].format != "" {
		format = opts[0].format
	}

	return &droppedMetric{
		metrics: map[string]map[string]int64{},
		logger:  logger,
		format:  format,
	}
}
