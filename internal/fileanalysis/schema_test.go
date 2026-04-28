//go:build test

package fileanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCurrentSchemaVersion verifies the schema version constant equals 19.
func TestCurrentSchemaVersion(t *testing.T) {
	assert.Equal(t, 19, CurrentSchemaVersion)
}
