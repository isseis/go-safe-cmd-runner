package filevalidator

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
)

// PrivilegeError represents privilege-related errors
type PrivilegeError struct {
	Operation string // "escalate" or "restore"
	UID       int
	Cause     error
}

func (e *PrivilegeError) Error() string {
	return fmt.Sprintf("privilege %s failed for UID %d: %v", e.Operation, e.UID, e.Cause)
}

func (e *PrivilegeError) Unwrap() error {
	return e.Cause
}

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
func OpenFileWithPrivileges(filepath string) (*os.File, error) {
	// まず通常権限でのアクセスを試行
	file, err := os.Open(filepath) //nolint:gosec // filepath is validated by caller
	if err == nil {
		return file, nil
	}

	// 権限エラーでない場合は、権限昇格しても解決しない
	if !os.IsPermission(err) {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}

	// 現在のUIDを保存
	originalUID := os.Getuid()

	// 既にrootの場合は権限昇格不要
	if originalUID == 0 {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}

	// 権限昇格
	if err := syscall.Seteuid(0); err != nil {
		return nil, &PrivilegeError{
			Operation: "escalate",
			UID:       0,
			Cause:     err,
		}
	}

	// deferで確実に権限復元
	defer func() {
		if restoreErr := syscall.Seteuid(originalUID); restoreErr != nil {
			slog.Error("Failed to restore privileges",
				slog.String("error", restoreErr.Error()),
				slog.String("file", filepath))
		}
	}()

	// ファイルオープン（権限昇格状態）
	file, err = os.Open(filepath) //nolint:gosec // filepath is validated by caller
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}

	return file, nil
}

// needsPrivileges determines if a file requires privilege escalation to access
func needsPrivileges(filepath string) bool {
	// ファイルアクセステストで権限必要性を判定
	_, err := os.Open(filepath) //nolint:gosec // filepath is validated by caller
	return os.IsPermission(err)
}

// IsPrivilegeError checks if error is a privilege-related error
func IsPrivilegeError(err error) bool {
	var privErr *PrivilegeError
	return errors.As(err, &privErr)
}
