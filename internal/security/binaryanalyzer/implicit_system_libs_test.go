//go:build test

package binaryanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsImplicitSystemLibrary_match(t *testing.T) {
	tests := []string{
		"libselinux",
		"libselinux.so.1",
		"libselinux.so.2",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.True(t, IsImplicitSystemLibrary(soname))
		})
	}
}

func TestIsImplicitSystemLibrary_noMatch(t *testing.T) {
	tests := []string{
		"libssl.so.3",
		"libcurl.so.4",
		"libc.so.6",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.False(t, IsImplicitSystemLibrary(soname))
		})
	}
}

func TestIsImplicitSystemLibrary_prefixBoundary(t *testing.T) {
	tests := []string{
		"libselinuxabc.so.1",
		"libselinuxutil.so.1",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.False(t, IsImplicitSystemLibrary(soname))
		})
	}
}
