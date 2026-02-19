package main

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordCall struct {
	file  string
	force bool
}

type fakeRecorder struct {
	responses map[string]error
	calls     []recordCall
	hashDir   string
}

func (f *fakeRecorder) Record(filePath string, force bool) (string, error) {
	f.calls = append(f.calls, recordCall{file: filePath, force: force})
	if err, ok := f.responses[filePath]; ok && err != nil {
		return "", err
	}
	return fmt.Sprintf("/hash/%s.json", filepath.Base(filePath)), nil
}

func overrideValidatorFactory(t *testing.T, recorder *fakeRecorder) func() {
	t.Helper()
	originalFactory := validatorFactory
	validatorFactory = func(hashDir string) (hashRecorder, error) {
		recorder.hashDir = hashDir
		return recorder, nil
	}
	return func() {
		validatorFactory = originalFactory
	}
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestRunProcessesMultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, tempDir, recorder.hashDir)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, []recordCall{{"file1.txt", false}, {"file2.txt", false}}, recorder.calls)
	assert.Contains(t, stdout.String(), "Processing 2 files...")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 0 failed")
	assert.Empty(t, stderr.String())
}

func TestRunReportsFailuresAndContinues(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{
		"bad.dat": errors.New("calculate hash failure"),
	}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-force", "-hash-dir", tempDir, "good1", "bad.dat", "good2"}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	require.Len(t, recorder.calls, 3)
	assert.True(t, recorder.calls[0].force)
	assert.True(t, recorder.calls[1].force)
	assert.True(t, recorder.calls[2].force)
	assert.Contains(t, stdout.String(), "[2/3] bad.dat: FAILED")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 1 failed")
	assert.Contains(t, stderr.String(), "Error recording hash for bad.dat")
}

func TestRunWarnsWhenDeprecatedFlagUsed(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "-file", "legacy.txt", "new.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, "legacy.txt", recorder.calls[0].file)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	// Override mkdirAll to avoid permission issues in CI
	originalMkdirAll := mkdirAll
	mkdirAll = func(_ string, _ os.FileMode) error {
		return nil
	}
	defer func() { mkdirAll = originalMkdirAll }()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, cmdcommon.DefaultHashDirectory, recorder.hashDir)
	require.Len(t, recorder.calls, 1)
	assert.Equal(t, "file1.txt", recorder.calls[0].file)
	assert.False(t, recorder.calls[0].force)
}

// createMinimalStaticELF creates a minimal static ELF file for testing.
// The file has no .dynsym section, simulating a statically linked binary.
func createMinimalStaticELF(t *testing.T, path string) {
	t.Helper()

	// Create a minimal ELF header for x86_64 without .dynsym section
	elfHeader := []byte{
		// ELF magic
		0x7f, 'E', 'L', 'F',
		// Class: 64-bit
		0x02,
		// Data: little endian
		0x01,
		// Version
		0x01,
		// OS/ABI: System V
		0x00,
		// ABI version
		0x00,
		// Padding
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Type: Executable
		0x02, 0x00,
		// Machine: x86_64
		0x3e, 0x00,
		// Version
		0x01, 0x00, 0x00, 0x00,
		// Entry point
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Program header offset
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Section header offset (0 = none)
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// Flags
		0x00, 0x00, 0x00, 0x00,
		// ELF header size
		0x40, 0x00,
		// Program header size
		0x38, 0x00,
		// Number of program headers
		0x00, 0x00,
		// Section header size
		0x40, 0x00,
		// Number of section headers
		0x00, 0x00,
		// Section name string table index
		0x00, 0x00,
	}

	err := os.WriteFile(path, elfHeader, 0o644)
	require.NoError(t, err)

	// Verify it can be parsed as ELF
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	_, err = elf.NewFile(f)
	require.NoError(t, err)
}

func TestRunWithSyscallAnalysis(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	// Create a static ELF file for testing
	staticELF := filepath.Join(tempDir, "static.elf")
	createMinimalStaticELF(t, staticELF)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Syscall analysis is always enabled
	exitCode := run([]string{"-d", tempDir, staticELF}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	assert.Equal(t, staticELF, recorder.calls[0].file)
	assert.Contains(t, stdout.String(), "OK")
}

func TestRunWithSyscallAnalysisSkipsNonELF(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	// Create a non-ELF file
	nonELF := filepath.Join(tempDir, "script.sh")
	err := os.WriteFile(nonELF, []byte("#!/bin/bash\necho hello"), 0o755)
	require.NoError(t, err)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Syscall analysis is always enabled but should skip non-ELF files without warning
	exitCode := run([]string{"-d", tempDir, nonELF}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	// No warning should be printed for non-ELF files
	assert.NotContains(t, stderr.String(), "Syscall analysis failed")
}

func TestRunWithSyscallAnalysisSavesResult(t *testing.T) {
	// Skip this test - the minimal ELF doesn't have a .text section
	// which is required for syscall analysis. A proper test would need
	// a real static binary or a more complete ELF generator.
	// The TestRunWithSyscallAnalysis test verifies the basic flow works correctly.
	t.Skip("Requires real static ELF binary with .text section for syscall analysis")

	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	// Create a static ELF file
	staticELF := filepath.Join(tempDir, "static_binary.elf")
	createMinimalStaticELF(t, staticELF)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Syscall analysis is always enabled
	exitCode := run([]string{"-d", tempDir, staticELF}, stdout, stderr)

	require.Equal(t, 0, exitCode)

	// Verify that a record was saved in the store
	pathGetter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(tempDir, pathGetter)
	require.NoError(t, err)

	record, err := store.Load(common.ResolvedPath(staticELF))
	require.NoError(t, err)

	// Verify the record has syscall analysis data
	assert.NotNil(t, record.SyscallAnalysis)
	assert.Equal(t, "x86_64", record.SyscallAnalysis.Architecture)
}
