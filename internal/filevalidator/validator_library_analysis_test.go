//go:build test

package filevalidator

import (
	"debug/elf"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
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
	syscalls   []common.SyscallInfo
	argResults []common.SyscallArgEvalResult
	err        error
	table      SyscallNumberTable
}

func (s *libraryTestSyscallAnalyzer) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return s.syscalls, s.argResults, nil, s.err
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

// validatorWithStore creates a Validator with a real dynamicanalysis.Store backed by the
// validator itself. Used for tests that exercise the full analyzeLibraries flow.
func validatorWithStore(t *testing.T) *Validator {
	t.Helper()
	v := validatorWithTempHashDir(t)
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynamicanalysis.New(storeDir, v)
	require.NoError(t, err)
	v.SetDynamicLibAnalysisStore(store)
	return v
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

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libfoo.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.SymbolAnalysis)
	assert.Contains(t, result.SymbolAnalysis.DetectedSymbols, "socket")
	assert.Empty(t, result.Warnings)
}

func TestAnalyzeOneLibrary_networkSyscallDetected(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{
		syscalls: []common.SyscallInfo{{Number: 41, Name: "socket"}},
		table:    &libraryTestSyscallTable{network: map[int]bool{41: true}},
	})

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libbar.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.SyscallAnalysis)
	assert.NotEmpty(t, result.SyscallAnalysis.DetectedSyscalls)
	assert.Empty(t, result.Warnings)
}

func TestAnalyzeOneLibrary_preservesArgEvalResults(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{
		syscalls: []common.SyscallInfo{{Number: 10, Name: "mprotect"}},
		argResults: []common.SyscallArgEvalResult{{
			SyscallName: "mprotect",
			Status:      common.SyscallArgEvalExecConfirmed,
			Details:     "prot=0x5",
		}},
	})

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libjit.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.SyscallAnalysis)
	require.Len(t, result.SyscallAnalysis.ArgEvalResults, 1)
	assert.Equal(t, "mprotect", result.SyscallAnalysis.ArgEvalResults[0].SyscallName)
	assert.Equal(t, common.SyscallArgEvalExecConfirmed, result.SyscallAnalysis.ArgEvalResults[0].Status)
	assert.Equal(t, "prot=0x5", result.SyscallAnalysis.ArgEvalResults[0].Details)
}

func TestAnalyzeOneLibrary_nonNetwork(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{syscalls: []common.SyscallInfo{{Number: 1, Name: "write"}}})

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libbaz.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// No network symbols detected; SymbolAnalysis may still be populated with empty data.
	if result.SymbolAnalysis != nil {
		assert.Empty(t, result.SymbolAnalysis.DetectedSymbols)
	}
	// Syscall analysis runs but write (syscall 1) is not a network syscall.
	assert.Empty(t, result.Warnings)
}

// TestAnalyzeOneLibrary_missingFileReturnsError verifies that a missing library file
// returns an error rather than a non-fatal warning.
func TestAnalyzeOneLibrary_missingFileReturnsError(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{})

	_, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libmissing.so.1",
		Path:   filepath.Join(safeTempDir(t), "missing.so"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open library file")
}

func TestAnalyzeOneLibrary_nonELFLibrarySkipsSyscallScan(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{syscalls: []common.SyscallInfo{{Number: 41, Name: "socket"}}})

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libscript.so.1",
		Path:   elfTestDataPath(t, "script.sh"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.SyscallAnalysis)
	assert.Nil(t, result.SymbolAnalysis)
	assert.Empty(t, result.Warnings)
}

func TestAnalyzeOneLibrary_unsupportedArchSkipsWarning(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{err: ErrUnsupportedArch})

	result, err := v.analyzeOneLibrary(fileanalysis.LibEntry{
		SOName: "libfoo.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.SyscallAnalysis)
	assert.Nil(t, result.SymbolAnalysis)
	assert.Empty(t, result.Warnings)
}

// TestAnalyzeLibraries_disabled verifies that library analysis is skipped when
// no dynamic library analysis store is configured.
func TestAnalyzeLibraries_disabled(t *testing.T) {
	v := validatorWithTempHashDir(t)
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{{SOName: "libfoo.so.1", Path: elfTestDataPath(t, "with_socket.elf")}},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Nil(t, record.SymbolAnalysis)
}

// TestAnalyzeLibraries_emptyDynLibDeps verifies that no analysis is performed when
// the record has no dynamic library dependencies.
func TestAnalyzeLibraries_emptyDynLibDeps(t *testing.T) {
	v := validatorWithStore(t)
	record := &fileanalysis.Record{}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Nil(t, record.SymbolAnalysis)
}

// TestAnalyzeLibraries_ExcludesWrapperAndVDSO verifies that syscall wrapper libraries
// (e.g., libc) and VDSO entries are excluded from library analysis.
func TestAnalyzeLibraries_excludesWrapperAndVDSO(t *testing.T) {
	v := validatorWithStore(t)

	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libc.so.6", Path: elfTestDataPath(t, "with_socket.elf"), Hash: "sha256:aaa"},
			{SOName: "linux-vdso.so.1", Path: "", Hash: ""},
			{SOName: "libssl.so.3", Path: elfTestDataPath(t, "with_socket.elf"), Hash: "sha256:bbb"},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Equal(t, 1, bin.calls)
}

// TestAnalyzeLibraries_sessionCache verifies that repeated analysis of the same
// library path and hash within a single session uses the in-session cache.
func TestAnalyzeLibraries_sessionCache(t *testing.T) {
	v := validatorWithStore(t)

	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	path := elfTestDataPath(t, "with_socket.elf")
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libfoo.so.1", Path: path, Hash: "sha256:same"},
			{SOName: "libfoo-alias.so.1", Path: path, Hash: "sha256:same"},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Equal(t, 1, bin.calls)
}

// TestAnalyzeLibraries_symbolAnalysisCreatedWhenNil verifies that DynamicLoadSymbols
// are merged and SymbolAnalysis is created when a library has dlopen/dlsym symbols.
func TestAnalyzeLibraries_symbolAnalysisCreatedWhenNil(t *testing.T) {
	v := validatorWithStore(t)

	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{
			Result: binaryanalyzer.NetworkDetected,
			DynamicLoadSymbols: []binaryanalyzer.DetectedSymbol{
				{Name: "dlopen", Category: "dynamic_load"},
			},
		},
	})

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libnet.so.1", Path: elfTestDataPath(t, "with_socket.elf"), Hash: "sha256:netlib"},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	require.NotNil(t, record.SymbolAnalysis)
	require.Contains(t, record.SymbolAnalysis.DynamicLoadSymbols, "dlopen")
}

// TestAnalyzeLibraries_RecordHasNoLibraryAnalysisField verifies that the record does not
// contain a library_analysis field after analysis (results are stored externally).
func TestAnalyzeLibraries_RecordHasNoLibraryAnalysisField(t *testing.T) {
	v := validatorWithStore(t)
	v.SetBinaryAnalyzer(&libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}})

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libfoo.so.1", Path: elfTestDataPath(t, "with_socket.elf"), Hash: "sha256:foo"},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	// Record.LibraryAnalysis no longer exists; this test confirms compilation
	// succeeds without referencing the removed field.
}

// TestAnalyzeLibraries_DynLibDepsPreservedOnReuse verifies that DynLibDeps entries
// are correctly recorded in the executable record even when the store reuses
// an existing analysis result.
func TestAnalyzeLibraries_DynLibDepsPreservedOnReuse(t *testing.T) {
	v := validatorWithStore(t)
	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	path := elfTestDataPath(t, "with_socket.elf")
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libfoo.so.1", Path: path, Hash: "sha256:foohash"},
		},
	}

	// First analysis run.
	require.NoError(t, v.analyzeLibraries(record))
	assert.Len(t, record.DynLibDeps, 1)
	assert.Equal(t, "libfoo.so.1", record.DynLibDeps[0].SOName)

	// Simulate a second run with the same DynLibDeps (store will reuse the result).
	record2 := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libfoo.so.1", Path: path, Hash: "sha256:foohash"},
		},
	}
	require.NoError(t, v.analyzeLibraries(record2))
	assert.Len(t, record2.DynLibDeps, 1)
	assert.Equal(t, "libfoo.so.1", record2.DynLibDeps[0].SOName)
}

// TestAnalyzeLibraries_MissingLibFileReturnsError verifies that a missing library file
// causes analyzeLibraries to return an error, and the record is not written.
func TestAnalyzeLibraries_MissingLibFileReturnsError(t *testing.T) {
	v := validatorWithStore(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{})

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libmissing.so.1", Path: filepath.Join(t.TempDir(), "no_such.so"), Hash: "sha256:x"},
		},
	}

	err := v.analyzeLibraries(record)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open library file")
}

// TestAnalyzeLibraries_VDSOExcluded verifies that VDSO entries are excluded from
// library analysis even when they are the only entry in DynLibDeps.
func TestAnalyzeLibraries_VDSOExcluded(t *testing.T) {
	v := validatorWithStore(t)
	bin := &libraryTestBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	v.SetBinaryAnalyzer(bin)

	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "linux-vdso.so.1", Path: "", Hash: ""},
		},
	}

	require.NoError(t, v.analyzeLibraries(record))
	assert.Equal(t, 0, bin.calls, "VDSO should not trigger library analysis")
}

// TestValidatorLibraryAnalyzer_Analyze_FileTooLargeReturnsError verifies that a library
// file exceeding the size limit causes analyzeLibraries to return an error.
func TestValidatorLibraryAnalyzer_Analyze_FileTooLargeReturnsError(t *testing.T) {
	v := validatorWithTempHashDir(t)
	v.SetSyscallAnalyzer(&libraryTestSyscallAnalyzer{})

	// Use a mock FileSystem that reports maxFileSize+1 for Stat().
	v.fileSystem = &oversizeFileSystem{FileSystem: v.fileSystem}

	lib := fileanalysis.LibEntry{
		SOName: "libbig.so.1",
		Path:   elfTestDataPath(t, "with_socket.elf"),
	}
	_, err := v.analyzeOneLibrary(lib)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "library file too large")
}

// oversizeFileSystem is a safefileio.FileSystem wrapper that reports oversized files.
// It embeds the real FileSystem and overrides SafeOpenFile to return an oversized file.
type oversizeFileSystem struct {
	safefileio.FileSystem
}

func (fs *oversizeFileSystem) SafeOpenFile(name string, flag int, perm os.FileMode) (safefileio.File, error) {
	f, err := fs.FileSystem.SafeOpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &oversizeStatFile{File: f}, nil
}

// oversizeStatFile wraps a real file but overrides Stat() to report an oversized file.
type oversizeStatFile struct {
	safefileio.File
}

func (f *oversizeStatFile) Stat() (os.FileInfo, error) {
	return &oversizeFileInfo{}, nil
}

// oversizeFileInfo is an os.FileInfo that reports maxFileSize+1.
type oversizeFileInfo struct{}

func (i *oversizeFileInfo) Name() string       { return "oversized.so" }
func (i *oversizeFileInfo) Size() int64        { return maxFileSize + 1 }
func (i *oversizeFileInfo) Mode() os.FileMode  { return 0o644 }
func (i *oversizeFileInfo) ModTime() time.Time { return time.Time{} }
func (i *oversizeFileInfo) IsDir() bool        { return false }
func (i *oversizeFileInfo) Sys() any           { return nil }
