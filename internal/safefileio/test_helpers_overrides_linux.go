//go:build linux && test

package safefileio

// openat2SyscallOverride is the test override for the raw openat2 system call.
// Tests use it to return EINTR (which no test can provoke for real, since the
// interruption comes from the Go runtime's preemption signal) and to observe
// the openHow value that reaches the kernel.
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see overrides_linux.go.
//
// Tests must not call t.Parallel() in this package: this variable is mutated
// by tests and shared package-wide, as are linkatFunc and
// generateTempLinkName. A test that replaces it must restore it with
// t.Cleanup, and must construct its FileSystem before installing the override,
// because NewFileSystem probes availability by issuing a real openat2 and
// would otherwise consume one stubbed call.
var openat2SyscallOverride = rawOpenat2

// openat2Syscall issues one openat2 system call, via the test override above.
func openat2Syscall(dirfd int, pathname string, how *openHow) (int, error) {
	return openat2SyscallOverride(dirfd, pathname, how)
}
