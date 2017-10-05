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
	"github.com/Sirupsen/logrus"
	"github.com/gogo/protobuf/proto"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
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

	addr, network, err := destination(passedFlags, hostport, *toSSF)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting destination address.")
	}
	logrus.WithField("net", network).
		WithField("addr", addr).
		Debugf("destination")

	if *shellCommand {
		var conn MinimalClient
		conn, err = statsd.New(addr)
		if err != nil {
			logrus.WithError(err).
				WithField("addr", addr).
				WithField("network", network).
				Fatal("Could not create statsd client")
		}

		var status int
		status, err = timeCommand(conn, flag.Args(), *name, tags(*tag))
		if err != nil {
			logrus.WithError(err).Fatal("Error timing command.")
		}
		os.Exit(status)
	} else if *mode == "metric" {
		logrus.Debug("Sending metric")
		if *toSSF {
			var nconn net.Conn
			nconn, err = net.Dial(network, addr)
			if err != nil {
				logrus.WithError(err).
					WithField("addr", addr).
					WithField("network", network).
					Fatal("Could not connect")
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
				logrus.WithError(err).
					WithField("addr", addr).
					WithField("network", network).
					Fatal("Could not create statsd client")
			}

			tags := tags(*tag)
			err = sendMetrics(conn, passedFlags, *name, tags)
			if err != nil {
				logrus.WithError(err).Fatal("Error sending metric(s).")
			}
		}
	} else if *mode == "event" {
		logrus.Debug("Sending event")
		nconn, _ := net.Dial(network, addr)
		pkt, err := buildEventPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Fatal("build event")
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
	} else if *mode == "sc" {
		logrus.Debug("Sending service check")
		nconn, _ := net.Dial(network, addr)
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

func destination(passedFlags map[string]flag.Value, hostport *string, useSSF bool) (addr string, network string, err error) {
	network = "udp"
	if passedFlags["hostport"] != nil {
		addr = passedFlags["hostport"].String()
	} else if hostport != nil {
		addr = *hostport
	} else {
		err = errors.New("you must specify a valid hostport")
		return
	}
	address, parseErr := protocol.ResolveAddr(addr)
	if parseErr != nil {
		// This is fine - we can attempt to treat the
		// host:port combination as a UDP address.
		return
	}
	// Looks like we got a listener spec URL, translate that into an address:
	network = address.Network()
	addr = address.String()
	return addr, network, err
}

func timeCommand(client MinimalClient, command []string, name string, tags []string) (int, error) {
	logrus.Debugf("Timing %q...", command)
	cmd := exec.Command(command[0], command[1:]...)
	exitStatus := 0
	start := time.Now()
	err := cmd.Run()
	if err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			return 1, err
		}
		status := exitError.ProcessState.Sys().(syscall.WaitStatus)
		exitStatus = status.ExitStatus()
	}
	elapsed := time.Since(start)
	exitTag := fmt.Sprintf("exit_status:%d", exitStatus)
	tags = append(tags, exitTag)
	logrus.Debugf("%q took %s", command, elapsed)
	err = client.Timing(name, elapsed, tags, 1)
	return exitStatus, err
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

func createMetrics(passedFlags map[string]flag.Value, name string, tags string) (*ssf.SSFSpan, error) {
	var err error
	span := &ssf.SSFSpan{}
	if passedFlags["gauge"] != nil {
		logrus.Debugf("Sending gauge '%s' -> %f", name, passedFlags["gauge"].String())
		value, err := strconv.ParseFloat(passedFlags["gauge"].String(), 32)
		if err != nil {
			return nil, err
		}
		metric := bareMetric(name, tags)
		metric.Metric = ssf.SSFSample_GAUGE
		metric.Value = float32(value)
		span.Metrics = append(span.Metrics, metric)
	}
	// TODO: figure out timing metrics
	if passedFlags["count"] != nil {
		logrus.Debugf("Sending count '%s' -> %s", name, passedFlags["count"].String())
		value, err := strconv.ParseInt(passedFlags["count"].String(), 10, 64)
		if err != nil {
			return nil, err
		}
		metric := bareMetric(name, tags)
		metric.Metric = ssf.SSFSample_COUNTER
		metric.Value = float32(value)
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

func sendMetrics(client MinimalClient, passedFlags map[string]flag.Value, name string, tags []string) error {
	var err error
	if passedFlags["gauge"] != nil {
		logrus.Debugf("Sending gauge '%s' -> %f", name, passedFlags["gauge"].String())
		value, err := strconv.ParseFloat(passedFlags["gauge"].String(), 64)
		if err != nil {
			return err
		}
		err = client.Gauge(name, value, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["timing"] != nil {
		logrus.Debugf("Sending timing '%s' -> %f", name, passedFlags["timing"].String())
		value, err := time.ParseDuration(passedFlags["timing"].String())
		if err != nil {
			return err
		}
		err = client.Timing(name, value, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["count"] != nil {
		logrus.Debugf("Sending count '%s' -> %s", name, passedFlags["count"].String())
		value, err := strconv.ParseInt(passedFlags["count"].String(), 10, 64)
		if err != nil {
			return err
		}
		err = client.Count(name, value, tags, 1)
		if err != nil {
			return err
		}
	}
	if passedFlags["gauge"] == nil && passedFlags["timing"] == nil && passedFlags["timeinms"] == nil && passedFlags["count"] == nil {
		logrus.Info("No metrics reported.")
	}
	return err
	// TODO: error on no metrics?
}
