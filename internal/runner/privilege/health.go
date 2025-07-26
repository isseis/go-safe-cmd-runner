// Package privilege provides health check functionality for privilege management.
package privilege

import (
	"context"
	"time"
)

// HealthStatus represents the health status of privilege management
type HealthStatus struct {
	IsSupported      bool          `json:"is_supported"`
	LastCheck        time.Time     `json:"last_check"`
	CheckDuration    time.Duration `json:"check_duration"`
	Error            string        `json:"error,omitempty"`
	SetuidConfigured bool          `json:"setuid_configured"`
	OriginalUID      int           `json:"original_uid"`
	CurrentUID       int           `json:"current_uid"`
	EffectiveUID     int           `json:"effective_uid"`
	CanElevate       bool          `json:"can_elevate"`
}

// GetHealthStatus performs a comprehensive health check and returns the current status
func (m *UnixPrivilegeManager) GetHealthStatus(ctx context.Context) HealthStatus {
	status := HealthStatus{
		IsSupported:      m.IsPrivilegedExecutionSupported(),
		SetuidConfigured: m.isSetuid,
		OriginalUID:      m.originalUID,
		CurrentUID:       m.GetCurrentUID(),
		EffectiveUID:     m.GetCurrentUID(), // For consistency with JSON naming
		LastCheck:        time.Now(),
	}

	if !status.IsSupported {
		status.Error = "Privileged execution not supported"
		status.CanElevate = false
		return status
	}

	// Perform actual health check with timing
	start := time.Now()
	err := m.HealthCheck(ctx)
	status.CheckDuration = time.Since(start)

	if err != nil {
		status.Error = err.Error()
		status.CanElevate = false
	} else {
		status.CanElevate = true
	}

	return status
}
