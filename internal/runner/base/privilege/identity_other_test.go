//go:build !linux && !windows && test

package privilege

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadSavedIDs_ReturnsSentinelErrorOnNonLinux verifies that the non-Linux
// implementation of readSavedIDs returns (0, 0, ErrSavedSetNotSupported). This
// code path is reachable on darwin (the development platform) and must not panic.
// The explicit error return ensures the saved-set invariant check is structurally
// skipped rather than relying on implicit equality of constant zero values.
func TestReadSavedIDs_ReturnsSentinelErrorOnNonLinux(t *testing.T) {
	suid, sgid, err := readSavedIDs()
	require.ErrorIs(t, err, ErrSavedSetNotSupported, "should return ErrSavedSetNotSupported on non-Linux")
	assert.Equal(t, 0, suid, "saved-set-uid is 0 on non-Linux")
	assert.Equal(t, 0, sgid, "saved-set-gid is 0 on non-Linux")
}
