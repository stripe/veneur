package jody

import (
	"fmt"
	"testing"

	"github.com/segmentio/fasthash/fasthashtest"
)

func TestHash64(t *testing.T) {
	// I couldn't find a reference implementation in Go, so this is a hacky
	// test that checks for values generated from the jodyhash command line
	// utility.

	referenceString64 := func(s string) uint64 {
		switch s {
		case "":
			return 0x0000000000000000
		case "A":
			return 0x000000002e8208ba
		case "Hello World!":
			return 0x60be57b5a53eb1c7
		case "DAB45194-42CC-4106-AB9F-2447FA4D35C2":
			return 0x587861d2b41e1997
		default:
			panic("test not implemented: " + s)
		}
	}

	referenceUint64 := func(u uint64) uint64 {
		if u != 42 {
			panic(fmt.Sprint("test not implemented:", u))
		}
		return 0x0007cf56f7fc0ba3
	}

	fasthashtest.TestHashString64(t, "jody", referenceString64, HashString64)
	fasthashtest.TestHashUint64(t, "jody", referenceUint64, HashUint64)
}

func BenchmarkHash64(b *testing.B) {
	fasthashtest.BenchmarkHashString64(b, "jody", nil, HashString64)
}
