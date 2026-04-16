package machodylib

import "strings"

// systemLibPrefixes are install name prefixes for libraries typically found
// in the dyld shared cache. When a library with one of these prefixes cannot
// be found on the filesystem, it is assumed to reside in the dyld shared cache
// and is skipped (hash verification delegated to code signing).
var systemLibPrefixes = []string{
	"/usr/lib/",
	"/usr/libexec/",
	"/System/Library/",
	"/Library/Apple/",
}

// IsDyldSharedCacheLib reports whether the given install name matches a
// system library prefix that indicates the library is likely part of the
// dyld shared cache.
//
// This function should only be called AFTER Resolve has failed (file not found
// on the filesystem). The combination of "system prefix + file not found"
// satisfies FR-3.1.5's two-condition test for dyld shared cache membership.
func IsDyldSharedCacheLib(installName string) bool {
	for _, prefix := range systemLibPrefixes {
		if strings.HasPrefix(installName, prefix) {
			return true
		}
	}

	return false
}
