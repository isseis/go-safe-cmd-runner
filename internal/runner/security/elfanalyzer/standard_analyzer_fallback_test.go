//go:build test

package elfanalyzer

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
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

func TestCategorizeELFSymbol(t *testing.T) {
	networkSymbols := binaryanalyzer.GetNetworkSymbols()

	assert.Equal(t, "socket", categorizeELFSymbol("socket", networkSymbols))
	assert.Equal(t, "syscall_wrapper", categorizeELFSymbol("read", networkSymbols))
	assert.Equal(t, "syscall_wrapper", categorizeELFSymbol("unknown_symbol", networkSymbols))
}
