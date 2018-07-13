package dataunit

// Size represents a measurement of data and can return that measurement
// in different units (B, KB, MB, GB, TB, PB, EB)
type Size int64

// Bytes returns the size as a uint64 of bytes
func (s Size) Bytes() int64 {
	return int64(s)
}

// Kilobytes returns the size as a float64 of kilobytes
func (s Size) Kilobytes() float64 {
	return float64(s) / float64(Kilobyte)
}

// Megabytes returns the size as a float64 of kilobytes
func (s Size) Megabytes() float64 {
	return float64(s) / float64(Megabyte)
}

// Gigabytes returns the size as a float64 of gigabytes
func (s Size) Gigabytes() float64 {
	return float64(s) / float64(Gigabyte)
}

// Terabytes returns the size as a float64 of terabytes
func (s Size) Terabytes() float64 {
	return float64(s) / float64(Terabyte)
}

// Petabytes returns the size as a float64 of petabytes
func (s Size) Petabytes() float64 {
	return float64(s) / float64(Petabyte)
}

// Exabytes returns the size as a float64 of exabytes
func (s Size) Exabytes() float64 {
	return float64(s) / float64(Exabyte)
}

// Common size units
const (
	Byte     Size = 1
	Kilobyte      = 1024 * Byte
	Megabyte      = 1024 * Kilobyte
	Gigabyte      = 1024 * Megabyte
	Terabyte      = 1024 * Gigabyte
	Petabyte      = 1024 * Terabyte
	Exabyte       = 1024 * Petabyte
)
