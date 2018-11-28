// Package trace provies an API for initiating and reporting SSF
// traces and attaching spans to them. Veneur's tracing API also
// provides an experimental opentracing compatibility layer.
//
// The Veneur tracing API is mostly independent of the opentracing
// compatibility layer, however OpenTracing compatibility may be
// removed in a future release.
//
// Setup and Structure of the API
//
// To use the API, user code must set its service name via the global
// variable Service, which is used by trace visualization tools to
// tell apart the spans reported by different services. Code using the
// trace API must set its Service, and set it only once, in the
// application's main package, ideally in the init() function:
//
//    trace.Service = "my-awesome-service"
//
// The trace API makes heavy use of the go stdlib's Contexts and is
// built on three abstractions: the Client, the ClientBackend and the
// Span.
//
// Contexts
//
// Since it is very common for networked code to use the go stdlib's
// Context interface, veneur's trace API is designed to be used
// together with Contexts. The API uses Contexts to store the current
// Span, and will automatically pick the appropriate parent span from
// the Context.
//
// To effectively use the trace API, user code needs to ensure that
// all functions that communicate externally (which includes using
// traces) take Contexts as parameters, and pass them down.
//
// See https://blog.golang.org/context for an introduction to using
// Context.
//
// Spans
//
// Spans represent the duration of a "unit of work" in a system: They
// have a start and an end time, a name, and additional data that can
// help identify what the work was. Example units of work could be the
// handling of an HTTP request, or a database read. For convenience,
// it is usually easiest to have a span represent a single function to
// delineate the start and end, which makes it easy to create the Span
// and report it via a defer statement.
//
// To create a new Span, use StartSpanFromContext. This function will
// create a Span with a parent ID pointing to the to the trace that
// came before it in the Context. Typically, users should pass "" as
// the name to let StartSpanFromContext figure out the correct name
// from caller information.
//
//   span, ctx := trace.StartSpanFromContext(ctx, "")
//   // report the span with the default client when this function returns:
//   defer span.Finish()
//   ... // do work here
//
// Reporting Spans via the trace Client
//
// Once constructed, SSF Spans must be reported to a system that
// processes them. To do that, package trace exports a trace
// Client. Typical applications will want to use the DefaultClient
// exposed by this package. By default, it is set up to send spans to
// veneur listening on the default SSF UDP port, 8128. An application
// can use SetDefaultClient to change the default client in its main
// function.
//
// To allow testing user code's Span reporting behavior, it is
// desirable to take a Client argument in tested functions and report
// spans to that client explicitly:
//
//   span, ctx := trace.StartSpanFromContext(ctx, "")
//   defer span.ClientFinish(trace.DefaultClient)
//
// In case it is desired to submit no Spans at all, nil is a supported
// trace Client value.
//
// Client Backends
//
// The trace Client can use various transports to send spans to a
// destination. It is the Backend's job to perform any necessary
// serialization.
//
// Package trace exports some networked backends (sending
// protobuf-encoded SSF spans over UDP and UNIX domain sockets), and
// allows users to define their own ClientBackends, which can be
// useful in tests. See the ClientBackend interface's documentation
// for details.
//
// Additional information on Spans
//
// There are several additional things that can be put on a Span: the
// most common ones are Errors, Tags and Samples. Spans can also be
// indicators. The following sections will talk about these in detail.
//
// Error handling
//
// Call span.Error with the error that caused a Span to
// fail. Different programs have different requirements for when a
// Span should be considered to have "failed". A typical rule of thumb
// is that if you have a traced function that returns an error and it
// would return an error, that function should also use
// span.Error. This way, anyone who views a trace visualization will
// be able to tell that an error happened there. To report an error,
// use the following:
//
//   if err != nil {
//           span.Error(err)
//           return err
//   }
//
// Tags
//
// Spans can hold a map of name-value pairs (both strings) on their
// Tags field. These will be reported to trace visualization tools as
// indexed tags, so it should be possible to search for spans with the
// relevant information.
//
// Note: a Span object, when created, will have a nil Tags map to
// reduce the number of unnecessary allocations. To add Tags to a
// Span, it is recommended to initialize the Tags object with a
// constructed map like so:
//
//   span.Tags = map[string]string{
//           "user_id": userID,
//           "action": "booped",
//   }
//
// Samples
//
// Veneur's trace API supports attaching SSF Samples to Spans. These
// can be metrics, service checks or events. Veneur will extract them
// from the span and report them to any configured metrics provider.
//
// In the future, veneur may add information that allows linking the
// extracted metrics to the spans on which they were reported.
//
// See the ssf package and the trace/metrics package for details on
// using Samples effectively.
//
// Indicator Spans
//
// Typically, a traced service has some units of work that indicate
// how well a service is working. For example, a hypothetical
// HTTP-based payments API might have a charge endpoint. The time that
// an API call takes on that endpoint and its successes or failures
// can be used to extract service-level indicators (SLI) for latency,
// throughput and error rates.
//
// Concretely, the veneur server privileges these Indicator Spans by
// doing exactly the above: It reports a timer metric tagged with the
// service name on the span, and a flag indicating whether the span
// had Error set.
//
// To report an indicator span, use:
//
//   span.Indicator = true
//
// OpenTracing Compatibility
//
// Package trace's data structure implement the OpenTracing
// interfaces. This allows users to apply all of OpenTracing's helper
// functions, e.g. to extract a Span from the context, use
// SpanFromContext and a type assertion:
//
//   span, ok := opentracing.SpanFromContext(ctx).(*trace.Span)
//
// See https://godoc.org/github.com/opentracing/opentracing-go for
// details on the OpenTracing API.
package trace
