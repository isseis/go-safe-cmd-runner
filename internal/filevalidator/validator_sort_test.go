//go:build test

package filevalidator

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Sort-stability tests for SymbolAnalysis slice fields
// ---------------------------------------------------------------------------

func TestRecord_DetectedSymbols_SortedAlphabetically(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
			{Name: "connect", Category: "network"},
			{Name: "bind", Category: "network"},
		},
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"bind", "connect", "socket"}, record.SymbolAnalysis.DetectedSymbols)
}

func TestRecord_DynamicLoadSymbols_SortedAlphabetically(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NoNetworkSymbols,
		dynamicLoadSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "dlvsym", Category: "dynamic_load"},
			{Name: "dlopen", Category: "dynamic_load"},
			{Name: "dlsym", Category: "dynamic_load"},
		},
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"dlopen", "dlsym", "dlvsym"}, record.SymbolAnalysis.DynamicLoadSymbols)
}

func TestRecord_KnownNetworkLibDeps_SortedAlphabetically(t *testing.T) {
	// Feed three known network libs in reverse-alphabetical order to verify sorting.
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "libssl.so.3", Path: "/usr/lib/libssl.so.3", Hash: "sha256:ccc"},
		{SOName: "libpython3.11.so.1.0", Path: "/usr/lib/libpython3.11.so.1.0", Hash: "sha256:bbb"},
		{SOName: "libcurl.so.4", Path: "/usr/lib/libcurl.so.4", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t,
		[]string{"libcurl.so.4", "libpython3.11.so.1.0", "libssl.so.3"},
		record.SymbolAnalysis.KnownNetworkLibDeps,
	)
}
