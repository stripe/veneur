package dataunit

import "testing"

func Test_SizeBytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want int64
	}{
		{
			name: "test bytes",
			s:    1 * Byte,
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Bytes(); got != int64(tt.want) {
				t.Errorf("Size.Bytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Kilobytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test kilobytes",
			s:    1536 * Byte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Kilobytes(); got != tt.want {
				t.Errorf("Size.Kilobytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Megabytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test megabytes",
			s:    1536 * Kilobyte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Megabytes(); got != tt.want {
				t.Errorf("Size.Megabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Gigabytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test gigabytes",
			s:    1536 * Megabyte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Gigabytes(); got != tt.want {
				t.Errorf("Size.Gigabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Terabytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test terabytes",
			s:    1536 * Gigabyte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Terabytes(); got != tt.want {
				t.Errorf("Size.Terabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Petabytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test petabytes",
			s:    1536 * Terabyte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Petabytes(); got != tt.want {
				t.Errorf("Size.Petabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSize_Exabytes(t *testing.T) {
	tests := []struct {
		name string
		s    Size
		want float64
	}{
		{
			name: "test exabytes",
			s:    1536 * Petabyte,
			want: float64(1.5),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Exabytes(); got != tt.want {
				t.Errorf("Size.Exabytes() = %v, want %v", got, tt.want)
			}
		})
	}
}
