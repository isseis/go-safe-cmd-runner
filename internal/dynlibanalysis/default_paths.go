package dynlibanalysis

import "debug/elf"

// DefaultSearchPaths returns the architecture-specific default library search paths.
// These are used as the last resort when RPATH/RUNPATH and ld.so.cache fail to resolve.
// The order is: multiarch paths (Debian/Ubuntu) -> /lib64, /usr/lib64 (RHEL) -> generic.
func DefaultSearchPaths(machine elf.Machine) []string {
	switch machine {
	case elf.EM_X86_64:
		return []string{
			"/lib/x86_64-linux-gnu",
			"/usr/lib/x86_64-linux-gnu",
			"/lib64",
			"/usr/lib64",
			"/lib",
			"/usr/lib",
		}
	case elf.EM_AARCH64:
		return []string{
			"/lib/aarch64-linux-gnu",
			"/usr/lib/aarch64-linux-gnu",
			"/lib64",
			"/usr/lib64",
			"/lib",
			"/usr/lib",
		}
	default:
		return []string{
			"/lib64",
			"/usr/lib64",
			"/lib",
			"/usr/lib",
		}
	}
}
