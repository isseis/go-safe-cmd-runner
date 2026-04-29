package verification

import (
	"log/slog"
	"reflect"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
)

// NewManagerForProduction creates a production verification manager.
func NewManagerForProduction(validator DirectoryValidator) (*Manager, error) {
	if isNilDirectoryValidator(validator) {
		return nil, ErrSecurityValidatorNotInitialized
	}

	// Log production manager creation for security audit trail
	logProductionManagerCreation()

	// Always use the default hash directory in production
	hashDir := cmdcommon.DefaultHashDirectory

	// Create manager with strict production constraints
	return newManagerInternal(hashDir,
		withCreationMode(CreationModeProduction),
		withSecurityLevel(SecurityLevelStrict),
		withDirectoryValidatorInternal(validator),
	)
}

func isNilDirectoryValidator(validator DirectoryValidator) bool {
	if validator == nil {
		return true
	}

	v := reflect.ValueOf(validator)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// NewManagerForDryRun creates a new verification manager for dry-run mode
// that skips hash directory validation since dry-run doesn't need actual file verification
func NewManagerForDryRun() (*Manager, error) {
	// Log dry-run manager creation for security audit trail
	logDryRunManagerCreation()

	// Always use the default hash directory in production
	hashDir := cmdcommon.DefaultHashDirectory

	// Create manager with dry-run constraints
	// File validator is enabled in dry-run mode to collect verification results
	return newManagerInternal(hashDir,
		withCreationMode(CreationModeProduction),
		withSecurityLevel(SecurityLevelStrict),
		withSkipHashDirectoryValidationInternal(),
		withDryRunModeInternal(),
	)
}

const (
	// callerDepthForNewManager is the stack depth passed to runtime.
	// Caller to obtain the caller of NewManagerForProduction.
	// Stack frames:
	//   0: runtime.Caller
	//   1: logProductionManagerCreation
	//   2: NewManagerForProduction (we want the caller of NewManagerForProduction)
	// If the call stack changes, this value may need to be updated.
	callerDepthForNewManager = 2

	// callerDepthForNewManagerForDryRun is the stack depth passed to runtime.
	// Caller to obtain the caller of NewManagerForDryRun.
	// Stack frames:
	//   0: runtime.Caller
	//   1: logDryRunManagerCreation
	//   2: NewManagerForDryRun (we want the caller of NewManagerForDryRun)
	// If the call stack changes, this value may need to be updated.
	callerDepthForNewManagerForDryRun = 2
)

// logProductionManagerCreation logs the creation of a production manager for security audit
func logProductionManagerCreation() {
	// Build base logging arguments
	args := []any{
		"api", "NewManagerForProduction",
		"hash_directory", cmdcommon.DefaultHashDirectory,
		"security_level", "strict",
	}

	// Add caller information if available
	if _, file, line, ok := runtime.Caller(callerDepthForNewManager); ok {
		args = append(args, "caller_file", file, "caller_line", line)
	}

	slog.Info("Production verification manager created", args...)
}

// logDryRunManagerCreation logs the creation of a dry-run manager for security audit
func logDryRunManagerCreation() {
	// Build base logging arguments
	args := []any{
		"api", "NewManagerForDryRun",
		"hash_directory", cmdcommon.DefaultHashDirectory,
		"security_level", "strict",
		"mode", "dry-run",
		"skip_hash_directory_validation", true,
		"file_validator_enabled", true,
	}

	// Add caller information if available
	if _, file, line, ok := runtime.Caller(callerDepthForNewManagerForDryRun); ok {
		args = append(args, "caller_file", file, "caller_line", line)
	}

	slog.Info("Dry-run verification manager created", args...)
}

// validateProductionConstraints validates that production security constraints are met
func validateProductionConstraints(hashDir string) error {
	// In production, only the default hash directory is allowed
	if hashDir != cmdcommon.DefaultHashDirectory {
		return NewHashDirectorySecurityError(
			hashDir,
			cmdcommon.DefaultHashDirectory,
			"production environment requires default hash directory",
		)
	}

	// Additional checks are redundant here since ValidateHashDirectory is
	// called when manager is used.
	slog.Debug("Production constraints validated successfully",
		"hash_directory", hashDir,
		"default_directory", cmdcommon.DefaultHashDirectory)

	return nil
}
