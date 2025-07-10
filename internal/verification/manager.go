package verification

import (
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Manager provides file verification capabilities
type Manager struct {
	config    *Config
	fs        common.FileSystem
	validator *filevalidator.Validator
	security  *security.Validator
}

// NewManager creates a new verification manager with the default file system
func NewManager(config *Config) (*Manager, error) {
	return NewManagerWithFS(config, common.NewDefaultFileSystem())
}

// NewManagerWithFS creates a new verification manager with a custom file system
func NewManagerWithFS(config *Config, fs common.FileSystem) (*Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("%w", ErrConfigNil)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	manager := &Manager{
		config: config,
		fs:     fs,
	}

	// Initialize components only if verification is enabled
	if config.IsEnabled() {
		// Initialize file validator with SHA256 algorithm
		validator, err := filevalidator.New(&filevalidator.SHA256{}, config.HashDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize file validator: %w", err)
		}
		manager.validator = validator

		// Initialize security validator
		securityValidator, err := security.NewValidatorWithFS(nil, fs)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize security validator: %w", err)
		}
		manager.security = securityValidator
	}

	return manager, nil
}

// IsEnabled returns true if verification is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.IsEnabled()
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// VerifyConfigFile verifies the integrity of a configuration file
func (m *Manager) VerifyConfigFile(configPath string) error {
	if !m.IsEnabled() {
		slog.Debug("Verification is disabled, skipping config file verification",
			"config_path", configPath)
		return nil
	}

	slog.Debug("Starting config file verification",
		"config_path", configPath,
		"hash_directory", m.config.HashDirectory)

	// Validate hash directory first
	if err := m.ValidateHashDirectory(); err != nil {
		return &Error{
			Op:   "ValidateHashDirectory",
			Path: m.config.HashDirectory,
			Err:  err,
		}
	}

	// Verify file hash using filevalidator
	if err := m.validator.Verify(configPath); err != nil {
		slog.Error("Config file verification failed",
			"config_path", configPath,
			"error", err)
		return &Error{
			Op:   "VerifyHash",
			Path: configPath,
			Err:  err,
		}
	}

	slog.Info("Config file verification completed successfully",
		"config_path", configPath,
		"hash_directory", m.config.HashDirectory)

	return nil
}

// ValidateHashDirectory validates the hash directory security
func (m *Manager) ValidateHashDirectory() error {
	if !m.IsEnabled() {
		return fmt.Errorf("%w", ErrVerificationDisabled)
	}

	if m.security == nil {
		return fmt.Errorf("%w", ErrSecurityValidatorNotInitialized)
	}

	// Validate directory permissions using security validator
	if err := m.security.ValidateDirectoryPermissions(m.config.HashDirectory); err != nil {
		return fmt.Errorf("hash directory validation failed: %w", err)
	}

	return nil
}
