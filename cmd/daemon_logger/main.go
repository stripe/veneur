package main

import (
	"flag"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/stripe/veneur"
)

var (
	configFile = flag.String("f", "", "The Veneur config file to read for settings.")
	hostport   = flag.String("hostport", "", "Hostname and port of destination. Must be used if config file is not present.")
	name       = flag.String("name", "", "Name of metric to report. Ex: daemontools.service.starts")
	gauge      = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	timing     = flag.Duration("timing", time.Now().Sub(time.Now()), "Report a 'timing' metric. Value must be parseable by time.ParseDuration.")
	timeinms   = flag.Float64("timeinms", 0, "Report a 'timing' metric, in milliseconds. Value must be float64.")
	count      = flag.Int64("count", 0, "Report a 'count' metric. Value must be an integer.")
	tag        = flag.String("tag", "", "Tag(s) for metric, comma separated. Ex: service:airflow")
)

func main() {
	flag.Parse()

	// hacky way to detect which flags were *actually* set
	passedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { passedFlags[f.Name] = true })

	addr := ""
	if passedFlags["f"] && !(configFile == nil || *configFile == "") {
		conf, err := veneur.ReadConfig(*configFile)
		if err != nil {
			logrus.WithError(err).Fatal("Error reading config file")
		}
		addr = conf.UdpAddress // TODO: conf.TcpAddress
	} else if passedFlags["hostport"] && !(hostport == nil || *hostport == "" || !strings.Contains(*hostport, ":")) {
		addr = *hostport
	} else {
		logrus.Fatal("You must either specify a Veneur config file or a valid hostport.")
	}

	conn, err := statsd.New(addr)
	if err != nil {
		panic("ERROR")
	}

	for _, elem := range strings.Split(*tag, ",") {
		conn.Tags = append(conn.Tags, elem)
	}

	if passedFlags["gauge"] {
		conn.Gauge(*name, *gauge, nil, 1)
		logrus.Infof("Sending gauge '%s' -> %f", *name, *gauge)
	}
	if passedFlags["timing"] {
		conn.Timing(*name, *timing, nil, 1)
		logrus.Infof("Sending timing '%s' -> %f", *name, *timing)
	}
	if passedFlags["timeinms"] {
		conn.TimeInMilliseconds(*name, *timeinms, nil, 1)
		logrus.Infof("Sending timeinms '%s' -> %f", *name, *timeinms)
	}
	if passedFlags["count"] {
		conn.Count(*name, *count, nil, 1)
		logrus.Infof("Sending count '%s' -> %d", *name, *count)
	}
}

// echo "daemontools.service.starts:1|c|#service:<%= @name %>" | nc -q 1 -u <%= @statsd_host %> <%= @statsd_port %>
