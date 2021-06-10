package tagging

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagSliceToMap(t *testing.T) {
	tags := []string{"foo:bar", "baz:gorch", "blerg"}
	mappedTags := ParseTagSliceToMap(tags)
	assert.Equal(t, "bar", mappedTags["foo"])
	assert.Equal(t, "gorch", mappedTags["baz"])
	assert.Equal(t, "", mappedTags["blerg"])
}
