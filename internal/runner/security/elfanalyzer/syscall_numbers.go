package elfanalyzer

// SyscallNumberTable defines the interface for syscall number lookup.
type SyscallNumberTable interface {
	// GetSyscallName returns the syscall name for the given number.
	// Returns empty string if the number is unknown.
	GetSyscallName(number int) string

	// IsNetworkSyscall returns true if the syscall is network-related.
	IsNetworkSyscall(number int) bool

	// GetNetworkSyscalls returns all network-related syscall numbers.
	GetNetworkSyscalls() []int
}

// SyscallDefinition defines a single syscall.
type SyscallDefinition struct {
	Number      int
	Name        string
	IsNetwork   bool
	Description string
}

// X86_64SyscallTable implements SyscallNumberTable for x86_64 Linux.
type X86_64SyscallTable struct {
	syscalls       map[int]SyscallDefinition
	networkNumbers []int
}

// NewX86_64SyscallTable creates a new syscall table for x86_64 Linux.
func NewX86_64SyscallTable() *X86_64SyscallTable {
	table := &X86_64SyscallTable{
		syscalls: make(map[int]SyscallDefinition),
	}

	// Network-related syscalls (as specified in requirements)
	networkSyscalls := []SyscallDefinition{
		{41, "socket", true, "Create a socket"},
		{42, "connect", true, "Connect to a remote address"},
		{43, "accept", true, "Accept a connection"},
		{44, "sendto", true, "Send data to address"},
		{45, "recvfrom", true, "Receive data from address"},
		{46, "sendmsg", true, "Send message"},
		{47, "recvmsg", true, "Receive message"},
		{49, "bind", true, "Bind to an address"},
		{50, "listen", true, "Listen for connections"},
		{53, "socketpair", true, "Create a pair of connected sockets"},
		{288, "accept4", true, "Accept a connection with flags"},
		{299, "recvmmsg", true, "Receive multiple messages"},
		{307, "sendmmsg", true, "Send multiple messages"},
	}

	for _, def := range networkSyscalls {
		table.syscalls[def.Number] = def
		table.networkNumbers = append(table.networkNumbers, def.Number)
	}

	// Common non-network syscalls (for reference/logging)
	nonNetworkSyscalls := []SyscallDefinition{
		{0, "read", false, "Read from file descriptor"},
		{1, "write", false, "Write to file descriptor"},
		{2, "open", false, "Open file"},
		{3, "close", false, "Close file descriptor"},
		{9, "mmap", false, "Map memory"},
		{11, "munmap", false, "Unmap memory"},
		{12, "brk", false, "Change data segment size"},
		{60, "exit", false, "Terminate process"},
		{231, "exit_group", false, "Terminate all threads"},
	}

	for _, def := range nonNetworkSyscalls {
		table.syscalls[def.Number] = def
	}

	return table
}

// GetSyscallName returns the syscall name for the given number.
func (t *X86_64SyscallTable) GetSyscallName(number int) string {
	if def, ok := t.syscalls[number]; ok {
		return def.Name
	}
	return ""
}

// IsNetworkSyscall returns true if the syscall is network-related.
func (t *X86_64SyscallTable) IsNetworkSyscall(number int) bool {
	if def, ok := t.syscalls[number]; ok {
		return def.IsNetwork
	}
	return false
}

// GetNetworkSyscalls returns all network-related syscall numbers.
// Returns a copy to prevent callers from mutating the internal state.
func (t *X86_64SyscallTable) GetNetworkSyscalls() []int {
	result := make([]int, len(t.networkNumbers))
	copy(result, t.networkNumbers)
	return result
}
