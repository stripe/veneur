package veneur

import (
	"github.com/DataDog/datadog-go/statsd"
)

type Server struct {
	Stats *statsd.Client

	Hostname string
}
