package veneur

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

// ConsumePanic is intended to be called inside a deferred function when recovering
// from a panic. It accepts the value of recover() as its only argument,
// and reports the panic to Sentry, prints the stack,  and then repanics (to ensure your program terminates)
func ConsumePanic(sentry *raven.Client, recorder *Recorder, hostname string, err interface{}) {
	if err == nil {
		return
	}

	if sentry != nil {
		p := raven.Packet{
			Level:      raven.FATAL,
			ServerName: hostname,
			Interfaces: []raven.Interface{
				// ignore 2 stack frames:
				// - the frame for ConsumePanic itself
				// - the frame for the deferred function that invoked ConsumePanic
				raven.NewStacktrace(2, 3, []string{"main", "github.com/stripe/veneur"}),
			},
		}

		// remember to block, since we're about to re-panic, which will probably terminate
		switch e := err.(type) {
		case error:
			p.Message = e.Error()
		case fmt.Stringer:
			p.Message = e.String()
		default:
			p.Message = fmt.Sprintf("%#v", e)
		}

		_, ch := sentry.Capture(&p, nil)
		recorder.SentryErrorCount()

		// we don't want the program to terminate before reporting to sentry
		<-ch
	}

	panic(err)
}

// logrus hook to send error/fatal/panic messages to sentry
type sentryHook struct {
	c        *raven.Client
	hostname string
	lv       []logrus.Level
}

var _ logrus.Hook = sentryHook{}

func (s sentryHook) Levels() []logrus.Level {
	return s.lv
}

func (s sentryHook) Fire(e *logrus.Entry) error {
	if s.c == nil {
		// raven.Client works when it is nil, but skip the useless work and don't hang on Fatal
		return nil
	}

	p := raven.Packet{
		ServerName: s.hostname,
		Interfaces: []raven.Interface{
			// ignore the stack frames for the Fire function itself
			// the logrus machinery that invoked Fire will also be hidden
			// because it is not an "in-app" library
			raven.NewStacktrace(2, 3, []string{"main", "github.com/stripe/veneur"}),
		},
	}

	packetExtraLength := len(e.Data)
	if err, ok := e.Data[logrus.ErrorKey].(error); ok {
		p.Message = err.Error()
		// don't send the error as an extra field
		packetExtraLength--
	} else {
		p.Message = e.Message
	}

	p.Extra = make(map[string]interface{}, packetExtraLength)
	for k, v := range e.Data {
		if k == logrus.ErrorKey {
			continue // already handled this key, don't put it into the Extra hash
		}
		p.Extra[k] = v
	}

	switch e.Level {
	case logrus.FatalLevel, logrus.PanicLevel:
		p.Level = raven.FATAL
	case logrus.ErrorLevel:
		p.Level = raven.ERROR
	case logrus.WarnLevel:
		p.Level = raven.WARNING
	case logrus.InfoLevel:
		p.Level = raven.INFO
	case logrus.DebugLevel:
		p.Level = raven.DEBUG
	}

	_, ch := s.c.Capture(&p, nil)

	if e.Level == logrus.PanicLevel || e.Level == logrus.FatalLevel {
		// we don't want the program to terminate before reporting to sentry
		return <-ch
	}
	return nil
}
