//go:build test

package fileanalysis

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCurrentSchemaVersion verifies the schema version constant equals 18 (AC-4).
func TestCurrentSchemaVersion(t *testing.T) {
	assert.Equal(t, 18, CurrentSchemaVersion)
}

// TestDetectedSymbolEntry_JSONDoesNotContainCategory verifies that the JSON output
// of DetectedSymbolEntry does not include a "category" field (AC-2: field removed from schema).
func TestDetectedSymbolEntry_JSONDoesNotContainCategory(t *testing.T) {
	entry := DetectedSymbolEntry{
		Name: "socket",
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))

	_, hasCategory := m["category"]
	assert.False(t, hasCategory, "category field must not appear in DetectedSymbolEntry JSON (schema v18)")
}
