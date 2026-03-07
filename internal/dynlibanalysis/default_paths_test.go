//go:build test

package dynlibanalysis

import (
	"debug/elf"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSearchPaths_X86_64(t *testing.T) {
	paths := DefaultSearchPaths(elf.EM_X86_64)
	assert.Contains(t, paths, "/lib/x86_64-linux-gnu")
	assert.Contains(t, paths, "/usr/lib/x86_64-linux-gnu")
	assert.Contains(t, paths, "/lib64")
	assert.Contains(t, paths, "/usr/lib64")
	assert.Contains(t, paths, "/lib")
	assert.Contains(t, paths, "/usr/lib")
	// Multiarch paths should come before generic paths
	x86Idx := indexOf(paths, "/lib/x86_64-linux-gnu")
	genericIdx := indexOf(paths, "/lib")
	assert.Less(t, x86Idx, genericIdx, "multiarch paths should precede generic paths")
}

func TestDefaultSearchPaths_AARCH64(t *testing.T) {
	paths := DefaultSearchPaths(elf.EM_AARCH64)
	assert.Contains(t, paths, "/lib/aarch64-linux-gnu")
	assert.Contains(t, paths, "/usr/lib/aarch64-linux-gnu")
	assert.Contains(t, paths, "/lib64")
	assert.Contains(t, paths, "/usr/lib64")
	assert.Contains(t, paths, "/lib")
	assert.Contains(t, paths, "/usr/lib")
	// Multiarch paths should come before generic paths
	aarch64Idx := indexOf(paths, "/lib/aarch64-linux-gnu")
	genericIdx := indexOf(paths, "/lib")
	assert.Less(t, aarch64Idx, genericIdx, "multiarch paths should precede generic paths")
}

func TestDefaultSearchPaths_Unknown(t *testing.T) {
	// Unknown architecture should return generic paths without multiarch
	paths := DefaultSearchPaths(elf.EM_386)
	assert.Contains(t, paths, "/lib64")
	assert.Contains(t, paths, "/usr/lib64")
	assert.Contains(t, paths, "/lib")
	assert.Contains(t, paths, "/usr/lib")
	assert.NotContains(t, paths, "/lib/x86_64-linux-gnu")
	assert.NotContains(t, paths, "/lib/aarch64-linux-gnu")
}

func TestDefaultSearchPaths_NonEmpty(t *testing.T) {
	machines := []elf.Machine{elf.EM_X86_64, elf.EM_AARCH64, elf.EM_386, elf.EM_ARM}
	for _, m := range machines {
		paths := DefaultSearchPaths(m)
		assert.NotEmpty(t, paths, "should return non-empty paths for machine %v", m)
	}
}

// indexOf returns the index of the first occurrence of target in slice, or -1 if not found.
func indexOf(slice []string, target string) int {
	for i, s := range slice {
		if s == target {
			return i
		}
	}
	return -1
}
