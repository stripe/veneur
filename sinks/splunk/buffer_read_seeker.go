package splunk

import (
	"bytes"
	"fmt"
	"io"
)

// struct bufferReadSeeker is a wrapper around bytes.Buffer that
// implements Read and Seek, so it can be used as a ReadSeeker for the
// HEC client (until that client gets a Reader-only interface).
type bufferReadSeeker struct {
	b *bytes.Buffer
}

func NewBufferReadSeeker(b *bytes.Buffer) io.ReadSeeker {
	return &bufferReadSeeker{b: b}
}

// Read proxies through to the contained bytes.Buffer Read method.
func (frs *bufferReadSeeker) Read(p []byte) (n int, err error) {
	return frs.b.Read(p)
}

var ErrSeekNotSupported = fmt.Errorf("method Seek is not supported on bufferSeekReaders")

// Seek returns ErrSeekNotSupported.
func (frs *bufferReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return 0, ErrSeekNotSupported
}
