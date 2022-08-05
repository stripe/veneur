package sources

import (
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
)

type SourceConfig interface{}

type Source interface {
	Name() string
	Start(ingest Ingest) error
	Stop()
}

type Ingest interface {
	IngestMetric(metric *samplers.UDPMetric)
	IngestMetricProto(metric *metricpb.Metric)
}
