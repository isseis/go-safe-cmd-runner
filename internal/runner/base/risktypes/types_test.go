package risktypes

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBinaryAnalysisClass_ZeroValueIsUncertain confirms the fail-closed contract:
// an uninitialized BinaryAnalysisClass must be Uncertain (the blocking verdict),
// never Clean.
func TestBinaryAnalysisClass_ZeroValueIsUncertain(t *testing.T) {
	var zero BinaryAnalysisClass
	assert.Equal(t, BinaryAnalysisUncertain, zero, "zero value must be Uncertain (fail-closed)")
	assert.NotEqual(t, BinaryAnalysisClean, zero, "zero value must not be Clean")
}

// TestBinaryAnalysisResult_ZeroValueIsUncertain confirms that a zero-valued
// result carries the Uncertain class.
func TestBinaryAnalysisResult_ZeroValueIsUncertain(t *testing.T) {
	var r BinaryAnalysisResult
	assert.Equal(t, BinaryAnalysisUncertain, r.Class)
}

func TestVerifiedFD_FdAndIdempotentClose(t *testing.T) {
	fd, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
	require.NoError(t, err)

	vfd := NewVerifiedFD(fd)
	assert.Equal(t, fd, vfd.Fd())

	assert.NoError(t, vfd.Close(), "first close should succeed")
	assert.NoError(t, vfd.Close(), "second close should be a no-op (idempotent)")
}

func TestVerifiedFD_NilReceiverClose(t *testing.T) {
	var vfd *VerifiedFD
	assert.NoError(t, vfd.Close(), "Close on a nil receiver must return nil")
}
