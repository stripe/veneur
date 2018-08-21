package splunk

import (
	"context"
	"time"

	hec "github.com/fuyufjh/splunk-hec-go"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type splunkSpanSink struct {
	*hec.Client
}

func NewSplunkSpanSink(server string, token string) (sinks.SpanSink, error) {
	client := hec.NewClient(server, token)
	return &splunkSpanSink{Client: client.(*hec.Client)}, nil
}

func (*splunkSpanSink) Start(*trace.Client) error {
	return nil
}

func (*splunkSpanSink) Flush() {
	// nothing to do.
}

// Name returns this sink's name
func (*splunkSpanSink) Name() string {
	return "splunk"
}

// Ingest takes in a span and passes it to Splunk using the
// HTTP Event Collector
func (sss *splunkSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	// Fake up a context with a reasonable timeout:
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	return sss.writeSpan(ctx, ssfSpan)
}

func (sss *splunkSpanSink) writeSpan(ctx context.Context, ssfSpan *ssf.SSFSpan) error {
	return sss.WriteEventWithContext(ctx, &hec.Event{
		// TODO: We're just serializing the regular SSF span
		// to JSON; we might want to do some translation to
		// fields so they look better in splunk (and also
		// maybe filter out some fields).
		Event: ssfSpan,
	})
}
