package elfanalyzer

// SyscallAnalysisStore defines the interface for syscall analysis result storage.
type SyscallAnalysisStore interface {
	// LoadSyscallAnalysis loads syscall analysis from storage.
	// expectedHash contains both the hash algorithm and the expected hash value.
	// Format: "sha256:<hex>".
	LoadSyscallAnalysis(filePath string, expectedHash string) (*SyscallAnalysisResult, error)
}
