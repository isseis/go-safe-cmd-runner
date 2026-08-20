// Package privilege manages elevation to root and restoration of the original
// privileges for operations that require them.
package privilege

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Manager interface for privilege management (extends runnertypes.PrivilegeManager)
type Manager interface {
	runnertypes.PrivilegeManager

	// Additional methods specific to privilege package
	GetCurrentUID() int
	GetOriginalUID() int
}

// NewManager creates a platform-appropriate privilege manager
func NewManager(logger *slog.Logger) Manager {
	return newPlatformManager(logger)
}
