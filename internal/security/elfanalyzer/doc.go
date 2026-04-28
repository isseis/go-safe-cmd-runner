// Package elfanalyzer provides ELF binary analysis for detecting network
// operation capability on Linux.
//
// # Overview
//
// The package implements binaryanalyzer.BinaryAnalyzer for ELF binaries via
// NewStandardELFAnalyzer. Two complementary techniques are combined:
//
//  1. Dynamic symbol detection (.dynsym / PLT): identifies imported
//     network-related library functions in dynamically linked binaries.
//
//  2. Syscall analysis: disassembles the binary's .text section to find
//     direct SYSCALL/SVC instructions and resolves the syscall number from
//     the preceding instruction stream. Results can be persisted and reused
//     across runs through the SyscallAnalysisStore interface.
//
// # Components
//
// Syscall analysis pipeline:
//   - syscall_analyzer.go  — top-level SyscallAnalyzer orchestrator;
//     produces SyscallAnalysisResult
//   - syscall_decoder.go   — MachineCodeDecoder interface and DecodedInstruction type
//   - x86_decoder.go       — x86_64 instruction decoder
//   - arm64_decoder.go     — arm64 instruction decoder
//   - syscall_numbers.go   — SyscallNumberTable interface
//   - x86_syscall_numbers.go / arm64_syscall_numbers.go — per-arch syscall tables
//   - syscall_table_selector.go — selects the correct table for a given arch
//   - syscall_store.go     — SyscallAnalysisStore interface for result caching
//
// Go wrapper resolution (handles indirect syscalls through the Go runtime):
//   - go_wrapper_resolver.go      — architecture-agnostic wrapper resolver
//   - x86_go_wrapper_resolver.go  — x86_64 specialisation
//   - arm64_go_wrapper_resolver.go — arm64 specialisation
//
// PLT / call-argument analysis:
//   - plt_analyzer.go — locates PLT stubs and evaluates argument registers at
//     call sites (used for mprotect prot-flag inference)
//
// pclntab parsing (Go binaries):
//   - pclntab_parser.go — parses .gopclntab to enumerate Go function boundaries,
//     enabling accurate wrapper-call resolution in Go binaries
//
// mprotect risk evaluation:
//   - mprotect_risk.go — infers PROT_EXEC risk from PLT argument analysis
//
// # Usage
//
//	analyzer := elfanalyzer.NewStandardELFAnalyzer(nil, nil)
//	output, err := analyzer.Analyze("/usr/bin/curl", "sha256:abc123...")
//
//	if output.IsNetworkCapable() {
//	    fmt.Printf("Network capability detected: %v\n", output.DetectedSymbols)
//	}
//
// # Limitations
//
//   - Syscall number resolution uses a bounded backward scan; highly
//     non-linear code (e.g. obfuscated binaries) may yield unresolved entries.
//   - pclntab parsing supports Go 1.20+ (magic 0xfffffff1); older Go binaries
//     fall back to best-effort wrapper resolution.
//   - Only Linux ELF64 (x86_64 and arm64) is fully supported.
//
// # Security Considerations
//
// File access uses safefileio to prevent symlink attacks and TOCTOU races.
// When analyzing execute-only binaries, supply a PrivilegeManager to
// NewStandardELFAnalyzer for temporary privilege escalation during reads.
package elfanalyzer
