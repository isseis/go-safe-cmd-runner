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
			Number:              41,
			Name:                "socket",
			IsNetwork:           true,
			Location:            0x401000,
			DeterminationMethod: "immediate",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		assert.Equal(t, float64(41), m["number"])
		assert.Equal(t, "socket", m["name"])
		assert.Equal(t, true, m["is_network"])
		assert.Equal(t, float64(0x401000), m["location"])
		assert.Equal(t, "immediate", m["determination_method"])
	})

	t.Run("name omitted when empty", func(t *testing.T) {
		info := common.SyscallInfo{
			Number:              -1,
			IsNetwork:           false,
			Location:            0x401010,
			DeterminationMethod: "unknown:scan_limit_exceeded",
		}
		data, err := json.Marshal(info)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		_, hasName := m["name"]
		assert.False(t, hasName, "name field should be omitted when empty")
	})
}

// TestSyscallSummary_JSONRoundTrip verifies JSON marshal/unmarshal of SyscallSummary.
func TestSyscallSummary_JSONRoundTrip(t *testing.T) {
	original := common.SyscallSummary{
		HasNetworkSyscalls:  true,
		IsHighRisk:          false,
		TotalDetectedEvents: 5,
		NetworkSyscallCount: 2,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded common.SyscallSummary
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
}

// TestSyscallAnalysisResultCore_JSONRoundTrip verifies JSON marshal/unmarshal of
// SyscallAnalysisResultCore, including omitempty behavior on HighRiskReasons.
func TestSyscallAnalysisResultCore_JSONRoundTrip(t *testing.T) {
	t.Run("with high risk reasons", func(t *testing.T) {
		original := common.SyscallAnalysisResultCore{
			Architecture: "x86_64",
			DetectedSyscalls: []common.SyscallInfo{
				{
					Number:              41,
					Name:                "socket",
					IsNetwork:           true,
					Location:            0x401000,
					DeterminationMethod: "immediate",
				},
			},
			HasUnknownSyscalls: false,
			HighRiskReasons:    []string{"unknown:indirect_setting"},
			Summary: common.SyscallSummary{
				HasNetworkSyscalls:  true,
				IsHighRisk:          true,
				TotalDetectedEvents: 1,
				NetworkSyscallCount: 1,
			},
		}

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded common.SyscallAnalysisResultCore
		require.NoError(t, json.Unmarshal(data, &decoded))

		assert.Equal(t, original, decoded)
	})

	t.Run("high_risk_reasons omitted when nil", func(t *testing.T) {
		core := common.SyscallAnalysisResultCore{
			Architecture:       "x86_64",
			DetectedSyscalls:   nil,
			HasUnknownSyscalls: false,
			HighRiskReasons:    nil,
			Summary:            common.SyscallSummary{},
		}

		data, err := json.Marshal(core)
		require.NoError(t, err)

		var m map[string]any
		require.NoError(t, json.Unmarshal(data, &m))

		_, hasHighRisk := m["high_risk_reasons"]
		assert.False(t, hasHighRisk, "high_risk_reasons should be omitted when nil")
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
