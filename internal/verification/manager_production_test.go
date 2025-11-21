package verification

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionNewManager tests the production NewManager API
func TestProductionNewManager(t *testing.T) {
	t.Run("successful_manager_creation", func(t *testing.T) {
		// Create a temporary directory to act as hash directory
		tmpDir := t.TempDir()
		hashDir := filepath.Join(tmpDir, "hashes")
		err := os.MkdirAll(hashDir, 0o755)
		require.NoError(t, err)

		// Temporarily override the default hash directory for testing
		originalHashDir := cmdcommon.DefaultHashDirectory
		defer func() { cmdcommon.DefaultHashDirectory = originalHashDir }()
		cmdcommon.DefaultHashDirectory = hashDir

		// Test manager creation
		manager, err := NewManager()

		// Verify successful creation
		assert.NoError(t, err, "NewManager should not return an error")
		assert.NotNil(t, manager, "Manager should not be nil")
		assert.Equal(t, hashDir, manager.hashDir, "Manager should use default hash directory")
		assert.NotNil(t, manager.fs, "File system should be initialized")
		assert.NotNil(t, manager.pathResolver, "Path resolver should be initialized")
	})

	t.Run("production_constraints_validation", func(t *testing.T) {
		// Test with non-existent hash directory to trigger validation error
		originalHashDir := cmdcommon.DefaultHashDirectory
		defer func() { cmdcommon.DefaultHashDirectory = originalHashDir }()
		cmdcommon.DefaultHashDirectory = "/non/existent/hash/directory"

		// Test manager creation
		manager, err := NewManager()

		// Verify that validation fails appropriately
		assert.Error(t, err, "NewManager should return an error for non-existent hash directory")
		assert.Nil(t, manager, "Manager should be nil on error")
		assert.Contains(t, err.Error(), "hash directory", "Error should mention hash directory")
	})

	t.Run("security_audit_logging", func(t *testing.T) {
		// Create a temporary directory to act as hash directory
		tmpDir := t.TempDir()
		hashDir := filepath.Join(tmpDir, "hashes")
		err := os.MkdirAll(hashDir, 0o755)
		require.NoError(t, err)

		// Temporarily override the default hash directory
		originalHashDir := cmdcommon.DefaultHashDirectory
		defer func() { cmdcommon.DefaultHashDirectory = originalHashDir }()
		cmdcommon.DefaultHashDirectory = hashDir

		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)

		// Create manager
		manager, err := NewManager()

		// Verify logging occurred
		assert.NoError(t, err)
		assert.NotNil(t, manager)

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "Production verification manager created")
		assert.Contains(t, logOutput, "api=NewManager")
		assert.Contains(t, logOutput, hashDir)
		assert.Contains(t, logOutput, "security_level=strict")
	})
}

// TestProductionNewManagerForDryRun tests the dry-run manager creation API
func TestProductionNewManagerForDryRun(t *testing.T) {
	t.Run("successful_dry_run_manager_creation", func(t *testing.T) {
		// Note: Dry-run doesn't require actual hash directory to exist
		originalHashDir := cmdcommon.DefaultHashDirectory
		defer func() { cmdcommon.DefaultHashDirectory = originalHashDir }()
		cmdcommon.DefaultHashDirectory = "/usr/local/etc/go-safe-cmd-runner/hashes"

		// Test manager creation
		manager, err := NewManagerForDryRun()

		// Verify successful creation
		assert.NoError(t, err, "NewManagerForDryRun should not return an error")
		assert.NotNil(t, manager, "Manager should not be nil")
		assert.Equal(t, cmdcommon.DefaultHashDirectory, manager.hashDir)
		assert.NotNil(t, manager.fs, "File system should be initialized")
		assert.True(t, manager.isDryRun, "Should be in dry-run mode")
	})

	t.Run("dry_run_security_audit_logging", func(t *testing.T) {
		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)

		// Create dry-run manager
		manager, err := NewManagerForDryRun()

		// Verify logging occurred
		assert.NoError(t, err)
		assert.NotNil(t, manager)

		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "Dry-run verification manager created")
		assert.Contains(t, logOutput, "api=NewManagerForDryRun")
		assert.Contains(t, logOutput, "mode=dry-run")
		assert.Contains(t, logOutput, "skip_hash_directory_validation=true")
		assert.Contains(t, logOutput, "file_validator_enabled=true")
	})

	t.Run("basic_functionality_difference", func(t *testing.T) {
		// Create temporary hash directory for production manager
		tmpDir := t.TempDir()
		hashDir := filepath.Join(tmpDir, "hashes")
		err := os.MkdirAll(hashDir, 0o755)
		require.NoError(t, err)

		originalHashDir := cmdcommon.DefaultHashDirectory
		defer func() { cmdcommon.DefaultHashDirectory = originalHashDir }()
		cmdcommon.DefaultHashDirectory = hashDir

		// Create both types of managers
		prodManager, err := NewManager()
		require.NoError(t, err)

		dryRunManager, err := NewManagerForDryRun()
		require.NoError(t, err)

		// Verify both managers are created with the same hash directory
		assert.Equal(t, prodManager.hashDir, dryRunManager.hashDir, "Both should use same hash directory")

		// Verify dry-run specific behavior
		assert.False(t, prodManager.isDryRun, "Production manager should not be in dry-run mode")
		assert.True(t, dryRunManager.isDryRun, "Dry-run manager should be in dry-run mode")
	})
}

// TestProductionManagerLogging tests the production manager logging
func TestProductionManagerLogging(t *testing.T) {
	t.Run("logging_includes_required_fields", func(t *testing.T) {
		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)

		// Call logging function
		logProductionManagerCreation()

		// Verify log content
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "Production verification manager created")
		assert.Contains(t, logOutput, "api=NewManager")
		assert.Contains(t, logOutput, "security_level=strict")
		assert.Contains(t, logOutput, "caller_file=")
		assert.Contains(t, logOutput, "caller_line=")
	})
}

// TestDryRunManagerLogging tests the dry-run manager logging
func TestDryRunManagerLogging(t *testing.T) {
	t.Run("logging_includes_dry_run_fields", func(t *testing.T) {
		// Capture log output
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		slog.SetDefault(logger)

		// Call logging function
		logDryRunManagerCreation()

		// Verify log content
		logOutput := logBuffer.String()
		assert.Contains(t, logOutput, "Dry-run verification manager created")
		assert.Contains(t, logOutput, "api=NewManagerForDryRun")
		assert.Contains(t, logOutput, "mode=dry-run")
		assert.Contains(t, logOutput, "skip_hash_directory_validation=true")
		assert.Contains(t, logOutput, "file_validator_enabled=true")
		assert.Contains(t, logOutput, "caller_file=")
		assert.Contains(t, logOutput, "caller_line=")
	})
}
