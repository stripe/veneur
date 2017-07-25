`veneur-emit` is a command line utility for emitting metrics to [Veneur](https://github.com/stripe/veneur).

Some common use cases:
* Instrument shell scripts
* Instrumenting shell-based tools like init scripts, startup scripts and more
* Testing

# Usage

`veneur-emit` can read an existing veneur [config file](https://github.com/stripe/veneur#configuration). If that's not convenient, you can specify it's configuration options directly.

```
Usage of veneur-emit:
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
  -f string
       The Veneur config file to read for settings.
  -gauge float
       Report a 'gauge' metric. Value must be float64.
  -hostport string
       Hostname and port of destination. Must be used if config file is not present.
  -mode string
       Mode for veneur-emit. Must be one of: 'metric', 'event', 'sc'. (default "metric")
  -name string
       Name of metric to report. Ex: 'daemontools.service.starts'
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
  -ssf
       Sends packets via SSF instead of StatsD. (https://github.com/stripe/veneur/blob/master/ssf/)
  -tag string
       Tag(s) for metric, comma separated. Ex: 'service:airflow'
  -timing duration
       Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).
```
