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
