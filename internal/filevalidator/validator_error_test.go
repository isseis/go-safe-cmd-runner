//go:build linux || freebsd || openbsd || netbsd

package filevalidator

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// TestErrorCases tests various error conditions and their messages
func TestErrorCases(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name        string
		setup       func() (string, error)
		wantErr     error
		errContains string
	}{
		{
			name: "non-existent file",
			setup: func() (string, error) {
				return filepath.Join(tempDir, "nonexistent.txt"), nil
			},
			wantErr:     os.ErrNotExist,
			errContains: "no such file or directory",
		},
		{
			name: "empty file path",
			setup: func() (string, error) {
				return "", nil
			},
			wantErr:     safefileio.ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name: "permission denied",
			setup: func() (string, error) {
				// Create a directory with no read permissions
				dirPath := filepath.Join(tempDir, "restricted")
				if err := os.Mkdir(dirPath, 0o000); err != nil {
					t.Fatalf("Failed to create restricted dir: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(dirPath, 0o755) }) // Ensure cleanup

				// Create a file in the restricted directory
				filePath := filepath.Join(dirPath, "test.txt")
				if err := os.WriteFile(filePath, []byte("test"), 0o400); err == nil {
					t.Fatal("Expected error when creating file in restricted dir")
				}

				return filePath, nil
			},
			wantErr:     os.ErrPermission,
			errContains: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath, err := tt.setup()
			if err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			// Test Record
			_, err = validator.Record(filePath, false)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Expected error type %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Test Verify
			err = validator.Verify(filePath)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Expected error type %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

// TestFilesystemEdgeCases tests various edge cases related to filesystem operations
func TestFilesystemEdgeCases(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	t.Run("file deleted between operations", func(t *testing.T) {
		// Create a test file
		filePath := filepath.Join(tempDir, "tempfile.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Record the file
		if _, err := validator.Record(filePath, false); err != nil {
			t.Fatalf("Failed to record file: %v", err)
		}

		// Delete the file
		if err := os.Remove(filePath); err != nil {
			t.Fatalf("Failed to delete test file: %v", err)
		}

		// Verify should fail with file not found
		err := validator.Verify(filePath)
		if err == nil {
			t.Fatal("Expected error for deleted file, got nil")
		}
		// Check the error type
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("Expected file not found error, got: %v", err)
		}
	})

	t.Run("directory instead of file", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "subdir")
		if err := os.Mkdir(dirPath, 0o755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		_, err := validator.Record(dirPath, false)
		if err == nil {
			t.Fatal("Expected error for directory, got nil")
		}
		if !errors.Is(err, safefileio.ErrInvalidFilePath) {
			t.Errorf("Expected invalid file path error, got: %v", err)
		}
	})

	t.Run("unreadable directory", func(t *testing.T) {
		// Create a directory with no read permissions
		dirPath := filepath.Join(tempDir, "noreaddir")
		if err := os.Mkdir(dirPath, 0o700); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Create a file in the directory first
		filePath := filepath.Join(dirPath, "test.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0o600); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Make the directory unreadable
		if err := os.Chmod(dirPath, 0o000); err != nil {
			t.Fatalf("Failed to change directory permissions: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(dirPath, 0o700) })

		err := validator.Verify(filePath)
		if err == nil {
			t.Fatal("Expected error for unreadable directory, got nil")
		}
		// Check for permission error in the error chain
		var perr *os.PathError
		if !errors.As(err, &perr) || !os.IsPermission(perr) {
			t.Errorf("Expected permission error, got: %v", err)
		}
	})

	// This test requires root privileges to create a read-only mount
	// Skipping by default, uncomment if running in a suitable environment
	t.Run("read-only filesystem", func(t *testing.T) {
		t.Skip("Skipping read-only filesystem test as it requires root privileges")

		// This test requires root privileges to create a read-only mount
		roDir := filepath.Join(tempDir, "ro")
		if err := os.Mkdir(roDir, 0o755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Try to make directory read-only (this will only work as root)
		if err := syscall.Mount("tmpfs", roDir, "tmpfs", syscall.MS_RDONLY, ""); err != nil {
			t.Skipf("Skipping read-only filesystem test: %v", err)
		}
		defer syscall.Unmount(roDir, 0)

		filePath := filepath.Join(roDir, "test.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		_, err := validator.Record(filePath, false)
		if err == nil {
			t.Fatal("Expected error for read-only filesystem, got nil")
		}
		if !errors.Is(err, os.ErrPermission) {
			t.Errorf("Expected permission error, got: %v", err)
		}
	})
}

// TestErrorMessages verifies that error messages are clear and helpful
func TestErrorMessages(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	tests := []struct {
		name        string
		filePath    string
		expectedErr error
		errContains string
		skipVerify  bool // Skip Verify test for certain cases
	}{
		{
			name:        "empty path",
			filePath:    "",
			expectedErr: safefileio.ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tempDir, "nonexistent.txt"),
			expectedErr: os.ErrNotExist,
			errContains: "no such file or directory",
			skipVerify:  true, // Skip verify as it's the same as Record for non-existent files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Record
			_, err := validator.Record(tt.filePath, false)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			// Check error type if expectedErr is set
			if tt.expectedErr != nil {
				if !errors.Is(err, tt.expectedErr) {
					t.Errorf("Error %v is not a %v", err, tt.expectedErr)
				}
			}

			// Skip Verify test if specified
			if tt.skipVerify {
				return
			}

			// Test Verify
			err = validator.Verify(tt.filePath)
			if err == nil {
				t.Fatal("Expected error for Verify, got nil")
			}

			// Check error type for Verify
			if tt.expectedErr != nil {
				if !errors.Is(err, tt.expectedErr) {
					t.Errorf("Verify error %v is not a %v", err, tt.expectedErr)
				}
			}
		})
	}
}
