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

type EmitMode uint

type Flags struct {
	HostPort  string
	Mode      string
	Debug     bool
	Command   bool
	ExtraArgs []string

	Name   string
	Gauge  float64
	Timing time.Duration
	Count  int64
	Set    string
	Tag    string
	ToSSF  bool

	Event struct {
		Title      string
		Text       string
		Time       string
		Hostname   string
		AggrKey    string
		Priority   string
		SourceType string
		AlertType  string
		Tags       string
	}

	ServiceCheck struct {
		Name      string
		Status    string
		Timestamp string
		Hostname  string
		Tags      string
		Msg       string
	}

	Span struct {
		TraceID   int64
		ParentID  int64
		StartTime string
		EndTime   string
		Service   string
		Indicator bool
		Tags      string
	}
}

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
	os.Exit(Main(os.Args))
}

func Main(args []string) int {
	flagStruct, passedFlags, err := flags(args)
	if err != nil {
		logrus.WithError(err).Error("Could not parse flags.")
		return 1
	}

	if flagStruct.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	validateFlagCombinations(passedFlags, flagStruct.ExtraArgs)

	addr, netAddr, err := destination(flagStruct.HostPort, flagStruct.ToSSF)
	if err != nil {
		logrus.WithError(err).Error("Error getting destination address.")
		return 1
	}
	logrus.WithField("net", netAddr.Network()).
		WithField("addr", netAddr.String()).
		WithField("ssf", flagStruct.ToSSF).
		Debugf("destination")

	if flagStruct.Mode == "event" {
		if flagStruct.ToSSF {
			logrus.WithField("mode", flagStruct.Mode).
				Error("Unsupported mode with SSF")
			return 1
		}
		logrus.Debug("Sending event")
		nconn, _ := net.Dial(netAddr.Network(), netAddr.String())
		pkt, err := buildEventPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Error("build event")
			return 1
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
		return 0
	}

	if flagStruct.Mode == "sc" {
		if flagStruct.ToSSF {
			logrus.WithField("mode", flagStruct.Mode).
				Error("Unsupported mode with SSF")
			return 1
		}
		logrus.Debug("Sending service check")
		nconn, _ := net.Dial(netAddr.Network(), netAddr.String())
		pkt, err := buildSCPacket(passedFlags)
		if err != nil {
			logrus.WithError(err).Error("build event")
			return 1
		}
		nconn.Write(pkt.Bytes())
		logrus.Debugf("Buffer string: %s", pkt.String())
		return 0
	}

	if flagStruct.Span.TraceID, err = inferTraceIDInt(flagStruct.Span.TraceID, envTraceID); err != nil {
		logrus.WithError(err).
			WithField("env_var", envTraceID).
			WithField("ID", "trace_id").
			Warn("Could not infer ID from environment")
	}
	if flagStruct.Span.ParentID, err = inferTraceIDInt(flagStruct.Span.ParentID, envSpanID); err != nil {
		logrus.WithError(err).
			WithField("env_var", envSpanID).
			WithField("ID", "parent_span_id").
			Warn("Could not infer ID from environment")
	}
	span, err := setupSpan(flagStruct.Span.TraceID, flagStruct.Span.ParentID, flagStruct.Name, flagStruct.Tag, flagStruct.Span.Service, flagStruct.Span.Tags, flagStruct.Span.Indicator)
	if err != nil {
		logrus.WithError(err).
			Error("Couldn't set up the main span")
		return 1
	}
	if span.TraceId != 0 {
		if !flagStruct.ToSSF {
			logrus.WithField("ssf", flagStruct.ToSSF).
				Error("Can't use tracing in non-ssf operation: Use -ssf to emit trace spans.")
			return 1
		}
		logrus.WithField("trace_id", span.TraceId).
			WithField("span_id", span.Id).
			WithField("parent_id", span.ParentId).
			WithField("service", span.Service).
			WithField("name", span.Name).
			Debug("Tracing is activated")
	}

	status, err := createMetric(span, passedFlags, flagStruct.Name, flagStruct.Tag, flagStruct.Command, flagStruct.ExtraArgs)
	if err != nil {
		logrus.WithError(err).Error("Error creating metrics.")
		return 1
	}
	if flagStruct.ToSSF {
		client, err := trace.NewClient(addr)
		if err != nil {
			logrus.WithError(err).
				WithField("address", addr).
				Error("Could not construct client")
			return 1
		}
		defer client.Close()
		err = sendSSF(client, span)
		if err != nil {
			logrus.WithError(err).Error("Could not send SSF span")
			return 1
		}
	} else {
		if netAddr.Network() != "udp" {
			logrus.WithField("address", addr).
				WithField("network", netAddr.Network()).
				Error("hostport must be a UDP address for statsd metrics")
			return 1
		}
		if len(span.Metrics) == 0 {
			logrus.Error("No metrics to send. Must pass metric data via at least one of -count, -gauge, -timing, or -set.")
			return 1
		}
		err = sendStatsd(netAddr.String(), span)
		if err != nil {
			logrus.WithError(err).Error("Could not send UDP metrics")
			return 1
		}
	}
	return status
}

func flags(args []string) (Flags, map[string]flag.Value, error) {
	var flagStruct Flags
	flagset := flag.NewFlagSet(args[0], flag.ContinueOnError)

	// Generic flags
	flagset.StringVar(&flagStruct.HostPort, "hostport", "", "Address of destination (hostport or listening address URL).")
	flagset.StringVar(&flagStruct.Mode, "mode", "metric", "Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'.")
	flagset.BoolVar(&flagStruct.Debug, "debug", false, "Turns on debug messages.")
	flagset.BoolVar(&flagStruct.Command, "command", false, "Turns on command-timing mode. veneur-emit will grab everything after the first non-known-flag argument, time its execution, and report it as a timing metric.")

	// Metric flags
	flagset.StringVar(&flagStruct.Name, "name", "", "Name of metric to report. Ex: 'daemontools.service.starts'")
	flagset.Float64Var(&flagStruct.Gauge, "gauge", 0, "Report a 'gauge' metric. Value must be float64.")
	flagset.DurationVar(&flagStruct.Timing, "timing", 0*time.Millisecond, "Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	flagset.Int64Var(&flagStruct.Count, "count", 0, "Report a 'count' metric. Value must be an integer.")
	flagset.StringVar(&flagStruct.Set, "set", "", "Report a 'set' metric with an arbitrary string value.")
	flagset.StringVar(&flagStruct.Tag, "tag", "", "Tag(s) for metric, comma separated. Ex: 'service:airflow'. Note: Any tags here are applied to all emitted data. See also mode-specific tag options (e.g. span_tags)")
	flagset.BoolVar(&flagStruct.ToSSF, "ssf", false, "Sends packets via SSF instead of StatsD. (https://github.com/stripe/veneur/blob/master/ssf/)")

	// Event flags
	// TODO: what should flags be called?
	flagset.StringVar(&flagStruct.Event.Title, "e_title", "", "Title of event. Ex: 'An exception occurred' *")
	flagset.StringVar(&flagStruct.Event.Text, "e_text", "", "Text of event. Insert line breaks with an esaped slash (\\\\n) *")
	flagset.StringVar(&flagStruct.Event.Time, "e_time", "", "Add timestamp to the event. Default is the current Unix epoch timestamp.")
	flagset.StringVar(&flagStruct.Event.Hostname, "e_hostname", "", "Hostname for the event.")
	flagset.StringVar(&flagStruct.Event.AggrKey, "e_aggr_key", "", "Add an aggregation key to group event with others with same key.")
	flagset.StringVar(&flagStruct.Event.Priority, "e_priority", "normal", "Priority of event. Must be 'low' or 'normal'.")
	flagset.StringVar(&flagStruct.Event.SourceType, "e_source_type", "", "Add source type to the event.")
	flagset.StringVar(&flagStruct.Event.AlertType, "e_alert_type", "info", "Alert type must be 'error', 'warning', 'info', or 'success'.")
	flagset.StringVar(&flagStruct.Event.Tags, "e_event_tags", "", "Tag(s) for event, comma separated. Ex: 'service:airflow,host_type:qa'")

	// Service check flags
	flagset.StringVar(&flagStruct.ServiceCheck.Name, "sc_name", "", "Service check name. *")
	flagset.StringVar(&flagStruct.ServiceCheck.Status, "sc_status", "", "Integer corresponding to check status. (OK = 0, WARNING = 1, CRITICAL = 2, UNKNOWN = 3)*")
	flagset.StringVar(&flagStruct.ServiceCheck.Timestamp, "sc_time", "", "Add timestamp to check. Default is current Unix epoch timestamp.")
	flagset.StringVar(&flagStruct.ServiceCheck.Hostname, "sc_hostname", "", "Add hostname to the event.")
	flagset.StringVar(&flagStruct.ServiceCheck.Tags, "sc_tags", "", "Tag(s) for service check, comma separated. Ex: 'service:airflow,host_type:qa'")
	flagset.StringVar(&flagStruct.ServiceCheck.Msg, "sc_msg", "", "Message describing state of current state of service check.")

	// Tracing flags
	flagset.Int64Var(&flagStruct.Span.TraceID, "trace_id", 0, "ID for the trace (top-level) span. Setting a trace ID activated tracing.")
	flagset.Int64Var(&flagStruct.Span.ParentID, "parent_span_id", 0, "ID of the parent span.")
	flagset.StringVar(&flagStruct.Span.StartTime, "span_starttime", "", "Date/time to set for the start of the span. See https://github.com/araddon/dateparse#extended-example for formatting.")
	flagset.StringVar(&flagStruct.Span.EndTime, "span_endtime", "", "Date/time to set for the end of the span. Format is same as -span_starttime.")
	flagset.StringVar(&flagStruct.Span.Service, "span_service", "veneur-emit", "Service name to associate with the span.")
	flagset.BoolVar(&flagStruct.Span.Indicator, "indicator", false, "Mark the reported span as an indicator span")
	flagset.StringVar(&flagStruct.Span.Tags, "span_tags", "", "Tag(s) for span, comma separated. Useful for avoiding high cardinality tags. Ex 'user_id:ac0b23,widget_id:284802'")

	err := flagset.Parse(args[1:])
	if err != nil {
		return flagStruct, nil, err
	}

	flagStruct.ExtraArgs = make([]string, len(flagset.Args()))
	copy(flagStruct.ExtraArgs, flagset.Args())

	// hacky way to detect which flags were *actually* set
	passedFlags := map[string]flag.Value{}
	flagset.Visit(func(f *flag.Flag) {
		passedFlags[f.Name] = f.Value
	})
	return flagStruct, passedFlags, nil
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

func destination(hostport string, useSSF bool) (string, net.Addr, error) {
	if hostport == "" {
		return "", nil, errors.New("you must specify a valid hostport")
	}
	netAddr, err := protocol.ResolveAddr(hostport)
	if err != nil {
		// This is fine - we can attempt to treat the
		// host:port combination as a UDP address:
		hostport = fmt.Sprintf("udp://%s", hostport)
		udpAddr, err := protocol.ResolveAddr(hostport)
		if err != nil {
			return "", nil, err
		}
		return hostport, udpAddr, nil
	}
	return hostport, netAddr, nil
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

func setupSpan(traceID, parentID int64, name, tags, service, spanTags string, indicator bool) (*ssf.SSFSpan, error) {
	span := &ssf.SSFSpan{}
	if traceID != 0 {
		span.TraceId = traceID
		span.ParentId = parentID
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
		span.Service = service
		span.Indicator = indicator
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
		// if the inner command returned nonzero, we will propagate its exit code
		// we don't need to also return an error
		err = nil
	}
	logrus.Debugf("%q took %s", command, ended.Sub(start))
	return
}

func createMetric(span *ssf.SSFSpan, passedFlags map[string]flag.Value, name string, tagStr string, command bool, extraArgs []string) (int, error) {
	var err error
	status := 0
	tags := tagsFromString(tagStr)

	if command {
		var start, ended time.Time

		status, start, ended, err = timeCommand(span, extraArgs)
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
		return 0, errors.New("Must provide both -span_startime and -span_endtime, or neither")
	}

	if shas || ehas {
		var start, end time.Time

		start, err = dateparse.ParseAny(sf.String())
		if err != nil {
			return 0, err
		}

		end, err = dateparse.ParseAny(ef.String())
		if err != nil {
			return 0, err
		}

		span.StartTimestamp = start.UnixNano()
		span.EndTimestamp = end.UnixNano()
	}

	// have to use passedFlags here so we can tell the difference between
	// zero (you explicitly passed zero) and zero (you didn't pass the flag at all)
	if passedFlags["timing"] != nil {
		logrus.Debugf("Sending timing '%s' -> %s", name, passedFlags["timing"].String())
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

func validateFlagCombinations(passedFlags map[string]flag.Value, extraArgs []string) error {
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
			return fmt.Errorf("Flag %q is only valid with \"-mode %s\"", flagname, fmode)
		}
	}

	// Sniff for args that were missing a dash.
	for _, arg := range extraArgs {
		if fmode, has := flagModeMappings[arg]; has && (fmode&mode) == mode {
			if _, has := passedFlags[arg]; !has {
				return fmt.Errorf("Passed %q as an argument, but it's a parameter name. Did you mean \"-%s\"?", arg, arg)
			}
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
