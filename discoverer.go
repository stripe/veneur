package veneur

// Discoverer is an interface for various service discovery mechanisms.
// You could implement your own by implementing this method! See consul.go
type Discoverer interface {
	GetDestinationsForService(string) ([]string, error)
}
