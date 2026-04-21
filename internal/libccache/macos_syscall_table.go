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
var macOSSyscallEntries = map[int]macOSSyscallEntry{
	3:   {name: "read", isNetwork: false},
	4:   {name: "write", isNetwork: false},
	5:   {name: "open", isNetwork: false},
	6:   {name: "close", isNetwork: false},
	27:  {name: "recvmsg", isNetwork: true},
	28:  {name: "sendmsg", isNetwork: true},
	29:  {name: "recvfrom", isNetwork: true},
	30:  {name: "accept", isNetwork: true},
	31:  {name: "getpeername", isNetwork: true},
	32:  {name: "getsockname", isNetwork: true},
	74:  {name: "mprotect", isNetwork: false},
	97:  {name: "socket", isNetwork: true},
	98:  {name: "connect", isNetwork: true},
	104: {name: "bind", isNetwork: true},
	105: {name: "setsockopt", isNetwork: true},
	106: {name: "listen", isNetwork: true},
	118: {name: "getsockopt", isNetwork: true},
	133: {name: "sendto", isNetwork: true},
	134: {name: "shutdown", isNetwork: true},
	135: {name: "socketpair", isNetwork: true},
}

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
