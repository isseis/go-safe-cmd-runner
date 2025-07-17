package verification

import (
	"fmt"
	"log/slog"
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
	// Clean the hash directory path
	if config.HashDirectory != "" {
		config.HashDirectory = filepath.Clean(config.HashDirectory)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	manager := &Manager{
		config: config,
		fs:     fs,
	}

	// Initialize file validator with SHA256 algorithm
	validator, err := filevalidator.New(&filevalidator.SHA256{}, config.HashDirectory)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize file validator: %w", err)
	}

	// Initialize security validator with default config
	securityConfig := security.DefaultConfig()
	securityValidator, err := security.NewValidatorWithFS(securityConfig, fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize security validator: %w", err)
	}

	// Initialize path resolver
	pathEnv := "" // Get from environment or use empty string for default
	pathResolver := NewPathResolver(pathEnv, securityValidator, false)

	manager.validator = validator
	manager.security = securityValidator
	manager.pathResolver = pathResolver

	return manager, nil
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() Config {
	return m.config
}

// VerifyConfigFile verifies the integrity of a configuration file
func (m *Manager) VerifyConfigFile(configPath string) error {
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
		slog.Error("CRITICAL: Global file verification failed - program will terminate",
			"failed_files", result.FailedFiles,
			"verified_files", result.VerifiedFiles,
			"total_files", result.TotalFiles)
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

// ResolvePath resolves a command to its full path with optional validation
func (m *Manager) ResolvePath(command string) (string, error) {
	if m.pathResolver == nil {
		return "", ErrPathResolverNotInitialized
	}

	// Always perform path resolution
	resolvedPath, err := m.pathResolver.ResolvePath(command)
	if err != nil {
		return "", err
	}

	// Always perform validation when Manager exists
	if err := m.pathResolver.ValidateCommand(resolvedPath); err != nil {
		return "", fmt.Errorf("unsafe command rejected: %w", err)
	}

	return resolvedPath, nil
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

	// Validate command security after path resolution
	if err := m.pathResolver.ValidateCommand(resolvedPath); err != nil {
		detail.Error = err
		return detail, fmt.Errorf("command validation failed: %w", err)
	}

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
