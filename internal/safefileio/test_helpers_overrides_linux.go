//go:build linux && test

package safefileio

// openat2SyscallOverride is the test override for the raw openat2 system call.
// It reaches conditions a test cannot otherwise produce: EINTR, which only the
// Go runtime's preemption signal raises, and the openHow value as the kernel
// receives it.
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see overrides_linux.go.
//
// Like linkatFunc, it is shared package-wide, so tests must not call
// t.Parallel() and must restore it with t.Cleanup. A test must also construct
// its FileSystem before installing an override, because NewFileSystem probes
// availability by issuing a real openat2 and would consume one stubbed call.
var openat2SyscallOverride = rawOpenat2

// openat2Syscall issues one openat2 system call, via the test override above.
func openat2Syscall(dirfd int, pathname string, how *openHow) (int, error) {
	return openat2SyscallOverride(dirfd, pathname, how)
}
