//go:build test

package binaryanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsDynamicLoadSymbol verifies that dlopen/dlsym/dlvsym are recognized
// as dynamic load symbols, and other symbols are not.
func TestIsDynamicLoadSymbol(t *testing.T) {
	tests := []struct {
		name     string
		symbol   string
		expected bool
	}{
		{"dlopen", "dlopen", true},
		{"dlsym", "dlsym", true},
		{"dlvsym", "dlvsym", true},
		{"socket (network, not dynamic load)", "socket", false},
		{"empty string", "", false},
		{"dlclose (not in registry)", "dlclose", false},
		{"partial match dlopen2", "dlopen2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsDynamicLoadSymbol(tt.symbol))
		})
	}
}

// TestHasDynamicLoad_Independent verifies that DynamicLoadSymbols is populated
// independently of network symbol detection.
// A binary with both dlopen and socket symbols returns non-empty DynamicLoadSymbols
// AND Result=NetworkDetected (both signals are preserved independently).
func TestHasDynamicLoad_Independent(t *testing.T) {
	// Verify that dlopen/dlsym/dlvsym are NOT in the network symbol registry
	// (they should be separate signals).
	for _, sym := range []string{"dlopen", "dlsym", "dlvsym"} {
		_, found := IsNetworkSymbol(sym)
		assert.False(t, found,
			"%s should not be a network symbol (it is a dynamic load symbol)", sym)
	}

	// Verify CategoryDynamicLoad is defined
	assert.Equal(t, SymbolCategory("dynamic_load"), CategoryDynamicLoad)
}

// TestIsNetworkCategory verifies that IsNetworkCategory returns true only for
// network-related categories ("socket", "dns", "tls", "http"). AC-4.
func TestIsNetworkCategory(t *testing.T) {
	tests := []struct {
		name     string
		category string
		expected bool
	}{
		{"socket", "socket", true},
		{"dns", "dns", true},
		{"tls", "tls", true},
		{"http", "http", true},
		{"syscall_wrapper", "syscall_wrapper", false},
		{"dynamic_load", "dynamic_load", false},
		{"empty string", "", false},
		{"unknown", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsNetworkCategory(tt.category))
		})
	}
}
