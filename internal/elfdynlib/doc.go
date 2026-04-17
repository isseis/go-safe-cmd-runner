// Package elfdynlib provides dynamic library dependency analysis
// and verification for ELF binaries.
//
// At record time (DynLibAnalyzer.Analyze), it resolves all DT_NEEDED entries
// recursively, computes hashes, and returns a snapshot of the dependency tree.
//
// At runner time (DynLibVerifier.Verify), it verifies that each recorded library
// file has not been tampered with by comparing its current SHA-256 hash against
// the recorded value.
//
// Note: ld.so.cache tampering is outside the threat model of this system.
// See docs/security/README.md for the security scope.
package elfdynlib
