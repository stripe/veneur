package fnv1a

import (
	"hash/fnv"
	"testing"

	"github.com/segmentio/fasthash"
	"github.com/segmentio/fasthash/fasthashtest"
)

func TestHash32(t *testing.T) {
	fasthashtest.TestHashString32(t, "fnv1a", fasthash.HashString32(fnv.New32a), HashString32)
	fasthashtest.TestHashUint32(t, "fnv1a", fasthash.HashUint32(fnv.New32a), HashUint32)
}

func BenchmarkHash32(b *testing.B) {
	fasthashtest.BenchmarkHashString32(b, "fnv1a", fasthash.HashString32(fnv.New32a), HashString32)
}
