package verification

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// Manager provides file verification capabilities
type Manager struct {
	config       Config
	fs           common.FileSystem
	validator    *filevalidator.Validator
	security     *security.Validator
	pathResolver *PathResolver
}

// NewManager creates a new verification manager with the default file system
func NewManager(config Config) (*Manager, error) {
	return NewManagerWithFS(config, common.NewDefaultFileSystem())
}

// NewManagerWithFS creates a new verification manager with a custom file system
func NewManagerWithFS(config Config, fs common.FileSystem) (*Manager, error) {
	// Make a copy of the config to avoid modifying the original
	configCopy := config

	// Clean the hash directory path before validation
	if configCopy.IsEnabled() && configCopy.HashDirectory != "" {
		configCopy.HashDirectory = filepath.Clean(configCopy.HashDirectory)
	}

	if err := configCopy.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	manager := &Manager{
		config: configCopy,
		fs:     fs,
	}

	// Initialize components only if verification is enabled
	if configCopy.IsEnabled() {
		// Initialize file validator with SHA256 algorithm
		validator, err := filevalidator.New(&filevalidator.SHA256{}, configCopy.HashDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize file validator: %w", err)
		}
		manager.validator = validator

		// Initialize security validator with default configuration
		securityConfig := security.DefaultConfig()
		securityValidator, err := security.NewValidatorWithFS(securityConfig, fs)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize security validator: %w", err)
		}
		manager.security = securityValidator

		// Initialize path resolver
		pathEnv := os.Getenv("PATH")
		manager.pathResolver = NewPathResolver(pathEnv, securityValidator, false)
	}

	return manager, nil
}

// IsEnabled returns true if verification is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.IsEnabled()
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() Config {
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
		return ErrVerificationDisabled
	}

	if m.security == nil {
		return ErrSecurityValidatorNotInitialized
	}

	// Validate directory permissions using security validator
	if err := m.security.ValidateDirectoryPermissions(m.config.HashDirectory); err != nil {
		return fmt.Errorf("hash directory validation failed: %w", err)
	}

	return nil
}

// VerifyGlobalFiles verifies the integrity of global files
func (m *Manager) VerifyGlobalFiles(globalConfig *runnertypes.GlobalConfig) (*Result, error) {
	if !m.IsEnabled() {
		return &Result{}, nil
	}

	result := &Result{
		TotalFiles:   len(globalConfig.VerifyFiles),
		FailedFiles:  []string{},
		SkippedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	// Update PathResolver with skip_standard_paths setting
	if m.pathResolver != nil {
		m.pathResolver.skipStandardPaths = globalConfig.SkipStandardPaths
	}

	for _, filePath := range globalConfig.VerifyFiles {
		// Check if file should be skipped
		if m.shouldSkipVerification(filePath) {
			result.SkippedFiles = append(result.SkippedFiles, filePath)
			slog.Info("Skipping global file verification for standard system path",
				"file", filePath)
			continue
		}

		// Verify file hash (no permission check, only hash comparison)
		if err := m.validator.Verify(filePath); err != nil {
			result.FailedFiles = append(result.FailedFiles, filePath)
			slog.Error("Global file verification failed",
				"file", filePath,
				"error", err)
		} else {
			result.VerifiedFiles++
		}
	}

	if len(result.FailedFiles) > 0 {
		return result, &VerificationError{
			Op:      "global",
			Details: result.FailedFiles,
			Err:     ErrGlobalVerificationFailed,
		}
	}

	return result, nil
}

// VerifyGroupFiles verifies the integrity of group files
func (m *Manager) VerifyGroupFiles(groupConfig *runnertypes.CommandGroup) (*Result, error) {
	if !m.IsEnabled() {
		return &Result{}, nil
	}

	// Collect all files to verify (explicit files + command files)
	allFiles := m.collectVerificationFiles(groupConfig)

	result := &Result{
		TotalFiles:   len(allFiles),
		FailedFiles:  []string{},
		SkippedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	for _, file := range allFiles {
		if m.shouldSkipVerification(file) {
			result.SkippedFiles = append(result.SkippedFiles, file)
			slog.Info("Skipping verification for standard system path",
				"group", groupConfig.Name,
				"file", file)
			continue
		}

		// Verify file hash (no permission check, only hash comparison)
		if err := m.validator.Verify(file); err != nil {
			result.FailedFiles = append(result.FailedFiles, file)
			slog.Error("Group file verification failed",
				"group", groupConfig.Name,
				"file", file,
				"error", err)
		} else {
			result.VerifiedFiles++
		}
	}

	if len(result.FailedFiles) > 0 {
		return result, &VerificationError{
			Op:      "group",
			Group:   groupConfig.Name,
			Details: result.FailedFiles,
			Err:     ErrGroupVerificationFailed,
		}
	}

	return result, nil
}

// shouldSkipVerification checks if a file should be skipped based on configuration
func (m *Manager) shouldSkipVerification(path string) bool {
	if m.pathResolver == nil {
		return false
	}
	return m.pathResolver.ShouldSkipVerification(path)
}

// collectVerificationFiles collects all files to verify for a group
func (m *Manager) collectVerificationFiles(groupConfig *runnertypes.CommandGroup) []string {
	var allFiles []string

	// Add explicit files
	allFiles = append(allFiles, groupConfig.VerifyFiles...)

	// Add command files
	if m.pathResolver != nil {
		for _, command := range groupConfig.Commands {
			resolvedPath, err := m.pathResolver.ResolvePath(command.Cmd)
			if err != nil {
				slog.Warn("Failed to resolve command path",
					"group", groupConfig.Name,
					"command", command.Cmd,
					"error", err.Error())
				continue
			}
			allFiles = append(allFiles, resolvedPath)
		}
	}

	// Remove duplicates
	return removeDuplicates(allFiles)
}

// VerifyCommandFile verifies the integrity of a single command file
func (m *Manager) VerifyCommandFile(command string) (*FileDetail, error) {
	detail := &FileDetail{
		Path: command,
	}

	start := time.Now()
	defer func() {
		detail.Duration = time.Since(start)
	}()

	if !m.IsEnabled() {
		detail.HashMatched = true // Consider disabled verification as success
		return detail, nil
	}

	// Resolve path
	if m.pathResolver == nil {
		detail.Error = ErrPathResolverNotInitialized
		return detail, ErrPathResolverNotInitialized
	}

	resolvedPath, err := m.pathResolver.ResolvePath(command)
	if err != nil {
		detail.Error = err
		return detail, fmt.Errorf("path resolution failed: %w", err)
	}
	detail.ResolvedPath = resolvedPath

	// Check if should skip verification
	if m.shouldSkipVerification(resolvedPath) {
		detail.HashMatched = true // Skip is treated as success
		return detail, nil
	}

	// Verify hash (no permission check, only hash comparison)
	if err := m.validator.Verify(resolvedPath); err != nil {
		detail.HashMatched = false
		detail.Error = err
		return detail, fmt.Errorf("command file verification failed: %w", err)
	}

	detail.HashMatched = true
	return detail, nil
}
