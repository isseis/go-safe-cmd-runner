package elfanalyzer

var (
	cachedX86SyscallTable   = NewX86_64SyscallTable()
	cachedArm64SyscallTable = NewARM64LinuxSyscallTable()
)

// SyscallTableForArchitecture returns the shared cached SyscallNumberTable for
// the given Linux architecture. It returns nil for unrecognized architectures.
func SyscallTableForArchitecture(arch string) SyscallNumberTable {
	switch arch {
	case "x86_64":
		return cachedX86SyscallTable
	case "arm64":
		return cachedArm64SyscallTable
	default:
		return nil
	}
}
