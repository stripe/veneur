package main

import (
	"bytes"
	"errors"
	"flag"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"fmt"
	"strconv"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

var (
	// Generic flags
	hostport     = flag.String("hostport", "", "Address of destination (hostport or listening address URL).")
	mode         = flag.String("mode", "metric", "Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'.")
	debug        = flag.Bool("debug", false, "Turns on debug messages.")
	command      = flag.String("command", "", "Command to time. This will exec 'command', time it, and emit a timer metric.")
	shellCommand = flag.Bool("shellCommand", false, "Turns on timeCommand mode. veneur-emit will grab everything after the first non-known-flag argument, time its execution, and report it as a timing metric.")

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

	addr, netAddr, err := destination(hostport, *toSSF)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting destination address.")
	}
	logrus.WithField("net", netAddr.Network()).
		WithField("addr", netAddr.String()).
		Debugf("destination")

	if *mode == "event" {
		if *toSSF {
			logrus.WithField("mode", *mode).
				Fatal("Unsupported mode with SSF")
		}
		logrus.Debug("Sending event")
		nconn, _ := net.Dial(netAddr.Network(), netAddr.String())
		pkt, err := buildEventPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Fatal("build event")
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
		return
	}

	if *mode == "sc" {
		if *toSSF {
			logrus.WithField("mode", *mode).
				Fatal("Unsupported mode with SSF")
		}
		logrus.Debug("Sending service check")
		nconn, _ := net.Dial(netAddr.Network(), netAddr.String())
		pkt, err := buildSCPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Fatal("build event")
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
		return
	}

	span, status, err := createMetric(passedFlags, *name, *tag)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating metric.")
	}
	if *toSSF {
		client, err := trace.NewClient(addr)
		if err != nil {
			logrus.WithError(err).
				WithField("address", addr).
				Fatal("Could not construct client")
		}
		defer client.Close()
		err = sendSSF(client, span)
		if err != nil {
			logrus.WithError(err).Fatal("Could not send SSF span")
		}
	} else {
		if netAddr.Network() != "udp" {
			logrus.WithField("address", addr).
				WithField("network", netAddr.Network()).
				Fatal("hostport must be a UDP address for statsd metrics")
		}
		sendStatsd(netAddr.String(), span)
	}
	os.Exit(status)
}

func flags() map[string]flag.Value {
	flag.Parse()
	// hacky way to detect which flags were *actually* set
	passedFlags := make(map[string]flag.Value)
	flag.Visit(func(f *flag.Flag) {
		passedFlags[f.Name] = f.Value
	})
	return passedFlags
}

func tags(tag string) []string {
	var tags []string
	if len(tag) == 0 {
		return tags
	}
	for _, elem := range strings.Split(tag, ",") {
		tags = append(tags, elem)
	}
	return tags
}

func destination(hostport *string, useSSF bool) (string, net.Addr, error) {
	var addr string
	if hostport != nil {
		addr = *hostport
	} else {
		return "", nil, errors.New("you must specify a valid hostport")
	}
	netAddr, err := protocol.ResolveAddr(addr)
	if err != nil {
		// This is fine - we can attempt to treat the
		// host:port combination as a UDP address:
		addr = fmt.Sprintf("udp://%s", addr)
		udpAddr, err := protocol.ResolveAddr(addr)
		if err != nil {
			return "", nil, err
		}
		return addr, udpAddr, nil
	}
	return addr, netAddr, nil
}

func timeCommand(command []string) (exitStatus int, start time.Time, ended time.Time, err error) {
	logrus.Debugf("Timing %q...", command)
	cmd := exec.Command(command[0], command[1:]...)
	start = time.Now()
	err = cmd.Run()
	ended = time.Now()
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			exitStatus = 1
			return
		}
		status := exitError.ProcessState.Sys().(syscall.WaitStatus)
		exitStatus = status.ExitStatus()
	}
	logrus.Debugf("%q took %s", command, ended.Sub(start))
	return
}

func bareMetric(name string, tags string) *ssf.SSFSample {
	metric := &ssf.SSFSample{}
	metric.Name = name
	metric.Tags = make(map[string]string)
	if tags != "" {
		for _, elem := range strings.Split(tags, ",") {
			tag := strings.Split(elem, ":")
			metric.Tags[tag[0]] = tag[1]
		}
	}
	return metric
}

func timingSample(duration time.Duration, name string, tags string) *ssf.SSFSample {
	m := bareMetric(name, tags)
	m.Metric = ssf.SSFSample_HISTOGRAM
	m.Unit = "ms"
	m.Value = float32(duration / time.Millisecond)
	return m
}

func createMetric(passedFlags map[string]flag.Value, name string, tags string) (*ssf.SSFSpan, int, error) {
	var err error
	status := 0
	span := &ssf.SSFSpan{}
	if *mode == "metric" {
		if *shellCommand {
			var start, ended time.Time

			status, start, ended, err = timeCommand(flag.Args())
			if err != nil {
				return nil, status, err
			}
			span.StartTimestamp = start.UnixNano()
			span.EndTimestamp = ended.UnixNano()
			span.Name = name
			span.Tags = make(map[string]string)
			if tags != "" {
				for _, elem := range strings.Split(tags, ",") {
					tag := strings.Split(elem, ":")
					span.Tags[tag[0]] = tag[1]
				}
			}
			span.Metrics = append(span.Metrics, timingSample(ended.Sub(start), name, tags))
		}

		if passedFlags["timing"] != nil {
			duration, err := time.ParseDuration(passedFlags["timing"].String())
			if err != nil {
				return nil, 0, err
			}
			span.Metrics = append(span.Metrics, timingSample(duration, name, tags))
		}

		if passedFlags["gauge"] != nil {
			logrus.Debugf("Sending gauge '%s' -> %f", name, passedFlags["gauge"].String())
			value, err := strconv.ParseFloat(passedFlags["gauge"].String(), 32)
			if err != nil {
				return nil, status, err
			}
			metric := bareMetric(name, tags)
			metric.Metric = ssf.SSFSample_GAUGE
			metric.Value = float32(value)
			span.Metrics = append(span.Metrics, metric)
		}

		if passedFlags["count"] != nil {
			logrus.Debugf("Sending count '%s' -> %s", name, passedFlags["count"].String())
			value, err := strconv.ParseInt(passedFlags["count"].String(), 10, 64)
			if err != nil {
				return nil, status, err
			}
			metric := bareMetric(name, tags)
			metric.Metric = ssf.SSFSample_COUNTER
			metric.Value = float32(value)
			span.Metrics = append(span.Metrics, metric)
		}
	}
	return span, status, err
}

// sendSSF sends a whole span to an SSF receiver.
func sendSSF(client *trace.Client, span *ssf.SSFSpan) error {
	done := make(chan error)
	err := trace.Record(client, span, done)
	if err != nil {
		return err
	}
	return <-done
}

// sendStatsd sends the metrics gathered in a span to a dogstatsd
// endpoint.
func sendStatsd(addr string, span *ssf.SSFSpan) error {
	client, err := statsd.New(addr)
	if err != nil {
		return err
	}
	// Report all the metrics in the span:
	for _, metric := range span.Metrics {
		tags := make([]string, 0, len(metric.Tags))
		for name, val := range metric.Tags {
			tags = append(tags, fmt.Sprintf("%s:%s", name, val))
		}
		switch metric.Metric {
		case ssf.SSFSample_COUNTER:
			err = client.Count(metric.Name, int64(metric.Value), tags, 1.0)
		case ssf.SSFSample_GAUGE:
			err = client.Gauge(metric.Name, float64(metric.Value), tags, 1.0)
		case ssf.SSFSample_HISTOGRAM:
			if metric.Unit == "ms" {
				err = client.TimeInMilliseconds(metric.Name, float64(metric.Value), tags, 1.0)
			} else {
				err = client.Histogram(metric.Name, float64(metric.Value), tags, 1.0)
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func buildEventPacket(passedFlags map[string]flag.Value) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	buffer.WriteString("_e")

	if passedFlags["e_title"] == nil {
		return bytes.Buffer{}, errors.New("missing event title")
	}

	if passedFlags["e_text"] == nil {
		return bytes.Buffer{}, errors.New("missing event text")
	}

	buffer.WriteString(fmt.Sprintf("{%d,%d}", len(passedFlags["e_title"].String()), len(passedFlags["e_text"].String())))
	buffer.WriteString(":")

	buffer.WriteString(passedFlags["e_title"].String())
	buffer.WriteString("|")

	text := strings.Replace(passedFlags["e_text"].String(), "\n", "\\n", -1)
	buffer.WriteString(text)

	if passedFlags["e_time"] != nil {
		buffer.WriteString(fmt.Sprintf("|d:%s", passedFlags["e_time"].String()))
	}

	if passedFlags["e_hostname"] != nil {
		buffer.WriteString(fmt.Sprintf("|h:%s", passedFlags["e_hostname"].String()))
	}

	if passedFlags["e_aggr_key"] != nil {
		buffer.WriteString(fmt.Sprintf("|k:%s", passedFlags["e_aggr_key"].String()))
	}

	if passedFlags["e_priority"] != nil {
		buffer.WriteString(fmt.Sprintf("|p:%s", passedFlags["e_priority"].String()))
	}

	if passedFlags["e_source_type"] != nil {
		buffer.WriteString(fmt.Sprintf("|s:%s", passedFlags["e_source_type"].String()))
	}

	if passedFlags["e_alert_type"] != nil {
		buffer.WriteString(fmt.Sprintf("|t:%s", passedFlags["e_alert_type"].String()))
	}

	if passedFlags["e_event_tags"] != nil {
		buffer.WriteString(fmt.Sprintf("|#%s", passedFlags["e_event_tags"].String()))
	}

	return buffer, nil
}

func buildSCPacket(passedFlags map[string]flag.Value) (bytes.Buffer, error) {
	var buffer bytes.Buffer
	buffer.WriteString("_sc")

	if passedFlags["sc_name"] == nil {
		return bytes.Buffer{}, errors.New("missing service check name")
	}

	if passedFlags["sc_status"] == nil {
		return bytes.Buffer{}, errors.New("missing service check status")
	}

	buffer.WriteString("|")
	buffer.WriteString(passedFlags["sc_name"].String())

	buffer.WriteString("|")
	buffer.WriteString(passedFlags["sc_status"].String())

	if passedFlags["sc_time"] != nil {
		buffer.WriteString(fmt.Sprintf("|d:%s", passedFlags["sc_time"].String()))
	}

	if passedFlags["sc_hostname"] != nil {
		buffer.WriteString(fmt.Sprintf("|h:%s", passedFlags["sc_hostname"].String()))
	}

	if passedFlags["sc_tags"] != nil {
		buffer.WriteString(fmt.Sprintf("|#%s", passedFlags["sc_tags"].String()))
	}

	if passedFlags["sc_msg"] != nil {
		buffer.WriteString(fmt.Sprintf("|m:%s", passedFlags["sc_msg"].String()))
	}

	return buffer, nil
}
