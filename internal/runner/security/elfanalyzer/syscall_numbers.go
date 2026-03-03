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
