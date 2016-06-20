package veneur

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

// call inside deferred recover, eg
// defer func() {
// 	ConsumePanic(recover())
// }
// will report panic to sentry, print stack and then repanic (to ensure your program terminates)
func (s *Server) ConsumePanic(err interface{}) {
	if err == nil {
		return
	}

	p := raven.Packet{
		Level:      raven.FATAL,
		ServerName: s.Hostname,
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

	_, ch := s.sentry.Capture(&p, nil)
	// we don't want the program to terminate before reporting to sentry
	<-ch

	panic(err)
}

// logrus hook to send error/fatal/panic messages to sentry
type sentryHook struct {
	c        *raven.Client
	hostname string
	lv       []log.Level
}

var _ log.Hook = sentryHook{}

func (s sentryHook) Levels() []log.Level {
	return s.lv
}

func (s sentryHook) Fire(e *log.Entry) error {
	p := raven.Packet{
		ServerName: s.hostname,
		Interfaces: []raven.Interface{
			// ignore the stack frames for the Fire function itself
			// the logrus machinery that invoked Fire will also be hidden
			// because it is not an "in-app" library
			raven.NewStacktrace(2, 3, []string{"main", "github.com/stripe/veneur"}),
		},
	}

	if err, ok := e.Data[log.ErrorKey].(error); ok {
		p.Message = err.Error()
	} else {
		p.Message = e.Message
	}

	p.Extra = make(map[string]interface{}, len(e.Data)-1)
	for k, v := range e.Data {
		if k == log.ErrorKey {
			continue // already handled this key, don't put it into the Extra hash
		}
		p.Extra[k] = v
	}

	switch e.Level {
	case log.FatalLevel, log.PanicLevel:
		p.Level = raven.FATAL
	case log.ErrorLevel:
		p.Level = raven.ERROR
	case log.WarnLevel:
		p.Level = raven.WARNING
	case log.InfoLevel:
		p.Level = raven.INFO
	case log.DebugLevel:
		p.Level = raven.DEBUG
	}

	_, ch := s.c.Capture(&p, nil)

	if e.Level == log.PanicLevel || e.Level == log.FatalLevel {
		// we don't want the program to terminate before reporting to sentry
		return <-ch
	} else {
		return nil
	}
}
