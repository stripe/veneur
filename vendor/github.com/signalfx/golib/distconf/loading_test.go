package distconf

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromLoaders(t *testing.T) {
	c := FromLoaders([]BackingLoader{MemLoader()})
	assert.Equal(t, 1, len(c.readers))
}

func TestFromLoadersWithErrors(t *testing.T) {
	c := FromLoaders([]BackingLoader{BackingLoaderFunc(func() (Reader, error) {
		return nil, errors.New("nope")
	})})
	assert.Equal(t, 0, len(c.readers))
}
