//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLibcLibrary(t *testing.T) {
	tests := []struct {
		name     string
		lib      string
		expected bool
	}{
		{
			name:     "glibc",
			lib:      "libc.so.6",
			expected: true,
		},
		{
			name:     "musl",
			lib:      "libc.musl-x86_64.so.1",
			expected: true,
		},
		{
			name:     "non libc",
			lib:      "libcurl.so.4",
			expected: false,
		},
		{
			name:     "empty",
			lib:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isLibcLibrary(tt.lib))
		})
	}
}
