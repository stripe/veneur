package protocol

import "fmt"

type errFramingIO struct {
	err error
}

func (err *errFramingIO) Error() string {
	return fmt.Sprintf("SSF framing I/O error: %v", err.err)
}

type errFrameVersion struct {
	actual uint8
}

func (err *errFrameVersion) Error() string {
	return fmt.Sprintf("SSF framing error: unexpected version number %d", err.actual)
}

type errFrameLength struct {
	actual uint32
}

func (err *errFrameLength) Error() string {
	return fmt.Sprintf("SSF framing error: length %d is too large", err.actual)
}

// IsFramingError returns true if an error is a wire protocol framing
// error. This indicates that the stream can no longer be used for
// reading SSF data and should be closed.
func IsFramingError(err error) bool {
	switch err.(type) {
	case *errFrameVersion:
		return true
	case *errFramingIO:
		return true
	case *errFrameLength:
		return true
	}
	return false
}
