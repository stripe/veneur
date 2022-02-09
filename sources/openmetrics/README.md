# OpenMetrics Source

The  `openmetrics` source is used to ingest metrics into Veneur by scraping
endpoints that support the [OpenMetrics](https://openmetrics.io/) specification.

## Development Status

This source is still under active development, and is subject to breaking
changes.

## Usage

In order to enable the source, add the following entry to the  `sources` field
in Veneur's configuration:
```
sources:
  - kind: openmetrics
    name: openmetrics
    config:
      scrape_interval: 30s
      scrape_target: http://example-service:8000/metrics
      scrape_timeout: 5s
```

The metrics source can be configured with the following attributes:

### allowlist

Optional. Type: regex.

A regular expression matching metric names that should be used by the source.

### denylist

Optional. Type: regex.

A regular expression matching metric names that should be ignored by the source.
This field is not used if `allowlist` is set.

### histogram_bucket_tag

Optional. Type: string. Default: `"le"`.

This field overrides the default histogram bucket tag specified by OpenMetrics.
This field generally should not be used.

### scrape_interval

Required. Type: duration.

The interval at which the source scrapes targets. Warning: Currently this _must_
be set to the flush interval for the OpenMetrics source to work. 

### scrape_target

Required. Type: URL.

### scrape_timeout

Optional. Type: duration. Default: `scrape_interval`.

The duration after which a scrape request times out. The value of
`scrape_timeout` must be less than or equal to the value of `scrape_interval`.

### summary_quantile_tag

Optional. Type: string. Default: `"quantile"`

This field overrides the default summary quantile tag specified by OpenMetrics.
This field generally should not be used.