package main

import (
    "flag"
    "fmt"
    "github.com/DataDog/datadog-go/statsd"
    "strings"
    "github.com/Sirupsen/logrus"
    "time"
)

var (
    hostport = flag.String("hostport", "", "Hostname and port of destination.")
    namespace = flag.String("ns", "", "Namespace for metrics. Ex: daemontools")
    name = flag.String("name", "", "Name of metric to report. Ex: service.starts")
    gauge = flag.Float64("gauge", 0, "Report a 'gauge' metric. Value must be float64.")
    timing = flag.Duration("timing", time.Now().Sub(time.Now()), "Report a 'timing' metric. Value must be parseable by time.ParseDuration.")
    timeinms = flag.Float64("timeinms", 0, "Report a 'timing' metric, in milliseconds. Value must be float64.")
    incr = flag.Bool("incr", false, "Report an 'incr' metric.")
    decr = flag.Bool("decr", false, "Report a 'decr' metric.")
    count = flag.Int("count", 0, "Report a 'count' metric. Value must be an integer.")
    tag = flag.String("tag", "", "Tag for metric. Ex: `service:airflow")
)


func main () {
    flag.Parse()

    if hostport == nil || *hostport == "" || !strings.Contains(*hostport, ":") {
       logrus.Fatal("You must specifiy a valid destination host and port.") 
    }

    fmt.Println("hostport: ", *hostport)
    fmt.Println("namespace: ", *namespace)
    fmt.Println("name: ", *name)
    fmt.Println("gauge: ", *gauge)
    fmt.Println("timing: ", *timing)
    fmt.Println("timeinms: ", *timeinms)
    fmt.Println("incr: ", *incr)
    fmt.Println("decr: ", *decr)
    fmt.Println("count: ", *count)

    conn, err := statsd.New(*hostport)
    if err != nil {
        panic("ERROR")
    }

    conn.Namespace = *namespace + "."
    conn.Tags = append(conn.Tags, *tag)

    if (*incr) {
        conn.Incr(*name, nil, 1)
    }



    /*
    // Flag instantiation
    var hostport, namespace, name, metric_type, tag, value string

    // Flag definitions
    flag.StringVar(&hostport, "hostport", "127.0.0.1:8080", "Hostname and port of destination.")
    flag.StringVar(&namespace, "namespace", "foo", "Namespace of metrics. Ex: daemontools")
    flag.StringVar(&name, "name", "foo", "Name of metric to report to Veneur. Ex: service.starts")
    flag.StringVar(&metric_type, "type", "foo", "Type of metric to report. One of: gauge, timing, timeinms, incr, decr, count.")
    flag.StringVar(&tag, "tag", "foo", "Tag for metric. Ex: #service:airflow")
    //flag.StringVar(&value, "value", "0", "Value of metric, Not used for 'incr' or 'decr'.")

    // add value

    // Parse flags
    flag.Parse()

    fmt.Println("hostport: ", hostport)
    if (!strings.Contains(hostport, ":")) {
        fmt.Println("Error: bad hostport")
        return 
    }

    // TODO: can we validate these?
    fmt.Println("namespace: ", namespace)
    fmt.Println("name: ", name)
    fmt.Println("type: ", metric_type)
    fmt.Println("tag: ", tag)
    fmt.Println("value: ", value)

    // Set up connection to server
    conn, err := statsd.New(hostport)
    if err != nil {
        panic("ERROR")
    }
    conn.Namespace = namespace + "."
    conn.Tags = append(conn.Tags, tag)

    // Validate type *and* send actual metric.
    switch metric_type {
        case "gauge":
            err = conn.Gauge(name, 15, nil, 1)
        case "timing":
            err = conn.Timing(name, 12, nil, 1)
        case "timeinms":
            err = conn.TimeInMilliseconds(name, 10, nil, 1)
        case "incr":
            err = conn.Incr(name, nil, 1)
        case "decr":
            err = conn.Decr(name, nil, 1)
        case "count":
            err = conn.Count(name, 10, nil, 1)
        default:
            fmt.Println("Unrecognized metric type. Must be one of gauge, timing, timeinms, incr, decr, count.")
    }
    */
}

// echo "daemontools.service.starts:1|c|#service:<%= @name %>" | nc -q 1 -u <%= @statsd_host %> <%= @statsd_port %>
