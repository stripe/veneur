package veneur

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/samplers"
)

type DroppedMetricsOpts struct {
	format string
}

type droppedMetric struct {
	metrics map[string]map[string]int64
	logger  *logrus.Entry
	cancel  chan bool
	format  string
	mu      sync.RWMutex
}

func (m *droppedMetric) Start(ctx context.Context, d time.Duration) {
	t := time.NewTicker(d)
	go func() {
		for {
			select {
			case <-t.C:
				m.mu.RLock()
				metrics := m.metrics
				m.mu.RUnlock()
				for reason, dm := range metrics {
					fields := logrus.Fields{"reason": reason}
					for k, v := range dm {
						fields[k] = v
					}

					m.logger.Logger.WithFields(fields).Warnf(m.format, "dropped metrics")
				}
			case <-ctx.Done():
				break
			}
		}
	}()
}

func (m *droppedMetric) Track(reason string, metric samplers.InterMetric) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.metrics[reason] == nil {
		m.metrics[reason] = map[string]int64{}
	}

	m.metrics[reason][metric.Name] += 1
}

func NewDroppedMetricsTracker(logger *logrus.Entry, opts ...DroppedMetricsOpts) *droppedMetric {
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
