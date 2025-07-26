package filevalidator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

// ValidatorWithPrivileges extends the base Validator with privilege management capabilities
type ValidatorWithPrivileges struct {
	*Validator
	privMgr privilege.Manager
	logger  *slog.Logger
}

// Error definitions for privileged validation
var (
	ErrHashValidationFailed = errors.New("hash validation failed")
)

// NewValidatorWithPrivileges creates a new validator with privilege management support
func NewValidatorWithPrivileges(
	algorithm HashAlgorithm,
	hashDir string,
	privMgr privilege.Manager,
	logger *slog.Logger,
) (*ValidatorWithPrivileges, error) {
	baseValidator, err := New(algorithm, hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create base validator: %w", err)
	}

	return &ValidatorWithPrivileges{
		Validator: baseValidator,
		privMgr:   privMgr,
		logger:    logger,
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

	// Define callbacks that both set the result and can be used to log the hash value
	privilegedAction := func() error {
		var recordErr error
		result, recordErr = v.RecordWithOptions(filePath, force)
		if recordErr == nil {
			logFields["hash"] = result
		}
		return recordErr
	}

	nonPrivilegedAction := func() error {
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
		privilege.OperationFileHashCalculation,
		"file_hash_record",
		privilegedAction,
		nonPrivilegedAction,
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
		privilege.OperationFileHashCalculation,
		"file_hash_verify",
		func() error { return v.Verify(filePath) },
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
	operation privilege.Operation,
	commandName string,
	privilegedAction func() error,
	nonPrivilegedAction func() error,
	successMsg string,
	failureMsg string,
	logFields map[string]any,
) error {
	if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
		elevationCtx := privilege.ElevationContext{
			Operation:   operation,
			CommandName: commandName,
			FilePath:    filePath,
		}

		err := v.privMgr.WithPrivileges(ctx, elevationCtx, privilegedAction)
		if err != nil {
			// Build error log args
			logArgs := []any{
				"file_path", filePath,
				"error", err,
			}
			for k, v := range logFields {
				logArgs = append(logArgs, k, v)
			}
			v.logger.Error(failureMsg, logArgs...)
			return fmt.Errorf("privileged %s failed: %w", failureMsg, err)
		}

		// Build success log args
		logArgs := []any{
			"file_path", filePath,
		}
		for k, v := range logFields {
			logArgs = append(logArgs, k, v)
		}
		v.logger.Info(successMsg, logArgs...)

		return nil
	}

	// Standard execution without privileges
	err := nonPrivilegedAction()
	if err != nil {
		// Build error log args for non-privileged failure
		logArgs := []any{
			"file_path", filePath,
			"error", err,
		}
		for k, v := range logFields {
			logArgs = append(logArgs, k, v)
		}
		v.logger.Error(failureMsg, logArgs...)
		return fmt.Errorf("%s failed: %w", failureMsg, err)
	}

	// Build debug log args for non-privileged success
	logArgs := []any{
		"file_path", filePath,
	}
	for k, v := range logFields {
		logArgs = append(logArgs, k, v)
	}
	v.logger.Debug(successMsg, logArgs...)

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
		privilege.OperationFileHashCalculation,
		"file_hash_validation",
		func() error { return v.validateFileHash(filePath, expectedHash) },
		func() error { return v.validateFileHash(filePath, expectedHash) },
		"File hash validated with privileges",
		"file hash validation",
		logFields,
	)
}

// validateFileHash is a helper method for direct hash validation
func (v *ValidatorWithPrivileges) validateFileHash(filePath string, expectedHash string) error {
	// #nosec G304 - filePath is validated by caller and comes from trusted sources
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			v.logger.Warn("Failed to close file", "file_path", filePath, "error", closeErr)
		}
	}()

	actualHash, err := v.algorithm.Sum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("%w: expected %s, got %s", ErrHashValidationFailed, expectedHash, actualHash)
	}

	return nil
}
