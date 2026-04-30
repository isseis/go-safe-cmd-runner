//go:build test

package binaryanalyzer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsKnownNetworkLibrary_NetworkLibs verifies that well-known network protocol libraries
// are recognized as known network libraries.
func TestIsKnownNetworkLibrary_NetworkLibs(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libcurl", "libcurl.so.4"},
		{"libssl", "libssl.so.3"},
		{"libssh", "libssh.so.4"},
		{"libssh2", "libssh2.so.1"},
		{"libzmq", "libzmq.so.5"},
		{"libnanomsg", "libnanomsg.so.5"},
		{"libnng", "libnng.so.1"},
		{"libnghttp2", "libnghttp2.so.14"},
		{"libwebsockets", "libwebsockets.so.19"},
		{"libmosquitto", "libmosquitto.so.1"},
		{"libnss3", "libnss3.so"},
		{"libuv", "libuv.so.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsKnownNetworkLibrary(tt.soname),
				"expected %s to be recognized as a known network library", tt.soname)
		})
	}
}

// TestIsKnownNetworkLibrary_LanguageRuntimes verifies that language runtime libraries
// are recognized as known network libraries (they have built-in network capabilities).
func TestIsKnownNetworkLibrary_LanguageRuntimes(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libruby", "libruby.so.3.2"},
		{"libpython", "libpython3.so"},
		{"libperl", "libperl.so.5.36"},
		{"libphp", "libphp.so"},
		{"liblua", "liblua.so.5.4"},
		{"libjvm", "libjvm.so"},
		{"libmono", "libmono.so.2"},
		{"libmonoboehm", "libmonoboehm.so.2"},
		{"libnode", "libnode.so.108"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsKnownNetworkLibrary(tt.soname),
				"expected %s to be recognized as a known network library", tt.soname)
		})
	}
}

// TestIsKnownNetworkLibrary_Excluded verifies that common system libraries not related
// to network operations are NOT recognized as known network libraries.
func TestIsKnownNetworkLibrary_Excluded(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libstdc++", "libstdc++.so.6"},
		{"libz", "libz.so.1"},
		{"libcrypto", "libcrypto.so.3"},
		{"libgnutls", "libgnutls.so.30"},
		{"libgcrypt", "libgcrypt.so.20"},
		{"libpthread", "libpthread.so.0"},
		{"libc", "libc.so.6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsKnownNetworkLibrary(tt.soname),
				"expected %s NOT to be recognized as a known network library", tt.soname)
		})
	}
}

// TestIsKnownNetworkLibrary_VersionedSONames verifies that versioned SONames
// (with version numbers in the filename) are correctly matched.
func TestIsKnownNetworkLibrary_VersionedSONames(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libruby.so.3.2", "libruby.so.3.2"},
		{"libpython3.11.so.1.0", "libpython3.11.so.1.0"},
		{"libcurl.so.4", "libcurl.so.4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsKnownNetworkLibrary(tt.soname),
				"expected versioned SOName %s to be recognized", tt.soname)
		})
	}
}

// TestIsKnownNetworkLibrary_PythonVersioned verifies that Python's unusual SOName format
// (libpython3.11.so.1.0, where version goes before .so) is matched via prefix matching.
func TestIsKnownNetworkLibrary_PythonVersioned(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libpython3.11.so.1.0", "libpython3.11.so.1.0"},
		{"libpython3.12.so.1.0", "libpython3.12.so.1.0"},
		{"libpython3.so", "libpython3.so"},
		{"libpython.so.1.0", "libpython.so.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsKnownNetworkLibrary(tt.soname),
				"expected Python SOName %s to be recognized via prefix match", tt.soname)
		})
	}
}

// TestIsKnownNetworkLibrary_Confusing verifies that library names that start with a registered
// prefix but are actually different libraries are NOT matched.
func TestIsKnownNetworkLibrary_Confusing(t *testing.T) {
	tests := []struct {
		name   string
		soname string
	}{
		{"libpythonista.so", "libpythonista.so"},
		{"libcurlpp.so", "libcurlpp.so"},
		{"libsslutil.so", "libsslutil.so"},
		{"libnodecompat.so", "libnodecompat.so"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, IsKnownNetworkLibrary(tt.soname),
				"expected confusing name %s NOT to be recognized", tt.soname)
		})
	}
}

// TestMatchesKnownPrefix verifies the safe prefix matching logic.
func TestMatchesKnownPrefix(t *testing.T) {
	tests := []struct {
		name     string
		soname   string
		prefix   string
		expected bool
	}{
		{"exact prefix with dot separator", "libruby.so.3.2", "libruby", true},
		{"exact prefix with dot separator (curl)", "libcurl.so.4", "libcurl", true},
		{"version number after prefix (python)", "libpython3.11.so.1.0", "libpython", true},
		{"confusing name (pythonista)", "libpythonista.so", "libpython", false},
		{"dash separator", "libssl-dev.so", "libssl", true},
		{"exact match (just prefix)", "libssl", "libssl", true},
		{"prefix with .so suffix", "libssl.so.3", "libssl", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, matchesKnownPrefix(tt.soname, tt.prefix),
				"matchesKnownPrefix(%q, %q)", tt.soname, tt.prefix)
		})
	}
}
