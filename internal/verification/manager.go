package verification

import (
	"debug/elf"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/shebang"
)

// Manager provides file verification capabilities
type Manager struct {
	hashDir                     string
	fs                          common.FileSystem
	safeFS                      safefileio.FileSystem // used for secure file I/O (e.g. ELF inspection)
	fileValidator               filevalidator.FileValidator
	networkSymbolStore          fileanalysis.NetworkSymbolStore // nil when cache is unavailable
	syscallAnalysisStore        fileanalysis.SyscallAnalysisStore
	dynlibVerifier              *elfdynlib.DynLibVerifier // initialized once at construction
	security                    *security.Validator
	pathResolver                *PathResolver
	isDryRun                    bool
	skipHashDirectoryValidation bool
	resultCollector             *ResultCollector
}

// VerifyAndReadConfigFile performs atomic verification and reading of a configuration file
// This prevents TOCTOU attacks by reading the file content once and verifying it against the hash
func (m *Manager) VerifyAndReadConfigFile(configPath string) ([]byte, error) {
	return m.verifyAndReadFile(configPath, "config")
}

// VerifyAndReadTemplateFile performs atomic verification and reading of a template file
func (m *Manager) VerifyAndReadTemplateFile(templatePath string) ([]byte, error) {
	return m.verifyAndReadFile(templatePath, "template")
}

// verifyAndReadFile is a private helper method that performs atomic verification and reading
// of files. It handles hash directory validation, file reading, and comprehensive logging
// for both configuration and template files to prevent TOCTOU attacks.
func (m *Manager) verifyAndReadFile(filePath string, fileType string) ([]byte, error) {
	slog.Debug("Starting atomic file verification and reading",
		"file_path", filePath,
		"file_type", fileType,
		"hash_directory", m.hashDir)

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	// Read and verify file content atomically using filevalidator
	content, err := m.readAndVerifyFileWithReadFallback(filePath, fileType)
	if err != nil {
		slog.Error("File verification and reading failed",
			"file_path", filePath,
			"file_type", fileType,
			"error", err)
		return nil, &OpError{
			Op:   "ReadAndVerifyHash",
			Path: filePath,
			Err:  err,
		}
	}

	slog.Info("File verification and reading completed successfully",
		"file_path", filePath,
		"file_type", fileType,
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
	if err := m.verifyFile(envFilePath, "env"); err != nil {
		// In dry-run mode, the failure is already recorded and logged by
		// verifyFile; treat it as non-fatal here.
		if m.isDryRun {
			return nil
		}
		slog.Error("Environment file verification failed",
			"env_file_path", envFilePath,
			"error", err)
		return &OpError{
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
	// Skip hash directory validation if explicitly requested or in dry-run mode
	if m.skipHashDirectoryValidation || m.isDryRun {
		slog.Debug("Skipping hash directory validation",
			"hash_directory", m.hashDir,
			"skip_validation", m.skipHashDirectoryValidation,
			"dry_run", m.isDryRun)
		return nil
	}

	if m.security == nil {
		return ErrSecurityValidatorNotInitialized
	}

	// Validate directory permissions using security validator
	if err := m.security.ValidateDirectoryPermissions(m.hashDir); err != nil {
		return fmt.Errorf("hash directory validation failed: %w", err)
	}

	return nil
}

// ensureHashDirectoryValidated calls ValidateHashDirectory and wraps any error
// into the package OpError type used by Manager public methods.
func (m *Manager) ensureHashDirectoryValidated() error {
	if err := m.ValidateHashDirectory(); err != nil {
		return &OpError{
			Op:   "ValidateHashDirectory",
			Path: m.hashDir,
			Err:  err,
		}
	}
	return nil
}

// VerifyGlobalFiles verifies the integrity of global files.
func (m *Manager) VerifyGlobalFiles(input *GlobalVerificationInput) (*Result, error) {
	if input == nil {
		return nil, ErrConfigNil
	}

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	result := &Result{
		TotalFiles:  len(input.ExpandedVerifyFiles),
		FailedFiles: []string{},
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	for _, filePath := range input.ExpandedVerifyFiles {
		// Verify file hash (try normal verification first, then with privileges if needed)
		if err := m.verifyFile(filePath, "global"); err != nil {
			result.FailedFiles = append(result.FailedFiles, filePath)
			slog.Error("Global file verification failed",
				"file", filePath,
				"error", err)
		} else {
			result.VerifiedFiles++
		}
	}

	if len(result.FailedFiles) > 0 {
		// In dry-run mode, failures are already recorded in the ResultCollector and
		// logged by verifyFile.  Return the accurate result without
		// treating the failures as fatal.
		if m.isDryRun {
			return result, nil
		}
		slog.Error("CRITICAL: Global file verification failed - program will terminate",
			"failed_files", result.FailedFiles,
			"verified_files", result.VerifiedFiles,
			"total_files", result.TotalFiles)
		return nil, &Error{
			Op:            "global",
			Details:       result.FailedFiles,
			TotalFiles:    result.TotalFiles,
			VerifiedFiles: result.VerifiedFiles,
			FailedFiles:   len(result.FailedFiles),
			Err:           ErrGlobalVerificationFailed,
		}
	}

	return result, nil
}

// VerifyGroupFiles verifies the integrity of group files.
func (m *Manager) VerifyGroupFiles(input *GroupVerificationInput) (*Result, error) {
	if input == nil {
		return nil, ErrConfigNil
	}

	// Ensure hash directory is validated
	if err := m.ensureHashDirectoryValidated(); err != nil {
		return nil, err
	}

	// Collect all files to verify (explicit files + command files)
	allFiles := m.collectVerificationFiles(input)

	result := &Result{
		TotalFiles:    len(allFiles),
		FailedFiles:   []string{},
		ContentHashes: make(map[string]string),
	}

	start := time.Now()
	defer func() {
		result.Duration = time.Since(start)
	}()

	groupName := input.Name

	for file := range allFiles {
		// Verify file hash and collect the computed hash for downstream consumers.
		contentHash, err := m.verifyFileWithHash(file, "group:"+groupName)
		if err != nil {
			result.FailedFiles = append(result.FailedFiles, file)
			slog.Error("Group file verification failed",
				"group", groupName,
				"file", file,
				"error", err)
		} else {
			result.VerifiedFiles++
			if contentHash != "" {
				result.ContentHashes[file] = contentHash
			}
		}
	}

	if len(result.FailedFiles) > 0 {
		// In dry-run mode, failures are already recorded in the ResultCollector and
		// logged by verifyFileWithHash.  Return the accurate result without treating
		// the failures as fatal.
		if m.isDryRun {
			return result, nil
		}
		return nil, &Error{
			Op:            "group",
			Group:         groupName,
			Details:       result.FailedFiles,
			TotalFiles:    result.TotalFiles,
			VerifiedFiles: result.VerifiedFiles,
			FailedFiles:   len(result.FailedFiles),
			Err:           ErrGroupVerificationFailed,
		}
	}

	return result, nil
}

// collectVerificationFiles collects all files to verify for a group.
func (m *Manager) collectVerificationFiles(input *GroupVerificationInput) map[string]struct{} {
	if input == nil {
		return make(map[string]struct{})
	}

	// Use map to automatically eliminate duplicates
	fileSet := make(map[string]struct{}, len(input.ExpandedVerifyFiles)+len(input.Commands))

	// Add explicit files with variables expanded
	for _, file := range input.ExpandedVerifyFiles {
		fileSet[file] = struct{}{}
	}

	// Add command files from pre-expanded runtime commands
	if m.pathResolver != nil && len(input.Commands) > 0 {
		for _, command := range input.Commands {
			// Use pre-expanded command path
			resolvedPath, err := m.pathResolver.ResolvePath(command.ExpandedCmd)
			if err != nil {
				slog.Warn("Failed to resolve command path",
					"group", input.Name,
					"command", command.ExpandedCmd,
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

	// Note: Command allowlist validation is performed by the caller (GroupExecutor)
	// after path resolution, using ValidateCommandAllowed() which checks both
	// global patterns and group-level cmd_allowed configuration.

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

// GetNetworkSymbolStore returns a NetworkSymbolStore backed by the same hash directory,
// or nil if not available (e.g. when fileValidator is a test mock or hash dir is absent).
func (m *Manager) GetNetworkSymbolStore() fileanalysis.NetworkSymbolStore {
	return m.networkSymbolStore
}

// GetSyscallAnalysisStore returns a SyscallAnalysisStore backed by the same hash
// directory, or nil if not available (e.g. when fileValidator is a test mock or
// hash dir is absent).
func (m *Manager) GetSyscallAnalysisStore() fileanalysis.SyscallAnalysisStore {
	return m.syscallAnalysisStore
}

// verifyFile attempts file verification using the configured fileValidator.
// In dry-run mode it records the result in the ResultCollector and logs the
// failure, but still returns the underlying error so callers can track
// accurate per-file success/failure counts.  Callers are responsible for
// suppressing fatality in dry-run mode.
func (m *Manager) verifyFile(filePath string, context string) error {
	if m.fileValidator == nil {
		// File validator is disabled - skip verification
		return nil
	}

	// Perform verification
	err := m.fileValidator.Verify(filePath)

	// In dry-run mode, record the result (warn-only mode).
	// The error is still returned so callers can count failures accurately.
	if m.isDryRun && m.resultCollector != nil {
		if err == nil {
			m.resultCollector.RecordSuccess()
		} else {
			m.resultCollector.RecordFailure(filePath, err, context)
			logVerificationFailure(filePath, context, err, "File verification")
		}
	}

	return err
}

// verifyFileWithHash verifies the file and returns the computed content hash on success.
// It mirrors verifyFile but also returns the hash so callers can forward
// it to downstream consumers (e.g. ELF analysis) to avoid re-reading the file.
// Returns ("", nil) when the file validator is disabled.
// In dry-run mode it records the result and logs failures, but still returns the
// underlying error and hash so callers can track accurate counts.
func (m *Manager) verifyFileWithHash(filePath string, context string) (string, error) {
	if m.fileValidator == nil {
		return "", nil
	}

	contentHash, err := m.fileValidator.VerifyWithHash(filePath)

	// In dry-run mode, record the result (warn-only mode).
	// The error and hash are still returned so callers can count failures accurately.
	if m.isDryRun && m.resultCollector != nil {
		if err == nil {
			m.resultCollector.RecordSuccess()
		} else {
			m.resultCollector.RecordFailure(filePath, err, context)
			logVerificationFailure(filePath, context, err, "File verification")
		}
	}

	if err != nil {
		return "", err
	}
	return contentHash, nil
}

// readAndVerifyFileWithReadFallback attempts file reading and verification.
// It has two os.ReadFile fallback paths:
//  1. When m.fileValidator == nil (file validation is disabled): verification is
//     skipped and the file is read directly via os.ReadFile.
//  2. In dry-run mode when verification fails: the failure is recorded in the
//     ResultCollector and logged, then os.ReadFile is used to re-attempt reading
//     the file so that callers can still process the content.
//
// "WithReadFallback" refers to both of the above "fall back to file reading"
// behaviors taken as a whole.
func (m *Manager) readAndVerifyFileWithReadFallback(filePath string, context string) ([]byte, error) {
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

	safeFS := safefileio.NewFileSystem(safefileio.FileSystemConfig{})

	manager := &Manager{
		hashDir:                     hashDir,
		fs:                          opts.fs,
		safeFS:                      safeFS,
		isDryRun:                    opts.isDryRun,
		skipHashDirectoryValidation: opts.skipHashDirectoryValidation,
	}

	// Initialize dynamic library verifier (parses /etc/ld.so.cache once at startup).
	manager.dynlibVerifier = elfdynlib.NewDynLibVerifier(safeFS)

	// Initialize file validator with hybrid hash path getter
	if opts.fileValidatorEnabled {
		validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
		if err != nil {
			// In dry-run mode, a permission error creating the hash directory is
			// recoverable: the operator may be checking configuration on a machine
			// where the hash directory is not writable (e.g. CI without sudo).
			// Binary analysis will be skipped for commands without a content hash,
			// but dry-run output remains useful for configuration validation.
			// All other errors (invalid path, not a directory, etc.) are fatal
			// in both modes.
			if opts.isDryRun && errors.Is(err, os.ErrPermission) {
				slog.Info("Hash directory not writable in dry-run mode; file verification and binary analysis will be skipped",
					"hash_directory", hashDir)
			} else {
				return nil, fmt.Errorf("failed to initialize file validator: %w", err)
			}
		} else {
			manager.fileValidator = validator
			if s := validator.Store(); s != nil {
				manager.networkSymbolStore = fileanalysis.NewNetworkSymbolStore(s)
				manager.syscallAnalysisStore = fileanalysis.NewSyscallAnalysisStore(s)
			}
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
		pathResolver = NewPathResolver(security.SecurePathEnv, securityValidator)
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

// VerifyCommandDynLibDeps performs dynamic library integrity verification for a command binary.
// It is called separately from VerifyGroupFiles to avoid the need to track
// which files in the verification set are command files vs explicit verify_files entries.
func (m *Manager) VerifyCommandDynLibDeps(cmdPath string) error {
	return m.verifyDynLibDeps(cmdPath)
}

// verifyDynLibDeps performs dynamic library integrity verification when a
// DynLibDeps snapshot is present in the analysis record.
func (m *Manager) verifyDynLibDeps(cmdPath string) error {
	if m.fileValidator == nil {
		return nil
	}

	record, err := m.fileValidator.LoadRecord(cmdPath)
	if err != nil {
		// No hash record: binary is not hash-verified, so dynlib check is not applicable.
		if errors.Is(err, fileanalysis.ErrRecordNotFound) {
			return nil
		}
		// Old schema record (schema_version < CurrentSchemaVersion): predates dynlib
		// tracking. Treat as no DynLibDeps data available and skip the check.
		// Records with a newer schema (Actual > Expected) are rejected as usual.
		if schemaErr, ok := errors.AsType[*fileanalysis.SchemaVersionMismatchError](err); ok && schemaErr.Actual < schemaErr.Expected {
			slog.Warn("Skipping dynlib verification: record predates dynlib tracking; re-run 'record' to enable",
				"cmd_path", cmdPath,
				"record_schema_version", schemaErr.Actual,
				"current_schema_version", schemaErr.Expected)
			return nil
		}
		return fmt.Errorf("failed to load record for dynlib verification: %w", err)
	}

	if len(record.DynLibDeps) > 0 {
		// DynLibDeps is recorded: verify library hashes.
		// m.dynlibVerifier is initialized once at Manager construction.
		return m.dynlibVerifier.Verify(record.DynLibDeps)
	}

	// DynLibDeps is not recorded: check if this is a dynamically linked ELF binary.
	hasDynDeps, err := m.hasDynamicLibraryDeps(cmdPath)
	if err != nil {
		return fmt.Errorf("failed to check dynamic library dependencies: %w", err)
	}

	if hasDynDeps {
		// ELF binary without DynLibDeps record → requires re-recording.
		return &dynlib.ErrDynLibDepsRequired{BinaryPath: cmdPath}
	}

	// Check if this is a dynamically linked Mach-O binary.
	hasMachODeps, err := m.hasMachODynamicLibraryDeps(cmdPath)
	if err != nil {
		return fmt.Errorf("failed to check Mach-O dynamic library dependencies: %w", err)
	}

	if hasMachODeps {
		// Mach-O binary without DynLibDeps record → requires re-recording.
		return &dynlib.ErrDynLibDepsRequired{BinaryPath: cmdPath}
	}

	// Non-ELF, non-Mach-O binary (or static/no-dependency binary) without DynLibDeps → normal.
	return nil
}

// hasDynamicLibraryDeps checks if the file at the given path is an ELF binary
// that has at least one DT_NEEDED entry (i.e., dynamically linked).
// Static ELFs and ELFs with no DT_NEEDED entries return (false, nil).
//
// Errors are classified as follows:
//   - SafeOpenFile failure (I/O error, permission denied, file not found) → (false, err): propagated to caller
//   - elf.NewFile failure (not an ELF, bad magic)                         → (false, nil): file is not an ELF binary
func (m *Manager) hasDynamicLibraryDeps(path string) (bool, error) {
	file, err := m.safeFS.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		// I/O error or permission denied: propagate, do not silently skip dynlib check.
		return false, fmt.Errorf("failed to open binary for ELF inspection: %w", err)
	}
	defer func() { _ = file.Close() }()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		// Not an ELF binary (bad magic, unsupported format, etc.).
		return false, nil
	}
	defer func() { _ = elfFile.Close() }()

	needed, err := elfFile.DynString(elf.DT_NEEDED)
	if err != nil || len(needed) == 0 {
		return false, nil
	}
	return true, nil
}

// hasMachODynamicLibraryDeps checks if the file at the given path is a Mach-O
// binary that has at least one LC_LOAD_DYLIB or LC_LOAD_WEAK_DYLIB entry
// pointing to a non-dyld-shared-cache library.
//
// Returns (false, nil) for non-Mach-O files and binaries whose dependencies
// are all in the dyld shared cache (absent from disk).
func (m *Manager) hasMachODynamicLibraryDeps(path string) (bool, error) {
	return machodylib.HasDynamicLibDeps(path, m.safeFS)
}

// VerifyCommandShebangInterpreter verifies the integrity of a shebang interpreter for a
// script command. It loads the analysis record for cmdPath, reads the ShebangInterpreter
// field, and verifies that:
//   - The interpreter binary's hash matches the stored record.
//   - For env-form shebangs, the command name resolves (via envVars["PATH"]) to the same
//     binary path that was recorded at record time.
//
// Returns nil if cmdPath has no analysis record or the record has no ShebangInterpreter.
func (m *Manager) VerifyCommandShebangInterpreter(cmdPath string, envVars map[string]string) error {
	if m.fileValidator == nil {
		return nil
	}

	record, err := m.fileValidator.LoadRecord(cmdPath)
	if err != nil {
		if errors.Is(err, fileanalysis.ErrRecordNotFound) {
			return nil
		}
		if schemaErr, ok := errors.AsType[*fileanalysis.SchemaVersionMismatchError](err); ok && schemaErr.Actual < schemaErr.Expected {
			// Old schema record (pre-shebang tracking): reject so callers that
			// invoke shebang verification directly (bypassing VerifyGroupFiles)
			// still enforce the schema version.
			return err
		}
		return fmt.Errorf("failed to load record for shebang verification: %w", err)
	}

	si := record.ShebangInterpreter
	if si == nil {
		return nil
	}

	// Re-resolve the raw shebang path and verify it still points to the same binary
	// that was recorded. This detects symlink redirection (e.g., /bin/sh redirected
	// to a different interpreter). Only checked when the field is present (schema 12+).
	if si.RawInterpreterPath != "" {
		if err := m.verifyInterpreterSymlinkTarget(si.RawInterpreterPath, si.InterpreterPath); err != nil {
			return err
		}
	}

	// Verify that the recorded interpreter binary still exists and matches its hash.
	if err := m.verifyInterpreterHash(si.InterpreterPath); err != nil {
		return err
	}

	if si.CommandName != "" {
		// Verify the resolved command binary before PATH re-resolution so that a
		// missing resolved_path record is reported as ErrInterpreterRecordNotFound
		// rather than being masked by a subsequent path mismatch error.
		if err := m.verifyInterpreterHash(si.ResolvedPath); err != nil {
			return err
		}
		// Verify that the runtime PATH resolves the command to the recorded binary.
		if err := m.verifyEnvPathResolution(si.CommandName, si.ResolvedPath, envVars); err != nil {
			return err
		}
	}

	return nil
}

// verifyInterpreterHash verifies the hash of the given interpreter binary.
// ErrHashFileNotFound (no record for that binary) is translated into
// *ErrInterpreterRecordNotFound so callers can distinguish "never recorded"
// from "tampered" (ErrMismatch).
func (m *Manager) verifyInterpreterHash(interpreterPath string) error {
	err := m.fileValidator.Verify(interpreterPath)
	if err == nil {
		return nil
	}
	if errors.Is(err, filevalidator.ErrHashFileNotFound) {
		return &ErrInterpreterRecordNotFound{Path: interpreterPath}
	}
	return err
}

// verifyEnvPathResolution resolves commandName through envVars["PATH"] and checks
// verifyInterpreterSymlinkTarget re-resolves rawPath via EvalSymlinks and checks
// that it still points to recordedResolvedPath. Returns *ErrInterpreterSymlinkRedirected
// when they differ, detecting symlink-redirection attacks.
func (m *Manager) verifyInterpreterSymlinkTarget(rawPath, recordedResolvedPath string) error {
	actual, err := filepath.EvalSymlinks(rawPath)
	if err != nil {
		return fmt.Errorf("failed to resolve interpreter path %q: %w", rawPath, err)
	}
	if actual != recordedResolvedPath {
		return &ErrInterpreterSymlinkRedirected{
			RawPath:      rawPath,
			RecordedPath: recordedResolvedPath,
			ActualPath:   actual,
		}
	}
	return nil
}

// that the result (after symlink resolution) matches recordedResolvedPath.
// Returns *ErrInterpreterPathMismatch when they differ.
func (m *Manager) verifyEnvPathResolution(commandName, recordedResolvedPath string, envVars map[string]string) error {
	resolved, err := shebang.ResolveEnvCommand(commandName, envVars["PATH"])
	if err != nil {
		return fmt.Errorf("cannot resolve interpreter %q in PATH: %w", commandName, err)
	}

	if resolved != recordedResolvedPath {
		return &ErrInterpreterPathMismatch{
			CommandName:  commandName,
			RecordedPath: recordedResolvedPath,
			ActualPath:   resolved,
		}
	}
	return nil
}
