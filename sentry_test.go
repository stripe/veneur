package veneur

import (
	"errors"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
)

// returns the result of calling recover() after s.ConsumePanic()
func consumeAndCatchPanic(s *Server) (result interface{}) {
	defer func() {
		result = recover()
	}()
	ConsumePanic(s.TraceClient, s.Hostname, "panic")
	return
}

func TestConsumePanicWithoutSentry(t *testing.T) {
	s := &Server{}
	// does nothing
	ConsumePanic(s.TraceClient, s.Hostname, nil)

	recovered := consumeAndCatchPanic(s)
	if recovered != "panic" {
		t.Error("ConsumePanic should panic", recovered)
	}
}

// Mock transport for capturing sent events.
type transportMock struct {
	events []*sentry.Event
}

func (t *transportMock) Configure(options sentry.ClientOptions) {}

func (t *transportMock) SendEvent(event *sentry.Event) {
	t.events = append(t.events, event)
}

func (t *transportMock) Flush(timeout time.Duration) bool {
	return true
}

func (t *transportMock) Events() []*sentry.Event {
	return t.events
}

func (t *transportMock) ClearEvents() {
	t.events = []*sentry.Event{}
}

func TestConsumePanicWithSentry(t *testing.T) {
	transport := &transportMock{}
	err := sentry.Init(sentry.ClientOptions{
		Transport: transport,
	})
	if err != nil {
		t.Fatal("failed to create sentry client with mock transport:", err)
	}

	s := &Server{}
	ConsumePanic(s.TraceClient, s.Hostname, nil)
	if len(transport.Events()) != 0 {
		t.Error("ConsumePanic(nil) should not send data:", transport.Events())
	}

	recovered := consumeAndCatchPanic(s)
	if recovered != "panic" {
		t.Errorf("ConsumePanic(panic) should panic, got %v instead", recovered)
	}
	if len(transport.Events()) != 1 {
		t.Error("expected 1 packet:", transport.Events())
	}
}

func TestHookWithoutSentry(t *testing.T) {
	// hook with a nil sentry client is used when sentry is disabled
	hook := &SentryHook{}

	entry := &logrus.Entry{}
	// must use Fatal so the call to Fire blocks and we can check the result
	entry.Level = logrus.FatalLevel
	err := hook.Fire(entry)
	if err != nil {
		t.Error("Fire returned an error:", err)
	}
}

func TestHook(t *testing.T) {
	hook := &SentryHook{}
	transport := &transportMock{}
	err := sentry.Init(sentry.ClientOptions{
		Transport: transport,
	})
	if err != nil {
		t.Fatal("failed to create sentry client with mock transport:", err)
	}

	// entry without any tags
	entry := &logrus.Entry{}
	// must use Fatal so the call to Fire blocks and we can check the result
	entry.Level = logrus.FatalLevel
	entry.Time = time.Now()
	entry.Message = "received zero-length trace packet"
	hook.Fire(entry)

	if len(transport.Events()) != 1 {
		t.Fatal("expected 1 packet", transport.Events())
	}
	if len(transport.Events()[0].Extra) != 0 {
		t.Error("expected no extra tags", transport.Events()[0].Extra)
	}
	transport.ClearEvents()

	// entry with an error tag that gets stripped
	entry.Data = map[string]interface{}{logrus.ErrorKey: errors.New("some error")}
	hook.Fire(entry)
	if len(transport.Events()) != 1 {
		t.Fatal("expected 1 packet", transport.Events())
	}
	if len(transport.Events()[0].Extra) != 0 {
		t.Error("expected no extra tags", transport.Events()[0].Extra)
	}
}
