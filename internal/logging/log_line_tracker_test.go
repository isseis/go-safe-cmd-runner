package logging

import (
	"sync"
	"testing"
)

func TestNewDefaultLogLineTracker(t *testing.T) {
	tracker := NewDefaultLogLineTracker()
	if tracker == nil {
		t.Error("NewDefaultLogLineTracker should return a non-nil instance")
	}

	// Initial line number should be 0
	if line := tracker.GetCurrentLine(); line != 0 {
		t.Errorf("Initial line number should be 0, got %d", line)
	}
}

func TestDefaultLogLineTracker_GetCurrentLine(t *testing.T) {
	tracker := NewDefaultLogLineTracker()

	// Initial value should be 0
	if line := tracker.GetCurrentLine(); line != 0 {
		t.Errorf("GetCurrentLine() = %d, expected 0", line)
	}

	// After increment, should return the incremented value
	tracker.IncrementLine()
	if line := tracker.GetCurrentLine(); line != 1 {
		t.Errorf("GetCurrentLine() = %d, expected 1", line)
	}
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
			if result != tt.expected {
				t.Errorf("IncrementLine() = %d, expected %d", result, tt.expected)
			}
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
	if line := tracker.GetCurrentLine(); line == 0 {
		t.Error("Line counter should not be zero before reset")
	}

	// Reset and verify it's zero
	tracker.Reset()
	if line := tracker.GetCurrentLine(); line != 0 {
		t.Errorf("After Reset(), GetCurrentLine() = %d, expected 0", line)
	}

	// Verify increment works after reset
	result := tracker.IncrementLine()
	if result != 1 {
		t.Errorf("After reset, IncrementLine() = %d, expected 1", result)
	}
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

	if result != expected {
		t.Errorf("After concurrent increments, GetCurrentLine() = %d, expected %d", result, expected)
	}
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
			if line < 0 {
				t.Errorf("GetCurrentLine() returned negative value: %d", line)
				return
			}
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
	if finalLine < 0 {
		t.Errorf("Final line count is negative: %d", finalLine)
	}
}

func TestLogLineTracker_Interface(t *testing.T) {
	// Test that DefaultLogLineTracker implements LogLineTracker interface
	var tracker LogLineTracker = NewDefaultLogLineTracker()

	// Test interface methods
	initialLine := tracker.GetCurrentLine()
	if initialLine != 0 {
		t.Errorf("Interface GetCurrentLine() = %d, expected 0", initialLine)
	}

	incrementedLine := tracker.IncrementLine()
	if incrementedLine != 1 {
		t.Errorf("Interface IncrementLine() = %d, expected 1", incrementedLine)
	}

	currentLine := tracker.GetCurrentLine()
	if currentLine != 1 {
		t.Errorf("Interface GetCurrentLine() after increment = %d, expected 1", currentLine)
	}

	tracker.Reset()
	resetLine := tracker.GetCurrentLine()
	if resetLine != 0 {
		t.Errorf("Interface GetCurrentLine() after reset = %d, expected 0", resetLine)
	}
}
