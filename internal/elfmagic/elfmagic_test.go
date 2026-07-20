//go:build test

package elfmagic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIs(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want bool
	}{
		{
			name: "valid ELF magic",
			b:    []byte("\x7fELF\x02\x01\x01\x00"),
			want: true,
		},
		{
			name: "Mach-O magic",
			b:    []byte("\xcf\xfa\xed\xfe"),
			want: false,
		},
		{
			name: "too short",
			b:    []byte("\x7f"),
			want: false,
		},
		{
			name: "empty",
			b:    []byte{},
			want: false,
		},
		{
			name: "PE magic (MZ)",
			b:    []byte("MZ\x90\x00"),
			want: false,
		},
		{
			name: "partial ELF prefix",
			b:    []byte("\x7f\x00\x00\x00"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Is(tt.b))
		})
	}
}
