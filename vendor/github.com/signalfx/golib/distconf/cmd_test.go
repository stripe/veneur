package distconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCmdLoader(t *testing.T) {
	l, err := CmdLoader("pre").Get()
	assert.NoError(t, err)
	assert.Equal(t, "pre", l.(*cmdDisco).prefix)
}

func TestPrefixParse(t *testing.T) {
	l := &cmdDisco{
		prefix: "pre",
		source: []string{"wha", "prebob=3"},
	}
	b, err := l.Get("bob")
	assert.NoError(t, err)
	assert.Equal(t, []byte("3"), b)

	b, err = l.Get("nothere")
	assert.NoError(t, err)
	assert.Nil(t, b)
}
