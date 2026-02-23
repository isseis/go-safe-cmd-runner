package security

import (
	"errors"

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
// It translates fileanalysis sentinel errors to elfanalyzer sentinel errors and
// converts the result type via the shared common.SyscallAnalysisResultCore embedding.
func (a *fileanalysisSyscallStoreAdapter) LoadSyscallAnalysis(filePath string, expectedHash string) (*elfanalyzer.SyscallAnalysisResult, error) {
	result, err := a.inner.LoadSyscallAnalysis(filePath, expectedHash)
	if err != nil {
		return nil, a.translateError(err)
	}
	return &elfanalyzer.SyscallAnalysisResult{
		SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
	}, nil
}

// translateError maps fileanalysis sentinel errors to the corresponding elfanalyzer sentinels.
// Non-sentinel errors are passed through unchanged.
func (a *fileanalysisSyscallStoreAdapter) translateError(err error) error {
	switch {
	case errors.Is(err, fileanalysis.ErrRecordNotFound):
		return elfanalyzer.ErrRecordNotFound
	case errors.Is(err, fileanalysis.ErrHashMismatch):
		return elfanalyzer.ErrHashMismatch
	case errors.Is(err, fileanalysis.ErrNoSyscallAnalysis):
		return elfanalyzer.ErrNoSyscallAnalysis
	default:
		return err
	}
}
