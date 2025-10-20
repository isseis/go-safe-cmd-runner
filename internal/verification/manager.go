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

const securePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"

// Manager provides file verification capabilities
type Manager struct {
	hashDir                     string
	fs                          common.FileSystem
	fileValidator               filevalidator.FileValidator
	security                    *security.Validator
	pathResolver                *PathResolver
	isDryRun                    bool
	skipHashDirectoryValidation bool
}

// VerifyAndReadConfigFile performs atomic verification and reading of a configuration file
// This prevents TOCTOU attacks by reading the file content once and verifying it against the hash
func (m *Manager) VerifyAndReadConfigFile(configPath string) ([]byte, error) {
	slog.Debug("Starting atomic config file verification and reading",
		"config_path", configPath,
		"hash_directory", m.hashDir)

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
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

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return err
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

	// Skip hash directory validation if explicitly requested or in dry-run mode
	if m.skipHashDirectoryValidation || m.isDryRun {
		slog.Debug("Skipping hash directory validation",
			"hash_directory", m.hashDir,
			"skip_validation", m.skipHashDirectoryValidation,
			"dry_run", m.isDryRun)
		return nil
	}

	// Validate directory permissions using security validator
	if err := m.security.ValidateDirectoryPermissions(m.hashDir); err != nil {
		return fmt.Errorf("hash directory validation failed: %w", err)
	}

	return nil
}

// ensureHashDirectoryValidated calls ValidateHashDirectory and wraps any error
// into the package Error type used by Manager public methods.
func (m *Manager) ensureHashDirectoryValidated() error {
	if err := m.ValidateHashDirectory(); err != nil {
		return &Error{
			Op:   "ValidateHashDirectory",
			Path: m.hashDir,
			Err:  err,
		}
	}
	return nil
}

// VerifyGlobalFiles verifies the integrity of global files
func (m *Manager) VerifyGlobalFiles(runtimeGlobal *runnertypes.RuntimeGlobal) (*Result, error) {
	if runtimeGlobal == nil {
		return nil, ErrConfigNil
	}

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	result := &Result{
		TotalFiles:   len(runtimeGlobal.ExpandedVerifyFiles),
		FailedFiles:  []string{},
		SkippedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	// Update PathResolver with skip_standard_paths setting
	if m.pathResolver != nil {
		m.pathResolver.skipStandardPaths = runtimeGlobal.Spec.SkipStandardPaths
	}

	for _, filePath := range runtimeGlobal.ExpandedVerifyFiles {
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
		return nil, &VerificationError{
			Op:            "global",
			Details:       result.FailedFiles,
			TotalFiles:    result.TotalFiles,
			VerifiedFiles: result.VerifiedFiles,
			FailedFiles:   len(result.FailedFiles),
			SkippedFiles:  len(result.SkippedFiles),
			Err:           ErrGlobalVerificationFailed,
		}
	}

	return result, nil
}

// VerifyGroupFiles verifies the integrity of group files
func (m *Manager) VerifyGroupFiles(groupSpec *runnertypes.GroupSpec) (*Result, error) {
	if groupSpec == nil {
		return nil, ErrConfigNil
	}

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	// Collect all files to verify (explicit files + command files)
	allFiles := m.collectVerificationFiles(groupSpec)

	result := &Result{
		TotalFiles:   len(allFiles),
		FailedFiles:  []string{},
		SkippedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	for file := range allFiles {
		if m.shouldSkipVerification(file) {
			result.SkippedFiles = append(result.SkippedFiles, file)
			slog.Info("Skipping verification for standard system path",
				"group", groupSpec.Name,
				"file", file)
			continue
		}

		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFileWithFallback(file); err != nil {
			result.FailedFiles = append(result.FailedFiles, file)
			slog.Error("Group file verification failed",
				"group", groupSpec.Name,
				"file", file,
				"error", err)
		} else {
			result.VerifiedFiles++
		}
	}

	if len(result.FailedFiles) > 0 {
		return nil, &VerificationError{
			Op:            "group",
			Group:         groupSpec.Name,
			Details:       result.FailedFiles,
			TotalFiles:    result.TotalFiles,
			VerifiedFiles: result.VerifiedFiles,
			FailedFiles:   len(result.FailedFiles),
			SkippedFiles:  len(result.SkippedFiles),
			Err:           ErrGroupVerificationFailed,
		}
	}

	return result, nil
}

// shouldSkipVerification checks if a file should be skipped based on configuration
func (m *Manager) shouldSkipVerification(path string) bool {
	// Skip verification if file validator is disabled
	if m.fileValidator == nil {
		return true
	}

	if m.pathResolver == nil {
		return false
	}
	return m.pathResolver.ShouldSkipVerification(path)
}

// collectVerificationFiles collects all files to verify for a group
func (m *Manager) collectVerificationFiles(groupSpec *runnertypes.GroupSpec) map[string]struct{} {
	if groupSpec == nil {
		return make(map[string]struct{})
	}

	// Use map to automatically eliminate duplicates
	fileSet := make(map[string]struct{}, len(groupSpec.VerifyFiles)+len(groupSpec.Commands))

	// Add explicit files (VerifyFiles from GroupSpec need to be expanded)
	// NOTE: In the new architecture, VerifyFiles expansion is done by RuntimeGroup
	// For now, we'll use the raw VerifyFiles from GroupSpec
	// TODO: Pass RuntimeGroup here to use ExpandedVerifyFiles
	for _, file := range groupSpec.VerifyFiles {
		fileSet[file] = struct{}{}
	}

	// Add command files
	if m.pathResolver != nil {
		for _, command := range groupSpec.Commands {
			// NOTE: In the new architecture, command.Cmd needs to be expanded to ExpandedCmd
			// For now, we'll use the raw Cmd from CommandSpec
			// TODO: Pass expanded commands here
			resolvedPath, err := m.pathResolver.ResolvePath(command.Cmd)
			if err != nil {
				slog.Warn("Failed to resolve command path",
					"group", groupSpec.Name,
					"command", command.Cmd,
					"error", err.Error())
				continue
			}
			fileSet[resolvedPath] = struct{}{}
		}
	}

	return fileSet
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
	if m.fileValidator == nil {
		// File validator is disabled (e.g., in dry-run mode) - skip verification
		return nil
	}
	return m.fileValidator.Verify(filePath)
}

// readAndVerifyFileWithFallback attempts file reading and verification with normal privileges first,
// then falls back to privileged access if permission errors occur
func (m *Manager) readAndVerifyFileWithFallback(filePath string) ([]byte, error) {
	if m.fileValidator == nil {
		// File validator is disabled (e.g., in dry-run mode) - fallback to normal file reading
		// #nosec G304 - filePath comes from verified configuration and is sanitized by path resolver
		return os.ReadFile(filePath)
	}
	return m.fileValidator.VerifyAndRead(filePath)
}

// newManagerInternal creates a new verification manager with internal configuration
// This is the core implementation used by both production and testing APIs
func newManagerInternal(hashDir string, options ...InternalOption) (*Manager, error) {
	// Apply default options
	opts := newInternalOptions()
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

	// Perform security constraint validation
	if err := validateSecurityConstraints(hashDir, opts); err != nil {
		return nil, err
	}

	manager := &Manager{
		hashDir:                     hashDir,
		fs:                          opts.fs,
		isDryRun:                    opts.isDryRun,
		skipHashDirectoryValidation: opts.skipHashDirectoryValidation,
	}

	// Initialize file validator with hybrid hash path getter
	if opts.fileValidatorEnabled {
		var err error

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

	// Initialize path resolver with secure fixed PATH (do not inherit from environment)
	// Use custom path resolver if provided, otherwise create the default one
	var pathResolver *PathResolver
	if opts.customPathResolver != nil {
		pathResolver = opts.customPathResolver
	} else {
		pathResolver = NewPathResolver(securePathEnv, securityValidator, false)
	}

	manager.security = securityValidator
	manager.pathResolver = pathResolver

	return manager, nil
}

// validateSecurityConstraints validates security constraints based on creation mode and security level
func validateSecurityConstraints(hashDir string, opts *managerInternalOptions) error {
	// In production mode with strict security, enforce additional constraints
	if opts.creationMode == CreationModeProduction && opts.securityLevel == SecurityLevelStrict {
		if err := validateProductionConstraints(hashDir); err != nil {
			return err
		}
	}

	// Validate the hash directory itself using the provided filesystem
	// Skip validation if explicitly requested (typically for testing)
	if !opts.skipHashDirectoryValidation {
		if err := validateHashDirectoryWithFS(hashDir, opts.fs); err != nil {
			return err
		}
	}

	return nil
}

// validateHashDirectoryWithFS performs basic validation of the hash directory using provided filesystem
func validateHashDirectoryWithFS(hashDir string, fs common.FileSystem) error {
	if hashDir == "" {
		return ErrHashDirectoryEmpty
	}

	// Check if directory exists
	exists, err := fs.FileExists(hashDir)
	if err != nil {
		return fmt.Errorf("cannot access hash directory: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrHashDirectoryInvalid, hashDir)
	}

	// Check if path is a directory
	isDir, err := fs.IsDir(hashDir)
	if err != nil {
		return fmt.Errorf("cannot check if path is directory: %w", err)
	}
	if !isDir {
		return fmt.Errorf("%w: path is not a directory: %s", ErrHashDirectoryInvalid, hashDir)
	}

	return nil
}
