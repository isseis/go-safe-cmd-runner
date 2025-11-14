package redaction

import (
	"sync"
	"time"
)

// Failure represents a single redaction failure event
type Failure struct {
	Key       string    // The attribute key that failed
	Err       error     // The error that occurred
	Timestamp time.Time // When the failure occurred
}

// InMemoryErrorCollector collects redaction failures in memory
// Safe for concurrent use
type InMemoryErrorCollector struct {
	mu       sync.RWMutex
	failures []Failure
	maxSize  int // Maximum number of failures to store (0 = unlimited)
}

// NewInMemoryErrorCollector creates a new in-memory error collector
// maxSize limits the number of stored failures (0 = unlimited)
func NewInMemoryErrorCollector(maxSize int) *InMemoryErrorCollector {
	return &InMemoryErrorCollector{
		failures: make([]Failure, 0),
		maxSize:  maxSize,
	}
}

// RecordFailure records a redaction failure
func (c *InMemoryErrorCollector) RecordFailure(key string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	failure := Failure{
		Key:       key,
		Err:       err,
		Timestamp: time.Now(),
	}

	// Add failure
	c.failures = append(c.failures, failure)

	// Enforce size limit if set
	if c.maxSize > 0 && len(c.failures) > c.maxSize {
		// Remove oldest failures to maintain size limit
		c.failures = c.failures[len(c.failures)-c.maxSize:]
	}
}

// GetFailures returns a copy of all collected failures
func (c *InMemoryErrorCollector) GetFailures() []Failure {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modification
	failures := make([]Failure, len(c.failures))
	copy(failures, c.failures)
	return failures
}

// Clear removes all collected failures
func (c *InMemoryErrorCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.failures = make([]Failure, 0)
}

// Count returns the number of collected failures
func (c *InMemoryErrorCollector) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.failures)
}

// HasFailures returns true if there are any collected failures
func (c *InMemoryErrorCollector) HasFailures() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.failures) > 0
}
