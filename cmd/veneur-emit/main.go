package main

import (
	"errors"
	"flag"
	"net"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/gogo/protobuf/proto"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/ssf"
)

var (
	configFile = flag.String("f", "", "The Veneur config file to read for settings.")
	hostport   = flag.String("hostport", "", "Hostname and port of destination. Must be used if config file is not present.")
	name       = flag.String("name", "", "Name of metric to report. Ex: daemontools.service.starts")
	gauge      = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	timing     = flag.Duration("timing", time.Now().Sub(time.Now()), "Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	count      = flag.Int64("count", 0, "Report a 'count' metric. Value must be an integer.")
	tag        = flag.String("tag", "", "Tag(s) for metric, comma separated. Ex: service:airflow")
	debug      = flag.Bool("debug", false, "Turns on debug messages.")
)

// MinimalClient represents the functions that we call on Clients in veneur-emit.
type MinimalClient interface {
	Gauge(name string, value float64, tags []string, rate float64) error
	Count(name string, value int64, tags []string, rate float64) error
	Timing(name string, value time.Duration, tags []string, rate float64) error
}

// MinimalConn represents the functions that we call on connections in veneur-emit.
type MinimalConn interface {
	Write([]byte) (int, error)
}

func main() {
	passedFlags := flags()

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	var config *veneur.Config
	if passedFlags["f"] {
		conf, err := veneur.ReadConfig(*configFile)
		if err != nil {
			logrus.WithError(err).Fatal("Error reading configuration file.")
		}
		config = &conf
	}

	addr, err := addr(passedFlags, config, hostport)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting destination address.")
	}
	logrus.Debugf("destination: %s", addr)

	var conn MinimalClient
	conn, err = statsd.New(addr)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}
	_ = conn

	var nconn net.Conn
	nconn, err = net.Dial("udp", addr)
	if err != nil {
		logrus.WithError(err).Fatal("Error!")
	}

	// tags := tags(*tag)

	var span *ssf.SSFSpan
	span, err = createMetrics(passedFlags, *name, *tag)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating metric(s).")
	}

	err = sendSpan(nconn, span)
	if err != nil {
		logrus.WithError(err).Fatal("Error sending metric(s).")
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
	if conf != nil {
		addr = conf.UdpAddress // TODO: conf.TcpAddress
	} else if passedFlags["hostport"] && hostport != nil {
		addr = *hostport
	} else {
		err = errors.New("you must either specify a Veneur config file or a valid hostport")
	}
	return addr, err
}

func bareMetric(name string, tags string) *ssf.SSFSample {
	metric := &ssf.SSFSample{}
	metric.Name = name
	metric.Tags = make(map[string]string)
	for _, elem := range strings.Split(tags, ",") {
		tag := strings.Split(elem, ":")
		metric.Tags[tag[0]] = tag[1]
	}
	return metric
}

func createMetrics(passedFlags map[string]bool, name string, tags string) (*ssf.SSFSpan, error) {
	var err error
	span := &ssf.SSFSpan{}
	if passedFlags["gauge"] {
		logrus.Debugf("Sending gauge '%s' -> %f", name, *gauge)
		metric := bareMetric(name, tags)
		metric.Metric = ssf.SSFSample_GAUGE
		metric.Value = float32(*gauge)
		span.Metrics = append(span.Metrics, metric)
	}
	// TODO: figure out timing metrics
	if passedFlags["count"] {
		logrus.Debugf("Sending count '%s' -> %d", name, *count)
		metric := bareMetric(name, tags)
		metric.Metric = ssf.SSFSample_COUNTER
		metric.Value = float32(*count)
		span.Metrics = append(span.Metrics, metric)
	}
	return span, err
}

func sendSpan(conn MinimalConn, span *ssf.SSFSpan) error {
	var err error
	var data []byte
	data, err = proto.Marshal(span)
	if err != nil {
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		return err
	}
	return nil
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
