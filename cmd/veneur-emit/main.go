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

// MinimalStatsd is a more easily dependency-injectable interface
// over statsd, containing only the functionality that this
// project uses.
// type MinimalStatsd interface {
// 	New(addr string) (*statsd.Client, error)
// }

// MinimalClient
type MinimalClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Timing(name string, value time.Duration, tags []string, rate float64) error
	TimeInMilliseconds(name string, value float64, tags []string, rate float64) error
}

func main() {
	flag.Parse()

	// hacky way to detect which flags were *actually* set
	passedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		passedFlags[f.Name] = true
		// fmt.Printf("%s \t\t %s\n", f.Name, f.Value)
	})

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

	var conn MinimalClient
	var err error
	conn, err = statsd.New(addr)
	// conn = (*MinimalClient)(tconn)
	// fmt.Println(reflect.TypeOf(conn))
	if err != nil {
		logrus.Fatal("ERROR")
	}

	var tags []string
	for _, elem := range strings.Split(*tag, ",") {
		tags = append(tags, elem)
	}
	err = sendMetrics(conn, passedFlags, *name, tags)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}
}

func sendMetrics(client MinimalClient, passedFlags map[string]bool, name string, tags []string) error {
	var err error
	if passedFlags["gauge"] {
		logrus.Infof("Sending gauge '%s' -> %f", name, *gauge)
		err = client.Gauge(name, *gauge, tags, 1)
	}
	if passedFlags["timing"] {
		err = client.Timing(name, *timing, tags, 1)
		logrus.Infof("Sending timing '%s' -> %f", name, *timing)
	}
	if passedFlags["timeinms"] {
		err = client.TimeInMilliseconds(name, *timeinms, tags, 1)
		logrus.Infof("Sending timeinms '%s' -> %f", name, *timeinms)
	}
	if passedFlags["count"] {
		err = client.Count(name, *count, tags, 1)
		logrus.Infof("Sending count '%s' -> %d", name, *count)
	}
	return err
}
