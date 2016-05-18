package veneur

import (
	"github.com/DataDog/datadog-go/statsd"
	log "github.com/Sirupsen/logrus"
)

// Stats is a global stats Client that all underlying bits will use.
var Stats *statsd.Client

// InitStats creates the DogStatsD client for use inside veneur.
func InitStats() {
	nstats, err := statsd.NewBuffered(Config.StatsAddr, 1024)
	if err != nil {
		log.WithError(err).Fatal("Error creating statsd logging")
	}
	Stats = nstats
	Stats.Namespace = "veneur."
	Stats.Tags = Config.Tags
}
