# Veneur Proxy

`veneur-proxy` is a proxy that consistently hashes metrics being forwarded to
veneur instances acting as global aggregators.

## How It Works

`veneur-proxy` implements the same gRPC API as the proxy source in a veneur
instance, accepting metrics send from other veneur instances. It hashes each
metric name and tag value pair using a
[consistent hash ring](https://en.wikipedia.org/wiki/Consistent_hashing)
to ensure that metrics from a given timeseries are always aggregated by the same
global veneur instance.

## Configuration

  * `debug`: Enables debug logging.
  * `dial_timeout`: Sets the timeout to dial downstream veneur instances.
  * `discovery_interval`: Sets the interval at which to discover downstream
    veneur instances.
  * `forward_addresses`: Adds statically defined downstream veneur instances.
  * `forward_service`: Sets service name of the downstream veneur instances
    for discovery.
  * `grpc_server.connection_timeout`: Sets the gRPC connection timeout for
    connections to downstream veneur instances.
  * `grpc_server.max_connection_idle`: Sets the maximum gRPC connection idle
    time for connections to downstream veneur instances.
  * `grpc_server.max_connection_age_grace`: Sets instances maximum gRPC
    connection duration for connections to downstream veneur instances. This
    value controls how long it takes for traffic to re-balance when a new veneur
    proxy instance is added.
  * `grpc_server.ping_timeout`: Sets the gRPC ping timeout for connections to
    downstream veneur instances.
  * `grpc_server.keepalive_timeout`: Sets the gRPC keepalive timeout for
    connections to downstream veneur instances.
  * `grpc_address`: Sets the gRPC address at which the instance listens for
     non-TLS connections.
  * `grpc_tls_address`: Sets the gRPC address at which the instance listens for
     TLS connections.
  * `enable_config`: Enables viewing the configuration via HTTP.
  * `enable_profiling`: Enables profiling via HTTP.
  * `http_address`: Sets the HTTP address at which the instance listens.
  * `ignore_tags`: Specifies matchers for tags that should be ignored when
    performing consistent hashing.
  * `runtime_metrics_interval`: Sets the interval at which runtime metrics are
    emitted.
  * `send_buffer_size`: Sets the size of the send buffer.
  * `sentry_dsn`: Sets the Sentry DSN.
  * `shutdown_timeout`: Sets the timeout after which the process will force
    exit.
  * `statsd`: Configures emission of statsd metrics.
  * `tls.ca_file`: Sets the TLS certificate authority file.
  * `tls.cert_file`: Sets the TLS certificate file.
  * `tls.key_file`: Sets the TLS key file.
