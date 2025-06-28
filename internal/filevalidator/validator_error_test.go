package filevalidator

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestErrorCases tests various error conditions and their messages
func TestErrorCases(t *testing.T) {
	tempDir := t.TempDir()
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
			errContains: "file does not exist",
		},
		{
			name: "empty file path",
			setup: func() (string, error) {
				return "", nil
			},
			wantErr:     ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name: "permission denied",
			setup: func() (string, error) {
				// Create a directory with no read permissions
				dirPath := filepath.Join(tempDir, "restricted")
				if err := os.Mkdir(dirPath, 0000); err != nil {
					t.Fatalf("Failed to create restricted dir: %v", err)
				}
				t.Cleanup(func() { os.Chmod(dirPath, 0755) }) // Ensure cleanup

				// Create a file in the restricted directory
				filePath := filepath.Join(dirPath, "test.txt")
				if err := os.WriteFile(filePath, []byte("test"), 0400); err == nil {
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
			err = validator.Record(filePath)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error message %q does not contain %q", err.Error(), tt.errContains)
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
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error message %q does not contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

// TestFilesystemEdgeCases tests various edge cases related to filesystem operations
func TestFilesystemEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	t.Run("file deleted between operations", func(t *testing.T) {
		// Create a test file
		filePath := filepath.Join(tempDir, "tempfile.txt")
		if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Record the file
		if err := validator.Record(filePath); err != nil {
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
		// Check both the error type and message
		if !os.IsNotExist(err) && !strings.Contains(err.Error(), "file does not exist") {
			t.Errorf("Expected file not found error, got: %v", err)
		}
	})

	t.Run("directory instead of file", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "subdir")
		if err := os.Mkdir(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		err := validator.Record(dirPath)
		if err == nil {
			t.Fatal("Expected error for directory, got nil")
		}
		if !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("Expected 'is a directory' error, got: %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("unreadable directory", func(t *testing.T) {
			// Create a directory with no read permissions
			dirPath := filepath.Join(tempDir, "noreaddir")
			if err := os.Mkdir(dirPath, 0700); err != nil {
				t.Fatalf("Failed to create directory: %v", err)
			}

			// Create a file in the directory first
			filePath := filepath.Join(dirPath, "test.txt")
			if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Make the directory unreadable
			if err := os.Chmod(dirPath, 0000); err != nil {
				t.Fatalf("Failed to change directory permissions: %v", err)
			}
			t.Cleanup(func() { os.Chmod(dirPath, 0700) })

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
			/*
				// This test requires root privileges to create a read-only mount
				roDir := filepath.Join(tempDir, "ro")
				if err := os.Mkdir(roDir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}

				// Try to make directory read-only (this will only work as root)
				if err := syscall.Mount("tmpfs", roDir, "tmpfs", syscall.MS_RDONLY, ""); err != nil {
					t.Skipf("Skipping read-only filesystem test: %v", err)
				}
				defer syscall.Unmount(roDir, 0)

				filePath := filepath.Join(roDir, "test.txt")
				if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}

				err := validator.Record(filePath)
				if err == nil {
					t.Fatal("Expected error for read-only filesystem, got nil")
				}
				if !os.IsPermission(err) && !strings.Contains(err.Error(), "read-only") {
					t.Errorf("Expected read-only or permission error, got: %v", err)
				}
			*/
		})
	}
}

// TestErrorMessages verifies that error messages are clear and helpful
func TestErrorMessages(t *testing.T) {
	tempDir := t.TempDir()
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
			expectedErr: ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tempDir, "nonexistent.txt"),
			errContains: "file does not exist",
			skipVerify:  true, // Skip verify as it's the same as Record for non-existent files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Record
			err := validator.Record(tt.filePath)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			// Check error message contains expected text
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Error message %q does not contain %q", err.Error(), tt.errContains)
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

			// Check error message contains expected text for Verify
			if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("Verify error message %q does not contain %q", err.Error(), tt.errContains)
			}
		})
	}
}
