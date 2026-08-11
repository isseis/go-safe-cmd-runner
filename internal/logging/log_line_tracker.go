package logging

import (
	"sync/atomic"
)

// LogLineTracker tracks log line numbers to provide file hints for error messages.
type LogLineTracker interface {
	// GetCurrentLine returns the estimated current log line number
	GetCurrentLine() int

	// IncrementLine increments the line counter and returns the new line number
	IncrementLine() int

	// Reset resets the line counter to zero
	Reset()
}

// DefaultLogLineTracker provides a thread-safe implementation of LogLineTracker
// using atomic operations for concurrent access.
type DefaultLogLineTracker struct {
	lineCounter atomic.Int64
}

// NewDefaultLogLineTracker creates a new DefaultLogLineTracker.
func NewDefaultLogLineTracker() *DefaultLogLineTracker {
	return &DefaultLogLineTracker{}
}

// GetCurrentLine returns the current estimated log line number.
func (t *DefaultLogLineTracker) GetCurrentLine() int {
	return int(t.lineCounter.Load())
}

// IncrementLine increments the line counter and returns the new line number.
func (t *DefaultLogLineTracker) IncrementLine() int {
	return int(t.lineCounter.Add(1))
}

// Reset resets the line counter to zero.
func (t *DefaultLogLineTracker) Reset() {
	t.lineCounter.Store(0)
}
