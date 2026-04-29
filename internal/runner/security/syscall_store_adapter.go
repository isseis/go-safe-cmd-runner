package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	secelfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

// fileanalysisSyscallStoreAdapter wraps fileanalysis.SyscallAnalysisStore and implements
// secelfanalyzer.SyscallAnalysisStore.
// This bridges the two packages without creating an import cycle:
//
//	secelfanalyzer → (no fileanalysis import)
//	fileanalysis   → (no secelfanalyzer import)
//	security       → both secelfanalyzer and fileanalysis
type fileanalysisSyscallStoreAdapter struct {
	inner fileanalysis.SyscallAnalysisStore
}

// NewELFSyscallStoreAdapter wraps a fileanalysis.SyscallAnalysisStore so it satisfies
// the secelfanalyzer.SyscallAnalysisStore interface.
// The returned value can be passed directly to secelfanalyzer.NewStandardELFAnalyzerWithSyscallStore.
func NewELFSyscallStoreAdapter(store fileanalysis.SyscallAnalysisStore) secelfanalyzer.SyscallAnalysisStore {
	return &fileanalysisSyscallStoreAdapter{inner: store}
}

// LoadSyscallAnalysis implements secelfanalyzer.SyscallAnalysisStore.
// Converts the result type via the shared common.SyscallAnalysisResultCore embedding.
// fileanalysis sentinel errors are passed through unchanged; secelfanalyzer checks them directly.
func (a *fileanalysisSyscallStoreAdapter) LoadSyscallAnalysis(filePath string, expectedHash string) (*secelfanalyzer.SyscallAnalysisResult, error) {
	result, err := a.inner.LoadSyscallAnalysis(filePath, expectedHash)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &secelfanalyzer.SyscallAnalysisResult{
		SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
	}, nil
}
