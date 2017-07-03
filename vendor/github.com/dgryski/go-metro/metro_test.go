package metro

import (
	"encoding/binary"
	"testing"
)

var key63 = []byte("012345678901234567890123456789012345678901234567890123456789012")

func Test64(t *testing.T) {

	tests := []struct {
		seed uint64
		want []byte
	}{

		{0, []byte{0x6B, 0x75, 0x3D, 0xAE, 0x06, 0x70, 0x4B, 0xAD}},
		{1, []byte{0x3B, 0x0D, 0x48, 0x1C, 0xF4, 0xB9, 0xB8, 0xDF}},
	}

	for _, tt := range tests {
		want := binary.LittleEndian.Uint64(tt.want)
		if got := Hash64(key63, tt.seed); got != want {
			t.Errorf("Hash64(%q, %d)=%x, want %x\n", key63, tt.seed, got, want)
		}
	}
}

func Test128(t *testing.T) {

	tests := []struct {
		seed uint64
		want []byte
	}{
		{0, []byte{0xC7, 0x7C, 0xE2, 0xBF, 0xA4, 0xED, 0x9F, 0x9B, 0x05, 0x48, 0xB2, 0xAC, 0x50, 0x74, 0xA2, 0x97}},
		{1, []byte{0x45, 0xA3, 0xCD, 0xB8, 0x38, 0x19, 0x9D, 0x7F, 0xBD, 0xD6, 0x8D, 0x86, 0x7A, 0x14, 0xEC, 0xEF}},
	}

	for _, tt := range tests {
		wanta := binary.LittleEndian.Uint64(tt.want)
		wantb := binary.LittleEndian.Uint64(tt.want[8:])
		if gota, gotb := Hash128(key63, tt.seed); gota != wanta || gotb != wantb {
			t.Errorf("Hash128d(%q, %d)=(%x, %x), want (%x, %x)\n", key63, tt.seed, gota, gotb, wanta, wantb)
		}
	}
}
