//go:build test && darwin

package machodylib

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractLibSystemKernel_Live skips the test when
// the dyld shared cache cannot be expected to be present (non-darwin or non-arm64).
// On darwin arm64, it attempts an actual extraction and verifies the invariants.
func TestExtractLibSystemKernel_Live(t *testing.T) {
	if runtime.GOOS != "darwin" || (runtime.GOARCH != "arm64") {
		t.Skip("dyld shared cache extraction only applicable on darwin/arm64")
	}

	// Check if any known cache file is present; skip if not.
	cacheFound := false
	for _, p := range dyldSharedCachePaths {
		if _, err := os.Stat(p); err == nil {
			cacheFound = true
			break
		}
	}
	if !cacheFound {
		t.Skipf("no dyld shared cache found in %v; skipping live extraction test", dyldSharedCachePaths)
	}

	result, err := ExtractLibSystemKernel()
	require.NoError(t, err)

	if result == nil {
		t.Skip("ExtractLibSystemKernel returned nil; " +
			"libsystem_kernel.dylib may not be present in this environment's dyld shared cache")
	}

	// Verify invariants on a successful extraction.
	assert.NotEmpty(t, result.Data, "extracted Data must not be empty")
	assert.True(t, strings.HasPrefix(result.Hash, "sha256:"),
		"extracted Hash must start with 'sha256:', got %q", result.Hash)
	assert.Greater(t, len(result.Hash), len("sha256:"),
		"extracted Hash must contain a hex digest after the prefix")
}

// TestExtractLibSystemKernel_NoCachePaths verifies that when no
// dyld shared cache is available, the function returns nil, nil.
//
// This test overrides the package-level dyldSharedCachePaths to point to
// non-existent paths, simulating the absence of a shared cache.
func TestExtractLibSystemKernel_NoCachePaths(t *testing.T) {
	original := dyldSharedCachePaths
	t.Cleanup(func() { dyldSharedCachePaths = original })

	// Point to paths that never exist.
	dyldSharedCachePaths = []string{
		"/nonexistent/dyld_shared_cache_arm64e",
		"/nonexistent/dyld_shared_cache_arm64",
	}

	result, err := ExtractLibSystemKernel()
	require.NoError(t, err)
	assert.Nil(t, result, "expected nil result when no cache paths exist")
}
