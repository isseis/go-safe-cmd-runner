//go:build test

package binaryanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSyscallWrapperLibrary_match(t *testing.T) {
	tests := []string{
		"libc.so.6",
		"libpthread.so.0",
		"ld-linux-x86-64.so.2",
		"linux-vdso.so.1",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.True(t, IsSyscallWrapperLibrary(soname))
		})
	}
}

func TestIsSyscallWrapperLibrary_noMatch(t *testing.T) {
	tests := []string{
		"libssl.so.3",
		"libcurl.so.4",
		"libstdc++.so.6",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.False(t, IsSyscallWrapperLibrary(soname))
		})
	}
}

func TestIsSyscallWrapperLibrary_prefixBoundary(t *testing.T) {
	tests := []string{
		"libcc.so.1",
		"libcpp.so.1",
	}

	for _, soname := range tests {
		t.Run(soname, func(t *testing.T) {
			assert.False(t, IsSyscallWrapperLibrary(soname))
		})
	}
}
