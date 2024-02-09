package sfxclient

// HTTPSinkOption can be passed to NewHTTPSink to customize it's behaviour
type HTTPSinkOption func(*HTTPSink)

// WithSAPMTraceExporter takes a reference to HTTPSink and configures it to export using the SAPM protocol.
func WithSAPMTraceExporter() HTTPSinkOption {
	return func(s *HTTPSink) {
		s.traceMarshal = sapmMarshal
		s.contentTypeHeader = contentTypeHeaderSAPM
		s.TraceEndpoint = TraceIngestSAPMEndpointV2
	}
}

// WithZipkinTraceExporter takes a reference to HTTPSink and configures it to export using the Zipkin protocol.
func WithZipkinTraceExporter() HTTPSinkOption {
	return func(s *HTTPSink) {
		s.traceMarshal = jsonMarshal
		s.contentTypeHeader = contentTypeHeaderJSON
		s.TraceEndpoint = TraceIngestEndpointV1
	}
}
