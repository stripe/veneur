package veneur

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

// ConsumePanic is intended to be called inside a deferred function when recovering
// from a panic. It accepts the value of recover() as its only argument,
// and reports the panic to Sentry, prints the stack,  and then repanics (to ensure your program terminates)
func ConsumePanic(cl *trace.Client, hostname string, err interface{}) {
	if err == nil {
		return
	}

	if sentry.CurrentHub().Client() != nil {
		event := sentry.NewEvent()
		event.Level = sentry.LevelFatal
		event.ServerName = hostname

		stacktrace := sentry.NewStacktrace()
		if len(stacktrace.Frames) >= 2 {
			// Very carefully, filter out the frame for ConsumePanic itself,
			// and the frame for the deferred function that invoked
			// ConsumePanic.
			stacktrace.Frames = stacktrace.Frames[:len(stacktrace.Frames)-2]
		}

		event.Exception = []sentry.Exception{
			sentry.Exception{
				Stacktrace: stacktrace,
			},
		}

		switch e := err.(type) {
		case error:
			event.Message = e.Error()
		case fmt.Stringer:
			event.Message = e.String()
		default:
			event.Message = fmt.Sprintf("%#v", e)
		}

		sentry.CaptureEvent(event)
		// TODO: what happens when we time out? We don't want it to hang.
		sentry.Flush(10 * time.Second)

		metrics.ReportOne(cl, ssf.Count("sentry.errors_total", 1, nil))
	}

	panic(err)
}

// logrus hook to send error/fatal/panic messages to sentry
type sentryHook struct {
	hostname string
	lv       []logrus.Level
}

var _ logrus.Hook = sentryHook{}

func (s sentryHook) Levels() []logrus.Level {
	return s.lv
}

func (s sentryHook) Fire(e *logrus.Entry) error {
	if sentry.CurrentHub().Client() == nil {
		return nil
	}

	event := sentry.NewEvent()
	event.ServerName = s.hostname

	stacktrace := sentry.NewStacktrace()
	if len(stacktrace.Frames) >= 2 {
		// Very carefully, filter out the frame for ConsumePanic itself,
		// and the frame for the deferred function that invoked
		// ConsumePanic.
		stacktrace.Frames = stacktrace.Frames[:len(stacktrace.Frames)-2]
	}
	event.Exception = []sentry.Exception{
		sentry.Exception{
			Stacktrace: stacktrace,
		},
	}

	packetExtraLength := len(e.Data)
	if err, ok := e.Data[logrus.ErrorKey].(error); ok {
		event.Message = err.Error()
		// don't send the error as an extra field
		packetExtraLength--
	} else {
		event.Message = e.Message
	}

	event.Extra = make(map[string]interface{}, packetExtraLength)
	for k, v := range e.Data {
		if k == logrus.ErrorKey {
			continue // already handled this key, don't put it into the Extra hash
		}
		event.Extra[k] = v
	}

	switch e.Level {
	case logrus.FatalLevel, logrus.PanicLevel:
		event.Level = sentry.LevelFatal
	case logrus.ErrorLevel:
		event.Level = sentry.LevelError
	case logrus.WarnLevel:
		event.Level = sentry.LevelWarning
	case logrus.InfoLevel:
		event.Level = sentry.LevelInfo
	case logrus.DebugLevel:
		event.Level = sentry.LevelDebug
	}

	sentry.CaptureEvent(event)
	if e.Level == logrus.PanicLevel || e.Level == logrus.FatalLevel {
		// TODO: what to do when timed out?
		sentry.Flush(10 * time.Second)
	}
	return nil
}
