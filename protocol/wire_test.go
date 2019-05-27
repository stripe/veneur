package protocol

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"testing"
	"time"

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
	// Write it to a buffer twice:
	buf := bytes.NewBuffer([]byte{})
	_, err := WriteSSF(buf, msg)
	require.NoError(t, err)
	_, err = WriteSSF(buf, msg)
	require.NoError(t, err)
	// Read the first frame:
	{
		span, err := ReadSSF(buf)
		require.NoError(t, err)
		assert.Equal(t, *msg, *span)
	}
	// Read the second frame:
	{
		span, err := ReadSSF(buf)
		require.NoError(t, err)
		assert.Equal(t, *msg, *span)
	}
}

func TestEOF(t *testing.T) {
	msg := &ssf.SSFSpan{
		Version:        1,
		TraceId:        1,
		Id:             2,
		ParentId:       3,
		StartTimestamp: 9000,
		EndTimestamp:   9001,
		Tags:           map[string]string{},
	}
	buf := bytes.NewBuffer([]byte{})
	_, err := WriteSSF(buf, msg)
	require.NoError(t, err)
	// First frame should work:
	{
		read, err := ReadSSF(buf)
		if assert.NoError(t, err) {
			assert.NotNil(t, read)
		}
	}
	// Second frame should return a plain EOF error:
	{
		read, err := ReadSSF(buf)
		assert.False(t, IsFramingError(err))
		if assert.Equal(t, io.EOF, err) {
			assert.Nil(t, read)
		}
	}
	// subsequent reads should get EOF too:
	{
		read, err := ReadSSF(buf)
		assert.False(t, IsFramingError(err))
		if assert.Equal(t, io.EOF, err) {
			assert.Nil(t, read)
		}
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
	}

	// Bad: illegal frame:
	{
		buf := bytes.NewBuffer([]byte{0x01, 0x00})
		read, err := ReadSSF(buf)
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
			read, err := ReadSSF(buf)
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
			read, err := ReadSSF(buf)
			if assert.Error(t, err) {
				// This is not a framing error:
				assert.False(t, IsFramingError(err))
			}
			assert.Nil(t, read)
		}
	}
}

func BenchmarkValidTrace(b *testing.B) {
	const Len = 1000
	input := make([]*ssf.SSFSpan, Len)
	for i, _ := range input {
		p := make([]byte, 10)
		_, err := rand.Read(p)
		if err != nil {
			b.Fatalf("Error generating data: %s", err)
		}
		msg := &ssf.SSFSpan{
			Version:        1,
			TraceId:        1,
			Id:             2,
			ParentId:       3,
			StartTimestamp: time.Now().Unix(),
			EndTimestamp:   time.Now().Add(5 * time.Second).Unix(),
			Tags: map[string]string{
				string(p[:4]):  string(p[5:]),
				string(p[3:7]): string(p[1:3]),
			},
		}

		switch r := i % 5; r {
		case 1:
			msg.Id = 0
		case 2:
			msg.TraceId = 0
		case 3:
			msg.StartTimestamp = 0
		case 4:
			msg.EndTimestamp = 0
		default:
			// do nothing
		}

		input[i] = msg
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ValidTrace(input[i%Len])
	}

}

func generateTags(n int) map[string]string {
	res := map[string]string{}
	for i := 0; i < n; i++ {
		res[fmt.Sprintf("tag-name-%d", i)] = fmt.Sprintf("tag-value-%d", i)
	}
	return res
}

func generateDims(n int) []*ssf.Dimension {
	res := make([]*ssf.Dimension, 0, n)
	for i := 0; i < n; i++ {
		res = append(res, &ssf.Dimension{
			Key:   fmt.Sprintf("tag-name-%d", i),
			Value: fmt.Sprintf("tag-value-%d", i),
		})
	}
	return res
}

const nTags = 400

func BenchmarkSSFParseSSF(b *testing.B) {
	p := make([]byte, 10)
	_, err := rand.Read(p)
	if err != nil {
		b.Fatalf("Error generating data: %s", err)
	}
	scenarios := []struct {
		name string
		msg  *ssf.SSFSpan
	}{
		{
			name: "span_with_Tags",
			msg: &ssf.SSFSpan{
				Version:        1,
				TraceId:        1,
				Id:             2,
				ParentId:       3,
				StartTimestamp: time.Now().Unix(),
				EndTimestamp:   time.Now().Add(5 * time.Second).Unix(),
				Tags:           generateTags(nTags),
			},
		},
		{
			name: "span_with_Dimensions",
			msg: &ssf.SSFSpan{
				Version:        1,
				TraceId:        1,
				Id:             2,
				ParentId:       3,
				StartTimestamp: time.Now().Unix(),
				EndTimestamp:   time.Now().Add(5 * time.Second).Unix(),
				Dimensions:     generateDims(nTags),
			},
		},
		{
			name: "sample_with_Tags",
			msg: &ssf.SSFSpan{
				Metrics: []*ssf.SSFSample{
					&ssf.SSFSample{
						Metric: ssf.SSFSample_GAUGE,
						Name:   "a_test_sample",
						Value:  100.0,
						Tags:   generateTags(nTags),
					},
				},
			},
		},
		{
			name: "sample_with_Dimensions",
			msg: &ssf.SSFSpan{
				Metrics: []*ssf.SSFSample{
					&ssf.SSFSample{
						Metric:     ssf.SSFSample_GAUGE,
						Name:       "a_test_sample",
						Value:      100.0,
						Dimensions: generateDims(nTags),
					},
				},
			},
		},
	}
	for _, elt := range scenarios {
		bench := elt
		b.Run(bench.name, func(t *testing.B) {
			const Len = 1000
			input := make([][]byte, Len)

			for i, _ := range input {
				msg := bench.msg
				data, err := msg.Marshal()
				assert.NoError(b, err)

				input[i] = data
			}
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := ParseSSF(input[i%Len])
				if err != nil {
					b.Fatalf("Error parsing SSF %s", err)
				}
			}
		})
	}
}

func BenchmarkSSFMarshal(b *testing.B) {
	p := make([]byte, 10)
	_, err := rand.Read(p)
	if err != nil {
		b.Fatalf("Error generating data: %s", err)
	}
	scenarios := []struct {
		name string
		msg  *ssf.SSFSpan
	}{
		{
			name: "span_with_Tags",
			msg: &ssf.SSFSpan{
				Version:        1,
				TraceId:        1,
				Id:             2,
				ParentId:       3,
				StartTimestamp: time.Now().Unix(),
				EndTimestamp:   time.Now().Add(5 * time.Second).Unix(),
				Tags:           generateTags(nTags),
			},
		},
		{
			name: "span_with_Dimensions",
			msg: &ssf.SSFSpan{
				Version:        1,
				TraceId:        1,
				Id:             2,
				ParentId:       3,
				StartTimestamp: time.Now().Unix(),
				EndTimestamp:   time.Now().Add(5 * time.Second).Unix(),
				Dimensions:     generateDims(nTags),
			},
		},
		{
			name: "sample_with_Tags",
			msg: &ssf.SSFSpan{
				Metrics: []*ssf.SSFSample{
					&ssf.SSFSample{
						Metric: ssf.SSFSample_GAUGE,
						Name:   "a_test_sample",
						Value:  100.0,
						Tags:   generateTags(nTags),
					},
				},
			},
		},
		{
			name: "sample_with_Dimensions",
			msg: &ssf.SSFSpan{
				Metrics: []*ssf.SSFSample{
					&ssf.SSFSample{
						Metric:     ssf.SSFSample_GAUGE,
						Name:       "a_test_sample",
						Value:      100.0,
						Dimensions: generateDims(nTags),
					},
				},
			},
		},
	}
	for _, elt := range scenarios {
		bench := elt
		b.Run(bench.name, func(t *testing.B) {
			for i := 0; i < b.N*3000; i++ {
				_, err := bench.msg.Marshal()
				if err != nil {
					b.Fatalf("Error parsing SSF %s", err)
				}
			}
		})
	}
}
