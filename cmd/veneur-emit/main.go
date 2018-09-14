package main

import (
	"bytes"
	"errors"
	"flag"
	"math"
	"math/big"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"fmt"
	"strconv"

	"crypto/rand"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/araddon/dateparse"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

var (
	// Generic flags
	hostport = flag.String("hostport", "", "Address of destination (hostport or listening address URL).")
	mode     = flag.String("mode", "metric", "Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'.")
	debug    = flag.Bool("debug", false, "Turns on debug messages.")
	command  = flag.Bool("command", false, "Turns on command-timing mode. veneur-emit will grab everything after the first non-known-flag argument, time its execution, and report it as a timing metric.")

	// Metric flags
	name   = flag.String("name", "", "Name of metric to report. Ex: 'daemontools.service.starts'")
	gauge  = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	timing = flag.Duration("timing", 0*time.Millisecond, "Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	count  = flag.Int64("count", 0, "Report a 'count' metric. Value must be an integer.")
	set    = flag.String("set", "", "Report a 'set' metric with an arbitrary string value.")
	tag    = flag.String("tag", "", "Tag(s) for metric, comma separated. Ex: 'service:airflow'. Note: Any tags here are applied to all emitted data. See also mode-specific tag options (e.g. span_tags)")
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

	// Tracing flags
	traceID   = flag.Int64("trace_id", 0, "ID for the trace (top-level) span. Setting a trace ID activated tracing.")
	parentID  = flag.Int64("parent_span_id", 0, "ID of the parent span.")
	spanStart = flag.String("span_starttime", "", "Date/time to set for the start of the span. See https://github.com/araddon/dateparse#extended-example for formatting.")
	spanEnd   = flag.String("span_endtime", "", "Date/time to set for the end of the span. Format is same as -span_starttime.")
	service   = flag.String("span_service", "veneur-emit", "Service name to associate with the span.")
	indicator = flag.Bool("indicator", false, "Mark the reported span as an indicator span")
	sTag      = flag.String("span_tags", "", "Tag(s) for span, comma separated. Useful for avoiding high cardinality tags. Ex 'user_id:ac0b23,widget_id:284802'")
)

type EmitMode uint

const (
	MetricMode EmitMode = 1 << iota
	EventMode
	ServiceCheckMode
	AllModes = MetricMode | EventMode | ServiceCheckMode
)

func (m EmitMode) String() string {
	switch m {
	case MetricMode:
		return "metric"
	case EventMode:
		return "event"
	case ServiceCheckMode:
		return "sc"
	case AllModes:
		return "any"
	}

	return ""
}

var flagModeMappings = map[string]EmitMode{}

func init() {
	for mode, flags := range map[EmitMode][]string{
		AllModes: []string{
			"hostport",
			"debug",
			"command",
		},
		MetricMode: []string{
			"name",
			"gauge",
			"timing",
			"count",
			"set",
			"tag",
			"ssf",
		},
		EventMode: []string{
			"e_title",
			"e_text",
			"e_time",
			"e_hostname",
			"e_aggr_key",
			"e_priority",
			"e_source_type",
			"e_alert_type",
			"e_event_tags",
		},
		ServiceCheckMode: []string{
			"sc_name",
			"sc_status",
			"sc_time",
			"sc_hostname",
			"sc_tags",
			"sc_msg",
		},
	} {
		for _, flag := range flags {
			flagModeMappings[flag] = mode
		}
	}
}

const (
	envTraceID = "VENEUR_EMIT_TRACE_ID"
	envSpanID  = "VENEUR_EMIT_PARENT_SPAN_ID"
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

	validateFlagCombinations(passedFlags)

	addr, netAddr, err := destination(hostport, *toSSF)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting destination address.")
	}
	logrus.WithField("net", netAddr.Network()).
		WithField("addr", netAddr.String()).
		WithField("ssf", *toSSF).
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

	if *traceID, err = inferTraceIDInt(*traceID, envTraceID); err != nil {
		logrus.WithError(err).
			WithField("env_var", envTraceID).
			WithField("ID", "trace_id").
			Warn("Could not infer ID from environment")
	}
	if *parentID, err = inferTraceIDInt(*parentID, envSpanID); err != nil {
		logrus.WithError(err).
			WithField("env_var", envSpanID).
			WithField("ID", "parent_span_id").
			Warn("Could not infer ID from environment")
	}
	span, err := setupSpan(traceID, parentID, *name, *tag, *sTag)
	if err != nil {
		logrus.WithError(err).
			Fatal("Couldn't set up the main span")
	}
	if span.TraceId != 0 {
		if !*toSSF {
			logrus.WithField("ssf", *toSSF).
				Fatal("Can't use tracing in non-ssf operation: Use -ssf to emit trace spans.")
		}
		logrus.WithField("trace_id", span.TraceId).
			WithField("span_id", span.Id).
			WithField("parent_id", span.ParentId).
			WithField("service", span.Service).
			WithField("name", span.Name).
			Debug("Tracing is activated")
	}

	status, err := createMetric(span, passedFlags, *name, *tag)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating metrics.")
	}
	if len(span.Metrics) == 0 {
		logrus.Fatal("No metrics to send. Must pass metric data via at least one of -count, -gauge, -timing, or -set.")
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
	passedFlags := map[string]flag.Value{}
	flag.Visit(func(f *flag.Flag) {
		passedFlags[f.Name] = f.Value
	})
	return passedFlags
}

func tagsFromString(csv string) map[string]string {
	tags := map[string]string{}
	if len(csv) == 0 {
		return tags
	}
	for _, elem := range strings.Split(csv, ",") {
		if len(elem) == 0 {
			continue
		}
		// Use SplitN here so we don't mess up on
		// values with colons inside them
		tag := strings.SplitN(elem, ":", 2)
		switch len(tag) {
		case 2:
			tags[tag[0]] = tag[1]
		case 1:
			tags[tag[0]] = ""
		}
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

func inferTraceIDInt(existingID int64, envKey string) (id int64, err error) {
	if existingID != 0 {
		return existingID, nil // nothing to do
	}
	if strID, ok := os.LookupEnv(envKey); ok {
		id, err = strconv.ParseInt(strID, 10, 64)
		if err != nil {
			return
		}
		logrus.WithFields(logrus.Fields{
			"env_var": envKey,
			"value":   id,
		}).Debug("Inferred ID from environment")
	}
	return
}

func setupSpan(traceID, parentID *int64, name, tags string, spanTags string) (*ssf.SSFSpan, error) {
	span := &ssf.SSFSpan{}
	if traceID != nil && *traceID != 0 {
		span.TraceId = *traceID
		if parentID != nil {
			span.ParentId = *parentID
		}
		bigid, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			return nil, err
		}
		span.Id = bigid.Int64()
		span.Name = name
		span.Tags = tagsFromString(tags)
		for k, v := range tagsFromString(spanTags) {
			span.Tags[k] = v
		}
		span.Service = *service
		span.Indicator = *indicator
	}
	return span, nil
}

func timeCommand(span *ssf.SSFSpan, command []string) (exitStatus int, start time.Time, ended time.Time, err error) {
	logrus.Debugf("Timing %q...", command)
	cmd := exec.Command(command[0], command[1:]...)

	// pass span IDs through on the environment so veneur-emits
	// further down the line can pick them up and construct a tree:
	cmd.Env = os.Environ()
	if span.TraceId != 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%d", envTraceID, span.TraceId))
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%d", envSpanID, span.Id))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	start = time.Now()
	err = cmd.Start()
	if err != nil {
		logrus.WithError(err).WithField("command", command).Error("Could not start command")
		exitStatus = 1
		return
	}

	err = cmd.Wait()
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

func createMetric(span *ssf.SSFSpan, passedFlags map[string]flag.Value, name string, tagStr string) (int, error) {
	var err error
	status := 0
	tags := tagsFromString(tagStr)

	if *mode == "metric" {
		if *command {
			var start, ended time.Time

			status, start, ended, err = timeCommand(span, flag.Args())
			if err != nil {
				return status, err
			}
			span.StartTimestamp = start.UnixNano()
			span.EndTimestamp = ended.UnixNano()
			span.Metrics = append(span.Metrics, ssf.Timing(name, ended.Sub(start), time.Millisecond, tags))
		}

		sf, shas := passedFlags["span_starttime"]
		ef, ehas := passedFlags["span_endtime"]
		if shas != ehas {
			logrus.Fatal("Must provide both -span_startime and -span_endtime, or neither")
		}

		if shas || ehas {
			var start, end time.Time

			start, err = dateparse.ParseAny(sf.String())
			if err != nil {
				logrus.WithError(err).Fatal("Error parsing -span_starttime")
			}

			end, err = dateparse.ParseAny(ef.String())
			if err != nil {
				logrus.WithError(err).Fatal("Error parsing -span_endtime")
			}

			span.StartTimestamp = start.UnixNano()
			span.EndTimestamp = end.UnixNano()
		}

		if passedFlags["timing"] != nil {
			duration, err := time.ParseDuration(passedFlags["timing"].String())
			if err != nil {
				return 0, err
			}
			span.Metrics = append(span.Metrics, ssf.Timing(name, duration, time.Millisecond, tags))
		}

		if passedFlags["gauge"] != nil {
			logrus.Debugf("Sending gauge '%s' -> %s", name, passedFlags["gauge"].String())
			value, err := strconv.ParseFloat(passedFlags["gauge"].String(), 32)
			if err != nil {
				return status, err
			}
			span.Metrics = append(span.Metrics, ssf.Gauge(name, float32(value), tags))
		}

		if passedFlags["count"] != nil {
			logrus.Debugf("Sending count '%s' -> %s", name, passedFlags["count"].String())
			value, err := strconv.ParseInt(passedFlags["count"].String(), 10, 64)
			if err != nil {
				return status, err
			}
			span.Metrics = append(span.Metrics, ssf.Count(name, float32(value), tags))
		}

		if passedFlags["set"] != nil {
			logrus.Debugf("Sending set '%s' -> %s", name, passedFlags["set"].String())
			span.Metrics = append(span.Metrics, ssf.Set(name, passedFlags["set"].String(), tags))
		}
	}
	return status, err
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
				// Treating the "ms" unit special is a
				// bit wonky, but it seems like the
				// right tool for the job here:
				err = client.TimeInMilliseconds(metric.Name, float64(metric.Value), tags, 1.0)
			} else {
				err = client.Histogram(metric.Name, float64(metric.Value), tags, 1.0)
			}
		case ssf.SSFSample_SET:
			err = client.Set(metric.Name, metric.Message, tags, 1.0)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func validateFlagCombinations(passedFlags map[string]flag.Value) {
	// Figure out which mode we're in
	var mode EmitMode
	mv, has := passedFlags["mode"]
	// "metric" is the default mode, so assume that if the flag wasn't passed.
	if !has {
		mode = MetricMode
	} else {
		switch mv.String() {
		case "metric":
			mode = MetricMode
		case "event":
			mode = EventMode
		case "sc":
			mode = ServiceCheckMode
		}
	}

	for flagname := range passedFlags {
		if fmode, has := flagModeMappings[flagname]; has && (fmode&mode) != mode {
			logrus.Fatalf("Flag %q is only valid with \"-mode %s\"", flagname, fmode)
		}
	}

	// Sniff for args that were missing a dash.
	for _, arg := range flag.Args() {
		if fmode, has := flagModeMappings[arg]; has && (fmode&mode) == mode {
			if _, has := passedFlags[arg]; !has {
				logrus.Fatalf("Passed %q as an argument, but it's a parameter name. Did you mean \"-%s\"?", arg, arg)
			}
		}
	}

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

	finalTags := map[string]string{}
	if passedFlags["e_event_tags"] != nil {
		finalTags = tagsFromString(passedFlags["e_event_tags"].String())
	}
	if passedFlags["tag"] != nil {
		for k, v := range tagsFromString(passedFlags["tag"].String()) {
			finalTags[k] = v
		}
	}
	if len(finalTags) > 0 {
		buffer.WriteString("|#") // Write the tag prefix bytes
		for k, v := range finalTags {
			buffer.WriteString(fmt.Sprintf("%s:%s,", k, v))
		}
		buffer.Truncate(buffer.Len() - 1) // Drop the last comma for cleanliness
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

	finalTags := map[string]string{}
	if passedFlags["sc_tags"] != nil {
		finalTags = tagsFromString(passedFlags["sc_tags"].String())
	}
	if passedFlags["tag"] != nil {
		for k, v := range tagsFromString(passedFlags["tag"].String()) {
			finalTags[k] = v
		}
	}
	if len(finalTags) > 0 {
		buffer.WriteString("|#") // Write the tag prefix bytes
		for k, v := range finalTags {
			buffer.WriteString(fmt.Sprintf("%s:%s,", k, v))
		}
		buffer.Truncate(buffer.Len() - 1) // Drop the last comma for cleanliness
	}

	if passedFlags["sc_msg"] != nil {
		buffer.WriteString(fmt.Sprintf("|m:%s", passedFlags["sc_msg"].String()))
	}

	return buffer, nil
}
