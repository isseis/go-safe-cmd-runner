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
	hashDir          string
	fs               common.FileSystem
	fileValidator    filevalidator.FileValidator
	security         *security.Validator
	pathResolver     *PathResolver
	privilegeManager runnertypes.PrivilegeManager
}

// Option is a function type for configuring Manager instances
type Option func(*managerOptions)

// managerOptions holds all configuration options for creating a Manager
type managerOptions struct {
	fs                   common.FileSystem
	fileValidatorEnabled bool
	privilegeManager     runnertypes.PrivilegeManager
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

// WithPrivilegeManager is an option for setting the privilege manager
func WithPrivilegeManager(privMgr runnertypes.PrivilegeManager) Option {
	return func(opts *managerOptions) {
		opts.privilegeManager = privMgr
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
		hashDir:          hashDir,
		fs:               opts.fs,
		privilegeManager: opts.privilegeManager,
	}

	// Initialize file validator with SHA256 algorithm
	if opts.fileValidatorEnabled {
		var err error

		// Use standard validator
		manager.fileValidator, err = filevalidator.New(&filevalidator.SHA256{}, hashDir)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize file validator: %w", err)
		}
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

	// Verify file hash using filevalidator (with privilege fallback)
	if err := m.verifyFileWithFallback(configPath); err != nil {
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

// VerifyAndReadConfigFile performs atomic verification and reading of a configuration file
// This prevents TOCTOU attacks by reading the file content once and verifying it against the hash
func (m *Manager) VerifyAndReadConfigFile(configPath string) ([]byte, error) {
	slog.Debug("Starting atomic config file verification and reading",
		"config_path", configPath,
		"hash_directory", m.hashDir)

	// Validate hash directory first
	if err := m.ValidateHashDirectory(); err != nil {
		return nil, &Error{
			Op:   "ValidateHashDirectory",
			Path: m.hashDir,
			Err:  err,
		}
	}

	// Read and verify file content atomically using filevalidator
	content, err := m.readAndVerifyFileWithFallback(configPath)
	if err != nil {
		slog.Error("Config file verification and reading failed",
			"config_path", configPath,
			"error", err)
		return nil, &Error{
			Op:   "ReadAndVerifyHash",
			Path: configPath,
			Err:  err,
		}
	}

	slog.Info("Config file verification and reading completed successfully",
		"config_path", configPath,
		"hash_directory", m.hashDir,
		"content_size", len(content))

	return content, nil
}

// VerifyEnvironmentFile verifies the integrity of an environment file using hash validation
func (m *Manager) VerifyEnvironmentFile(envFilePath string) error {
	slog.Debug("Starting environment file verification",
		"env_file_path", envFilePath,
		"hash_directory", m.hashDir)

	// Validate hash directory first
	if err := m.ValidateHashDirectory(); err != nil {
		return &Error{
			Op:   "ValidateHashDirectory",
			Path: m.hashDir,
			Err:  err,
		}
	}

	// Verify file hash using filevalidator (with privilege fallback)
	if err := m.verifyFileWithFallback(envFilePath); err != nil {
		slog.Error("Environment file verification failed",
			"env_file_path", envFilePath,
			"error", err)
		return &Error{
			Op:   "VerifyHash",
			Path: envFilePath,
			Err:  err,
		}
	}

	slog.Info("Environment file verification completed successfully",
		"env_file_path", envFilePath,
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

		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFileWithFallback(filePath); err != nil {
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

		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFileWithFallback(file); err != nil {
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

// verifyFileWithFallback attempts file verification with normal privileges first,
// then falls back to privileged verification if permission errors occur
func (m *Manager) verifyFileWithFallback(filePath string) error {
	// Try normal verification first
	err := m.fileValidator.Verify(filePath)
	if err == nil {
		return nil // Success with normal privileges
	}

	// Check if this is a permission-related error that might be resolved with privilege escalation
	if !isPermissionRelatedError(err) {
		return err // Return original error for non-permission issues
	}

	// Permission error detected - try with privilege escalation if available
	if m.privilegeManager == nil {
		slog.Debug("Permission error encountered but no privilege manager available",
			"file", filePath,
			"error", err)
		return err // Return original permission error
	}

	slog.Debug("Attempting privileged file verification",
		"file", filePath,
		"reason", "permission_denied_normal_access")

	// Try verification with privileges
	return m.fileValidator.VerifyWithPrivileges(filePath, m.privilegeManager)
}

// isPermissionRelatedError checks if an error is related to file permissions
func isPermissionRelatedError(err error) bool {
	if err == nil {
		return false
	}

	// Check for standard permission errors
	if os.IsPermission(err) {
		return true
	}

	// Check for path errors that might be permission-related
	// This covers cases where intermediate directory permissions prevent access
	if pathErr, ok := err.(*os.PathError); ok {
		return os.IsPermission(pathErr.Err)
	}

	return false
}

// readAndVerifyFileWithFallback attempts file reading and verification with normal privileges first,
// then falls back to privileged access if permission errors occur
func (m *Manager) readAndVerifyFileWithFallback(filePath string) ([]byte, error) {
	// Try normal verification and reading first
	content, err := m.fileValidator.VerifyAndRead(filePath)
	if err == nil {
		return content, nil // Success with normal privileges
	}

	// Check if this is a permission-related error that might be resolved with privilege escalation
	if !isPermissionRelatedError(err) {
		return nil, err // Return original error for non-permission issues
	}

	// Permission error detected - try with privilege escalation if available
	if m.privilegeManager == nil {
		slog.Debug("Permission error encountered but no privilege manager available",
			"file", filePath,
			"error", err)
		return nil, err // Return original permission error
	}

	slog.Debug("Attempting privileged file verification and reading",
		"file", filePath,
		"reason", "permission_denied_normal_access")

	// Try verification and reading with privileges
	return m.fileValidator.VerifyAndReadWithPrivileges(filePath, m.privilegeManager)
}
