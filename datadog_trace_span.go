package veneur

// DatadogTraceSpan represents a trace span as JSON for the
// Datadog tracing API.
type DatadogTraceSpan struct {
	Duration int64              `json:"duration"`
	Error    int64              `json:"error"`
	Meta     map[string]string  `json:"meta"`
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
