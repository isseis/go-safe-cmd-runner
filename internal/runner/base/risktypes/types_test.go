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

// TestVerifiedFD_FdAndIdempotentClose confirms that Fd returns the wrapped
// descriptor and that a second Close runs no syscall. The two subtests detect an
// unguarded second close independently: the first through the EBADF that close(2)
// returns for an already-closed descriptor, the second by handing the freed
// descriptor number to a fresh open and observing that the second Close leaves it
// open -- were the guard absent, that call would close a descriptor this
// VerifiedFD does not own.
func TestVerifiedFD_FdAndIdempotentClose(t *testing.T) {
	// newClosableFD opens /dev/null and returns it wrapped. Cleanup goes through
	// VerifiedFD.Close so a failed assertion cannot leak the descriptor, and so
	// cleanup itself never performs a double close.
	newClosableFD := func(t *testing.T) (int, *VerifiedFD) {
		t.Helper()
		fd, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
		require.NoError(t, err)
		vfd := NewVerifiedFD(fd)
		t.Cleanup(func() { _ = vfd.Close() })
		return fd, vfd
	}

	t.Run("second close returns nil and runs no syscall", func(t *testing.T) {
		fd, vfd := newClosableFD(t)
		assert.Equal(t, fd, vfd.Fd())

		assert.NoError(t, vfd.Close(), "first close should succeed")
		require.False(t, fdIsOpen(fd), "first close must actually close the descriptor")

		// An unguarded second close would reach close(2) and return EBADF.
		assert.NoError(t, vfd.Close(), "second close should be a no-op (idempotent)")
	})

	t.Run("second close does not close a descriptor reusing the number", func(t *testing.T) {
		fd, vfd := newClosableFD(t)
		require.NoError(t, vfd.Close())
		require.False(t, fdIsOpen(fd))

		newFD, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
		require.NoError(t, err)
		t.Cleanup(func() { _ = syscall.Close(newFD) })
		require.Equal(t, fd, newFD, "this test requires the kernel to reuse the freed fd number")

		assert.NoError(t, vfd.Close(), "second close should be a no-op (idempotent)")
		assert.True(t, fdIsOpen(newFD), "second close must not run syscall.Close on the reused descriptor")
	})
}

func TestVerifiedFD_NilReceiverClose(t *testing.T) {
	var vfd *VerifiedFD
	assert.NoError(t, vfd.Close(), "Close on a nil receiver must return nil")
}

// fdIsOpen reports whether fd refers to an open descriptor.
func fdIsOpen(fd int) bool {
	var stat syscall.Stat_t
	return syscall.Fstat(fd, &stat) == nil
}

// TestVerifiedCommandPlan_Close confirms Close releases the command identity's
// descriptor and every chained artifact's descriptor, is idempotent, and is safe
// on a zero plan.
func TestVerifiedCommandPlan_Close(t *testing.T) {
	openFD := func(t *testing.T) int {
		t.Helper()
		fd, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
		require.NoError(t, err)
		return fd
	}

	t.Run("closes identity and artifact descriptors", func(t *testing.T) {
		idFD := openFD(t)
		artFD := openFD(t)
		plan := &VerifiedCommandPlan{
			Identity: &VerifiedIdentity{FD: NewVerifiedFD(idFD)},
			Artifacts: []ExecutedArtifact{
				{Identity: &VerifiedIdentity{FD: NewVerifiedFD(artFD)}},
				{Identity: nil}, // an unbound (rejected) artifact must not panic
			},
		}

		require.True(t, fdIsOpen(idFD))
		require.True(t, fdIsOpen(artFD))

		assert.NoError(t, plan.Close())
		assert.False(t, fdIsOpen(idFD), "identity descriptor should be closed")
		assert.False(t, fdIsOpen(artFD), "artifact descriptor should be closed")

		assert.NoError(t, plan.Close(), "second Close must be a no-op")
	})

	t.Run("zero plan", func(t *testing.T) {
		var plan VerifiedCommandPlan
		assert.NoError(t, plan.Close())
	})

	t.Run("identity without descriptor", func(t *testing.T) {
		plan := &VerifiedCommandPlan{Identity: &VerifiedIdentity{ResolvedPath: "/bin/echo"}}
		assert.NoError(t, plan.Close())
	})
}
