//go:build !linux && !windows && test

package privilege

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadSavedIDs_NoopOnNonLinux verifies that the non-Linux implementation
// of readSavedIDs returns (0, 0) without error. This code path is
// reachable on darwin (the development platform) and must not panic.
func TestReadSavedIDs_NoopOnNonLinux(t *testing.T) {
	suid, sgid, err := readSavedIDs()
	require.NoError(t, err, "should not error on non-Linux platforms")
	assert.Equal(t, 0, suid, "saved-set-uid is 0 on non-Linux")
	assert.Equal(t, 0, sgid, "saved-set-gid is 0 on non-Linux")
}
