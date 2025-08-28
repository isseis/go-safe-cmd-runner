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
	lineCounter int64
}

// NewDefaultLogLineTracker creates a new DefaultLogLineTracker.
func NewDefaultLogLineTracker() *DefaultLogLineTracker {
	return &DefaultLogLineTracker{}
}

// GetCurrentLine returns the current estimated log line number.
func (t *DefaultLogLineTracker) GetCurrentLine() int {
	return int(atomic.LoadInt64(&t.lineCounter))
}

// IncrementLine increments the line counter and returns the new line number.
func (t *DefaultLogLineTracker) IncrementLine() int {
	return int(atomic.AddInt64(&t.lineCounter, 1))
}

// Reset resets the line counter to zero.
func (t *DefaultLogLineTracker) Reset() {
	atomic.StoreInt64(&t.lineCounter, 0)
}
