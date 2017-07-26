package hyperloglog

import (
	"math"
	"testing"
)

func TestCLZ32(t *testing.T) {
	n := clz32(0xffffffff)
	if n != 0 {
		t.Error(n)
	}

	n = clz32(0x08000000)
	if n != 4 {
		t.Error(n)
	}

	n = clz32(0x00000000)
	if n != 32 {
		t.Error(n)
	}

	n = clz32(0x00000001)
	if n != 31 {
		t.Error(n)
	}

	n = clz32(0x01bf82af)
	if n != 7 {
		t.Error(n)
	}

	n = clz32(0x80000000)
	if n != 0 {
		t.Error(n)
	}

	n = clz32(0x00100000)
	if n != 11 {
		t.Error(n)
	}

	n = clz32(0x00000100)
	if n != 23 {
		t.Error(n)
	}

	n = clz32(0x00000010)
	if n != 27 {
		t.Error(n)
	}
}

func TestCLZ64(t *testing.T) {
	n := clz64(0xffffffffffffffff)
	if n != 0 {
		t.Error(n)
	}

	n = clz64(0x0800000000000000)
	if n != 4 {
		t.Error(n)
	}

	n = clz64(0x0000000000000000)
	if n != 64 {
		t.Error(n)
	}

	n = clz64(0x0000000000000001)
	if n != 63 {
		t.Error(n)
	}

	n = clz64(0x01bf82af00000000)
	if n != 7 {
		t.Error(n)
	}

	n = clz64(0x8000000000000000)
	if n != 0 {
		t.Error(n)
	}
}

func TestEB32(t *testing.T) {
	n := eb32(0xffffffff, 3, 1)
	if n != 3 {
		t.Error(n)
	}

	n = eb32(0xffffffff, 32, 0)
	if n != 0xffffffff {
		t.Error(n)
	}

	n = eb32(0xffffffff, 35, 0)
	if n != 0xffffffff {
		t.Error(n)
	}

	n = eb32(0xffffffff, 32, 10)
	if n != 0x3fffff {
		t.Error(n)
	}

	n = eb32(0xf001, 32, 16)
	if n != 0 {
		t.Error(n)
	}

	n = eb32(0xf001, 16, 0)
	if n != 0xf001 {
		t.Error(n)
	}

	n = eb32(0xf001, 12, 0)
	if n != 1 {
		t.Error(n)
	}

	n = eb32(0xf001, 16, 1)
	if n != 0x7800 {
		t.Error(n)
	}

	n = eb32(0x1211, 13, 2)
	if n != 0x484 {
		t.Error(n)
	}

	n = eb32(0x10000000, 32, 1)
	if n != 0x8000000 {
		t.Error(n)
	}
}

func TestEB64(t *testing.T) {
	n := eb64(0xffffffffffffffff, 3, 1)
	if n != 3 {
		t.Error(n)
	}

	n = eb64(0xffffffffffffffff, 64, 0)
	if n != 0xffffffffffffffff {
		t.Error(n)
	}

	n = eb64(0xffffffffffffffff, 68, 0)
	if n != 0xffffffffffffffff {
		t.Error(n)
	}

	n = eb64(0xffffffffffffffff, 64, 10)
	if n != 0x3fffffffffffff {
		t.Error(n)
	}

	n = eb64(0xf001, 64, 16)
	if n != 0 {
		t.Error(n)
	}

	n = eb64(0xf001, 16, 0)
	if n != 0xf001 {
		t.Error(n)
	}

	n = eb64(0xf001, 12, 0)
	if n != 1 {
		t.Error(n)
	}

	n = eb64(0xf001, 16, 1)
	if n != 0x7800 {
		t.Error(n)
	}

	n = eb64(0x1211, 13, 2)
	if n != 0x484 {
		t.Error(n)
	}

	n = eb64(0x100000000000, 64, 1)
	if n != 0x80000000000 {
		t.Error(n)
	}
}

func TestCountZeros(t *testing.T) {
	n := countZeros([]uint8{10, 9, 8, 7})
	if n != 0 {
		t.Error(n)
	}

	n = countZeros([]uint8{})
	if n != 0 {
		t.Error(n)
	}

	n = countZeros([]uint8{10, 9, 0, 8, 7})
	if n != 1 {
		t.Error(n)
	}

	n = countZeros([]uint8{0, 10, 9, 1, 8, 7})
	if n != 1 {
		t.Error(n)
	}

	n = countZeros([]uint8{10, 9, 1, 8, 7, 0})
	if n != 1 {
		t.Error(n)
	}

	n = countZeros([]uint8{10, 0, 9, 1, 8, 7, 0})
	if n != 2 {
		t.Error(n)
	}

	n = countZeros([]uint8{0, 0, 9, 1, 8, 7, 0})
	if n != 3 {
		t.Error(n)
	}

	n = countZeros([]uint8{0, 0, 0, 0, 0, 0})
	if n != 6 {
		t.Error(n)
	}
}

func TestAlpha(t *testing.T) {
	v := alpha(16)
	if v != 0.673 {
		t.Error(v)
	}

	v = alpha(32)
	if v != 0.697 {
		t.Error(v)
	}

	v = alpha(64)
	if v != 0.709 {
		t.Error(v)
	}

	v = alpha(128)
	if math.Abs(v-0.71527) > 0.00001 {
		t.Error(v)
	}
}

func TestCalculateEstimate(t *testing.T) {
	// Test values between 31 and 64 to make sure bit shifting is using 64 bits.
	v := calculateEstimate([]uint8{33, 0, 63, 12, 62, 5, 53})
	if v < 0.00001 {
		t.Error(v)
	}
}
