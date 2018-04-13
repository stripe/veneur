package fnv1

import (
	"hash/fnv"
	"testing"

	"github.com/segmentio/fasthash"
	"github.com/segmentio/fasthash/fasthashtest"
)

func TestHash32(t *testing.T) {
	fasthashtest.TestHashString32(t, "fnv1", fasthash.HashString32(fnv.New32), HashString32)
	fasthashtest.TestHashUint32(t, "fnv1", fasthash.HashUint32(fnv.New32), HashUint32)
}

func BenchmarkHash32(b *testing.B) {
	fasthashtest.BenchmarkHashString32(b, "fnv1", fasthash.HashString32(fnv.New32a), HashString32)
}
