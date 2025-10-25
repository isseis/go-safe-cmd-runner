package runner

import (
	"context"
	"testing"
	"time"
)

func TestNewProcessMonitor(t *testing.T) {
	monitor := NewProcessMonitor()
	if monitor == nil {
		t.Fatal("NewProcessMonitor returned nil")
	}
	if monitor.processes == nil {
		t.Error("processes map not initialized")
	}
}

func TestProcessMonitor_Register(t *testing.T) {
	monitor := NewProcessMonitor()

	monitor.Register(123, "test-command")

	processes := monitor.GetRunningProcesses()
	if len(processes) != 1 {
		t.Fatalf("Expected 1 process, got %d", len(processes))
	}

	if processes[0].PID != 123 {
		t.Errorf("Expected PID 123, got %d", processes[0].PID)
	}
	if processes[0].CommandName != "test-command" {
		t.Errorf("Expected command name 'test-command', got %s", processes[0].CommandName)
	}
}

func TestProcessMonitor_Unregister(t *testing.T) {
	monitor := NewProcessMonitor()

	monitor.Register(123, "test-command")
	monitor.Register(456, "another-command")

	monitor.Unregister(123)

	processes := monitor.GetRunningProcesses()
	if len(processes) != 1 {
		t.Fatalf("Expected 1 process after unregister, got %d", len(processes))
	}

	if processes[0].PID != 456 {
		t.Errorf("Expected PID 456 to remain, got %d", processes[0].PID)
	}
}

func TestProcessMonitor_StartStop(t *testing.T) {
	monitor := NewProcessMonitor()
	ctx := context.Background()

	// Initially not running
	if monitor.IsRunning() {
		t.Error("Monitor should not be running initially")
	}

	// Start monitoring
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Should be running now
	if !monitor.IsRunning() {
		t.Error("Monitor should be running after Start()")
	}

	// Starting again should be safe (idempotent)
	err = monitor.Start(ctx)
	if err != nil {
		t.Errorf("Second Start() returned error: %v", err)
	}

	// Stop monitoring
	monitor.Stop()

	// Should not be running
	if monitor.IsRunning() {
		t.Error("Monitor should not be running after Stop()")
	}

	// Stopping again should be safe (idempotent)
	monitor.Stop()
}

func TestProcessMonitor_MonitoringLoop(t *testing.T) {
	monitor := NewProcessMonitor()
	ctx := context.Background()

	// Set short intervals for testing
	monitor.SetCheckInterval(100 * time.Millisecond)
	monitor.SetWarnThreshold(50 * time.Millisecond)

	// Register a process
	monitor.Register(123, "test-command")

	// Start monitoring
	err := monitor.Start(ctx)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer monitor.Stop()

	// Wait for at least one check cycle
	time.Sleep(200 * time.Millisecond)

	// Verify process is still registered
	processes := monitor.GetRunningProcesses()
	if len(processes) != 1 {
		t.Errorf("Expected 1 process, got %d", len(processes))
	}
}

func TestProcessMonitor_SetIntervals(t *testing.T) {
	monitor := NewProcessMonitor()

	// Test SetCheckInterval
	monitor.SetCheckInterval(10 * time.Second)
	if monitor.checkInterval != 10*time.Second {
		t.Errorf("Expected check interval 10s, got %v", monitor.checkInterval)
	}

	// Test SetWarnThreshold
	monitor.SetWarnThreshold(15 * time.Minute)
	if monitor.warnThreshold != 15*time.Minute {
		t.Errorf("Expected warn threshold 15m, got %v", monitor.warnThreshold)
	}
}

func TestProcessMonitor_CheckLongRunningProcesses(t *testing.T) {
	monitor := NewProcessMonitor()

	// Register a process that started in the past
	monitor.Register(123, "long-running-command")
	// Manually set the start time to 10 minutes ago
	monitor.mutex.Lock()
	monitor.processes[123].StartTime = time.Now().Add(-10 * time.Minute)
	monitor.mutex.Unlock()

	// Check for long-running processes with 5 minute threshold
	// This should log a warning (we can't easily test the log output here)
	monitor.CheckLongRunningProcesses(5 * time.Minute)

	// Verify the process is still registered
	processes := monitor.GetRunningProcesses()
	if len(processes) != 1 {
		t.Errorf("Expected 1 process, got %d", len(processes))
	}
}

func TestProcessMonitor_GetRunningProcesses(t *testing.T) {
	monitor := NewProcessMonitor()

	// Register multiple processes
	monitor.Register(123, "command1")
	monitor.Register(456, "command2")
	monitor.Register(789, "command3")

	processes := monitor.GetRunningProcesses()
	if len(processes) != 3 {
		t.Errorf("Expected 3 processes, got %d", len(processes))
	}

	// Verify that we got a copy, not a reference
	monitor.Register(999, "command4")
	if len(processes) == 4 {
		t.Error("GetRunningProcesses should return a copy, not a reference")
	}
}

func TestProcessMonitor_ConcurrentAccess(t *testing.T) {
	monitor := NewProcessMonitor()

	// Test concurrent registration and unregistration
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			monitor.Register(id, "concurrent-command")
			time.Sleep(10 * time.Millisecond)
			monitor.Unregister(id)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// All processes should be unregistered
	processes := monitor.GetRunningProcesses()
	if len(processes) != 0 {
		t.Errorf("Expected 0 processes after concurrent operations, got %d", len(processes))
	}
}
