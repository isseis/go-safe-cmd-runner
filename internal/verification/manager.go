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
	hashDir       string
	fs            common.FileSystem
	fileValidator *filevalidator.Validator
	security      *security.Validator
	pathResolver  *PathResolver
}

// Option is a function type for configuring Manager instances
type Option func(*managerOptions)

// managerOptions holds all configuration options for creating a Manager
type managerOptions struct {
	fs                   common.FileSystem
	fileValidatorEnabled bool
}

func newOptions() *managerOptions {
	return &managerOptions{
		fileValidatorEnabled: true,
		fs:                   common.NewDefaultFileSystem(),
	}
}

// withFS is an option for setting the file system (for testing purposes)
func withFS(fs common.FileSystem) Option {
	return func(opts *managerOptions) {
		opts.fs = fs
	}
}

// withFileValidatorDisabled is an option for disabling the file validator (for testing purposes)
func withFileValidatorDisabled() Option {
	return func(opts *managerOptions) {
		opts.fileValidatorEnabled = false
	}
}

// NewManager creates a new verification manager with the default file system
func NewManager(hashDir string) (*Manager, error) {
	return NewManagerWithOpts(hashDir)
}

// NewManagerWithOpts creates a new verification manager with a custom file system
func NewManagerWithOpts(hashDir string, options ...Option) (*Manager, error) {
	// Apply default options
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	// Clean the hash directory path
	if hashDir == "" {
		return nil, ErrHashDirectoryEmpty
	}
	if hashDir != "" {
		hashDir = filepath.Clean(hashDir)
	}

	manager := &Manager{
		hashDir: hashDir,
		fs:      opts.fs,
	}

	// Initialize file validator with SHA256 algorithm
	if opts.fileValidatorEnabled {
		fileValidator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize file validator: %w", err)
		}
		manager.fileValidator = fileValidator
	}

	// Initialize security validator with default config
	securityConfig := security.DefaultConfig()
	securityValidator, err := security.NewValidatorWithFS(securityConfig, opts.fs)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize security validator: %w", err)
	}

	// Initialize path resolver
	pathEnv := os.Getenv("PATH") // Default to PATH environment variable if not explicitly set
	pathResolver := NewPathResolver(pathEnv, securityValidator, false)

	manager.security = securityValidator
	manager.pathResolver = pathResolver

	return manager, nil
}

// VerifyConfigFile verifies the integrity of a configuration file
func (m *Manager) VerifyConfigFile(configPath string) error {
	slog.Debug("Starting config file verification",
		"config_path", configPath,
		"hash_directory", m.hashDir)

	// Validate hash directory first
	if err := m.ValidateHashDirectory(); err != nil {
		return &Error{
			Op:   "ValidateHashDirectory",
			Path: m.hashDir,
			Err:  err,
		}
	}

	// Verify file hash using filevalidator
	if err := m.fileValidator.Verify(configPath); err != nil {
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
		"hash_directory", m.hashDir)

	return nil
}

// ValidateHashDirectory validates the hash directory security
func (m *Manager) ValidateHashDirectory() error {
	if m.security == nil {
		return ErrSecurityValidatorNotInitialized
	}

	// Validate directory permissions using security validator
	if err := m.security.ValidateDirectoryPermissions(m.hashDir); err != nil {
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
		if err := m.fileValidator.Verify(filePath); err != nil {
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
		if err := m.fileValidator.Verify(file); err != nil {
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
	allFiles := make([]string, 0, len(groupConfig.VerifyFiles)+len(groupConfig.Commands))

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

// ResolvePath resolves a command to its full path with security validation
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
	if err := m.fileValidator.Verify(resolvedPath); err != nil {
		detail.HashMatched = false
		detail.Error = err
		return detail, fmt.Errorf("command file verification failed: %w", err)
	}

	detail.HashMatched = true
	return detail, nil
}
