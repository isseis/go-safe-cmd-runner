//go:build test

package filevalidator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type shebangCacheSpyBinaryAnalyzer struct {
	output      binaryanalyzer.AnalysisOutput
	callsByPath map[string]int
}

func (s *shebangCacheSpyBinaryAnalyzer) AnalyzeNetworkSymbols(filePath, _ string) binaryanalyzer.AnalysisOutput {
	if s.callsByPath == nil {
		s.callsByPath = make(map[string]int)
	}
	s.callsByPath[filePath]++
	return s.output
}

func TestSaveRecord_ShebangInterpreterCacheReuse(t *testing.T) {
	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	interpreterPath, err := filepath.EvalSymlinks("/bin/sh")
	if err != nil {
		t.Skip("skipping: /bin/sh not available in this environment")
	}

	spy := &shebangCacheSpyBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols},
	}
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	validator.SetBinaryAnalyzer(spy)

	scriptA := commontesting.WriteExecutableFile(t, scriptDir, "a.sh", []byte("#!/bin/sh\necho A\n"))
	scriptB := commontesting.WriteExecutableFile(t, scriptDir, "b.sh", []byte("#!/bin/sh\necho B\n"))

	_, _, err = validator.SaveRecord(scriptA, false)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptB, false)
	require.NoError(t, err)

	assert.Equal(t, 1, spy.callsByPath[interpreterPath], "interpreter should be analyzed exactly once in one Validator session")
}

func TestSaveRecord_ShebangInterpreterCacheOutputEquivalence(t *testing.T) {
	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	interpreterPath, err := filepath.EvalSymlinks("/bin/sh")
	if err != nil {
		t.Skip("skipping: /bin/sh not available in this environment")
	}

	spy := &shebangCacheSpyBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols},
	}
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	validator.SetBinaryAnalyzer(spy)

	scriptA := commontesting.WriteExecutableFile(t, scriptDir, "a.sh", []byte("#!/bin/sh\necho A\n"))
	scriptB := commontesting.WriteExecutableFile(t, scriptDir, "b.sh", []byte("#!/bin/sh\necho B\n"))

	_, _, err = validator.SaveRecord(scriptA, false)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptB, false)
	require.NoError(t, err)

	recordA, err := validator.LoadRecord(scriptA)
	require.NoError(t, err)
	recordB, err := validator.LoadRecord(scriptB)
	require.NoError(t, err)

	assert.Equal(t, recordA.ShebangChain, recordB.ShebangChain)
	assert.Equal(t, recordA.DynLibDeps, recordB.DynLibDeps)
	assert.Equal(t, recordA.SymbolAnalysis, recordB.SymbolAnalysis)
	assert.Equal(t, recordA.SyscallAnalysis, recordB.SyscallAnalysis)
	assert.Equal(t, recordA.AnalysisWarnings, recordB.AnalysisWarnings)
	assert.Equal(t, recordA.ShebangInterpreter, recordB.ShebangInterpreter)
	assert.Equal(t, 1, spy.callsByPath[interpreterPath], "second record should reuse cached interpreter analysis")

	assertShebangInterpreterDepPresent(t, recordB, interpreterPath)
}

func TestSaveRecord_ShebangInterpreterCacheHashChangeReanalyzes(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	if runtime.GOOS != "linux" {
		t.Skipf("test requires Linux (got %s)", runtime.GOOS)
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("test requires cc (install build-essential)")
	}

	interpreterPath := filepath.Join(dir, "test-interpreter")
	buildTestInterpreterBinary(t, interpreterPath, 1)

	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	spy := &shebangCacheSpyBinaryAnalyzer{output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}}
	validator.SetBinaryAnalyzer(spy)

	scriptA := commontesting.WriteExecutableFile(t, dir, "a.sh", []byte(fmt.Sprintf("#!%s\necho A\n", interpreterPath)))
	_, _, err = validator.SaveRecord(scriptA, false)
	require.NoError(t, err)

	hashA, err := validator.prefixedHashForPath(interpreterPath)
	require.NoError(t, err)

	buildTestInterpreterBinary(t, interpreterPath, 2)
	hashB, err := validator.prefixedHashForPath(interpreterPath)
	require.NoError(t, err)
	require.NotEqual(t, hashA, hashB)

	scriptB := commontesting.WriteExecutableFile(t, dir, "b.sh", []byte(fmt.Sprintf("#!%s\necho B\n", interpreterPath)))
	_, _, err = validator.SaveRecord(scriptB, false)
	require.NoError(t, err)

	assert.Equal(t, 2, spy.callsByPath[interpreterPath], "interpreter should be re-analyzed after hash change")

	cacheKeys := make(map[libCacheKey]struct{})
	for key := range validator.processedInterpreterAnalysis {
		if key.Path == interpreterPath {
			cacheKeys[key] = struct{}{}
		}
	}
	assert.Len(t, cacheKeys, 2)
	_, okA := cacheKeys[libCacheKey{Path: interpreterPath, Hash: hashA}]
	assert.True(t, okA)
	_, okB := cacheKeys[libCacheKey{Path: interpreterPath, Hash: hashB}]
	assert.True(t, okB)
}

func TestSaveRecord_ShebangInterpreterCacheEnvForm(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")

	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	envPath, err := filepath.EvalSymlinks("/usr/bin/env")
	if err != nil {
		t.Skip("skipping: /usr/bin/env not available in this environment")
	}
	shFound, err := exec.LookPath("sh")
	require.NoError(t, err)
	resolvedShPath, err := filepath.EvalSymlinks(shFound)
	require.NoError(t, err)

	spy := &shebangCacheSpyBinaryAnalyzer{
		output: binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols},
	}
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	validator.SetBinaryAnalyzer(spy)

	scriptA := commontesting.WriteExecutableFile(t, scriptDir, "a.sh", []byte("#!/usr/bin/env sh\necho A\n"))
	scriptB := commontesting.WriteExecutableFile(t, scriptDir, "b.sh", []byte("#!/usr/bin/env sh\necho B\n"))

	_, _, err = validator.SaveRecord(scriptA, false)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(scriptB, false)
	require.NoError(t, err)

	assert.Equal(t, 1, spy.callsByPath[envPath], "env binary should be analyzed exactly once")
	assert.Equal(t, 1, spy.callsByPath[resolvedShPath], "resolved command binary should be analyzed exactly once")
}

func assertShebangInterpreterDepPresent(t *testing.T, record *fileanalysis.Record, interpreterPath string) {
	t.Helper()
	for _, dep := range record.DynLibDeps {
		if dep.Path == interpreterPath {
			return
		}
	}
	t.Fatalf("record.DynLibDeps does not include interpreter path: %s", interpreterPath)
}

func buildTestInterpreterBinary(t *testing.T, outPath string, variant int) {
	t.Helper()

	srcPath := outPath + ".c"
	src := fmt.Sprintf(`#include <unistd.h>
int main(void) {
	return %d;
}
`, variant)
	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0o644))

	cmd := exec.Command("cc", "-O0", "-o", outPath, srcPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile test interpreter: %s", string(out))
	require.NoError(t, os.Chmod(outPath, 0o755))
}
