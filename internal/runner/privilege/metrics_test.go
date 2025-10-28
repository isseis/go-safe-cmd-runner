package privilege_test

import (
	"errors"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/stretchr/testify/assert"
)

func TestMetrics_RecordElevationSuccess(t *testing.T) {
	metrics := &privilege.Metrics{}

	// Record first success
	duration1 := 10 * time.Millisecond
	metrics.RecordElevationSuccess(duration1)

	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.ElevationAttempts)
	assert.Equal(t, int64(1), snapshot.ElevationSuccesses)
	assert.Equal(t, int64(0), snapshot.ElevationFailures)
	assert.Equal(t, duration1, snapshot.TotalElevationTime)
	assert.Equal(t, duration1, snapshot.AverageElevationTime)
	assert.Equal(t, duration1, snapshot.MaxElevationTime)
	assert.Equal(t, 1.0, snapshot.SuccessRate)
	assert.NotZero(t, snapshot.LastElevationTime)

	// Record second success with longer duration
	duration2 := 20 * time.Millisecond
	metrics.RecordElevationSuccess(duration2)

	snapshot = metrics.GetSnapshot()
	assert.Equal(t, int64(2), snapshot.ElevationAttempts)
	assert.Equal(t, int64(2), snapshot.ElevationSuccesses)
	assert.Equal(t, int64(0), snapshot.ElevationFailures)
	assert.Equal(t, duration1+duration2, snapshot.TotalElevationTime)
	assert.Equal(t, (duration1+duration2)/2, snapshot.AverageElevationTime)
	assert.Equal(t, duration2, snapshot.MaxElevationTime) // Longer duration becomes max
	assert.Equal(t, 1.0, snapshot.SuccessRate)
}

// Test error definitions
var (
	ErrTestPrivilegeElevationFailure = errors.New("test privilege elevation failure")
	ErrTestFailure                   = errors.New("failure")
	ErrTestError                     = errors.New("test error")
)

func TestMetrics_RecordElevationFailure(t *testing.T) {
	metrics := &privilege.Metrics{}

	// Record a failure
	metrics.RecordElevationFailure(ErrTestPrivilegeElevationFailure)

	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(1), snapshot.ElevationAttempts)
	assert.Equal(t, int64(0), snapshot.ElevationSuccesses)
	assert.Equal(t, int64(1), snapshot.ElevationFailures)
	assert.Equal(t, time.Duration(0), snapshot.TotalElevationTime)
	assert.Equal(t, time.Duration(0), snapshot.AverageElevationTime)
	assert.Equal(t, time.Duration(0), snapshot.MaxElevationTime)
	assert.Equal(t, 0.0, snapshot.SuccessRate)
	assert.Equal(t, ErrTestPrivilegeElevationFailure.Error(), snapshot.LastError)
}

func TestMetrics_MixedOperations(t *testing.T) {
	metrics := &privilege.Metrics{}

	// Record success
	metrics.RecordElevationSuccess(15 * time.Millisecond)

	// Record failure
	metrics.RecordElevationFailure(ErrTestFailure)

	// Record another success
	metrics.RecordElevationSuccess(25 * time.Millisecond)

	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(3), snapshot.ElevationAttempts)
	assert.Equal(t, int64(2), snapshot.ElevationSuccesses)
	assert.Equal(t, int64(1), snapshot.ElevationFailures)
	assert.Equal(t, 40*time.Millisecond, snapshot.TotalElevationTime)
	assert.Equal(t, 20*time.Millisecond, snapshot.AverageElevationTime)
	assert.Equal(t, 25*time.Millisecond, snapshot.MaxElevationTime)
	assert.InDelta(t, 0.667, snapshot.SuccessRate, 0.001) // 2/3 ≈ 0.667
	assert.Equal(t, ErrTestFailure.Error(), snapshot.LastError)
}

func TestMetrics_Reset(t *testing.T) {
	metrics := &privilege.Metrics{}

	// Add some data
	metrics.RecordElevationSuccess(10 * time.Millisecond)
	metrics.RecordElevationFailure(ErrTestError)

	// Verify data exists
	snapshot := metrics.GetSnapshot()
	assert.NotZero(t, snapshot.ElevationAttempts)

	// Reset and verify everything is cleared
	metrics.Reset()
	snapshot = metrics.GetSnapshot()

	assert.Equal(t, int64(0), snapshot.ElevationAttempts)
	assert.Equal(t, int64(0), snapshot.ElevationSuccesses)
	assert.Equal(t, int64(0), snapshot.ElevationFailures)
	assert.Equal(t, time.Duration(0), snapshot.TotalElevationTime)
	assert.Equal(t, time.Duration(0), snapshot.AverageElevationTime)
	assert.Equal(t, time.Duration(0), snapshot.MaxElevationTime)
	assert.Equal(t, 0.0, snapshot.SuccessRate)
	assert.Empty(t, snapshot.LastError)
	assert.True(t, snapshot.LastElevationTime.IsZero())
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	metrics := &privilege.Metrics{}

	// Test that GetSnapshot doesn't race with updates
	// This is a basic test - in production, more extensive race testing would be done
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			metrics.RecordElevationSuccess(time.Millisecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			snapshot := metrics.GetSnapshot()
			// Just verify we can read without panicking
			_ = snapshot.ElevationAttempts
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify final state
	snapshot := metrics.GetSnapshot()
	assert.Equal(t, int64(100), snapshot.ElevationAttempts)
	assert.Equal(t, int64(100), snapshot.ElevationSuccesses)
	assert.Equal(t, 1.0, snapshot.SuccessRate)
}

// TestUpdateSuccessRate_AllCases tests success rate calculation in various scenarios
func TestUpdateSuccessRate_AllCases(t *testing.T) {
	t.Run("initial_state_zero_attempts", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 0.0, snapshot.SuccessRate, "Success rate should be 0.0 when no attempts")
		assert.Equal(t, int64(0), snapshot.ElevationAttempts)
	})

	t.Run("first_success", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationSuccess(10 * time.Millisecond)
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 1.0, snapshot.SuccessRate, "Success rate should be 1.0 after first success")
	})

	t.Run("first_failure", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationFailure(errors.New("test error"))
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 0.0, snapshot.SuccessRate, "Success rate should be 0.0 after first failure")
	})

	t.Run("mixed_50_percent", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationSuccess(10 * time.Millisecond)
		metrics.RecordElevationFailure(errors.New("test error"))
		snapshot := metrics.GetSnapshot()
		assert.InDelta(t, 0.5, snapshot.SuccessRate, 0.001, "Success rate should be 0.5 with 1 success and 1 failure")
	})

	t.Run("all_successes", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		for i := 0; i < 10; i++ {
			metrics.RecordElevationSuccess(time.Millisecond)
		}
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 1.0, snapshot.SuccessRate, "Success rate should be 1.0 with all successes")
		assert.Equal(t, int64(10), snapshot.ElevationSuccesses)
	})

	t.Run("all_failures", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		for i := 0; i < 10; i++ {
			metrics.RecordElevationFailure(errors.New("test error"))
		}
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 0.0, snapshot.SuccessRate, "Success rate should be 0.0 with all failures")
		assert.Equal(t, int64(10), snapshot.ElevationFailures)
	})

	t.Run("after_reset", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationSuccess(10 * time.Millisecond)
		metrics.Reset()
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 0.0, snapshot.SuccessRate, "Success rate should be 0.0 after reset")
		assert.Equal(t, int64(0), snapshot.ElevationAttempts)
	})

	t.Run("boundary_100_percent", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationSuccess(10 * time.Millisecond)
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 1.0, snapshot.SuccessRate, "Success rate should be exactly 1.0")
	})

	t.Run("boundary_0_percent", func(t *testing.T) {
		metrics := &privilege.Metrics{}
		metrics.RecordElevationFailure(errors.New("test"))
		snapshot := metrics.GetSnapshot()
		assert.Equal(t, 0.0, snapshot.SuccessRate, "Success rate should be exactly 0.0")
	})
}
