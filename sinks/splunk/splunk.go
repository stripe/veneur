package splunk

import (
	"context"
	"crypto/tls"
	"net/http"
	"strconv"
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
	client := hec.NewClient(server, token).(*hec.Client)

	tlsCfg := &tls.Config{}
	tlsCfg.InsecureSkipVerify = true // TODO: make the CA cert and host names configurable

	trnsp := &http.Transport{TLSClientConfig: tlsCfg}
	httpC := &http.Client{Transport: trnsp}
	client.SetHTTPClient(httpC)

	return &splunkSpanSink{Client: client}, nil
}

func (*splunkSpanSink) Start(*trace.Client) error {
	return nil
}

func (*splunkSpanSink) Flush() {
	// nothing to do. Eventually, we should submit batches instead.
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

// Splunk can't handle int64s, and we don't
// want to round our traceID to the thousands place.
// This is mildly redundant, but oh well
type serializedSSF struct {
	TraceId        string            `json:"trace_id"`
	Id             string            `json:"id"`
	ParentId       string            `json:"parent_id"`
	StartTimestamp string            `json:"start_timestamp"`
	EndTimestamp   string            `json:"end_timestamp"`
	Error          bool              `json:"error"`
	Service        string            `json:"service"`
	Tags           map[string]string `json:"tags"`
	Indicator      bool              `json:"indicator"`
	Name           string            `json:"name"`
}

func (sss *splunkSpanSink) writeSpan(ctx context.Context, ssfSpan *ssf.SSFSpan) error {

	serialized := serializedSSF{
		TraceId:        strconv.FormatInt(ssfSpan.TraceId, 10),
		Id:             strconv.FormatInt(ssfSpan.Id, 10),
		ParentId:       strconv.FormatInt(ssfSpan.ParentId, 10),
		StartTimestamp: strconv.FormatInt(ssfSpan.StartTimestamp, 10),
		EndTimestamp:   strconv.FormatInt(ssfSpan.EndTimestamp, 10),
		Error:          ssfSpan.Error,
		Service:        ssfSpan.Service,
		Tags:           ssfSpan.Tags,
		Indicator:      ssfSpan.Indicator,
		Name:           ssfSpan.Name,
	}

	start := time.Unix(int64((time.Duration(ssfSpan.StartTimestamp) / time.Second)), 0)

	event := &hec.Event{
		Event: serialized,
	}

	event.SetTime(start)

	return sss.WriteEventWithContext(ctx, event)
}
