package elfanalyzer

// ARM64LinuxSyscallTable implements SyscallNumberTable for arm64 Linux.
type ARM64LinuxSyscallTable struct {
	syscalls       map[int]SyscallDefinition
	networkNumbers []int
}

// NewARM64LinuxSyscallTable creates a new syscall table for arm64 Linux.
// Network syscall numbers are from the ARM64 Linux ABI (ARCH_NR_SYSCALLS
// base: include/uapi/asm-generic/unistd.h).
func NewARM64LinuxSyscallTable() *ARM64LinuxSyscallTable { //nolint:dupl // table initialisation is structurally identical to x86_64
	table := &ARM64LinuxSyscallTable{
		syscalls: make(map[int]SyscallDefinition),
	}

	// Network-related syscalls (requirements FR-3.1.5)
	networkSyscalls := []SyscallDefinition{
		{198, "socket", true, "Create a socket"},
		{199, "socketpair", true, "Create a pair of connected sockets"},
		{200, "bind", true, "Bind to an address"},
		{201, "listen", true, "Listen for connections"},
		{202, "accept", true, "Accept a connection"},
		{203, "connect", true, "Connect to a remote address"},
		{206, "sendto", true, "Send data to address"},
		{207, "recvfrom", true, "Receive data from address"},
		{211, "sendmsg", true, "Send message"},
		{212, "recvmsg", true, "Receive message"},
		{242, "accept4", true, "Accept a connection with flags"},
		{243, "recvmmsg", true, "Receive multiple messages"},
		{269, "sendmmsg", true, "Send multiple messages"},
	}

	for _, def := range networkSyscalls {
		table.syscalls[def.Number] = def
		table.networkNumbers = append(table.networkNumbers, def.Number)
	}

	// Common non-network syscalls (for reference/logging)
	nonNetworkSyscalls := []SyscallDefinition{
		{63, "read", false, "Read from file descriptor"},
		{64, "write", false, "Write to file descriptor"},
		{56, "openat", false, "Open file relative to directory"},
		{57, "close", false, "Close file descriptor"},
		{222, "mmap", false, "Map memory"},
		{215, "munmap", false, "Unmap memory"},
		{214, "brk", false, "Change data segment size"},
		{93, "exit", false, "Terminate process"},
		{94, "exit_group", false, "Terminate all threads"},
	}

	for _, def := range nonNetworkSyscalls {
		table.syscalls[def.Number] = def
	}

	return table
}

// GetSyscallName returns the syscall name for the given number.
func (t *ARM64LinuxSyscallTable) GetSyscallName(number int) string {
	if def, ok := t.syscalls[number]; ok {
		return def.Name
	}
	return ""
}

// IsNetworkSyscall returns true if the syscall is network-related.
func (t *ARM64LinuxSyscallTable) IsNetworkSyscall(number int) bool {
	if def, ok := t.syscalls[number]; ok {
		return def.IsNetwork
	}
	return false
}

// GetNetworkSyscalls returns all network-related syscall numbers.
// Returns a copy to prevent callers from mutating the internal state.
func (t *ARM64LinuxSyscallTable) GetNetworkSyscalls() []int {
	result := make([]int, len(t.networkNumbers))
	copy(result, t.networkNumbers)
	return result
}
