// Package privilege provides metrics collection for privilege operations.
package privilege

import (
	"sync"
	"time"
)

// Metrics contains operational metrics for privilege management
type Metrics struct {
	mu                   sync.RWMutex
	ElevationAttempts    int64         `json:"elevation_attempts"`
	ElevationSuccesses   int64         `json:"elevation_successes"`
	ElevationFailures    int64         `json:"elevation_failures"`
	TotalElevationTime   time.Duration `json:"total_elevation_time"`
	AverageElevationTime time.Duration `json:"average_elevation_time"`
	MaxElevationTime     time.Duration `json:"max_elevation_time"`
	LastElevationTime    time.Time     `json:"last_elevation_time"`
	LastError            string        `json:"last_error,omitempty"`
	SuccessRate          float64       `json:"success_rate"`
}

// RecordElevationSuccess records a successful privilege elevation
func (m *Metrics) RecordElevationSuccess(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ElevationAttempts++
	m.ElevationSuccesses++
	m.TotalElevationTime += duration

	if m.ElevationSuccesses > 0 {
		m.AverageElevationTime = m.TotalElevationTime / time.Duration(m.ElevationSuccesses)
	}

	if duration > m.MaxElevationTime {
		m.MaxElevationTime = duration
	}

	m.LastElevationTime = time.Now()
	m.updateSuccessRate()
}

// RecordElevationFailure records a failed privilege elevation
func (m *Metrics) RecordElevationFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ElevationAttempts++
	m.ElevationFailures++
	m.LastError = err.Error()
	m.updateSuccessRate()
}

// updateSuccessRate calculates the current success rate (should be called with lock held)
func (m *Metrics) updateSuccessRate() {
	if m.ElevationAttempts > 0 {
		m.SuccessRate = float64(m.ElevationSuccesses) / float64(m.ElevationAttempts)
	} else {
		m.SuccessRate = 0.0
	}
}

// GetSnapshot returns a thread-safe copy of the current metrics
func (m *Metrics) GetSnapshot() Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	return Metrics{
		ElevationAttempts:    m.ElevationAttempts,
		ElevationSuccesses:   m.ElevationSuccesses,
		ElevationFailures:    m.ElevationFailures,
		TotalElevationTime:   m.TotalElevationTime,
		AverageElevationTime: m.AverageElevationTime,
		MaxElevationTime:     m.MaxElevationTime,
		LastElevationTime:    m.LastElevationTime,
		LastError:            m.LastError,
		SuccessRate:          m.SuccessRate,
	}
}

// Reset clears all metrics (primarily for testing)
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ElevationAttempts = 0
	m.ElevationSuccesses = 0
	m.ElevationFailures = 0
	m.TotalElevationTime = 0
	m.AverageElevationTime = 0
	m.MaxElevationTime = 0
	m.LastElevationTime = time.Time{}
	m.LastError = ""
	m.SuccessRate = 0.0
}
