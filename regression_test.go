package veneur

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	proto "github.com/golang/protobuf/proto"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

func validSample() ssf.SSFSpan {
	return *&ssf.SSFSpan{
		TraceId:        1,
		Id:             1,
		StartTimestamp: 1,
		EndTimestamp:   10,
	}
}

// Tests that setting the tag "Name" will result in a span with span.Name being set,
// and the tag "Name" being deleted.
func TestTagNameSetNameNotSet(t *testing.T) {
	sample := validSample()
	sample.Tags = make(map[string]string)
	sample.Tags["name"] = "testName"

	buf, err := proto.Marshal(&sample)
	assert.NoError(t, err, "Eror when marshalling sample")

	newSample, metrics, errSSF := samplers.ParseSSF(buf)
	assert.Equal(t, sample.Tags["name"], newSample.Name, "Name via Tag did not propogate")
	assert.Zero(t, len(metrics))
	assert.NoError(t, errSSF)
	assert.Empty(t, newSample.Tags["name"])
}

// Tests that setting a tag "Name" and span.Name won't change
// span.Name to be the tag "Name", and that the tag is still there
// after we parse the packet.
func TestTagNameSetNameSet(t *testing.T) {
	sample := validSample()
	sample.Tags = make(map[string]string)
	sample.Tags["name"] = "testName"
	sample.Name = "realName"

	buf, err := proto.Marshal(&sample)
	assert.NoError(t, err, "Error when marshalling sample")

	newSample, metrics, errSSF := samplers.ParseSSF(buf)
	assert.Equal(t, sample.Name, newSample.Name, "Name did not propogate")
	assert.Zero(t, len(metrics))
	assert.NoError(t, errSSF)
	assert.NotEmpty(t, newSample.Tags["name"])
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

func TestOperation(t *testing.T) {
	pbFile := filepath.Join("fixtures", "protobuf", "span-with-operation-062017.pb")
	pb, err := os.Open(pbFile)
	assert.NoError(t, err)
	defer pb.Close()

	packet, err := ioutil.ReadAll(pb)
	assert.NoError(t, err)

	sample, metrics, errSSF := samplers.ParseSSF(packet)
	assert.NoError(t, errSSF)
	assert.Zero(t, len(metrics))
	assert.NotNil(t, sample)
}
