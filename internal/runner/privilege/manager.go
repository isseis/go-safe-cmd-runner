package privilege

import (
	"context"
	"log/slog"
)

// Manager interface for privilege management
type Manager interface {
	// WithPrivileges executes a function with elevated privileges
	// This is the ONLY public method to ensure safe privilege management
	WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error

	// IsPrivilegedExecutionSupported checks if privileged execution is available
	IsPrivilegedExecutionSupported() bool

	// GetCurrentUID returns the current effective user ID
	GetCurrentUID() int

	// GetOriginalUID returns the original user ID
	GetOriginalUID() int

	// HealthCheck verifies privilege escalation works
	HealthCheck(ctx context.Context) error

	// GetHealthStatus performs comprehensive health check and returns status
	GetHealthStatus(ctx context.Context) HealthStatus

	// GetMetrics returns operational metrics
	GetMetrics() Metrics
}

// NewManager creates a platform-appropriate privilege manager
func NewManager(logger *slog.Logger) Manager {
	return newPlatformManager(logger)
}
