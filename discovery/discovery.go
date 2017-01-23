package discovery

// Discoverer is an interface for various
// service discovery mechanisms.
type Discoverer interface {
	UpdateDestinations(string) ([]string, error)
}
