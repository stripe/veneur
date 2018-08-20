package splunk

type SplunkSpanSink struct {
}

// Name rutrns this sink's name
func (SplunkSpanSink) Name() string {
	return "splunk"
}

// Ingesttakes in a span and passes it to Splunk using the
// HTTP Event Collector
func (sss *SplunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
}
