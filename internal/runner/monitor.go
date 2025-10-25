// Package runner provides command execution functionality with timeout control and monitoring
package runner

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultCheckInterval is the default interval for checking long-running processes
	DefaultCheckInterval = 5 * time.Minute
	// DefaultWarnThreshold is the default threshold for warning about long-running processes
	DefaultWarnThreshold = 5 * time.Minute
)

// ProcessInfo holds information about a running process
type ProcessInfo struct {
	CommandName string
	StartTime   time.Time
	PID         int
}

// ProcessMonitor tracks and monitors running processes
type ProcessMonitor struct {
	processes      map[int]*ProcessInfo
	mutex          sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	checkInterval  time.Duration
	warnThreshold  time.Duration
	monitorStarted bool
}

// NewProcessMonitor creates a new process monitor
func NewProcessMonitor() *ProcessMonitor {
	return &ProcessMonitor{
		processes:     make(map[int]*ProcessInfo),
		checkInterval: DefaultCheckInterval,
		warnThreshold: DefaultWarnThreshold,
	}
}

// Register adds a process to the monitor
func (m *ProcessMonitor) Register(pid int, commandName string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.processes[pid] = &ProcessInfo{
		CommandName: commandName,
		StartTime:   time.Now(),
		PID:         pid,
	}
}

// Unregister removes a process from the monitor
func (m *ProcessMonitor) Unregister(pid int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.processes, pid)
}

// GetRunningProcesses returns a copy of currently monitored processes
func (m *ProcessMonitor) GetRunningProcesses() []ProcessInfo {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	processes := make([]ProcessInfo, 0, len(m.processes))
	for _, info := range m.processes {
		processes = append(processes, *info)
	}
	return processes
}

// CheckLongRunningProcesses checks for processes running longer than the threshold
// and logs warnings. This should be called periodically by a monitoring goroutine.
func (m *ProcessMonitor) CheckLongRunningProcesses(threshold time.Duration) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	now := time.Now()
	for _, info := range m.processes {
		duration := now.Sub(info.StartTime)
		if duration > threshold {
			slog.Warn("Long-running process detected",
				"command", info.CommandName,
				"pid", info.PID,
				"duration_minutes", int(duration.Minutes()))
		}
	}
}

// Start begins monitoring registered processes in a background goroutine.
// It periodically checks for long-running processes and logs warnings.
// The monitoring interval and warning threshold can be customized using SetCheckInterval
// and SetWarnThreshold before calling Start.
func (m *ProcessMonitor) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.monitorStarted {
		return nil // Already started
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.monitorStarted = true

	go m.monitorLoop()

	return nil
}

// Stop gracefully shuts down the monitoring goroutine
func (m *ProcessMonitor) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.monitorStarted {
		return // Not started
	}

	if m.cancel != nil {
		m.cancel()
	}
	m.monitorStarted = false
}

// monitorLoop is the main monitoring loop that runs in a background goroutine
func (m *ProcessMonitor) monitorLoop() {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CheckLongRunningProcesses(m.warnThreshold)
		case <-m.ctx.Done():
			return
		}
	}
}

// SetCheckInterval sets the interval at which the monitor checks for long-running processes.
// This must be called before Start() to take effect.
func (m *ProcessMonitor) SetCheckInterval(interval time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.checkInterval = interval
}

// SetWarnThreshold sets the duration threshold for warning about long-running processes.
// This must be called before Start() to take effect.
func (m *ProcessMonitor) SetWarnThreshold(threshold time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.warnThreshold = threshold
}

// IsRunning returns whether the monitor is currently running
func (m *ProcessMonitor) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.monitorStarted
}
