package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"

	proto "github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

/*
If tag name is set and Name is null, we want Span name to be set when it exits.
If span Name is set, we want it to stay set even with name tag
Emit a span with operation set and make sure nothing breaks

tag name set; name null
tag name set, name set
tag name null, name set
operation set
*/

func validSample() ssf.SSFSpan {
	return *&ssf.SSFSpan{
		TraceId:        1,
		Id:             1,
		StartTimestamp: 1,
		EndTimestamp:   10,
	}
}

func TestTagNameSet(t *testing.T) {
	sample := validSample()
	sample.Tags = make(map[string]string)
	sample.Tags["name"] = "testName"

	t.Run("nameNotSet", func(t *testing.T) {
		buf, err := proto.Marshal(&sample)
		assert.NoError(t, err, "Eror when marshalling sample")

		newSample, metrics, errSSF := samplers.ParseSSF(buf)
		assert.Equal(t, sample.Tags["name"], newSample.Name, "Name via Tag did not propogate")
		assert.Zero(t, len(metrics))
		assert.NoError(t, errSSF)
	})

	t.Run("nameSet", func(t *testing.T) {
		sample.Name = "realName"

		buf, err := proto.Marshal(&sample)
		assert.NoError(t, err, "Error when marshalling sample")

		newSample, metrics, errSSF := samplers.ParseSSF(buf)
		assert.Equal(t, sample.Name, newSample.Name, "Name did not propogate")
		assert.Zero(t, len(metrics))
		assert.NoError(t, errSSF)
	})
}

func TestNoTagName(t *testing.T) {
	sample := validSample()
	sample.Name = "realName"

	buf, err := proto.Marshal(&sample)
	assert.NoError(t, err)

	newSample, metrics, errSSF := samplers.ParseSSF(buf)
	assert.Equal(t, sample.Name, newSample.Name, "Name did not propogate")
	assert.Zero(t, len(metrics))
	assert.NoError(t, errSSF)
}
