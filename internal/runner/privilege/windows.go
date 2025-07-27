//go:build windows

package privilege

import (
	"context"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

type WindowsPrivilegeManager struct {
	logger  *slog.Logger
	metrics Metrics
}

func newPlatformManager(logger *slog.Logger) Manager {
	return &WindowsPrivilegeManager{
		logger: logger,
	}
}

func (m *WindowsPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx runnertypes.ElevationContext, fn func() error) error {
	m.logger.Error("Privileged execution requested on unsupported platform",
		"operation", elevationCtx.Operation,
		"command", elevationCtx.CommandName)
	return runnertypes.runnertypes.ErrPlatformNotSupported
}

// ElevatePrivileges is not supported on Windows
func (m *WindowsPrivilegeManager) ElevatePrivileges() error {
	return runnertypes.runnertypes.ErrPlatformNotSupported
}

// DropPrivileges is not supported on Windows
func (m *WindowsPrivilegeManager) DropPrivileges() error {
	return runnertypes.runnertypes.ErrPlatformNotSupported
}

func (m *WindowsPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return false
}

func (m *WindowsPrivilegeManager) GetCurrentUID() int {
	return -1 // Windows doesn't use UIDs
}

func (m *WindowsPrivilegeManager) GetOriginalUID() int {
	return -1 // Windows doesn't use UIDs
}

func (m *WindowsPrivilegeManager) HealthCheck(ctx context.Context) error {
	return runnertypes.ErrPlatformNotSupported
}

// GetHealthStatus returns health status for Windows (always unsupported)
func (m *WindowsPrivilegeManager) GetHealthStatus(ctx context.Context) HealthStatus {
	return HealthStatus{
		IsSupported:      false,
		SetuidConfigured: false,
		OriginalUID:      -1,
		CurrentUID:       -1,
		EffectiveUID:     -1,
		CanElevate:       false,
		Error:            "Privileged execution not supported on Windows",
	}
}

// GetMetrics returns metrics snapshot for Windows
func (m *WindowsPrivilegeManager) GetMetrics() Metrics {
	return m.metrics.GetSnapshot()
}
