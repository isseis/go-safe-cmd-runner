package verification

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
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
	resultCollector             *ResultCollector
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
	content, err := m.readAndVerifyFileWithFallback(configPath, "config")
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
	if err := m.verifyFileWithFallback(envFilePath, "env"); err != nil {
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
		m.pathResolver.skipStandardPaths = runtimeGlobal.SkipStandardPaths()
	}

	for _, filePath := range runtimeGlobal.ExpandedVerifyFiles {
		// Check if file should be skipped
		if m.shouldSkipVerification(filePath) {
			if m.isDryRun && m.resultCollector != nil {
				m.resultCollector.RecordSkip()
			}
			result.SkippedFiles = append(result.SkippedFiles, filePath)
			slog.Info("Skipping global file verification for standard system path",
				"file", filePath)
			continue
		}

		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFileWithFallback(filePath, "global"); err != nil {
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
func (m *Manager) VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*Result, error) {
	if runtimeGroup == nil {
		return nil, ErrConfigNil
	}

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	// Collect all files to verify (explicit files + command files)
	allFiles := m.collectVerificationFiles(runtimeGroup)

	result := &Result{
		TotalFiles:   len(allFiles),
		FailedFiles:  []string{},
		SkippedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	groupName := runnertypes.ExtractGroupName(runtimeGroup)

	for file := range allFiles {
		if m.shouldSkipVerification(file) {
			if m.isDryRun && m.resultCollector != nil {
				m.resultCollector.RecordSkip()
			}
			result.SkippedFiles = append(result.SkippedFiles, file)
			slog.Info("Skipping verification for standard system path",
				"group", groupName,
				"file", file)
			continue
		}

		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFileWithFallback(file, "group:"+groupName); err != nil {
			result.FailedFiles = append(result.FailedFiles, file)
			slog.Error("Group file verification failed",
				"group", groupName,
				"file", file,
				"error", err)
		} else {
			result.VerifiedFiles++
		}
	}

	if len(result.FailedFiles) > 0 {
		return nil, &VerificationError{
			Op:            "group",
			Group:         groupName,
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
func (m *Manager) collectVerificationFiles(runtimeGroup *runnertypes.RuntimeGroup) map[string]struct{} {
	if runtimeGroup == nil || runtimeGroup.Spec == nil {
		return make(map[string]struct{})
	}

	groupSpec := runtimeGroup.Spec

	// Use map to automatically eliminate duplicates
	fileSet := make(map[string]struct{}, len(runtimeGroup.ExpandedVerifyFiles)+len(groupSpec.Commands))

	// Add explicit files with variables expanded
	for _, file := range runtimeGroup.ExpandedVerifyFiles {
		fileSet[file] = struct{}{}
	}

	// Add command files
	if m.pathResolver != nil {
		groupContext := fmt.Sprintf("group[%s]", groupSpec.Name)
		for _, command := range groupSpec.Commands {
			// Expand command path using group variables
			expandedCmd, err := config.ExpandString(
				command.Cmd,
				runtimeGroup.ExpandedVars,
				groupContext,
				"cmd")
			if err != nil {
				slog.Warn("Failed to expand command path",
					"group", groupSpec.Name,
					"command", command.Cmd,
					"error", err.Error())
				continue
			}

			// Resolve expanded command path
			resolvedPath, err := m.pathResolver.ResolvePath(expandedCmd)
			if err != nil {
				slog.Warn("Failed to resolve command path",
					"group", groupSpec.Name,
					"command", expandedCmd,
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

// GetVerificationSummary returns the file verification summary for dry-run mode
// Returns nil if not in dry-run mode or if result collector is not initialized
func (m *Manager) GetVerificationSummary() *FileVerificationSummary {
	if m.resultCollector == nil {
		return nil
	}
	summary := m.resultCollector.GetSummary()
	return &summary
}

// verifyFileWithFallback attempts file verification with normal privileges first,
// then falls back to privileged verification if permission errors occur
// In dry-run mode, it records the verification result without returning errors
func (m *Manager) verifyFileWithFallback(filePath string, context string) error {
	if m.fileValidator == nil {
		// File validator is disabled - skip verification
		return nil
	}

	// Perform verification
	err := m.fileValidator.Verify(filePath)

	// In dry-run mode, record the result and return nil (warn-only mode)
	if m.isDryRun && m.resultCollector != nil {
		if err == nil {
			m.resultCollector.RecordSuccess()
		} else {
			// Record failure and log based on severity
			m.resultCollector.RecordFailure(filePath, err, context)
			logVerificationFailure(filePath, context, err, "File verification")
		}
		return nil
	}

	// In normal mode, return the error
	return err
}

// readAndVerifyFileWithFallback attempts file reading and verification with normal privileges first,
// then falls back to privileged access if permission errors occur
// In dry-run mode, it records the verification result without returning errors
func (m *Manager) readAndVerifyFileWithFallback(filePath string, context string) ([]byte, error) {
	if m.fileValidator == nil {
		// File validator is disabled - fallback to normal file reading
		// #nosec G304 - filePath comes from verified configuration and is sanitized by path resolver
		return os.ReadFile(filePath)
	}

	// Perform verification and reading
	content, err := m.fileValidator.VerifyAndRead(filePath)

	// In dry-run mode, record the result and handle differently
	if m.isDryRun && m.resultCollector != nil {
		if err == nil {
			m.resultCollector.RecordSuccess()
		} else {
			// Record failure and log based on severity
			m.resultCollector.RecordFailure(filePath, err, context)
			logVerificationFailure(filePath, context, err, "File verification and read")
		}

		// In dry-run mode, try to read the file even if verification failed
		if err != nil {
			// #nosec G304 - filePath comes from verified configuration
			content, err = os.ReadFile(filePath)
		}
	}

	return content, err
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
		validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
		if err != nil {
			// In dry-run mode, handle recoverable errors differently. Only keep going when
			// the error is considered recoverable; otherwise fail fast as before.
			if opts.isDryRun {
				if !shouldContinueOnValidatorError(err, hashDir) {
					return nil, fmt.Errorf("failed to initialize file validator: %w", err)
				}
			} else {
				return nil, fmt.Errorf("failed to initialize file validator: %w", err)
			}
		} else {
			manager.fileValidator = validator
		}
	}

	// Initialize security validator with default config
	securityConfig := security.DefaultConfig()
	securityValidator, err := security.NewValidator(securityConfig, security.WithFileSystem(opts.fs))
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

	// Initialize result collector for dry-run mode
	if opts.isDryRun {
		manager.resultCollector = NewResultCollector(hashDir)

		// Check if hash directory exists
		exists, err := opts.fs.FileExists(hashDir)
		switch {
		case err != nil:
			slog.Info("Unable to check hash directory existence in dry-run mode",
				"hash_directory", hashDir,
				"error", err)
			manager.resultCollector.SetHashDirStatus(false)
		case !exists:
			slog.Info("Hash directory does not exist in dry-run mode",
				"hash_directory", hashDir)
			manager.resultCollector.SetHashDirStatus(false)
		default:
			manager.resultCollector.SetHashDirStatus(true)
		}
	}

	return manager, nil
}

// shouldContinueOnValidatorError determines if execution should continue in dry-run mode
// when file validator initialization fails.
// Returns true for recoverable errors (directory not found, permission denied),
// false for configuration errors (invalid path, not a directory, etc.)
func shouldContinueOnValidatorError(err error, hashDir string) bool {
	// Check if hash directory does not exist - recoverable in dry-run mode
	if errors.Is(err, filevalidator.ErrHashDirNotExist) {
		slog.Info("Hash directory not found - skipping file verification",
			"hash_directory", hashDir,
			"mode", "dry-run",
			"error", err.Error())
		return true
	}

	// Check if permission denied - recoverable in dry-run mode
	if errors.Is(err, os.ErrPermission) {
		slog.Info("Hash directory permission denied - skipping file verification",
			"hash_directory", hashDir,
			"mode", "dry-run",
			"error", err.Error())
		return true
	}

	// For other errors (invalid path, not a directory, etc.), fail immediately
	// These indicate configuration problems that should be fixed
	slog.Error("File validator initialization failed with non-recoverable error",
		"hash_directory", hashDir,
		"mode", "dry-run",
		"error", err.Error())
	return false
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
