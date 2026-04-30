package binaryanalyzer

import "strings"

// knownNetworkLibPrefixes lists SOName prefixes for known network-related libraries.
// Match against SOName using safe prefix matching.
// Examples: "libruby.so.3.2" matches "libruby",
//
//	"libpython3.11.so.1.0" matches "libpython",
//	"libpythonista.so" does not match.
var knownNetworkLibPrefixes = map[string]struct{}{
	// =====================================================
	// Network protocol libraries
	// =====================================================

	// Network communication such as HTTP/FTP/SMTP
	"libcurl": {},

	// TLS connections (network-oriented)
	// Note: Exclude libcrypto because it is also used for disk encryption and similar purposes
	"libssl": {},

	// SSH connections
	"libssh":  {},
	"libssh2": {},

	// Network messaging
	"libzmq":     {},
	"libnanomsg": {},
	"libnng":     {},

	// HTTP/2 protocol implementation
	"libnghttp2": {},

	// WebSocket
	"libwebsockets": {},

	// MQTT (IoT messaging)
	"libmosquitto": {},

	// Mozilla NSS (Firefox-family TLS implementation)
	"libnss3": {},

	// libuv: asynchronous I/O (Node.js core, including network I/O)
	"libuv": {},

	// =====================================================
	// Language runtime libraries
	// =====================================================

	// Ruby runtime (can perform network communication via scripts)
	"libruby": {},

	// Python runtime (includes socket, urllib, http, and similar modules in the standard library)
	"libpython": {},

	// Perl runtime (can perform network communication via LWP, IO::Socket, and similar modules)
	"libperl": {},

	// PHP runtime (includes curl, fsockopen, and similar features)
	"libphp": {},

	// Lua runtime (can perform network communication via extensions such as LuaSocket)
	"liblua": {},

	// Java VM (includes java.net in the standard library)
	"libjvm": {},

	// Mono .NET runtime (includes System.Net in the standard library)
	"libmono":      {},
	"libmonoboehm": {},

	// Embedded Node.js runtime
	"libnode": {},
}

// matchesKnownPrefix reports whether the SOName matches a registered prefix.
// "libpython" matches "libpython3.11.so.1.0",
// but does not match "libpythonista.so".
func matchesKnownPrefix(soname, prefix string) bool {
	if !strings.HasPrefix(soname, prefix) {
		return false
	}

	rest := soname[len(prefix):]
	if len(rest) == 0 {
		return true
	}

	return rest[0] == '.' || rest[0] == '-' || (rest[0] >= '0' && rest[0] <= '9')
}

// IsKnownNetworkLibrary reports whether the SOName matches the known network library list.
// soname: DT_NEEDED value (for example, "libruby.so.3.2", "libcurl.so.4", "libpython3.11.so.1.0")
func IsKnownNetworkLibrary(soname string) bool {
	for prefix := range knownNetworkLibPrefixes {
		if matchesKnownPrefix(soname, prefix) {
			return true
		}
	}
	return false
}
