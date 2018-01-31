package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func TestEmptyMetrics(t *testing.T) {
	err := Report(trace.DefaultClient, []*ssf.SSFSample{})
	assert.Error(t, err)
	assert.IsType(t, NoMetrics{}, err)

	assert.Error(t, ReportAsync(trace.DefaultClient, []*ssf.SSFSample{}, nil))
	assert.Error(t, err)
	assert.IsType(t, NoMetrics{}, err)
}
