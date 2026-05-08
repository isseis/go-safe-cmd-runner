package filevalidator

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
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

func TestAnalysisAggregate_DetectedSymbol_DedupByNameAndSourcePath_DebugMode(t *testing.T) {
	aggregate := newAnalysisAggregate(true)
	aggregate.addRecord(&fileanalysis.Record{
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
		},
	}, "/app/bin/main", roleMain)
	aggregate.addRecord(&fileanalysis.Record{
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
		},
	}, "/usr/bin/python3", roleShebangInterpreter)

	result := aggregate.symbolAnalysis()
	require.NotNil(t, result)
	assert.Equal(t, []fileanalysis.DetectedSymbol{{Name: "socket", SourcePath: "/app/bin/main"}, {Name: "socket", SourcePath: "/usr/bin/python3"}}, result.DetectedSymbols)
}

func TestAnalysisAggregate_DetectedSymbol_DedupByNameOnly_NonDebugMode(t *testing.T) {
	aggregate := newAnalysisAggregate(false)
	aggregate.addRecord(&fileanalysis.Record{
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
		},
	}, "/app/bin/main", roleMain)
	aggregate.addRecord(&fileanalysis.Record{
		SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
			DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
		},
	}, "/usr/bin/python3", roleShebangInterpreter)

	result := aggregate.symbolAnalysis()
	require.NotNil(t, result)
	assert.Equal(t, []fileanalysis.DetectedSymbol{{Name: "socket"}}, result.DetectedSymbols)
}

func TestAnalysisAggregate_AddRecord_SourcePathNotOverwrittenIfAlreadySet(t *testing.T) {
	aggregate := newAnalysisAggregate(true)
	aggregate.addRecord(&fileanalysis.Record{
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []common.SyscallInfo{{
					Number: 41,
					Name:   "socket",
					Occurrences: []common.SyscallOccurrence{{
						Location:            0,
						DeterminationMethod: "lib_cache_match",
						Source:              "libc_symbol_import",
						SourcePath:          "/original/source",
					}},
				}},
			},
		},
	}, "/other/path", roleMain)

	result := aggregate.syscallAnalysis()
	require.NotNil(t, result)
	require.Len(t, result.DetectedSyscalls, 1)
	require.Len(t, result.DetectedSyscalls[0].Occurrences, 1)
	assert.Equal(t, "/original/source", result.DetectedSyscalls[0].Occurrences[0].SourcePath)
}

func TestAnalysisAggregate_AllRolesDistinct(t *testing.T) {
	aggregate := newAnalysisAggregate(true)
	aggregate.addRecord(&fileanalysis.Record{
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []common.SyscallInfo{{
					Number:      1,
					Name:        "write",
					Occurrences: []common.SyscallOccurrence{{Location: 1, DeterminationMethod: "immediate"}},
				}},
			},
		},
	}, "/app/bin/main", roleMain)
	aggregate.addRecord(&fileanalysis.Record{
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []common.SyscallInfo{{
					Number:      2,
					Name:        "read",
					Occurrences: []common.SyscallOccurrence{{Location: 2, DeterminationMethod: "immediate"}},
				}},
			},
		},
	}, "/usr/bin/python3", roleShebangInterpreter)
	aggregate.addDynamicResult(&dynamicanalysis.Result{
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				DetectedSyscalls: []common.SyscallInfo{{
					Number:      3,
					Name:        "open",
					Occurrences: []common.SyscallOccurrence{{Location: 3, DeterminationMethod: "immediate"}},
				}},
			},
		},
	}, "/usr/lib/libfoo.so.1", roleDynLib)

	result := aggregate.syscallAnalysis()
	require.NotNil(t, result)
	var sourcePaths []string
	for _, syscallInfo := range result.DetectedSyscalls {
		for _, occurrence := range syscallInfo.Occurrences {
			sourcePaths = append(sourcePaths, occurrence.SourcePath)
		}
	}
	assert.ElementsMatch(t, []string{"/app/bin/main", "/usr/bin/python3", "/usr/lib/libfoo.so.1"}, sourcePaths)
}
