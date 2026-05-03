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
	calls  int
}

func (s *libraryTestBinaryAnalyzer) AnalyzeNetworkSymbols(_, _ string) binaryanalyzer.AnalysisOutput {
	s.calls++
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

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
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
		table:    &libraryTestSyscallTable{network: map[int]bool{41: true}},
	})

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
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

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
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

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libmissing.so.1",
		Path:   filepath.Join(safeTempDir(t), "missing.so"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	require.NotEmpty(t, warnings)
	assert.Contains(t, warnings[0], "failed to open library file")
}

func TestAnalyzeOneLibrary_nonELFLibrarySkipsSyscallScan(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{syscalls: []common.SyscallInfo{{Number: 41, Name: "socket"}}})

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
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

	entry, hasNetwork, _, warnings, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libfoo.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.False(t, hasNetwork)
	assert.Nil(t, entry.SyscallAnalysis)
	assert.Empty(t, warnings)
}

func TestAnalyzeLibraries_disabled(t *testing.T) {
	v := validatorWithTempHashDir(t)
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{{SOName: "libfoo.so.1", Path: elfTestDataPath(t, "with_socket.elf")}},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Nil(t, record.LibraryAnalysis)
}

func TestAnalyzeLibraries_emptyDynLibDeps(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetLibraryAnalysisEnabled(true)
	record := &fileanalysis.Record{}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Nil(t, record.LibraryAnalysis)
}

func TestAnalyzeLibraries_excludesWrapperAndVDSO(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetLibraryAnalysisEnabled(true)

	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libc.so.6", Path: elfTestDataPath(t, "with_socket.elf")},
			{SOName: "linux-vdso.so.1", Path: ""},
			{SOName: "libssl.so.3", Path: elfTestDataPath(t, "with_socket.elf")},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	require.Len(t, record.LibraryAnalysis, 1)
	assert.Equal(t, "libssl.so.3", record.LibraryAnalysis[0].SOName)
	assert.Equal(t, 1, bin.calls)
}

func TestAnalyzeLibraries_sessionCache(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetLibraryAnalysisEnabled(true)

	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	path := elfTestDataPath(t, "with_socket.elf")
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libfoo.so.1", Path: path},
			{SOName: "libfoo-alias.so.1", Path: path},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Equal(t, 1, bin.calls)
	require.Len(t, record.LibraryAnalysis, 2)
	assert.Equal(t, "libfoo.so.1", record.LibraryAnalysis[0].SOName)
	assert.Equal(t, "libfoo-alias.so.1", record.LibraryAnalysis[1].SOName)
}

func TestAnalyzeLibraries_symbolAnalysisCreatedWhenNil(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetLibraryAnalysisEnabled(true)

	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NetworkDetected,
			DetectedSymbols: []binaryanalyzer.DetectedSymbol{
				{Name: "socket", Category: "socket"},
			},
		},
	})

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{{SOName: "libnet.so.1", Path: elfTestDataPath(t, "with_socket.elf")}},
	}

	require.NoError(t, v.analyzeLibraries(record))
	require.NotNil(t, record.SymbolAnalysis)
	require.Equal(t, []string{"libnet.so.1"}, record.SymbolAnalysis.DetectedLibraryNetworkDeps)
}
