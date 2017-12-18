package veneur

import (
	"errors"
	"testing"
	"time"

	"github.com/getsentry/raven-go"
	"github.com/sirupsen/logrus"
)

// returns the result of calling recover() after s.ConsumePanic()
func consumeAndCatchPanic(s *Server) (result interface{}) {
	defer func() {
		result = recover()
	}()
	ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, "panic")
	return
}

func TestConsumePanicWithoutSentry(t *testing.T) {
	s := &Server{}
	// does nothing
	ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, nil)

	recovered := consumeAndCatchPanic(s)
	if recovered != "panic" {
		t.Error("ConsumePanic should panic", recovered)
	}
}

type fakeSentryTransport struct {
	packets []*raven.Packet
}

func (t *fakeSentryTransport) Send(url string, authHeader string, packet *raven.Packet) error {
	t.packets = append(t.packets, packet)
	return nil
}

func TestConsumePanicWithSentry(t *testing.T) {
	s := &Server{}
	var err error
	s.Sentry, err = raven.NewClient("", nil)
	if err != nil {
		t.Fatal("failed to create sentry client:", err)
	}
	fakeTransport := &fakeSentryTransport{}
	s.Sentry.Transport = fakeTransport

	// nil does nothing
	ConsumePanic(s.Sentry, s.TraceClient, s.Hostname, nil)
	if len(fakeTransport.packets) != 0 {
		t.Error("ConsumePanic(nil) should not send data:", fakeTransport.packets)
	}

	recovered := consumeAndCatchPanic(s)
	if recovered != "panic" {
		t.Error("ConsumePanic should panic", recovered)
	}
	if len(fakeTransport.packets) != 1 {
		t.Error("expected 1 packet:", fakeTransport.packets)
	}
}

func TestHookWithoutSentry(t *testing.T) {
	// hook with a nil sentry client is used when sentry is disabled
	hook := &sentryHook{}

	entry := &logrus.Entry{}
	// must use Fatal so the call to Fire blocks and we can check the result
	entry.Level = logrus.FatalLevel
	err := hook.Fire(entry)
	if err != nil {
		t.Error("Fire returned an error:", err)
	}
}

func TestHook(t *testing.T) {
	hook := &sentryHook{}
	var err error
	hook.c, err = raven.NewClient("", nil)
	if err != nil {
		t.Fatal("error creating sentry client:", err)
	}
	fakeTransport := &fakeSentryTransport{}
	hook.c.Transport = fakeTransport

	// entry without any tags
	entry := &logrus.Entry{}
	// must use Fatal so the call to Fire blocks and we can check the result
	entry.Level = logrus.FatalLevel
	entry.Time = time.Now()
	entry.Message = "received zero-length trace packet"
	hook.Fire(entry)

	if len(fakeTransport.packets) != 1 {
		t.Fatal("expected 1 packet", fakeTransport.packets)
	}
	if len(fakeTransport.packets[0].Extra) != 0 {
		t.Error("expected no extra tags", fakeTransport.packets[0].Extra)
	}
	fakeTransport.packets = nil

	// entry with an error tag that gets stripped
	entry.Data = map[string]interface{}{logrus.ErrorKey: errors.New("some error")}
	hook.Fire(entry)
	if len(fakeTransport.packets) != 1 {
		t.Fatal("expected 1 packet", fakeTransport.packets)
	}
	if len(fakeTransport.packets[0].Extra) != 0 {
		t.Error("expected no extra tags", fakeTransport.packets[0].Extra)
	}
	fakeTransport.packets = nil
}
