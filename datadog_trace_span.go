package veneur

type DatadogTraceSpan struct {
	Duration float64  `json:"duration"`
	Meta     struct{} `json:"meta"`
	Metrics  struct{} `json:"metrics"`
	Name     string   `json:"name"`
	ParentID int      `json:"parent_id"`
	Resource string   `json:"resource"`
	Service  string   `json:"service"`
	SpanID   int      `json:"span_id"`
	Start    int      `json:"start"`
	TraceID  int      `json:"trace_id"`
	Type     string   `json:"type"`
}
