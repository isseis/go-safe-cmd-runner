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
	assert.Equal(t, []fileanalysis.DetectedSymbol{{Name: "bind"}, {Name: "connect"}, {Name: "socket"}}, record.SymbolAnalysis.DetectedSymbols)
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
	assert.Equal(t, []fileanalysis.DetectedSymbol{{Name: "dlopen"}, {Name: "dlsym"}, {Name: "dlvsym"}}, record.SymbolAnalysis.DynamicLoadSymbols)
}
