# Trace

Veneur provides an experimental API for tracing requests between hosts. The API is in an experimental state and is subject to change.

The main Veneur tracing API is defined in the `trace.go` file; however, an OpenTracing compatibility layer is provided for convenience as well. These are functionally equivalent, though it is recommended not to mix the functions. (In other words, if a trace is started using Veneur's tracing API, avoid using the OpenTracing API functions on it or future children).

Eventually, these two interfaces will be consolidated.

# Tracing and Metrics Client

Veneur exposes a novel Go client for combined tracing and metrics that emits [SSF](https://github.com/stripe/veneur/tree/master/ssf#sensor-sensibility-format) formatted data.

## Usage

```
import (
  "github.com/stripe/veneur/trace"
)

client, err := trace.NewClient("udp://localhost:8200", Capacity(1024), ParallelBackends(1))
```

The `trace.NewClient` function creates a new client will dial the specified address and can accept any number of options as arguments as shown with `Capacity` and `ParallelBackends` above. The client exposes methods for emitting spans and for emitting metrics without a span.

### Design

The `NewClient` function uses [Dave Cheney's function options API](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis). TKTK link to docs?

The client is designed in a split style where the client is a "frontend" that interacts with one or more backends. These backends are one of packet- or stream-based. The number is controlled by `ParallelBackends`. Each backend — you probably will default to using just 1 of them — will start a separate goroutine to vie for handling of any SSF items to send.



### Spans

This client does not create spans, it only handles reporting them.

##
