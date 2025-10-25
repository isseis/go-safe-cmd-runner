// Package runner provides command execution functionality with timeout control and monitoring
package runner

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// ProcessInfo holds information about a running process
type ProcessInfo struct {
	CommandName string
	StartTime   time.Time
	PID         int
}

// UnlimitedExecutionMonitor tracks commands running without timeout
type UnlimitedExecutionMonitor struct {
	processes map[int]*ProcessInfo
	mutex     sync.RWMutex
}

// NewUnlimitedExecutionMonitor creates a new monitor for unlimited execution commands
func NewUnlimitedExecutionMonitor() *UnlimitedExecutionMonitor {
	return &UnlimitedExecutionMonitor{
		processes: make(map[int]*ProcessInfo),
	}
}

// Register adds a process to the monitor
func (m *UnlimitedExecutionMonitor) Register(pid int, commandName string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.processes[pid] = &ProcessInfo{
		CommandName: commandName,
		StartTime:   time.Now(),
		PID:         pid,
	}
}

// Unregister removes a process from the monitor
func (m *UnlimitedExecutionMonitor) Unregister(pid int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	delete(m.processes, pid)
}

// MonitorUnlimitedExecution starts monitoring a command running without timeout.
// Returns a cancel function that should be called when the command finishes.
// Logs warnings periodically for long-running processes.
func MonitorUnlimitedExecution(ctx context.Context, pid int, cmdName string) context.CancelFunc {
	monitorCtx, cancel := context.WithCancel(ctx)
	startTime := time.Now()

	const warningInterval = 5 * time.Minute

	go func() {
		// Log warnings every 5 minutes for long-running processes
		ticker := time.NewTicker(warningInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				duration := time.Since(startTime)
				slog.Warn("Command running for extended period with unlimited timeout",
					"command", cmdName,
					"pid", pid,
					"duration_minutes", int(duration.Minutes()))
			case <-monitorCtx.Done():
				return
			}
		}
	}()

	return cancel
}

// GetRunningProcesses returns a copy of currently monitored processes
func (m *UnlimitedExecutionMonitor) GetRunningProcesses() []ProcessInfo {
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
func (m *UnlimitedExecutionMonitor) CheckLongRunningProcesses(threshold time.Duration) {
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
