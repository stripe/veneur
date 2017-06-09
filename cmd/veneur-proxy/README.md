`veneur-proxy` is a proxy that sits two sets of [Veneur](https://github.com/stripe/veneur) instances: those that run on all your hosts and those that act as global aggregators.

# Setup

ADD DIAGRAM HERE

`veneur-proxy` acts as a stateless bridge. The following is a guide for setting up a Veneur pipeline.

* Be running [consul](https://www.consul.io) in your infrastructure.
* Set up 3 or more Veneur instances as global. Maybe call the consul service `veneur-global-srv`?
* Set up 3 or more `veneur-proxy` instances on hosts, use either DNS — we use [consul](https://www.consul.io) — or load-balancers to expose them.
* Configure the `veneur-proxy` instances to find your global instances via `consul_forward_service_name` [configuration option](https://github.com/stripe/veneur#configuration). If you used `veneur-global-srv` as above, then enter that value!
* Point your per-host Veneur instances to the proxies via a host and port pair using the `forward_address` [configuration option](https://github.com/stripe/veneur#forwarding). This step is dependent on the way you chose to load balancer your proxies.
* Profit!

# How It Works

`veneur-proxy` implements the same API as a global Veneur, accepting metrics send from other Veneur instances. It hashes each metric name and tag value/pair combination, using a [consistent hash ring](https://en.wikipedia.org/wiki/Consistent_hashing) to ensure that metrics are always aggregated at the same global Veneur instance.

## configuration

Use the `consul_refresh_interval` to specify how often Veneur should refresh it's list.

* `debug`: Enable or disable debug logging with true/false.
* `enable_profiling`: Enable or disable go profiling. Danger, might fill up your disk if not cared for.
* `http_address`: The `host:port` pair in which this program will listen for HTTP commands.
* `consul_refresh_interval`: How often to refresh from Consul's healthy nodes. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration)
* `stats_address`: The `host:port` destination to send `veneur-proxy`'s metrics.
* `forward_address`: Use a static host for forwarding…
* `consul_forward_service_name`: The name of a consul service for consistent forwarding.
* `sentry_dsn`: A [Sentry](https://sentry.io) DSN to which errors will be sent.

## Concerns

* When metrics are accepted, the act of forwarding them is **asynchronous**. As far as the client is concerned the HTTP operation always succeeds. This tradeoff is because any failures are expected to be short in duration and there is no mechanism for notify clients of failure due to the nature of UDP.
* The list of global servers is locked when refreshing and flushing to avoid race conditions. If your retrieval of consul hosts (see metric `veneur.discoverer.update_duration_ns`) or flushes (see metric `veneur.flush.total_duration_ns`) are slow, you see one or the other slow down.
* A [consistent hash ring](https://en.wikipedia.org/wiki/Consistent_hashing) is used mitigate the impact of changes in Consul's list of healthy nodes. This is not perfect, and you can expect some churn whenever the list of healthy nodes changes in Consul.

# Operation

## Replacing A Global Veneur

Using either Consul's health checks or other means, remove the instance you're working on. Within `consul_refresh_interval` the proxies should remove the host and rebalance the ring. To add the new host, simply turn it on and wait for it to show up in Consul. `veneur-proxy` will do the rest.

## Replacing A Proxy Veneur

Using either Consul or some sort of load balancer, remove the proxy instance. Per-instance veneurs should stop flushing to the proxies. After this time you can replace and add a new proxy, as all proxy work is stateless.

## Monitoring

ADD METRICS HERE
