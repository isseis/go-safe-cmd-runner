// Package dynlibanalysis provides dynamic library dependency analysis
// and verification for ELF binaries.
//
// At record time (DynLibAnalyzer.Analyze), it resolves all DT_NEEDED entries
// recursively, computes hashes, and returns a snapshot of the dependency tree.
//
// At runner time (DynLibVerifier.Verify), it performs two-stage verification:
// Stage 1 checks that library hashes match the recorded values (tamper detection),
// Stage 2 re-resolves each (ParentPath, SOName) pair and checks that the resolved
// path matches the recorded path (LD_LIBRARY_PATH hijack detection).
//
// Resolution follows the ld.so(8) search order:
//  1. DT_RPATH of the loading ELF (when DT_RUNPATH is absent)
//  2. Inherited DT_RPATH from ancestor ELFs (terminates when child has DT_RUNPATH)
//  3. LD_LIBRARY_PATH (runner time only)
//  4. DT_RUNPATH of the loading ELF
//  5. /etc/ld.so.cache
//  6. Architecture-specific default paths (/lib, /usr/lib, etc.)
package dynlibanalysis
