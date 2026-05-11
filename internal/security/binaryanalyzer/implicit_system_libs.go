package binaryanalyzer

// implicitSystemLibPrefixes lists SOName prefixes for libraries that are
// widely linked by executables without being part of the executable's
// intended behavior.
//
// Selection criteria:
//  1. The library is widely linked implicitly by common executables.
//  2. The library does not represent a general application dependency such
//     as libssl or libcurl.
//  3. The library has been observed to cause false positives when analyzed
//     recursively from the linking executable.
//
// Libraries not in this list remain under recursive analysis so that
// application libraries and language runtimes keep their full coverage.
var implicitSystemLibPrefixes = []string{
	"libselinux", // SELinux userspace library; its internal imports are not a
	// reliable indicator of the linking executable's behavior.
}

// IsImplicitSystemLibrary reports whether soname is a known implicit system
// library that should be excluded from library-level recursive analysis.
func IsImplicitSystemLibrary(soname string) bool {
	for _, prefix := range implicitSystemLibPrefixes {
		if matchesKnownPrefix(soname, prefix) {
			return true
		}
	}
	return false
}
