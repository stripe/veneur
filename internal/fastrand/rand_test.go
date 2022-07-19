package fastrand

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandPoolUsage(t *testing.T) {
	t.Parallel()

	s := NewRandPool()

	ok := false
	// seeing four int64(0)s in a row is very (1/(2^(8*4))) unlikely, and
	// indicates something is broken in our randomness pool
	for i := 0; i < 4; i++ {
		if s.Int63() != 0 {
			ok = true
			break
		}
	}
	require.True(t, ok)
}

func TestGlobalUsage(t *testing.T) {
	t.Parallel()

	const ε = .00002

	ok := false
	// seeing four int64(0)s in a row is very (1/(2^(8*4))) unlikely, and
	// indicates something is broken in our randomness pool
	for i := 0; i < 4; i++ {
		if Int63() != 0 {
			ok = true
			break
		}
	}
	require.True(t, ok)

	// same type of test for Float64, but check against ε rather than literal 0.0
	for i := 0; i < 4; i++ {
		n := Float64()
		require.False(t, math.IsNaN(n))
		// Float64's return value is in [0, 1) (always non-negative) so no Abs
		// is needed for this comparison
		if n > ε {
			ok = true
			break
		}
	}
	require.True(t, ok)
}
