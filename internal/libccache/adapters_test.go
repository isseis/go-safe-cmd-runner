package libccache

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"

	elfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyscallAdapterAnalyzeSyscallsFromELF_PassesThroughDeterminationStats(t *testing.T) {
	elfPath := filepath.Join("..", "runner", "security", "elfanalyzer", "testdata", "arm64_network_program", "arm64_network_program.elf")

	f, err := os.Open(elfPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.Close()
	})

	ef, err := elf.NewFile(f)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ef.Close()
	})

	adapter := NewSyscallAdapter(elfanalyzer.NewSyscallAnalyzer())

	_, _, stats, err := adapter.AnalyzeSyscallsFromELF(ef)
	require.NoError(t, err)
	assert.NotNil(t, stats)
}
