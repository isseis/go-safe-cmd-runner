package filevalidator

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepCollector_AddEntries_DedupByPath(t *testing.T) {
	collector := newDepCollector(true)

	first := fileanalysis.LibEntry{
		SOName: "libfoo.so",
		Path:   "/usr/lib/libfoo.so.1",
		Hash:   "sha256:111",
	}
	second := fileanalysis.LibEntry{
		SOName: "libfoo_alias.so",
		Path:   "/usr/lib/libfoo.so.1",
		Hash:   "sha256:111",
	}

	require.NoError(t, collector.addEntry("/app/bin/main", first))
	require.NoError(t, collector.addEntry("/usr/bin/python3", second))

	entries := collector.entries()
	require.Len(t, entries, 1)
	assert.Equal(t, "/usr/lib/libfoo.so.1", entries[0].Path)
	assert.Equal(t, "sha256:111", entries[0].Hash)

	debug := collector.debugRecord()
	require.NotNil(t, debug)
	require.Contains(t, debug.DepSources, "/usr/lib/libfoo.so.1")
	assert.Equal(t, []string{"/app/bin/main", "/usr/bin/python3"}, debug.DepSources["/usr/lib/libfoo.so.1"])
}

func TestDepCollector_AddEntries_HashMismatch(t *testing.T) {
	collector := newDepCollector(false)

	require.NoError(t, collector.addEntry("/app/bin/main", fileanalysis.LibEntry{
		SOName: "libfoo.so",
		Path:   "/usr/lib/libfoo.so.1",
		Hash:   "sha256:111",
	}))

	err := collector.addEntry("/usr/bin/python3", fileanalysis.LibEntry{
		SOName: "libfoo.so",
		Path:   "/usr/lib/libfoo.so.1",
		Hash:   "sha256:222",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, errDependencyHashMismatch)
}
