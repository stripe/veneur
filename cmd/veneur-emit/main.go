package main

import (
	"errors"
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
	debug      = flag.Bool("debug", false, "Turns on debug messages.")
)

// MinimalClient represents the functions that we call on Clients in veneur-emit.
type MinimalClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Timing(name string, value time.Duration, tags []string, rate float64) error
	TimeInMilliseconds(name string, value float64, tags []string, rate float64) error
}

func main() {
	passedFlags := flags()

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	var config *veneur.Config
	var err error
	if passedFlags["f"] {
		conf, e := veneur.ReadConfig(*configFile)
		if e != nil {
			logrus.WithError(err).Fatal("Error reading configuration file.")
		}
		config = &conf
	}

	addr, err := addr(passedFlags, config, hostport)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}
	logrus.Debugf("destination: %s", addr)

	var conn MinimalClient
	conn, err = statsd.New(addr)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}

	tags := tags(*tag)
	err = sendMetrics(conn, passedFlags, *name, tags)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}
}

func flags() map[string]bool {
	flag.Parse()
	// hacky way to detect which flags were *actually* set
	passedFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) {
		passedFlags[f.Name] = true
	})
	return passedFlags
}

func tags(tag string) []string {
	var tags []string
	for _, elem := range strings.Split(tag, ",") {
		tags = append(tags, elem)
	}
	return tags
}

func addr(passedFlags map[string]bool, conf *veneur.Config, hostport *string) (string, error) {
	addr := ""
	var err error
	if passedFlags["f"] && (conf != nil) {
		addr = conf.UdpAddress // TODO: conf.TcpAddress
	} else if passedFlags["hostport"] && !(hostport == nil || *hostport == "" || !strings.Contains(*hostport, ":")) {
		addr = *hostport
	} else {
		err = errors.New("you must either specify a Veneur config file or a valid hostport")
	}
	return addr, err
}

func sendMetrics(client MinimalClient, passedFlags map[string]bool, name string, tags []string) error {
	var err error
	if passedFlags["gauge"] {
		logrus.Debugf("Sending gauge '%s' -> %f", name, *gauge)
		err = client.Gauge(name, *gauge, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["timing"] {
		logrus.Debugf("Sending timing '%s' -> %f", name, *timing)
		err = client.Timing(name, *timing, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["timeinms"] {
		logrus.Debugf("Sending timeinms '%s' -> %f", name, *timeinms)
		err = client.TimeInMilliseconds(name, *timeinms, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["count"] {
		logrus.Debugf("Sending count '%s' -> %d", name, *count)
		err = client.Count(name, *count, tags, 1)
		if err != nil {
			return err
		}
	}
	if !passedFlags["gauge"] && !passedFlags["timing"] && !passedFlags["timeinms"] && !passedFlags["count"] {
		logrus.Info("No metrics reported.")
	}
	return err
	// TODO: error on no metrics?
}
