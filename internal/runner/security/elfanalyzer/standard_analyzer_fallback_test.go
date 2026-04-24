//go:build test

package elfanalyzer

import (
	"debug/elf"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
)

func TestHasOnlyLibcImportedLibraries(t *testing.T) {
	tests := []struct {
		name     string
		libs     []string
		expected bool
	}{
		{
			name:     "glibc only",
			libs:     []string{"libc.so.6"},
			expected: true,
		},
		{
			name:     "musl only",
			libs:     []string{"libc.musl-x86_64.so.1"},
			expected: true,
		},
		{
			name:     "glibc plus libcurl",
			libs:     []string{"libc.so.6", "libcurl.so.4"},
			expected: false,
		},
		{
			name:     "non libc only",
			libs:     []string{"libcurl.so.4"},
			expected: false,
		},
		{
			name:     "empty",
			libs:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasOnlyLibcImportedLibraries(tt.libs))
		})
	}
}

func TestBuildDetectedSymbols_FallbackSkipsMixedLibraryImports(t *testing.T) {
	networkSymbols := binaryanalyzer.GetNetworkSymbols()
	detected, dynamicLoadSyms := buildDetectedSymbols(
		[]elf.Symbol{
			{
				Name:    "curl_easy_perform",
				Section: elf.SHN_UNDEF,
				Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
			},
			{
				Name:    "socket",
				Section: elf.SHN_UNDEF,
				Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
			},
		},
		false,
		false,
		networkSymbols,
	)

	assert.Empty(t, detected)
	assert.Empty(t, dynamicLoadSyms)
}

func TestBuildDetectedSymbols_FallbackKeepsPureLibcImports(t *testing.T) {
	networkSymbols := binaryanalyzer.GetNetworkSymbols()
	detected, _ := buildDetectedSymbols(
		[]elf.Symbol{
			{
				Name:    "socket",
				Section: elf.SHN_UNDEF,
				Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
			},
			{
				Name:    "read",
				Section: elf.SHN_UNDEF,
				Info:    uint8(elf.STT_FUNC) | uint8(elf.STB_GLOBAL<<4),
			},
		},
		false,
		true,
		networkSymbols,
	)

	assert.Equal(t, []binaryanalyzer.DetectedSymbol{
		{Name: "socket", Category: "socket"},
		{Name: "read", Category: "syscall_wrapper"},
	}, detected)
}
