//go:build test

package fileanalysis

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordJSON_DepsOnlyPathHash(t *testing.T) {
	record := &Record{
		SchemaVersion: CurrentSchemaVersion,
		FilePath:      "/tmp/tool",
		ContentHash:   "sha256:toolhash",
		DynLibDeps: []LibEntry{
			{
				SOName: "libc.so.6",
				Path:   "/lib/x86_64-linux-gnu/libc.so.6",
				Hash:   "sha256:libhash",
			},
		},
	}

	data, err := json.Marshal(record)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	depsRaw, ok := decoded["deps"]
	require.True(t, ok)
	deps, ok := depsRaw.([]any)
	require.True(t, ok)
	require.Len(t, deps, 1)

	entry, ok := deps[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/lib/x86_64-linux-gnu/libc.so.6", entry["path"])
	assert.Equal(t, "sha256:libhash", entry["hash"])
	_, hasSOName := entry["soname"]
	assert.False(t, hasSOName)
}

func TestRecordJSON_ShebangChainOnlyRefPath(t *testing.T) {
	record := &Record{
		SchemaVersion: CurrentSchemaVersion,
		FilePath:      "/tmp/script.sh",
		ContentHash:   "sha256:scripthash",
		ShebangChain: []ShebangChainEntry{
			{Ref: "/usr/bin/env", Path: "/usr/bin/env"},
			{Ref: "python3", Path: "/usr/bin/python3.12"},
		},
		ShebangInterpreter: &ShebangInterpreterInfo{
			RawInterpreterPath: "/usr/bin/env",
			InterpreterPath:    "/usr/bin/env",
			CommandName:        "python3",
			ResolvedPath:       "/usr/bin/python3.12",
		},
	}

	data, err := json.Marshal(record)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(data, &decoded))

	chainRaw, ok := decoded["shebang_chain"]
	require.True(t, ok)
	chain, ok := chainRaw.([]any)
	require.True(t, ok)
	require.Len(t, chain, 2)

	first, ok := chain[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "/usr/bin/env", first["ref"])
	assert.Equal(t, "/usr/bin/env", first["path"])
	_, hasInterpreterField := decoded["shebang_interpreter"]
	assert.False(t, hasInterpreterField)
}
