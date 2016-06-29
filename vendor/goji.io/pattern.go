package goji

// httpMethods is an internal interface for the HTTPMethods pattern
// optimization. See the documentation on Pattern for more.
type httpMethods interface {
	HTTPMethods() map[string]struct{}
}

// pathPrefix is an internal interface for the PathPrefix pattern optimization.
// See the documentation on Pattern for more.
type pathPrefix interface {
	PathPrefix() string
}
