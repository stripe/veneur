package main

import (
	"bytes"
	"errors"
	"flag"
	"net"
	"strings"
	"time"

	"fmt"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/Sirupsen/logrus"
	"github.com/gogo/protobuf/proto"
	"github.com/stripe/veneur"
	"github.com/stripe/veneur/ssf"
)

var (
	// Generic flags
	configFile = flag.String("f", "", "The Veneur config file to read for settings.")
	hostport   = flag.String("hostport", "", "Hostname and port of destination. Must be used if config file is not present.")
	mode       = flag.String("mode", "metric", "Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'.")
	debug      = flag.Bool("debug", false, "Turns on debug messages.")

	// Metric flags
	name   = flag.String("name", "", "Name of metric to report. Ex: 'daemontools.service.starts'")
	gauge  = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	timing = flag.Duration("timing", 0*time.Millisecond, "Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	count  = flag.Int64("count", 0, "Report a 'count' metric. Value must be an integer.")
	tag    = flag.String("tag", "", "Tag(s) for metric, comma separated. Ex: 'service:airflow'")
	toSSF  = flag.Bool("ssf", false, "Sends packets via SSF instead of StatsD. (https://github.com/stripe/veneur/blob/master/ssf/)")

	// Event flags
	// TODO: what should flags be called?
	eTitle      = flag.String("e_title", "", "Title of event. Ex: 'An exception occurred' *")
	eText       = flag.String("e_text", "", "Text of event. Insert line breaks with an esaped slash (\\\\n) *")
	eTimestamp  = flag.String("e_time", "", "Add timestamp to the event. Default is the current Unix epoch timestamp.")
	eHostname   = flag.String("e_hostname", "", "Hostname for the event.")
	eAggrKey    = flag.String("e_aggr_key", "", "Add an aggregation key to group event with others with same key.")
	ePriority   = flag.String("e_priority", "normal", "Priority of event. Must be 'low' or 'normal'.")
	eSourceType = flag.String("e_source_type", "", "Add source type to the event.")
	eAlertType  = flag.String("e_alert_type", "info", "Alert type must be 'error', 'warning', 'info', or 'success'.")
	eTag        = flag.String("e_event_tags", "", "Tag(s) for event, comma separated. Ex: 'service:airflow,host_type:qa'")

	// Service check flags
	scName      = flag.String("sc_name", "", "Service check name. *")
	scStatus    = flag.String("sc_status", "", "Integer corresponding to check status. (OK = 0, WARNING = 1, CRITICAL = 2, UNKNOWN = 3)*")
	scTimestamp = flag.String("sc_time", "", "Add timestamp to check. Default is current Unix epoch timestamp.")
	scHostname  = flag.String("sc_hostname", "", "Add hostname to the event.")
	scTags      = flag.String("sc_tags", "", "Tag(s) for service check, comma separated. Ex: 'service:airflow,host_type:qa'")
	scMsg       = flag.String("sc_msg", "", "Message describing state of current state of service check.")
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

	if *mode == "metric" {
		logrus.Debug("Sending metric")
		if *toSSF {
			var nconn net.Conn
			nconn, err = net.Dial("udp", addr)
			if err != nil {
				logrus.WithError(err).Fatal("Error!")
			}

			var span *ssf.SSFSpan
			span, err = createMetrics(passedFlags, *name, *tag)
			if err != nil {
				logrus.WithError(err).Fatal("Error creating metric(s).")
			}

			err = sendSpan(nconn, span)
			if err != nil {
				logrus.WithError(err).Fatal("Error sending metric(s).")
			}
		} else {
			var conn MinimalClient
			conn, err = statsd.New(addr)
			if err != nil {
				logrus.WithError(err).Fatal("Error!")
			}

			tags := tags(*tag)
			err = sendMetrics(conn, passedFlags, *name, tags)
			if err != nil {
				logrus.WithError(err).Fatal("Error sending metric(s).")
			}
		}
	} else if *mode == "event" {
		logrus.Debug("Sending event")
		nconn, _ := net.Dial("udp", addr)
		pkt, err := buildEventPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Fatal("build event")
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
	} else if *mode == "sc" {
		logrus.Debug("Sending service check")
		nconn, _ := net.Dial("udp", addr)
		pkt, err := buildSCPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Fatal("build event")
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
	} else {
		logrus.Fatalf("Mode '%s' is invalid.", *mode)
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

// TODO: validity checking? or just fire and forget
func buildEventPacket(passedFlags map[string]bool) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	buffer.WriteString("_e")

	if !passedFlags["e_title"] {
		return bytes.Buffer{}, errors.New("missing event title")
	}
	if !passedFlags["e_text"] {
		return bytes.Buffer{}, errors.New("missing event text")
	}

	buffer.WriteString(fmt.Sprintf("{%d,%d}", len(*eTitle), len(*eText)))
	buffer.WriteString(":")

	buffer.WriteString(*eTitle)
	buffer.WriteString("|")

	buffer.WriteString(*eText)
	buffer.WriteString("|")

	if passedFlags["e_time"] {
		buffer.WriteString(fmt.Sprintf("|d:%s", *eTimestamp))
	}

	if passedFlags["e_hostname"] {
		buffer.WriteString(fmt.Sprintf("|h:%s", *eHostname))
	}

	if passedFlags["e_aggr_key"] {
		buffer.WriteString(fmt.Sprintf("|k:%s", *eAggrKey))
	}

	if passedFlags["e_priority"] {
		buffer.WriteString(fmt.Sprintf("|p:%s", *ePriority))
	}

	if passedFlags["e_source_type"] {
		buffer.WriteString(fmt.Sprintf("|s:%s", *eSourceType))
	}

	if passedFlags["e_alert_type"] {
		buffer.WriteString(fmt.Sprintf("|t:%s", *eAlertType))
	}

	if passedFlags["e_event_tags"] {
		buffer.WriteString(fmt.Sprintf("|#%s", *eTag))
	}

	return buffer, nil
}

func buildSCPacket(passedFlags map[string]bool) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	buffer.WriteString("_sc")

	if !passedFlags["sc_name"] {
		return bytes.Buffer{}, errors.New("missing service check name")
	}

	if !passedFlags["sc_status"] {
		return bytes.Buffer{}, errors.New("missing service check status")
	}

	buffer.WriteString("|")
	buffer.WriteString(*scName)

	buffer.WriteString("|")
	buffer.WriteString(*scStatus)

	if passedFlags["sc_time"] {
		buffer.WriteString(fmt.Sprintf("|d:%s", *scTimestamp))
	}

	if passedFlags["sc_hostname"] {
		buffer.WriteString(fmt.Sprintf("|h:%s", *scHostname))
	}

	if passedFlags["sc_tags"] {
		buffer.WriteString(fmt.Sprintf("|#%s", *scTags))
	}

	if passedFlags["sc_msg"] {
		buffer.WriteString(fmt.Sprintf("|m:%s", *scMsg))
	}

	return buffer, nil
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
