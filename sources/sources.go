package sources

import "github.com/stripe/veneur/v14/samplers"

type SourceConfig interface{}

type Source interface {
	Name() string
	Start(ingest Ingest) error
	Stop()
}

type Ingest interface {
	IngestMetric(metric *samplers.UDPMetric)
}
