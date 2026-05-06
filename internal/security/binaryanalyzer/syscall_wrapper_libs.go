package binaryanalyzer

import "strings"

// matchesKnownPrefix reports whether soname starts with prefix followed by
// a version separator (".", "-", or digit), or equals prefix exactly.
// This prevents "libpythonista.so" from matching the "libpython" prefix.
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

// syscallWrapperPrefixes lists SOName prefixes for OS-ABI syscall wrapper
// libraries that should be excluded from library-level network analysis.
var syscallWrapperPrefixes = []string{
	"libc",
	"libpthread",
	"libdl",
	"librt",
	"libgcc_s",
	"ld-linux",
	"ld-linux-x86-64",
	"ld-linux-aarch64",
	"linux-vdso",
}

// IsSyscallWrapperLibrary reports whether soname is a known syscall wrapper
// library that should be excluded from library-level analysis.
func IsSyscallWrapperLibrary(soname string) bool {
	for _, prefix := range syscallWrapperPrefixes {
		if matchesKnownPrefix(soname, prefix) {
			return true
		}
	}
	return false
}
