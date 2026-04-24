package libccache

// macOSSyscallEntry is the internal representation of a BSD syscall entry.
type macOSSyscallEntry struct {
	name      string
	isNetwork bool
}

// MacOSSyscallTable implements SyscallNumberTable for macOS arm64 BSD syscalls.
type MacOSSyscallTable struct{}

// Verify interface compliance at compile time.
var _ SyscallNumberTable = MacOSSyscallTable{}

// macOSSyscallEntries defines the macOS arm64 BSD syscall table.
// Keys are syscall numbers without the BSD class prefix 0x2000000.
// sendmmsg / recvmmsg are Linux-specific and are not present on macOS.
// The actual map is defined in macos_syscall_numbers.go (auto-generated).

// networkSyscallWrapperNames lists network-related syscall wrapper names used when
// matching import symbols as a fallback for binaries where the libSystem cache is unavailable.
// sendmmsg / recvmmsg are Linux-specific and are therefore excluded on macOS.
var networkSyscallWrapperNames = []string{
	"socket", "connect", "bind", "listen", "accept",
	"sendto", "recvfrom", "sendmsg", "recvmsg",
	"socketpair", "shutdown", "setsockopt", "getsockopt",
	"getpeername", "getsockname",
}

// GetSyscallName implements SyscallNumberTable.
func (t MacOSSyscallTable) GetSyscallName(number int) string {
	if e, ok := macOSSyscallEntries[number]; ok {
		return e.name
	}
	return ""
}

// IsNetworkSyscall implements SyscallNumberTable.
func (t MacOSSyscallTable) IsNetworkSyscall(number int) bool {
	if e, ok := macOSSyscallEntries[number]; ok {
		return e.isNetwork
	}
	return false
}
