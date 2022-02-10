# OpenMetrics Source Demo

In order to run the demo, from this directory, run

```
go run github.com/stripe/veneur/v14/sources/openmetrics/demo
```

In the output, you should see
  - a counter `demo_sample_counter`, that increments once per flush interval
  - a gauge, `demo_sample_gauge`, that cycles through the values 1, 5, 4, 6, 2,
    and 3.
  - a histogram, `demo_sample_histogram`, that aggregates the values from the
    sample gauge into three buckets: `≤ 2.0`, `≤ 5.0`, and `≤ +∞`.
  - a summary, `demo_sample_summary`, that captures the 50th and 84th
    percentiles of a normally distributed random variable with a mean of zero
    and standard deviation of 10, sampled once per flush interval. The 50th
    percentile should converge to zero and the 90th percentile should converge
    to approximately 12.8.