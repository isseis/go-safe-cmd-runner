// Package elfanalyzer provides ELF binary analysis for detecting network operation capability.
//
// This package analyzes the dynamic symbol table (.dynsym) of ELF binaries to identify
// imported network-related functions from shared libraries. It is designed to work with
// dynamically linked binaries on Linux systems.
//
// # Usage
//
//	analyzer := elfanalyzer.NewStandardELFAnalyzer(nil, nil)
//	output := analyzer.AnalyzeNetworkSymbols("/usr/bin/curl", "")
//
//	if output.IsNetworkCapable() {
//	    fmt.Printf("Network symbols detected: %v\n", output.DetectedSymbols)
//	}
//
// # Limitations
//
// - Static binaries (e.g., Go binaries) return StaticBinary result
// - Requires read access to the binary (execute-only binaries need privilege escalation)
// - Only analyzes .dynsym section (does not detect syscalls or runtime network operations)
//
// # Security Considerations
//
// This analyzer uses safefileio to prevent symlink attacks and TOCTOU race conditions.
// When analyzing execute-only binaries, provide a PrivilegeManager to enable
// temporary privilege escalation during file access.
package elfanalyzer
