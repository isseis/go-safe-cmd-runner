package logging

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDefaultLogLineTracker(t *testing.T) {
	tracker := NewDefaultLogLineTracker()
	assert.NotNil(t, tracker, "NewDefaultLogLineTracker should return a non-nil instance")

	// Initial line number should be 0
	assert.Equal(t, 0, tracker.GetCurrentLine(), "Initial line number should be 0")
}

func TestDefaultLogLineTracker_GetCurrentLine(t *testing.T) {
	tracker := NewDefaultLogLineTracker()

	// Initial value should be 0
	assert.Equal(t, 0, tracker.GetCurrentLine(), "Initial GetCurrentLine() should be 0")

	// After increment, should return the incremented value
	tracker.IncrementLine()
	assert.Equal(t, 1, tracker.GetCurrentLine(), "GetCurrentLine() after one increment should be 1")
}

func TestDefaultLogLineTracker_IncrementLine(t *testing.T) {
	tracker := NewDefaultLogLineTracker()

	tests := []struct {
		name     string
		expected int
	}{
		{"first increment", 1},
		{"second increment", 2},
		{"third increment", 3},
		{"fourth increment", 4},
		{"fifth increment", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tracker.IncrementLine()
			assert.Equal(t, tt.expected, result, "IncrementLine() result mismatch")
		})
	}
}

func TestDefaultLogLineTracker_Reset(t *testing.T) {
	tracker := NewDefaultLogLineTracker()

	// Increment a few times
	tracker.IncrementLine()
	tracker.IncrementLine()
	tracker.IncrementLine()

	// Verify it's not zero
	assert.NotEqual(t, 0, tracker.GetCurrentLine(), "Line counter should not be zero before reset")

	// Reset and verify it's zero
	tracker.Reset()
	assert.Equal(t, 0, tracker.GetCurrentLine(), "After Reset(), GetCurrentLine() should be 0")

	// Verify increment works after reset
	result := tracker.IncrementLine()
	assert.Equal(t, 1, result, "After reset, IncrementLine() should return 1")
}

func TestDefaultLogLineTracker_ThreadSafety(t *testing.T) {
	tracker := NewDefaultLogLineTracker()
	const numGoroutines = 100
	const incrementsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Start multiple goroutines that increment the counter
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				tracker.IncrementLine()
			}
		}()
	}

	wg.Wait()

	// Expected total increments
	expected := numGoroutines * incrementsPerGoroutine
	result := tracker.GetCurrentLine()

	assert.Equal(t, expected, result, "After concurrent increments, GetCurrentLine() mismatch")
}

func TestDefaultLogLineTracker_ConcurrentReadWrite(t *testing.T) {
	tracker := NewDefaultLogLineTracker()
	const numOperations = 1000

	var wg sync.WaitGroup

	// Start a goroutine that continuously increments
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			tracker.IncrementLine()
		}
	}()

	// Start a goroutine that continuously reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < numOperations; i++ {
			line := tracker.GetCurrentLine()
			// Line should never be negative
			assert.GreaterOrEqual(t, line, 0, "GetCurrentLine() returned negative value")
		}
	}()

	// Start a goroutine that resets periodically
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			tracker.Reset()
		}
	}()

	wg.Wait()

	// Final line count should be non-negative
	finalLine := tracker.GetCurrentLine()
	assert.GreaterOrEqual(t, finalLine, 0, "Final line count should be non-negative")
}

func TestLogLineTracker_Interface(t *testing.T) {
	// Test that DefaultLogLineTracker implements LogLineTracker interface
	var tracker LogLineTracker = NewDefaultLogLineTracker()

	// Test interface methods
	initialLine := tracker.GetCurrentLine()
	assert.Equal(t, 0, initialLine, "Interface GetCurrentLine() initial value mismatch")

	incrementedLine := tracker.IncrementLine()
	assert.Equal(t, 1, incrementedLine, "Interface IncrementLine() result mismatch")

	currentLine := tracker.GetCurrentLine()
	assert.Equal(t, 1, currentLine, "Interface GetCurrentLine() after increment mismatch")

	tracker.Reset()
	resetLine := tracker.GetCurrentLine()
	assert.Equal(t, 0, resetLine, "Interface GetCurrentLine() after reset mismatch")
}
