package privilege

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Manager interface for privilege management (extends runnertypes.PrivilegeManager)
type Manager interface {
	runnertypes.PrivilegeManager

	// Additional methods specific to privilege package
	GetCurrentUID() int
	GetOriginalUID() int
	GetMetrics() Metrics
}

// NewManager creates a platform-appropriate privilege manager
func NewManager(logger *slog.Logger) Manager {
	return newPlatformManager(logger)
}
