//go:build test

package machodylib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsDyldSharedCacheLib_SystemPrefixes(t *testing.T) {
	tests := []struct {
		installName string
		expected    bool
	}{
		{"/usr/lib/libSystem.B.dylib", true},
		{"/usr/lib/libc++.1.dylib", true},
		{"/usr/libexec/rosetta/libRosettaRuntime", true},
		{"/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation", true},
		{"/System/Library/PrivateFrameworks/Foo.framework/Foo", true},
		{"/Library/Apple/usr/lib/libssl.dylib", true},
		// Non-system paths
		{"/usr/local/lib/libssl.dylib", false},
		{"/opt/homebrew/lib/libssl.dylib", false},
		{"/usr/local/opt/openssl/lib/libssl.dylib", false},
		{"@rpath/libssl.dylib", false},
		{"libssl.dylib", false},
		{"/usr/lib2/libfoo.dylib", false},
	}

	for _, tt := range tests {
		t.Run(tt.installName, func(t *testing.T) {
			result := IsDyldSharedCacheLib(tt.installName)
			assert.Equal(t, tt.expected, result, "IsDyldSharedCacheLib(%q)", tt.installName)
		})
	}
}
