package safepool

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewPool(t *testing.T) {
	const theNumber = int(3)
	p := NewPool(func() int { return theNumber })
	n := p.Get()
	require.Equal(t, theNumber, n)
	p.Put(n)
}
