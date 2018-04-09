package fasthashtest

import "testing"

// TestHashString64 is the implementation of a test suite to verify the
// behavior of a hashing algorithm.
func TestHashString64(t *testing.T, name string, reference func(string) uint64, algorithm func(string) uint64) {
	t.Run(name, func(t *testing.T) {
		for _, s := range [...]string{"", "A", "Hello World!", "DAB45194-42CC-4106-AB9F-2447FA4D35C2"} {
			t.Run(s, func(t *testing.T) {
				if reference == nil {
					algorithm(s)
				} else {
					sum1 := reference(s)
					sum2 := algorithm(s)

					if sum1 != sum2 {
						t.Errorf("invalid hash, expected %x but got %x", sum1, sum2)
					}
				}
			})
		}
	})
}

// TestHashUint64 is the implementation of a test suite to verify the
// behavior of a hashing algorithm.
func TestHashUint64(t *testing.T, name string, reference func(uint64) uint64, algorithm func(uint64) uint64) {
	t.Run(name, func(t *testing.T) {
		if reference == nil {
			algorithm(42)
		} else {
			sum1 := reference(42)
			sum2 := algorithm(42)

			if sum1 != sum2 {
				t.Errorf("invalid hash, expected %x but got %x", sum1, sum2)
			}
		}
	})
}

// BenchmarkHashString64 is the implementation of a benchmark suite to compare
// the CPU and memory efficiency of a hashing algorithm against a reference
// implementation.
func BenchmarkHashString64(b *testing.B, name string, reference func(string) uint64, algorithm func(string) uint64) {
	b.Run(name, func(b *testing.B) {
		if reference != nil {
			b.Run("reference", func(b *testing.B) { benchmark(b, reference) })
		}
		b.Run("algorithm", func(b *testing.B) { benchmark(b, algorithm) })
	})
}

func benchmark(b *testing.B, hash func(string) uint64) {
	const uuid = "DAB45194-42CC-4106-AB9F-2447FA4D35C2"

	for i := 0; i != b.N; i++ {
		hash(uuid)
	}

	b.SetBytes(int64(len(uuid)))
}
