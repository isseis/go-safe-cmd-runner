package binaryanalyzer

// implicitSystemLibPrefixes lists SOName prefixes for libraries that are
// widely linked by executables without being part of the executable's
// intended behavior.
//
// Selection criteria (all must hold):
//  1. Widely linked implicitly by common executables (e.g., GNU coreutils)
//     even when not used directly.
//  2. The library itself does not use BSD socket / DNS resolution APIs,
//     or uses only non-network kernel interfaces (e.g., /sys, netlink for
//     kernel facility communication).
//  3. False positives have been observed where symbols/syscalls imported
//     by these libraries do not match the linking executable's actual
//     behavior.
var implicitSystemLibPrefixes = []string{
	"libselinux", // SELinux userspace library; transitively linked by many
	// binaries but its imported dlsym/execve/open symbols do
	// not reflect the linking executable's behavior.
}

// IsImplicitSystemLibrary reports whether soname is a known implicit system
// library that should be excluded from library-level recursive analysis.
//
// Implicit system libraries are libraries that are widely linked but rarely
// used directly (e.g., libselinux). They are excluded for false-positive
// reduction. See implicitSystemLibPrefixes for the list and selection
// criteria.
func IsImplicitSystemLibrary(soname string) bool {
	for _, prefix := range implicitSystemLibPrefixes {
		if matchesKnownPrefix(soname, prefix) {
			return true
		}
	}
	return false
}
