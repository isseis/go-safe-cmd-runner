package dynlibanalysis

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// DynLibVerifier performs hash verification of recorded library dependencies.
type DynLibVerifier struct {
	fs safefileio.FileSystem
}

// NewDynLibVerifier creates a new verifier.
func NewDynLibVerifier(fs safefileio.FileSystem) *DynLibVerifier {
	return &DynLibVerifier{fs: fs}
}

// Verify checks that each recorded library file has not been tampered with
// by comparing its current hash against the recorded hash.
//
// Returns nil if all hashes match.
// Returns a descriptive error if any check fails.
//
// Note: ld.so.cache tampering is outside the threat model of this system.
// An attacker capable of modifying /etc/ld.so.cache already has root privileges
// and can compromise the system through more direct means. See docs/security/README.md.
func (v *DynLibVerifier) Verify(deps []fileanalysis.LibEntry) error {
	if len(deps) == 0 {
		return nil
	}

	for _, entry := range deps {
		if entry.Path == "" {
			return &ErrEmptyLibraryPath{
				SOName: entry.SOName,
			}
		}

		actualHash, err := computeFileHash(v.fs, entry.Path)
		if err != nil {
			return fmt.Errorf("failed to read library %s at %s: %w",
				entry.SOName, entry.Path, err)
		}

		if actualHash != entry.Hash {
			return &ErrLibraryHashMismatch{
				SOName:       entry.SOName,
				Path:         entry.Path,
				ExpectedHash: entry.Hash,
				ActualHash:   actualHash,
			}
		}
	}

	return nil
}
