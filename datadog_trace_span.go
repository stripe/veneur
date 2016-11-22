package veneur

type DatadogTraceSpan struct {
	Duration float64  `json:"duration"`
	Meta     struct{} `json:"meta"`
	Metrics  struct{} `json:"metrics"`
	Name     string   `json:"name"`
	ParentID int64    `json:"parent_id"`
	Resource string   `json:"resource"`
	Service  string   `json:"service"`
	SpanID   int64    `json:"span_id"`
	Start    int64    `json:"start"`
	TraceID  int64    `json:"trace_id"`
	Type     string   `json:"type"`
}
