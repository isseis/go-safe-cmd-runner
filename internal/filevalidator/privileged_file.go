package filevalidator

import (
	"errors"
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
// This function uses the existing privilege management infrastructure
func OpenFileWithPrivileges(filepath string, privManager runnertypes.PrivilegeManager) (*os.File, error) {
	// まず通常権限でのアクセスを試行
	file, err := os.Open(filepath) //nolint:gosec // filepath is validated by caller
	if err == nil {
		return file, nil
	}

	// 権限エラーでない場合は、権限昇格しても解決しない
	if !os.IsPermission(err) {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}

	// PrivilegeManager が提供されていない場合はエラー
	if privManager == nil {
		return nil, fmt.Errorf("failed to open file %s: privilege manager not available: %w", filepath, err)
	}

	// 権限昇格がサポートされているかチェック
	if !privManager.IsPrivilegedExecutionSupported() {
		return nil, fmt.Errorf("failed to open file %s: privileged execution not supported: %w", filepath, err)
	}

	var privilegedFile *os.File
	privErr := privManager.WithPrivileges(runnertypes.ElevationContext{
		Operation: runnertypes.OperationFileValidation,
		FilePath:  filepath,
	}, func() error {
		var openErr error
		privilegedFile, openErr = os.Open(filepath) //nolint:gosec // filepath is validated by caller
		return openErr
	})

	if privErr != nil {
		return nil, fmt.Errorf("failed to open file %s with privileges: %w", filepath, privErr)
	}

	return privilegedFile, nil
}

// IsPrivilegeError checks if error is a privilege-related error
// This function now uses the existing privilege management error handling
func IsPrivilegeError(err error) bool {
	return errors.Is(err, runnertypes.ErrPrivilegedExecutionNotAvailable)
}
