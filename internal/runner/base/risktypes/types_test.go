package risktypes

import (
	"os"
	"sync"
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

// TestVerifiedFD_ConcurrentClose exercises the thread-safe close contract: with
// many callers racing on Close, the descriptor must be closed exactly once (no
// double-close), and every call must return nil. Run under -race to catch data
// races on the closed flag.
func TestVerifiedFD_ConcurrentClose(t *testing.T) {
	fd, err := syscall.Open(os.DevNull, syscall.O_RDONLY, 0)
	require.NoError(t, err)

	vfd := NewVerifiedFD(fd)

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			errs[i] = vfd.Close()
		}()
	}
	wg.Wait()

	for i, e := range errs {
		assert.NoErrorf(t, e, "concurrent Close call %d must return nil", i)
	}
}
