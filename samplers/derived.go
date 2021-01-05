package samplers

import (
	"github.com/stripe/veneur/v14/ssf"
)

// DerivedMetricsProcessor processes any metric created from events or service checks into
// the worker channels for flushing
type DerivedMetricsProcessor interface {
	SendSample(sample *ssf.SSFSample) error
}
