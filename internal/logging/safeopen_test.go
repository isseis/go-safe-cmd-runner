package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSafeFileOpener_Success(t *testing.T) {
	opener := NewSafeFileOpener()

	if opener == nil {
		t.Fatal("NewSafeFileOpener() returned nil")
	}

	if opener.fs == nil {
		t.Error("NewSafeFileOpener() created opener with nil filesystem")
	}
}

func TestOpenFile_Success(t *testing.T) {
	tempDir := t.TempDir()
	opener := NewSafeFileOpener()

	tests := []struct {
		name     string
		filepath string
		flag     int
		perm     os.FileMode
	}{
		{
			name:     "create new file",
			filepath: filepath.Join(tempDir, "test1.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o600,
		},
		{
			name:     "create file in subdirectory",
			filepath: filepath.Join(tempDir, "subdir", "test2.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o600,
		},
		{
			name:     "create file with different permissions",
			filepath: filepath.Join(tempDir, "test3.log"),
			flag:     os.O_CREATE | os.O_WRONLY | os.O_TRUNC,
			perm:     0o644,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := opener.OpenFile(tt.filepath, tt.flag, tt.perm)
			if err != nil {
				t.Fatalf("OpenFile() error = %v", err)
			}
			defer file.Close()

			if file == nil {
				t.Error("OpenFile() returned nil file")
			}

			// Verify file was created
			if _, err := os.Stat(tt.filepath); os.IsNotExist(err) {
				t.Errorf("OpenFile() did not create file at %s", tt.filepath)
			}

			// Write some data to verify the file is writable
			_, err = file.Write([]byte("test data\n"))
			if err != nil {
				t.Errorf("Failed to write to file: %v", err)
			}
		})
	}
}

func TestOpenFile_PermissionDenied(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")

	// Create a read-only directory
	if err := os.Mkdir(readOnlyDir, 0o444); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0o755) // Restore permissions for cleanup

	opener := NewSafeFileOpener()
	filepath := filepath.Join(readOnlyDir, "test.log")

	file, err := opener.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)

	if err == nil {
		if file != nil {
			file.Close()
		}
		t.Error("OpenFile() expected error for read-only directory, got nil")
	}
}

func TestOpenFile_SymlinkAttack(t *testing.T) {
	tempDir := t.TempDir()
	opener := NewSafeFileOpener()

	// Create a target file
	targetFile := filepath.Join(tempDir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("original"), 0o644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	// Create a symlink
	symlinkPath := filepath.Join(tempDir, "symlink.log")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Try to open the symlink - should be rejected by safefileio
	file, err := opener.OpenFile(symlinkPath, os.O_WRONLY, 0o600)

	if err == nil {
		if file != nil {
			file.Close()
		}
		t.Error("OpenFile() should reject symlinks, but succeeded")
	}
}

func TestGenerateRunID_Uniqueness(t *testing.T) {
	// Generate multiple IDs and verify they are unique
	ids := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		id := GenerateRunID()

		if id == "" {
			t.Error("GenerateRunID() returned empty string")
		}

		if ids[id] {
			t.Errorf("GenerateRunID() generated duplicate ID: %s", id)
		}

		ids[id] = true
	}

	if len(ids) != iterations {
		t.Errorf("Expected %d unique IDs, got %d", iterations, len(ids))
	}
}

func TestGenerateRunID_Format(t *testing.T) {
	id := GenerateRunID()

	// ULID should be 26 characters
	if len(id) != 26 {
		t.Errorf("GenerateRunID() returned ID with length %d, expected 26", len(id))
	}

	// ULID should only contain specific characters (Crockford's Base32)
	validChars := "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	for _, c := range id {
		if !strings.ContainsRune(validChars, c) {
			t.Errorf("GenerateRunID() returned ID with invalid character: %c", c)
		}
	}
}

func TestValidateLogDir_Valid(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string
		wantErr   bool
	}{
		{
			name: "existing writable directory",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: false,
		},
		{
			name: "non-existing directory that can be created",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "newdir")
			},
			wantErr: false,
		},
		{
			name: "nested directory that can be created",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "a", "b", "c")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			err := ValidateLogDir(dir)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogDir() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify directory was created
			if err == nil {
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					t.Errorf("ValidateLogDir() did not create directory at %s", dir)
				}
			}
		})
	}
}

func TestValidateLogDir_NotExist(t *testing.T) {
	// This test is actually covered by TestValidateLogDir_Valid
	// because ValidateLogDir creates the directory if it doesn't exist

	// Test case: directory that cannot be created due to invalid path
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "empty directory path",
			dir:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogDir(tt.dir)

			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogDir() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && err != ErrEmptyLogDirectory {
				t.Logf("ValidateLogDir() error = %v", err)
			}
		})
	}
}

func TestValidateLogDir_NotWritable(t *testing.T) {
	// Skip if running as root (no permission errors)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")

	// Create a read-only directory
	if err := os.Mkdir(readOnlyDir, 0o444); err != nil {
		t.Fatalf("Failed to create read-only directory: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0o755) // Restore permissions for cleanup

	err := ValidateLogDir(readOnlyDir)

	if err == nil {
		t.Error("ValidateLogDir() expected error for read-only directory, got nil")
	}
}
