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
	if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
		elevationCtx := privilege.ElevationContext{
			Operation:   privilege.OperationFileHashCalculation,
			CommandName: "file_hash_record",
			FilePath:    filePath,
		}

		var result string
		err := v.privMgr.WithPrivileges(ctx, elevationCtx, func() error {
			var recordErr error
			result, recordErr = v.RecordWithOptions(filePath, force)
			return recordErr
		})
		if err != nil {
			return "", fmt.Errorf("privileged file hash recording failed: %w", err)
		}

		v.logger.Info("File hash recorded with privileges",
			"file_path", filePath,
			"hash", result,
			"force", force)

		return result, nil
	}

	// Standard recording without privileges
	result, err := v.RecordWithOptions(filePath, force)
	if err != nil {
		return "", fmt.Errorf("file hash recording failed: %w", err)
	}

	v.logger.Debug("File hash recorded",
		"file_path", filePath,
		"hash", result,
		"force", force)

	return result, nil
}

// VerifyWithPrivileges validates file hash with privilege elevation if needed
func (v *ValidatorWithPrivileges) VerifyWithPrivileges(
	ctx context.Context,
	filePath string,
	needsPrivileges bool,
) error {
	if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
		elevationCtx := privilege.ElevationContext{
			Operation:   privilege.OperationFileHashCalculation,
			CommandName: "file_hash_verify",
			FilePath:    filePath,
		}

		err := v.privMgr.WithPrivileges(ctx, elevationCtx, func() error {
			return v.Verify(filePath)
		})
		if err != nil {
			v.logger.Error("Privileged file hash verification failed",
				"file_path", filePath,
				"error", err)
			return fmt.Errorf("privileged file hash verification failed: %w", err)
		}

		v.logger.Info("File hash verified with privileges",
			"file_path", filePath)

		return nil
	}

	// Standard verification without privileges
	err := v.Verify(filePath)
	if err != nil {
		v.logger.Error("File hash verification failed",
			"file_path", filePath,
			"error", err)
		return fmt.Errorf("file hash verification failed: %w", err)
	}

	v.logger.Debug("File hash verified",
		"file_path", filePath)

	return nil
}

// ValidateFileHashWithPrivileges validates a file against an expected hash with privilege support
func (v *ValidatorWithPrivileges) ValidateFileHashWithPrivileges(
	ctx context.Context,
	filePath string,
	expectedHash string,
	needsPrivileges bool,
) error {
	if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
		elevationCtx := privilege.ElevationContext{
			Operation:   privilege.OperationFileHashCalculation,
			CommandName: "file_hash_validation",
			FilePath:    filePath,
		}

		err := v.privMgr.WithPrivileges(ctx, elevationCtx, func() error {
			return v.validateFileHash(filePath, expectedHash)
		})
		if err != nil {
			v.logger.Error("Privileged file hash validation failed",
				"file_path", filePath,
				"expected_hash", expectedHash,
				"error", err)
			return fmt.Errorf("privileged file hash validation failed: %w", err)
		}

		v.logger.Info("File hash validated with privileges",
			"file_path", filePath,
			"expected_hash", expectedHash)

		return nil
	}

	// Standard validation without privileges
	err := v.validateFileHash(filePath, expectedHash)
	if err != nil {
		v.logger.Error("File hash validation failed",
			"file_path", filePath,
			"expected_hash", expectedHash,
			"error", err)
		return fmt.Errorf("file hash validation failed: %w", err)
	}

	v.logger.Debug("File hash validated",
		"file_path", filePath,
		"expected_hash", expectedHash)

	return nil
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
