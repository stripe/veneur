package main

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-go/statsd"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

func main() {
	flag.Parse()

	c, _ := statsd.New("127.0.0.1:8200")

	resp, _ := http.Get("http://localhost:9090/metrics")
	d := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
	var mf dto.MetricFamily
	for {
		err := d.Decode(&mf)
		if err != nil {
			break
		}

		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			for _, counter := range mf.GetMetric() {
				var tags []string
				labels := counter.GetLabel()
				for _, pair := range labels {
					tags = append(tags, fmt.Sprintf("%s:%s", pair.GetName(), pair.GetValue()))
				}
				c.Count(mf.GetName(), int64(counter.GetCounter().GetValue()), tags, 1.0)
			}
		case dto.MetricType_GAUGE:
			for _, gauge := range mf.GetMetric() {
				var tags []string
				labels := gauge.GetLabel()
				for _, pair := range labels {
					tags = append(tags, fmt.Sprintf("%s:%s", pair.GetName(), pair.GetValue()))
				}
				c.Gauge(mf.GetName(), float64(gauge.GetGauge().GetValue()), tags, 1.0)
			}
		}
	}
}
