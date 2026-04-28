package libccache

import (
	"debug/elf"
	"path/filepath"
	"testing"

	elfanalyzer "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyscallAdapterAnalyzeSyscallsFromELF_PassesThroughDeterminationStats(t *testing.T) {
	elfPath := filepath.Join("..", "runner", "security", "elfanalyzer", "testdata", "arm64_network_program", "arm64_network_program.elf")

	ef, err := elf.Open(elfPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ef.Close()
	})

	adapter := NewSyscallAdapter(elfanalyzer.NewSyscallAnalyzer())

	_, _, stats, err := adapter.AnalyzeSyscallsFromELF(ef)
	require.NoError(t, err)
	assert.NotNil(t, stats)
}
