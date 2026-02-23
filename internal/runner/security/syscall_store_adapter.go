package security

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// fileanalysisSyscallStoreAdapter wraps fileanalysis.SyscallAnalysisStore and implements
// elfanalyzer.SyscallAnalysisStore.
// This bridges the two packages without creating an import cycle:
//
//	elfanalyzer → (no fileanalysis import)
//	fileanalysis → (no elfanalyzer import)
//	security     → both elfanalyzer and fileanalysis
type fileanalysisSyscallStoreAdapter struct {
	inner fileanalysis.SyscallAnalysisStore
}

// NewELFSyscallStoreAdapter wraps a fileanalysis.SyscallAnalysisStore so it satisfies
// the elfanalyzer.SyscallAnalysisStore interface.
// The returned value can be passed directly to elfanalyzer.NewStandardELFAnalyzerWithSyscallStore.
func NewELFSyscallStoreAdapter(store fileanalysis.SyscallAnalysisStore) elfanalyzer.SyscallAnalysisStore {
	return &fileanalysisSyscallStoreAdapter{inner: store}
}

// LoadSyscallAnalysis implements elfanalyzer.SyscallAnalysisStore.
// Converts the result type via the shared common.SyscallAnalysisResultCore embedding.
// fileanalysis sentinel errors are passed through unchanged; elfanalyzer checks them directly.
func (a *fileanalysisSyscallStoreAdapter) LoadSyscallAnalysis(filePath string, expectedHash string) (*elfanalyzer.SyscallAnalysisResult, error) {
	result, err := a.inner.LoadSyscallAnalysis(filePath, expectedHash)
	if err != nil {
		return nil, err
	}
	return &elfanalyzer.SyscallAnalysisResult{
		SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
	}, nil
}
