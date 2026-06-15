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
