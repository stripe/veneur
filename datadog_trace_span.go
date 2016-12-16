package veneur

import "github.com/stripe/veneur/ssf"

type DatadogTraceSpan struct {
	Duration int64              `json:"duration"`
	Error    int64              `json:"error"`
	Meta     []*ssf.SSFTag      `json:"meta"`
	Metrics  map[string]float64 `json:"metrics"`
	Name     string             `json:"name"`
	ParentID int64              `json:"parent_id,omitempty"`
	Resource string             `json:"resource,omitempty"`
	Service  string             `json:"service"`
	SpanID   int64              `json:"span_id"`
	Start    int64              `json:"start"`
	TraceID  int64              `json:"trace_id"`
	Type     string             `json:"type"`
}
