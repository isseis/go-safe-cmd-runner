//go:build test

package filevalidator

import (
	"debug/elf"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type libraryTestBinaryAnalyzer struct {
	output binaryanalyzer.AnalysisOutput
}

func (s *libraryTestBinaryAnalyzer) AnalyzeNetworkSymbols(_, _ string) binaryanalyzer.AnalysisOutput {
	return s.output
}

type libraryTestSyscallTable struct {
	network map[int]bool
}

func (t *libraryTestSyscallTable) GetSyscallName(_ int) string {
	return ""
}

func (t *libraryTestSyscallTable) IsNetworkSyscall(number int) bool {
	if t == nil {
		return false
	}
	return t.network[number]
}

type libraryTestSyscallAnalyzer struct {
	syscalls []common.SyscallInfo
	err      error
	table    SyscallNumberTable
}

func (s *libraryTestSyscallAnalyzer) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return s.syscalls, nil, nil, s.err
}

func (s *libraryTestSyscallAnalyzer) EvaluatePLTCallArgs(_ *elf.File, _ string) (*common.SyscallArgEvalResult, error) {
	return nil, nil
}

func (s *libraryTestSyscallAnalyzer) GetSyscallTable(_ elf.Machine) (SyscallNumberTable, bool) {
	if s.table == nil {
		return nil, false
	}
	return s.table, true
}

func validatorWithTempHashDir(t *testing.T) *Validator {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	return v
}

func elfTestDataPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(thisFile), "..", "security", "elfanalyzer", "testdata", name)
}

func TestAnalyzeOneLibrary_networkSymbolDetected(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NetworkDetected,
			DetectedSymbols: []binaryanalyzer.DetectedSymbol{
				{Name: "socket", Category: "socket"},
			},
		},
	})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libfoo.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.True(t, hasNetwork)
	require.NotNil(t, entry.SymbolAnalysis)
	assert.Contains(t, entry.SymbolAnalysis.DetectedSymbols, "socket")
	assert.Empty(t, warnings)
}

func TestAnalyzeOneLibrary_networkSyscallDetected(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{
		syscalls: []common.SyscallInfo{{Number: 41, Name: "socket"}},
		table: &libraryTestSyscallTable{network: map[int]bool{41: true}},
	})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libbar.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.True(t, hasNetwork)
	require.NotNil(t, entry.SyscallAnalysis)
	assert.NotEmpty(t, entry.SyscallAnalysis.DetectedSyscalls)
	assert.Empty(t, warnings)
}

func TestAnalyzeOneLibrary_nonNetwork(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{syscalls: []common.SyscallInfo{{Number: 1, Name: "write"}}})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libbaz.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	assert.Empty(t, warnings)
}

func TestAnalyzeOneLibrary_missingFileAddsWarning(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libmissing.so.1",
		Path:   filepath.Join(safeTempDir(t), "missing.so"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "failed to open library ELF")
}

func TestAnalyzeOneLibrary_nonELFLibrarySkipsSyscallScan(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{syscalls: []common.SyscallInfo{{Number: 41, Name: "socket"}}})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libscript.so.1",
		Path:   elfTestDataPath(t, "script.sh"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	assert.Nil(t, entry.SyscallAnalysis)
	assert.Empty(t, warnings)
}

func TestAnalyzeOneLibrary_unsupportedArchSkipsWarning(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{err: ErrUnsupportedArch})

	entry, hasNetwork, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libfoo.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	assert.Nil(t, entry.SyscallAnalysis)
	assert.Empty(t, warnings)
}
