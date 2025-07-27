package filevalidator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// ValidatorWithPrivileges extends the base Validator with privilege management capabilities
type ValidatorWithPrivileges struct {
	*Validator
	privMgr      runnertypes.PrivilegeManager
	logger       *slog.Logger
	secValidator *security.Validator
}

// Error definitions for privileged validation
var (
	ErrHashValidationFailed = errors.New("hash validation failed")
)

// NewValidatorWithPrivileges creates a new validator with privilege management support
func NewValidatorWithPrivileges(
	algorithm HashAlgorithm,
	hashDir string,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) (*ValidatorWithPrivileges, error) {
	return NewValidatorWithPrivilegesAndLogging(algorithm, hashDir, privMgr, logger, security.DefaultLoggingOptions())
}

// NewValidatorWithPrivilegesAndLogging creates a new validator with custom logging options
func NewValidatorWithPrivilegesAndLogging(
	algorithm HashAlgorithm,
	hashDir string,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
	loggingOpts security.LoggingOptions,
) (*ValidatorWithPrivileges, error) {
	baseValidator, err := New(algorithm, hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create base validator: %w", err)
	}

	// Create security validator with logging options
	secConfig := security.DefaultConfig()
	secConfig.LoggingOptions = loggingOpts
	secValidator, err := security.NewValidator(secConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create security validator: %w", err)
	}

	return &ValidatorWithPrivileges{
		Validator:    baseValidator,
		privMgr:      privMgr,
		logger:       logger,
		secValidator: secValidator,
	}, nil
}

// RecordWithPrivileges calculates and records file hash with privilege elevation if needed
func (v *ValidatorWithPrivileges) RecordWithPrivileges(
	ctx context.Context,
	filePath string,
	needsPrivileges bool,
	force bool,
) (string, error) {
	var result string
	logFields := map[string]any{
		"force": force,
	}

	// Define single action that sets the result and logs the hash value
	action := func() error {
		var recordErr error
		result, recordErr = v.RecordWithOptions(filePath, force)
		if recordErr == nil {
			logFields["hash"] = result
		}
		return recordErr
	}

	err := v.executeWithPrivilegesIfNeeded(
		ctx,
		filePath,
		needsPrivileges,
		runnertypes.OperationFileHashCalculation,
		"file_hash_record",
		action,
		"File hash recorded with privileges",
		"file hash recording",
		logFields,
	)
	if err != nil {
		return "", err
	}

	return result, nil
}

// VerifyWithPrivileges validates file hash with privilege elevation if needed
func (v *ValidatorWithPrivileges) VerifyWithPrivileges(
	ctx context.Context,
	filePath string,
	needsPrivileges bool,
) error {
	return v.executeWithPrivilegesIfNeeded(
		ctx,
		filePath,
		needsPrivileges,
		runnertypes.OperationFileHashCalculation,
		"file_hash_verify",
		func() error { return v.Verify(filePath) },
		"File hash verified with privileges",
		"file hash verification",
		map[string]any{},
	)
}

// executeWithPrivilegesIfNeeded is a helper method that encapsulates the common privilege execution logic
func (v *ValidatorWithPrivileges) executeWithPrivilegesIfNeeded(
	ctx context.Context,
	filePath string,
	needsPrivileges bool,
	operation runnertypes.Operation,
	commandName string,
	action func() error,
	successMsg string,
	failureMsg string,
	logFields map[string]any,
) error {
	var err error
	wasPrivileged := false

	// Execute action with or without privileges
	if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
		elevationCtx := runnertypes.ElevationContext{
			Operation:   operation,
			CommandName: commandName,
			FilePath:    filePath,
		}
		err = v.privMgr.WithPrivileges(ctx, elevationCtx, action)
		wasPrivileged = true
	} else {
		err = action()
	}

	// Build safe log arguments with sensitive data protection
	logArgs := []any{"file_path", filePath}

	// Create safe log fields
	safeFields := v.secValidator.CreateSafeLogFields(logFields)
	if err != nil {
		safeFields["error"] = v.secValidator.SanitizeErrorForLogging(err)
	}

	// Convert safe fields to log arguments
	for k, v := range safeFields {
		logArgs = append(logArgs, k, v)
	}

	// Log and return based on result
	if err != nil {
		v.logger.Error(failureMsg, logArgs...)
		if wasPrivileged {
			return fmt.Errorf("privileged %s failed: %w", failureMsg, err)
		}
		return fmt.Errorf("%s failed: %w", failureMsg, err)
	}

	// Log success with safe fields
	if wasPrivileged {
		v.logger.Info(successMsg, logArgs...)
	} else {
		v.logger.Debug(successMsg, logArgs...)
	}

	return nil
}

// ValidateFileHashWithPrivileges validates a file against an expected hash with privilege support
func (v *ValidatorWithPrivileges) ValidateFileHashWithPrivileges(
	ctx context.Context,
	filePath string,
	expectedHash string,
	needsPrivileges bool,
) error {
	logFields := map[string]any{
		"expected_hash": expectedHash,
	}

	return v.executeWithPrivilegesIfNeeded(
		ctx,
		filePath,
		needsPrivileges,
		runnertypes.OperationFileHashCalculation,
		"file_hash_validation",
		func() error {
			actualHash, err := v.validateFileHashWithLogging(filePath, expectedHash)
			if actualHash != "" {
				logFields["actual_hash"] = actualHash
			}
			return err
		},
		"File hash validated with privileges",
		"file hash validation",
		logFields,
	)
}

// validateFileHashWithLogging is a helper method that validates a file hash and returns the actual hash
func (v *ValidatorWithPrivileges) validateFileHashWithLogging(filePath string, expectedHash string) (string, error) {
	// Validate the file path first
	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", fmt.Errorf("file path validation failed: %w", err)
	}

	// #nosec G304 - filePath is validated by validatePath above
	file, err := os.Open(targetPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			v.logger.Warn("Failed to close file", "file_path", targetPath, "error", closeErr)
		}
	}()

	actualHash, err := v.algorithm.Sum(file)
	if err != nil {
		return "", fmt.Errorf("failed to calculate file hash: %w", err)
	}

	if actualHash != expectedHash {
		return actualHash, fmt.Errorf("%w: expected %s, got %s", ErrHashValidationFailed, expectedHash, actualHash)
	}

	return actualHash, nil
}
