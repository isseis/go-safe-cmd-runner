package verification

import (
	"log/slog"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
)

// NewManager creates a new verification manager using the default hash directory
// This is the production API that enforces strict security constraints
func NewManager() (*Manager, error) {
	// Log production manager creation for security audit trail
	logProductionManagerCreation()

	// Always use the default hash directory in production
	hashDir := cmdcommon.DefaultHashDirectory

	// Create manager with strict production constraints
	return newManagerInternal(hashDir,
		withCreationMode(CreationModeProduction),
		withSecurityLevel(SecurityLevelStrict),
	)
}

const (
	// callerDepthForNewManager represents the stack depth to get the caller of NewManager
	callerDepthForNewManager = 2
)

// logProductionManagerCreation logs the creation of a production manager for security audit
func logProductionManagerCreation() {
	// Build base logging arguments
	args := []any{
		"api", "NewManager",
		"hash_directory", cmdcommon.DefaultHashDirectory,
		"security_level", "strict",
	}

	// Add caller information if available
	if _, file, line, ok := runtime.Caller(callerDepthForNewManager); ok {
		args = append(args, "caller_file", file, "caller_line", line)
	}

	slog.Info("Production verification manager created", args...)
}

// validateProductionConstraints validates that production security constraints are met
func validateProductionConstraints(hashDir string) error {
	// In production, only the default hash directory is allowed
	if hashDir != cmdcommon.DefaultHashDirectory {
		return NewHashDirectorySecurityError(
			hashDir,
			cmdcommon.DefaultHashDirectory,
			"production environment requires default hash directory",
		)
	}

	// Additional checks are redundant here since ValidateHashDirectory is
	// called when manager is used.
	slog.Debug("Production constraints validated successfully",
		"hash_directory", hashDir,
		"default_directory", cmdcommon.DefaultHashDirectory)

	return nil
}
