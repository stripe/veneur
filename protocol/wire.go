// Package protocol contains routines for implementing veneur's SSF
// wire protocol to read and write framed SSF samples on a streaming
// network link or other non-seekable medium.
//
// SSF Wire Protocol
//
// SSF uses protobufs internally, which aren't encapsulated or framed
// in any way that would allow them to be read on a streaming network
// connection. To counteract that, the SSF Wire Protocol frames SSF
// messages in the following way:
//
//   [ 8 bits - version and type of message]
//   [32 bits - length of framed message in octets]
//   [<length> - SSF message]
//
// The version and type of message can currently only be set to the
// value 0, which means that what follows is a protobuf-encoded
// ssf.SSFSpan.
//
// The length of the framed message is a number of octets (8-bit
// bytes) in network byte order (big-endian), specifying the number of
// octets taken up by the SSF message that follows directly on the
// stream. To avoid DoS'ing Veneur instances, no lengths greater than
// MaxSSFPacketLength (currently 16MB) can be read or encoded.
//
// Since this protocol does not contain any re-syncing hints, any
// framing error on the stream is automatically fatal. The stream must
// be considered unreadable from that point on and should be closed.
package protocol

import (
	"io"
	"sync"

	"encoding/binary"

	"github.com/gogo/protobuf/proto"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

// MaxSSFPacketLength is the maximum length of an SSF packet. This is
// currently 16MB.
const MaxSSFPacketLength uint32 = 16 * 1024 * 1024

// SSFFrameLength is the length of an SSF Frame. This is currently 5
// bytes - 1 byte for the version and 4 bytes for the 32-bit content
// length.
const SSFFrameLength uint32 = 1 + 4

// The only version we support right now: A frame with a length
// followed by an ssf.SSFSpan.
const version0 uint8 = 0

func readFrame(in io.Reader, length int) ([]byte, error) {
	bts := make([]byte, length)
	read := 0
	for {
		n, err := in.Read(bts[read:])
		if err != nil {
			return []byte{}, err
		}
		read += n
		if read == length {
			return bts, nil
		}
	}
}

// ReadSSF reads a framed SSF span from a stream and returns a parsed
// SSFSpan structure and a set of statsd metrics.
//
// If this function returns an error, client code must check it with
// IsFramingError to decide if the error means the stream is
// unrecoverably broken. The error is EOF only if no bytes were read
// at the start of a message (e.g. if a connection was closed after
// the last message).

func ReadSSF(in io.Reader) (*samplers.Message, error) {
	var version uint8
	var length uint32
	if err := binary.Read(in, binary.BigEndian, &version); err != nil {
		if err == io.EOF {
			// EOF/hang-ups at the start of a new message
			// are fine, pass them through as-is.
			return nil, err
		}
		return nil, &errFramingIO{err}
	}
	if version != version0 {
		return nil, &errFrameVersion{version}
	}
	if err := binary.Read(in, binary.BigEndian, &length); err != nil {
		return nil, &errFramingIO{err}
	}
	if length > MaxSSFPacketLength {
		return nil, &errFrameLength{length}
	}
	bts, err := readFrame(in, int(length))
	if err != nil {
		return nil, &errFramingIO{err}
	}
	return samplers.ParseSSF(bts)
}

var pbufPool = sync.Pool{
	New: func() interface{} {
		return proto.NewBuffer(nil)
	},
}

// WriteSSF writes an SSF span with a preceding v0 frame onto a stream
// and returns the number of bytes written, as well as an error.
//
// If the error matches IsFramingError, the stream must be considered
// poisoned and should not be re-used.
func WriteSSF(out io.Writer, ssf *ssf.SSFSpan) (int, error) {
	pbuf := pbufPool.Get().(*proto.Buffer)
	err := pbuf.Marshal(ssf)
	if err != nil {
		// This is not a framing error, as we haven't written
		// anything to the stream yet.
		return 0, err
	}
	defer func() {
		// Make sure we reset the scratch protobuffer (by default, it
		// would retain its contents) and put it back into the pool:
		pbuf.Reset()
		pbufPool.Put(pbuf)
	}()

	if err = binary.Write(out, binary.BigEndian, version0); err != nil {
		return 0, &errFramingIO{err}
	}
	if err = binary.Write(out, binary.BigEndian, uint32(len(pbuf.Bytes()))); err != nil {
		return 0, &errFramingIO{err}
	}
	n, err := out.Write(pbuf.Bytes())
	if err != nil {
		return n, &errFramingIO{err}
	}
	return n, nil
}
