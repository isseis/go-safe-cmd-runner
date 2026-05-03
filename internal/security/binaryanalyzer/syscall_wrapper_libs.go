package binaryanalyzer

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
