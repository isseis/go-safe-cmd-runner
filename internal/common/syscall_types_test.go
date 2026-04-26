package common_test

import (
	"encoding/json"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyscallInfo_JSONTags verifies that SyscallInfo fields serialize with
// correct JSON keys and that omitempty is respected on the Name field.
func TestSyscallInfo_JSONTags(t *testing.T) {
	t.Run("with name", func(t *testing.T) {
		info := common.SyscallInfo{
			Number:    41,
			Name:      "socket",
			IsNetwork: true,
			Occurrences: []common.SyscallOccurrence{
				{Location: 0x401000, DeterminationMethod: "immediate"},
			},
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		assert.Equal(t, float64(41), m["number"])
		assert.Equal(t, "socket", m["name"])
		assert.Equal(t, true, m["is_network"])

		occs, ok := m["occurrences"].([]any)
		require.True(t, ok)
		require.Len(t, occs, 1)
		occ := occs[0].(map[string]any)
		assert.Equal(t, float64(0x401000), occ["location"])
		assert.Equal(t, "immediate", occ["determination_method"])
	})

	t.Run("name omitted when empty", func(t *testing.T) {
		info := common.SyscallInfo{
			Number:    -1,
			IsNetwork: false,
			Occurrences: []common.SyscallOccurrence{
				{Location: 0x401010, DeterminationMethod: "unknown:scan_limit_exceeded"},
			},
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		_, hasName := m["name"]
		assert.False(t, hasName, "name field should be omitted when empty")
	})

	t.Run("source field omitempty when empty", func(t *testing.T) {
		info := common.SyscallInfo{
			Number: 41,
			Occurrences: []common.SyscallOccurrence{
				{Location: 0x401000, DeterminationMethod: "immediate"},
			},
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		occs, ok := m["occurrences"].([]any)
		require.True(t, ok)
		require.Len(t, occs, 1)
		occ := occs[0].(map[string]any)
		_, hasSource := occ["source"]
		assert.False(t, hasSource, "source field should be omitted when empty")
	})

	t.Run("source field present when set", func(t *testing.T) {
		info := common.SyscallInfo{
			Number: 83,
			Occurrences: []common.SyscallOccurrence{
				{Location: 0, DeterminationMethod: "immediate", Source: "libc_symbol_import"},
			},
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		occs, ok := m["occurrences"].([]any)
		require.True(t, ok)
		require.Len(t, occs, 1)
		occ := occs[0].(map[string]any)
		assert.Equal(t, "libc_symbol_import", occ["source"])
	})
}

// TestSyscallAnalysisResultCore_JSONRoundTrip verifies JSON marshal/unmarshal of
// SyscallAnalysisResultCore, including omitempty behavior on AnalysisWarnings.
func TestSyscallAnalysisResultCore_JSONRoundTrip(t *testing.T) {
	t.Run("with analysis warnings", func(t *testing.T) {
		original := common.SyscallAnalysisResultCore{
			Architecture: "x86_64",
			DetectedSyscalls: []common.SyscallInfo{
				{
					Number:    41,
					Name:      "socket",
					IsNetwork: true,
					Occurrences: []common.SyscallOccurrence{
						{Location: 0x401000, DeterminationMethod: "immediate"},
					},
				},
			},
			AnalysisWarnings: []string{"unknown:indirect_setting"},
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded common.SyscallAnalysisResultCore
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, original, decoded)
	})

	t.Run("analysis_warnings omitted when nil", func(t *testing.T) {
		core := common.SyscallAnalysisResultCore{
			Architecture:     "x86_64",
			DetectedSyscalls: nil,
			AnalysisWarnings: nil,
		}

		data, err := json.Marshal(core)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		_, hasWarnings := m["analysis_warnings"]
		assert.False(t, hasWarnings, "analysis_warnings should be omitted when nil")
	})

	t.Run("empty detected_syscalls round-trips as empty slice", func(t *testing.T) {
		original := common.SyscallAnalysisResultCore{
			Architecture:     "x86_64",
			DetectedSyscalls: []common.SyscallInfo{},
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded common.SyscallAnalysisResultCore
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, original.Architecture, decoded.Architecture)
		assert.NotNil(t, decoded.DetectedSyscalls)
		assert.Len(t, decoded.DetectedSyscalls, 0)
	})
}
