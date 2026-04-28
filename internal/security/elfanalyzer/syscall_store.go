package elfanalyzer

// SyscallAnalysisStore defines the interface for syscall analysis result storage.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// expectedHash contains both the hash algorithm and the expected hash value.
	// Format: "sha256:<hex>" (e.g., "sha256:abc123...def789").
	// Returns (result, nil) if found and hash matches.
	// Returns (nil, fileanalysis.ErrRecordNotFound) if not found.
	// Returns (nil, fileanalysis.ErrHashMismatch) if hash mismatch.
	// Returns (nil, nil) if no result exists in storage (e.g., analysis was
	// not applicable, skipped, or completed without stored results).
	// Returns (nil, error) on other errors.
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, error)
}
