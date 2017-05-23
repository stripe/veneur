package main

import (
	"flag"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
)

var (
	hostport  = flag.String("hostport", "", "Hostname and port of destination.")
	namespace = flag.String("ns", "", "Namespace for metrics. Ex: daemontools")
	name      = flag.String("name", "", "Name of metric to report. Ex: service.starts")
	gauge     = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	timing    = flag.Duration("timing", time.Now().Sub(time.Now()), "Report a 'timing' metric. Value must be parseable by time.ParseDuration.")
	timeinms  = flag.Float64("timeinms", 0, "Report a 'timing' metric, in milliseconds. Value must be float64.")
	incr      = flag.Bool("incr", false, "Report an 'incr' metric.")
	decr      = flag.Bool("decr", false, "Report a 'decr' metric.")
	count     = flag.Int64("count", 0, "Report a 'count' metric. Value must be an integer.")
	tag       = flag.String("tag", "", "Tag for metric. Ex: `service:airflow")
)

func main() {
	flag.Parse()

	// hacky way to detect which flags were *actually* set
	passedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { passedFlags[f.Name] = true })

	if hostport == nil || *hostport == "" || !strings.Contains(*hostport, ":") {
		logrus.Fatal("You must specifiy a valid destination host and port.")
	}

	// fmt.Println("hostport: ", *hostport)
	// fmt.Println("namespace: ", *namespace)
	// fmt.Println("name: ", *name)
	// fmt.Println("gauge: ", *gauge)
	// fmt.Println("timing: ", *timing)
	// fmt.Println("timeinms: ", *timeinms)
	// fmt.Println("incr: ", *incr)
	// fmt.Println("decr: ", *decr)
	// fmt.Println("count: ", *count)

	conn, err := statsd.New(*hostport)
	if err != nil {
		panic("ERROR")
	}

	conn.Namespace = *namespace + "."
	conn.Tags = append(conn.Tags, *tag)

	if passedFlags["gauge"] {
		conn.Gauge(*name, *gauge, nil, 1)
	}
	if passedFlags["timing"] {
		conn.Timing(*name, *timing, nil, 1)
	}
	if passedFlags["timeinms"] {
		conn.TimeInMilliseconds(*name, *timeinms, nil, 1)
	}
	if *incr {
		conn.Incr(*name, nil, 1)
	}
	if *decr {
		conn.Decr(*name, nil, 1)
	}
	if passedFlags["count"] {
		conn.Count(*name, *count, nil, 1)
	}
}

// echo "daemontools.service.starts:1|c|#service:<%= @name %>" | nc -q 1 -u <%= @statsd_host %> <%= @statsd_port %>
