package main

import "flag"
import "fmt"
import "github.com/DataDog/datadog-go/statsd"
import "strings"

func main () {
    // Flag instantiation
    var hostport, namespace, name, metric_type, tag string

    // Flag definitions
    flag.StringVar(&hostport, "hostport", "127.0.0.1:8080", "Hostname and port of destination.")
    flag.StringVar(&namespace, "namespace", "foo", "Namespace of metrics. Ex: daemontools")
    flag.StringVar(&name, "name", "foo", "Name of metric to report to Veneur. Ex: service.starts")
    flag.StringVar(&metric_type, "type", "foo", "Type of metric to report. One of: guage, timing, timeinms, incr, decr, count.")
    flag.StringVar(&tag, "tag", "foo", "Tag for metric. Ex: #service:airflow")

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

    // Set up connection to server
    conn, err := statsd.New(hostport)
    if err != nil {
        panic("ERROR")
    }
    conn.Namespace = namespace + "."
    conn.Tags = append(conn.Tags, tag)

    // Validate type *and* send actual metric.
    switch metric_type {
        case "guage":
            err = conn.Gauge(name, 12, nil, 1)
        case "timing":
            fmt.Println("TIMING not supported yet.")
            //err = conn.Timing(name, duration, nil, 1)
        case "timeinms":
            err = conn.TimeInMilliseconds(name, 12, nil, 1)
        case "incr":
            err = conn.Incr(name, nil, 1)
        case "decr":
            err = conn.Decr(name, nil, 1)
        case "count":
            err = conn.Count(name, 2, nil, 1)
        default:
            panic("Unrecognized metric type. Must be one of guage, timing, timeinms, incr, decr, count.")
    }
}

// echo "daemontools.service.starts:1|c|#service:<%= @name %>" | nc -q 1 -u <%= @statsd_host %> <%= @statsd_port %>
