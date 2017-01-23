package veneur

import (
	"testing"

	"github.com/getsentry/raven-go"
)

// returns the result of calling recover() after s.ConsumePanic()
func consumeAndCatchPanic(s *Server) (result interface{}) {
	defer func() {
		result = recover()
	}()
	s.ConsumePanic("panic")
	return
}

func TestConsumePanicWithoutSentry(t *testing.T) {
	s := &Server{}
	// does nothing
	s.ConsumePanic(nil)

	recovered := consumeAndCatchPanic(s)
	if recovered != "panic" {
		t.Error("ConsumePanic should panic", recovered)
	}
}

type FakeSentryTransport struct {
	packets []*raven.Packet
}

func (t *FakeSentryTransport) Send(url string, authHeader string, packet *raven.Packet) error {
	t.packets = append(t.packets, packet)
	return nil
}

func TestConsumePanicWithSentry(t *testing.T) {
	s := &Server{}
	var err error
	s.sentry, err = raven.NewClient("", nil)
	if err != nil {
		t.Fatal("failed to create sentry client:", err)
	}
	fakeTransport := &FakeSentryTransport{}
	s.sentry.Transport = fakeTransport

	// nil does nothing
	s.ConsumePanic(nil)
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
