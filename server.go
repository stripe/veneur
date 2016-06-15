package veneur

import (
	"github.com/DataDog/datadog-go/statsd"
)

type Server struct {
	Workers []*Worker

	Stats *statsd.Client

	Hostname string

	DDHostname string
	DDAPIKey   string
}
