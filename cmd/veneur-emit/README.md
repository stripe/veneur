`veneur-emit` is a command line utility for emitting metrics to [Veneur](https://github.com/stripe/veneur).

Some common use cases:

- Instrument shell scripts
- Instrumenting shell-based tools like init scripts, startup scripts and more
- Testing

# Usage

Emitting a metric with veneur-emit:

```
$ veneur-emit -hostport udp://example.com:8125 -count 3 -name "my.test.metric" -tag "host:my.machine.local"
```

Full usage:

```
Usage of veneur-emit:
  -command
        Turns on command-timing mode. veneur-emit will grab everything after the first non-known-flag argument, time its execution, and report it as a timing metric.
  -count int
        Report a 'count' metric. Value must be an integer.
  -debug
        Turns on debug messages.
  -e_aggr_key string
        Add an aggregation key to group event with others with same key.
  -e_alert_type string
        Alert type must be 'error', 'warning', 'info', or 'success'. (default "info")
  -e_event_tags string
        Tag(s) for event, comma separated. Ex: 'service:airflow,host_type:qa'
  -e_hostname string
        Hostname for the event.
  -e_priority string
        Priority of event. Must be 'low' or 'normal'. (default "normal")
  -e_source_type string
        Add source type to the event.
  -e_text string
        Text of event. Insert line breaks with an esaped slash (\\n) *
  -e_time string
        Add timestamp to the event. Default is the current Unix epoch timestamp.
  -e_title string
        Title of event. Ex: 'An exception occurred' *
  -error
        Mark the reported span as having errored
  -gauge float
        Report a 'gauge' metric. Value must be float64.
  -grpc
        Sets the emitting protocal to gRPC. This cannot be used with "udp" hostports.
  -hostport string
        Address of destination (hostport or listening address URL).
  -indicator
        Mark the reported span as an indicator span
  -mode string
        Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'. (default "metric")
  -name string
        Name of metric to report. Ex: 'daemontools.service.starts'
  -parent_span_id int
        ID of the parent span.
  -proxy string
        Address or URL to be used for proxying gRPC requests. The request will be sent to the proxy with the "hostport" attached so the proxy can forward the request. This can only be used with the "-grpc" flag.
  -sc_hostname string
        Add hostname to the event.
  -sc_msg string
        Message describing state of current state of service check.
  -sc_name string
        Service check name. *
  -sc_status string
        Integer corresponding to check status. (OK = 0, WARNING = 1, CRITICAL = 2, UNKNOWN = 3)*
  -sc_tags string
        Tag(s) for service check, comma separated. Ex: 'service:airflow,host_type:qa'
  -sc_time string
        Add timestamp to check. Default is current Unix epoch timestamp.
  -set string
        Report a 'set' metric with an arbitrary string value.
  -span_endtime string
        Date/time to set for the end of the span. Format is same as -span_starttime.
  -span_service string
        Service name to associate with the span. (default "veneur-emit")
  -span_starttime string
        Date/time to set for the start of the span. See https://github.com/araddon/dateparse#extended-example for formatting.
  -ssf
        Sends packets via SSF instead of StatsD. (https://github.com/stripe/veneur/blob/master/ssf/)
  -tag string
        Tag(s) for metric, comma separated. Ex: 'service:airflow'
  -timing duration
        Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).
  -trace_id int
        ID for the trace (top-level) span. Setting a trace ID activated tracing.
```

Veneur-emit supports two different backend modes: dogstatsd mode (the
default) and SSF (`-ssf`) mode. In dogstatsd mode, veneur-emit can
submit metrics, servie checks and events in dogstatsd format. In SSF
mode, veneur-emit can emit trace spans and metrics to a veneur
instance.

## Dogstatsd mode

### Dogstatsd Protocols

We currently support **TCP** `(tcp://)`, **UDP** `(udp://)`, **Unix Socket** `(unix://)`, and **gRPC** `(tcp://)` protocols for emitting in dogstatsd. 

#### UDP protocol:

We support the UDP protocol as part of the `-hostport` argument. This is the default for hostports that don't provide a protocol.

**Example command:**

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -name ...
```

**Example command without specified protocol (uses UDP default):**
```sh
veneur-emit -hostport 127.0.0.1:8200 -name ...
```

#### TCP protocol:

We support the TCP protocol as part of the `-hostport` argument.

**Example command:**

```sh
veneur-emit -hostport tcp://127.0.0.1:8200 -name ...
```

#### Unix Socket protocol:

We support Unix Sockets as part of the `-hostport` argument.

**Example command:**

```sh
veneur-emit -hostport unix:///var/run/veneur/dogstats.sock -name ...
```

#### gRPC protocol:

We support the gRPC protocol through the `-grpc` flag. When this flag is provided, the `-hostport` argument is treated as the URI of a Veneur instance serving gRPC requests.

**Example command:**

```sh
veneur-emit -hostport tcp://127.0.0.1:8200 -grpc -name ...
```

### Dogstatsd Examples

All of the below commands can be used with the `-grpc` flag to send them over gRPC.

Increment a counter in dogstatsd mode:

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -name that.metric.name -tag hi:there -count 1
```

Time a command in dogstatsd mode:

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -name some.command.timer -tag purpose:demonstration -command sleep 30
```

Submit a service check in dogstatsd mode (this isn't supported in SSF yet):

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -sc_name my.service.check -sc_msg "I'm not dead" -sc_status OK
```

Submit an event in dogstatsd mode (this isn't supported in SSF yet):

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -e_text "Something went wrong:\\n\\nTell a lie, it's all good." -e_title "I'm just testing" -e_source_type "demonstration"
```

Submit a "set" metric (the count of unique values across a time interval):

```sh
veneur-emit -hostport udp://127.0.0.1:8200 -name some.set.metric -set customer_a
```

## SSF mode

In SSF mode, veneur-emit will construct and submit an SSF span with
optional metrics. SSF mode does not yet support events or service
checks. SSF mode is not the default and needs to be enabled by providing the `-ssf` flag.

### SSF Protocols

Currently we support **UDP** `(udp://)`, **Unix Socket** `(unix://)`, and **gRPC** `(tcp://)` protocols for emitting in SSF.

### UDP protocol:

We support the UDP protocol as part of the `-hostport` argument. This is the default for hostports that don't provide a protocol.

**Example command:**

```sh
veneur-emit -ssf -hostport udp://127.0.0.1:8200 -name ...
```

**Example command without specified protocol (uses UDP default):**
```sh
veneur-emit -ssf -hostport 127.0.0.1:8200 -name ...
```

#### Unix Socket protocol:

We support Unix Sockets as part of the `-hostport` argument.

**Example command:**

```sh
veneur-emit -ssf -hostport unix:///var/run/veneur/ssf.sock -name ....
```

#### gRPC protocol:

We support the gRPC protocol through the `-grpc` flag. When this flag is provided, we treat the `-hostport` argument as a TCP address or URL for the underlying gRPC connection.

**Example command:**

```sh
veneur-emit -ssf -hostport tcp://127.0.0.1:8200 -grpc -name ...
```

### SSF Examples

Increment a counter in SSF mode:

```sh
veneur-emit -ssf -hostport unix:///var/run/veneur/ssf.sock -name that.metric.name -tag hi:there -count 1
```

Time a command in SSF mode:

```sh
veneur-emit -ssf -hostport unix:///var/run/veneur/ssf.sock -name some.command.timer -tag purpose:demonstration -command sleep 30
```

Time a command in SSF mode and submit a trace span for the process
(the `-trace_id` and `-parent_span_id` arguments need to be be set to
the values from the parent of whatever is calling veneur-emit):

```sh
veneur-emit -ssf -hostport unix:///var/run/veneur/ssf.sock -span_service 'testing' -trace_id 99 -parent_span_id 9999 -name some.command.timer -tag purpose:demonstration -command sleep 30
```

When timing a command in SSF mode, if the command returns non-zero
status code, the error flag will be set on the span, as if `-error`
were passed in:

```sh
veneur-emit -ssf -hostport unix:///var/run/veneur/ssf.sock -name some.command.timer -command not_a_real_command
```

## gRPC

[gRPC](https://grpc.io/) is protocol for making remote service calls. In Veneur we use this to support emitting/receiving metrics over HTTP. 
More details about our support of gRPC can be found below:

### Supported Modes

We currently support all emitting modes over gRPC. This means dogstatsd (metrics, events, service checks) and ssf spans. 
To enable gRPC, just add the `-grpc` flag to a command and it will send with the gRPC protocol. 
NOTE that gRPC uses TCP as its underlying protocol which makes it incompatible with UDP addresses.

### Specifying a Proxy

When using gRPC, you can also specify a proxy address/URL with the `-proxy` argument. 
When you provide a proxy address, Veneur will send the request to the proxy and attach the `-hostport` argument so the proxy can forward the request.
The [go http proxy](https://golang.org/pkg/vendor/golang.org/x/net/http/httpproxy/) is an example of a supported proxy.
Unix Sockets, TCP addresses, and URLs are supported values for the proxy arguments.


