package veneur

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
	"github.com/getsentry/raven-go"
)

var Sentry *raven.Client

func InitSentry() {
	if Config.SentryDSN == "" {
		Sentry = nil
		return
	}
	sentry, err := raven.NewWithTags(Config.SentryDSN, nil)
	if err != nil {
		log.WithError(err).Fatal("Error creating sentry client")
	}
	Sentry = sentry
}

// call inside deferred recover, eg
// defer func() {
// 	ConsumePanic(recover())
// }
// will report panic to sentry, print stack and then repanic (to ensure your program terminates)
func ConsumePanic(err interface{}) {
	if err == nil {
		return
	}

	p := raven.Packet{
		Level:      raven.FATAL,
		ServerName: Config.Hostname,
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

	_, ch := Sentry.Capture(&p, nil)
	// we don't want the program to terminate before reporting to sentry
	<-ch

	panic(err)
}
