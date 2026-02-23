//go:build test

package security

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFileanalysisSyscallStore is an in-memory mock of fileanalysis.SyscallAnalysisStore.
type mockFileanalysisSyscallStore struct {
	result *fileanalysis.SyscallAnalysisResult
	err    error
}

func (m *mockFileanalysisSyscallStore) LoadSyscallAnalysis(_ string, _ string) (*fileanalysis.SyscallAnalysisResult, error) {
	return m.result, m.err
}

func (m *mockFileanalysisSyscallStore) SaveSyscallAnalysis(_, _ string, _ *fileanalysis.SyscallAnalysisResult) error {
	return nil
}

func TestNewELFSyscallStoreAdapter_ReturnResult(t *testing.T) {
	core := common.SyscallAnalysisResultCore{
		Architecture:       "x86_64",
		HasUnknownSyscalls: false,
		DetectedSyscalls: []common.SyscallInfo{
			{Number: 41, Name: "socket", IsNetwork: true},
		},
		Summary: common.SyscallSummary{
			HasNetworkSyscalls:  true,
			NetworkSyscallCount: 1,
		},
	}
	inner := &mockFileanalysisSyscallStore{
		result: &fileanalysis.SyscallAnalysisResult{SyscallAnalysisResultCore: core},
	}

	adapter := NewELFSyscallStoreAdapter(inner)
	got, err := adapter.LoadSyscallAnalysis("/bin/foo", "sha256:abc")

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, core, got.SyscallAnalysisResultCore)
}

func TestNewELFSyscallStoreAdapter_PassesThroughErrors(t *testing.T) {
	for _, sentinel := range []error{
		fileanalysis.ErrRecordNotFound,
		fileanalysis.ErrHashMismatch,
		fileanalysis.ErrNoSyscallAnalysis,
		errors.New("unexpected store error"),
	} {
		inner := &mockFileanalysisSyscallStore{err: sentinel}
		adapter := NewELFSyscallStoreAdapter(inner)

		_, err := adapter.LoadSyscallAnalysis("/bin/foo", "sha256:abc")

		assert.ErrorIs(t, err, sentinel)
	}
}
