package protocol

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
)

func TestReadSSFStream(t *testing.T) {
	msg := &ssf.SSFSpan{
		Version:        1,
		TraceId:        1,
		Id:             2,
		ParentId:       3,
		StartTimestamp: 9000,
		EndTimestamp:   9001,
		Tags:           map[string]string{},
	}
	// Write it to a reader twice:
	buf := bytes.NewBuffer([]byte{})
	_, err := WriteSSF(buf, msg)
	require.NoError(t, err)
	_, err = WriteSSF(buf, msg)
	require.NoError(t, err)
	// Read the first frame:
	{
		read, _, err := ReadSSF(buf)
		require.NoError(t, err)
		assert.Equal(t, *msg, *read)
	}
	// Read the second frame:
	{
		read, _, err := ReadSSF(buf)
		require.NoError(t, err)
		assert.Equal(t, *msg, *read)
	}
}

func TestReadSSFStreamBad(t *testing.T) {
	msg := &ssf.SSFSpan{
		Version:        1,
		TraceId:        1,
		Id:             2,
		ParentId:       3,
		StartTimestamp: 9000,
		EndTimestamp:   9001,
		Tags:           map[string]string{},
	}

	// Bad: illegal frame:
	{
		buf := bytes.NewBuffer([]byte{0x01, 0x00})
		read, _, err := ReadSSF(buf)
		if assert.Error(t, err) {
			assert.True(t, IsFramingError(err))
			assert.Nil(t, read)
		}
	}

	// Bad: wrong length of packet in header:
	{
		buf := bytes.NewBuffer([]byte{})
		_, err := WriteSSF(buf, msg)
		if assert.NoError(t, err) {
			// Mess with the length byte:
			buf.Bytes()[1] = 0xff
			read, _, err := ReadSSF(buf)
			if assert.Error(t, err) {
				assert.True(t, IsFramingError(err))
			}
			assert.Nil(t, read)
		}
	}

	// Bad: invalid protobuf in SSF message:
	{
		buf := bytes.NewBuffer([]byte{})
		_, err := WriteSSF(buf, msg)
		if assert.NoError(t, err) {
			// Mess with some bytes post the length:
			buf.Bytes()[7] = 0xff
			read, _, err := ReadSSF(buf)
			if assert.Error(t, err) {
				// This is not a framing error:
				assert.False(t, IsFramingError(err))
			}
			assert.Nil(t, read)
		}
	}
}
