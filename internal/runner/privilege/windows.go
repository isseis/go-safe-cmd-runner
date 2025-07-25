//go:build windows

package privilege

import (
	"context"
	"log/slog"
)

type WindowsPrivilegeManager struct {
	logger *slog.Logger
}

func newPlatformManager(logger *slog.Logger) Manager {
	return &WindowsPrivilegeManager{
		logger: logger,
	}
}

func (m *WindowsPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error {
	m.logger.Error("Privileged execution requested on unsupported platform",
		"operation", elevationCtx.Operation,
		"command", elevationCtx.CommandName)
	return ErrPlatformNotSupported
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
	return ErrPlatformNotSupported
}
