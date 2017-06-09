`veneur-emit` is a command line utility for emitting metrics to [Veneur](https://github.com/stripe/veneur).

# Usage

```
Usage of ./veneur-emit:
  -count int
    	Report a 'count' metric. Value must be an integer.
  -debug
    	Turns on debug messages.
  -f string
    	The Veneur config file to read for settings.
  -gauge float
    	Report a 'gauge' metric. Value must be float64.
  -hostport string
    	Hostname and port of destination. Must be used if config file is not present.
  -name string
    	Name of metric to report. Ex: daemontools.service.starts
  -tag string
    	Tag(s) for metric, comma separated. Ex: service:airflow
  -timing duration
    	Report a 'timing' metric. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration). (default -10ns)
```
