//go:build test

package security

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestValidator_ValidateOutputWritePermission(t *testing.T) {
	// Get current user info for testing
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) string // Returns the path to test
		uid         int
		wantErr     bool
		errContains string
	}{
		{
			name: "valid_output_file_write_permission",
			setupFunc: func(t *testing.T) string {
				// Create a secure temporary directory first
				tempDir := t.TempDir()
				err := os.Chmod(tempDir, 0o755)
				require.NoError(t, err)

				tempFile, err := os.CreateTemp(tempDir, "test_output_*")
				require.NoError(t, err)
				defer tempFile.Close()

				// Set file permissions to be writable by owner
				err = os.Chmod(tempFile.Name(), 0o600)
				require.NoError(t, err)

				return tempFile.Name()
			},
			uid:     currentUID,
			wantErr: false,
		},
		{
			name: "non_existent_file_in_writable_directory",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				// Set secure directory permissions for test
				err := os.Chmod(tempDir, 0o755)
				require.NoError(t, err)
				return filepath.Join(tempDir, "non_existent.txt")
			},
			uid:     currentUID,
			wantErr: false,
		},
		{
			name: "empty_output_path",
			setupFunc: func(_ *testing.T) string {
				return ""
			},
			uid:         currentUID,
			wantErr:     true,
			errContains: "empty output path",
		},
		{
			name: "relative_path",
			setupFunc: func(_ *testing.T) string {
				return "relative/path.txt"
			},
			uid:         currentUID,
			wantErr:     true,
			errContains: "output path must be absolute",
		},
		{
			name: "directory_instead_of_file",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				return tempDir
			},
			uid:         currentUID,
			wantErr:     true,
			errContains: "is not a regular file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use permissive config for tests that depend on real filesystem
			// (specifically /tmp directory permissions)
			var config *Config
			if tt.name == "valid_output_file_write_permission" ||
				tt.name == "non_existent_file_in_writable_directory" ||
				tt.name == "directory_instead_of_file" {
				config = NewPermissiveTestConfig()
			} else {
				config = DefaultConfig()
			}
			validator, err := NewValidatorWithGroupMembership(config, nil)
			require.NoError(t, err)

			outputPath := tt.setupFunc(t)

			// For cleanup, only remove files we created
			if tt.name == "valid_output_file_write_permission" {
				defer os.Remove(outputPath)
			}

			err = validator.ValidateOutputWritePermission(outputPath, tt.uid)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_checkWritePermission(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (string, os.FileInfo)
		uid         int
		useTestMode bool
		wantErr     bool
		errContains string
	}{
		{
			name: "owner_writable_file",
			setupFunc: func(t *testing.T) (string, os.FileInfo) {
				tempFile, err := os.CreateTemp("", "owner_writable_*")
				require.NoError(t, err)
				defer tempFile.Close()

				err = os.Chmod(tempFile.Name(), 0o600)
				require.NoError(t, err)

				info, err := os.Lstat(tempFile.Name())
				require.NoError(t, err)

				return tempFile.Name(), info
			},
			uid:     currentUID,
			wantErr: false,
		},
		{
			name: "world_writable_file_blocked_in_normal_mode",
			setupFunc: func(t *testing.T) (string, os.FileInfo) {
				tempFile, err := os.CreateTemp("", "world_writable_*")
				require.NoError(t, err)
				defer tempFile.Close()

				// Set file permissions to be world-writable with owner-writable
				// This forces the code to check world-writable permissions for non-owners
				err = os.Chmod(tempFile.Name(), 0o666) // rw-rw-rw-
				require.NoError(t, err)

				info, err := os.Lstat(tempFile.Name())
				require.NoError(t, err)

				return tempFile.Name(), info
			},
			uid:         65534, // Use 'nobody' user to test world-writable check
			useTestMode: false,
			wantErr:     true,
			errContains: "writable by others",
		},
		{
			name: "world_writable_file_allowed_in_test_mode",
			setupFunc: func(t *testing.T) (string, os.FileInfo) {
				tempFile, err := os.CreateTemp("", "world_writable_test_*")
				require.NoError(t, err)
				defer tempFile.Close()

				// Set file permissions to be world-writable with owner-writable
				err = os.Chmod(tempFile.Name(), 0o666) // rw-rw-rw-
				require.NoError(t, err)

				info, err := os.Lstat(tempFile.Name())
				require.NoError(t, err)

				return tempFile.Name(), info
			},
			uid:         65534, // Use 'nobody' user to test world-writable check
			useTestMode: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config *Config
			if tt.useTestMode {
				config = NewPermissiveTestConfig()
			} else {
				config = DefaultConfig()
			}

			groupMembership := groupmembership.New()
			validator, err := NewValidatorWithGroupMembership(config, groupMembership)
			require.NoError(t, err)

			path, fileInfo := tt.setupFunc(t)
			defer os.Remove(path)

			err = validator.checkWritePermission(path, fileInfo, tt.uid)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_validateOutputDirectoryAccess(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		uid         int
		wantErr     bool
		errContains string
	}{
		{
			name: "writable_directory_by_owner_with_permissive_config",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				// Ensure directory is writable by owner
				err := os.Chmod(tempDir, 0o700)
				require.NoError(t, err)
				return tempDir
			},
			uid:     currentUID,
			wantErr: false,
		},
		{
			name: "non_existent_directory_with_permissive_config",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				nonExistentDir := filepath.Join(tempDir, "non_existent")
				return nonExistentDir
			},
			uid:     currentUID,
			wantErr: false, // Should check parent directory
		},
		{
			name: "directory_without_write_permission_with_permissive_config",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				// Remove write permissions for all
				err := os.Chmod(tempDir, 0o555)
				require.NoError(t, err)
				return tempDir
			},
			uid:         currentUID,
			wantErr:     true,
			errContains: "write permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use permissive config for all tests since this is testing the internal helper
			// and we want to focus on write permission logic, not directory security
			config := NewPermissiveTestConfig()
			validator, err := NewValidatorWithGroupMembership(config, nil)
			require.NoError(t, err)

			dirPath := tt.setupFunc(t)

			err = validator.validateOutputDirectoryAccess(dirPath, tt.uid)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_validateOutputFileWritePermission(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) (string, os.FileInfo)
		uid         int
		wantErr     bool
		errContains string
	}{
		{
			name: "writable_file_by_owner",
			setupFunc: func(t *testing.T) (string, os.FileInfo) {
				tempFile, err := os.CreateTemp("", "test_file_*")
				require.NoError(t, err)
				defer tempFile.Close()

				err = os.Chmod(tempFile.Name(), 0o600)
				require.NoError(t, err)

				info, err := os.Lstat(tempFile.Name())
				require.NoError(t, err)

				return tempFile.Name(), info
			},
			uid:     currentUID,
			wantErr: false,
		},
		{
			name: "file_without_write_permission",
			setupFunc: func(t *testing.T) (string, os.FileInfo) {
				tempFile, err := os.CreateTemp("", "test_file_*")
				require.NoError(t, err)
				defer tempFile.Close()

				err = os.Chmod(tempFile.Name(), 0o400) // Read-only
				require.NoError(t, err)

				info, err := os.Lstat(tempFile.Name())
				require.NoError(t, err)

				return tempFile.Name(), info
			},
			uid:         currentUID,
			wantErr:     true,
			errContains: "write permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			validator, err := NewValidatorWithGroupMembership(config, nil)
			require.NoError(t, err)

			filePath, fileInfo := tt.setupFunc(t)
			defer os.Remove(filePath)

			err = validator.validateOutputFileWritePermission(filePath, fileInfo, tt.uid)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_isUserInGroup(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	currentGID, err := strconv.Atoi(currentUser.Gid)
	require.NoError(t, err)

	tests := []struct {
		name    string
		uid     int
		gid     uint32
		want    bool
		wantErr bool
	}{
		{
			name:    "user_in_primary_group",
			uid:     currentUID,
			gid:     uint32(currentGID),
			want:    true,
			wantErr: false,
		},
		{
			name:    "user_not_in_group",
			uid:     currentUID,
			gid:     99999, // Very unlikely to be a real group
			want:    false,
			wantErr: false,
		},
		{
			name:    "invalid_user",
			uid:     99999, // Very unlikely to be a real user
			gid:     uint32(currentGID),
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			groupMembership := groupmembership.New()
			validator, err := NewValidatorWithGroupMembership(config, groupMembership)
			require.NoError(t, err)

			inGroup, err := validator.isUserInGroup(tt.uid, tt.gid)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, inGroup)
			}
		})
	}
}

// Test integration with existing directory validation
func TestValidator_ValidateOutputWritePermission_Integration(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)

	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	t.Run("integration_with_directory_validation", func(t *testing.T) {
		// Create a directory structure that should pass validation
		tempDir := t.TempDir()
		// Set secure permissions on parent directory
		err := os.Chmod(tempDir, 0o755)
		require.NoError(t, err)

		subDir := filepath.Join(tempDir, "subdir")
		err = os.MkdirAll(subDir, 0o755)
		require.NoError(t, err)

		outputFile := filepath.Join(subDir, "output.txt")

		// Use permissive config for integration test that depends on real filesystem
		config := NewPermissiveTestConfig()
		validator, err := NewValidatorWithGroupMembership(config, nil)
		require.NoError(t, err)

		// This should work - leverages existing ValidateDirectoryPermissions
		err = validator.ValidateOutputWritePermission(outputFile, currentUID)
		assert.NoError(t, err)
	})
}

func TestValidator_EvaluateOutputSecurityRisk(t *testing.T) {
	// Get current user for testing
	currentUser, err := user.Current()
	require.NoError(t, err)
	homeDir := currentUser.HomeDir

	config := DefaultConfig()
	validator, err := NewValidatorWithGroupMembership(config, nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		path       string
		workDir    string
		expectRisk runnertypes.RiskLevel
	}{
		{
			name:       "critical_passwd_file",
			path:       "/etc/passwd",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_shadow_file",
			path:       "/etc/shadow",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_sudoers_file",
			path:       "/etc/sudoers",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_boot_directory",
			path:       "/boot/vmlinuz",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_ssh_private_key",
			path:       "/home/user/.ssh/id_rsa",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_authorized_keys",
			path:       "/home/user/.ssh/authorized_keys",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_etc_directory",
			path:       "/etc/myconfig.conf",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "high_var_log_directory",
			path:       "/var/log/myapp.log",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelHigh,
		},
		{
			name:       "critical_usr_bin_directory",
			path:       "/usr/bin/myapp",
			workDir:    "/home/user",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "low_workdir_file",
			path:       "/home/user/project/output.txt",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelLow,
		},
		{
			name:       "low_workdir_subdir",
			path:       "/home/user/project/logs/output.txt",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelLow,
		},
		{
			name:       "low_home_directory",
			path:       homeDir + "/output.txt",
			workDir:    "/tmp",
			expectRisk: runnertypes.RiskLevelLow,
		},
		{
			name:       "low_home_subdir",
			path:       homeDir + "/documents/output.txt",
			workDir:    "/tmp",
			expectRisk: runnertypes.RiskLevelLow,
		},
		{
			name:       "medium_tmp_directory",
			path:       "/tmp/output.txt",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:       "medium_opt_directory",
			path:       "/opt/myapp/output.txt",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:       "medium_other_user_home",
			path:       "/home/otheruser/output.txt",
			workDir:    "/home/user/project",
			expectRisk: runnertypes.RiskLevelMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, err := validator.EvaluateOutputSecurityRisk(tt.path, tt.workDir)
			require.NoError(t, err)
			assert.Equal(t, tt.expectRisk, risk, "Risk level mismatch for path: %s", tt.path)
		})
	}
}

func TestValidator_EvaluateOutputSecurityRisk_CaseInsensitive(t *testing.T) {
	config := DefaultConfig()
	validator, err := NewValidatorWithGroupMembership(config, nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		path       string
		expectRisk runnertypes.RiskLevel
	}{
		{
			name:       "critical_uppercase_passwd",
			path:       "/ETC/PASSWD",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_mixed_case_ssh",
			path:       "/home/user/.SSH/ID_RSA",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_uppercase_etc",
			path:       "/ETC/config.conf",
			expectRisk: runnertypes.RiskLevelCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, err := validator.EvaluateOutputSecurityRisk(tt.path, "/tmp")
			require.NoError(t, err)
			assert.Equal(t, tt.expectRisk, risk, "Case-insensitive risk evaluation failed for: %s", tt.path)
		})
	}
}

func TestValidator_EvaluateOutputSecurityRisk_EdgeCases(t *testing.T) {
	config := DefaultConfig()
	validator, err := NewValidatorWithGroupMembership(config, nil)
	require.NoError(t, err)

	t.Run("empty_path", func(t *testing.T) {
		risk, err := validator.EvaluateOutputSecurityRisk("", "/home/user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty path")
		assert.Equal(t, runnertypes.RiskLevelUnknown, risk)
	})

	t.Run("empty_workdir", func(t *testing.T) {
		risk, err := validator.EvaluateOutputSecurityRisk("/tmp/output.txt", "")
		require.NoError(t, err)
		assert.Equal(t, runnertypes.RiskLevelMedium, risk)
	})

	t.Run("path_equals_workdir", func(t *testing.T) {
		workDir := "/home/user/project"
		risk, err := validator.EvaluateOutputSecurityRisk(workDir+"/output.txt", workDir)
		require.NoError(t, err)
		assert.Equal(t, runnertypes.RiskLevelLow, risk)
	})

	t.Run("invalid_home_directory", func(t *testing.T) {
		// This test assumes the path doesn't match user's actual home
		risk, err := validator.EvaluateOutputSecurityRisk("/nonexistent/home/output.txt", "/tmp")
		require.NoError(t, err)
		assert.Equal(t, runnertypes.RiskLevelMedium, risk)
	})
}

func TestValidator_EvaluateOutputSecurityRisk_SpecialPatterns(t *testing.T) {
	config := DefaultConfig()
	validator, err := NewValidatorWithGroupMembership(config, nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		path       string
		expectRisk runnertypes.RiskLevel
	}{
		{
			name:       "critical_wallet_file",
			path:       "/home/user/wallet.dat",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_keystore_file",
			path:       "/home/user/keystore",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "critical_gnupg_directory",
			path:       "/home/user/.gnupg/secring.gpg",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "proc_directory",
			path:       "/proc/meminfo",
			expectRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:       "sys_directory",
			path:       "/sys/kernel/debug",
			expectRisk: runnertypes.RiskLevelCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, err := validator.EvaluateOutputSecurityRisk(tt.path, "/tmp")
			require.NoError(t, err)
			assert.Equal(t, tt.expectRisk, risk, "Special pattern risk evaluation failed for: %s", tt.path)
		})
	}
}

// TestValidator_EvaluateOutputSecurityRisk_WorkDirRequirements tests the workDir validation requirements
func TestValidator_EvaluateOutputSecurityRisk_WorkDirRequirements(t *testing.T) {
	config := DefaultConfig()
	validator, err := NewValidatorWithGroupMembership(config, nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		path        string
		workDir     string
		expectRisk  runnertypes.RiskLevel
		expectError bool
		errorText   string
		desc        string
	}{
		{
			name:        "non_absolute_workdir_error",
			path:        "output.txt",
			workDir:     "relative/path", // Non-absolute workDir
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be absolute",
			desc:        "Non-absolute workDir should return error",
		},
		{
			name:        "non_clean_workdir_error",
			path:        "output.txt",
			workDir:     "/home/user/../user/project", // Non-clean workDir
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be pre-cleaned",
			desc:        "Non-clean workDir should return error",
		},
		{
			name:        "non_clean_workdir_with_dot_error",
			path:        "output.txt",
			workDir:     "/home/user/./project", // Non-clean workDir with dot
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be pre-cleaned",
			desc:        "Non-clean workDir with dot should return error",
		},
		{
			name:        "non_clean_workdir_trailing_slash_error",
			path:        "output.txt",
			workDir:     "/home/user/project/", // Non-clean workDir with trailing slash
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be pre-cleaned",
			desc:        "Non-clean workDir with trailing slash should return error",
		},
		{
			name:        "absolute_and_clean_workdir_valid",
			path:        "output.txt",
			workDir:     "/home/user/project", // Absolute and clean workDir
			expectRisk:  runnertypes.RiskLevelLow,
			expectError: false,
			desc:        "Absolute and clean workDir should work normally",
		},
		{
			name:        "empty_workdir_valid",
			path:        "/tmp/output.txt",
			workDir:     "", // Empty workDir is allowed
			expectRisk:  runnertypes.RiskLevelMedium,
			expectError: false,
			desc:        "Empty workDir should be allowed",
		},
		{
			name:        "relative_path_with_non_absolute_workdir_error",
			path:        "../output.txt",
			workDir:     "not/absolute", // Non-absolute workDir with relative path
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be absolute",
			desc:        "Relative path with non-absolute workDir should return error",
		},
		{
			name:        "absolute_path_with_non_absolute_workdir_error",
			path:        "/tmp/output.txt",
			workDir:     "relative", // Non-absolute workDir with absolute path
			expectRisk:  runnertypes.RiskLevelUnknown,
			expectError: true,
			errorText:   "workDir must be absolute",
			desc:        "Absolute path with non-absolute workDir should return error due to programming error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, err := validator.EvaluateOutputSecurityRisk(tt.path, tt.workDir)
			if tt.expectError {
				assert.Error(t, err, "Test failed: %s. Expected error but got none for path=%q, workDir=%q", tt.desc, tt.path, tt.workDir)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText, "Error message mismatch for %s", tt.desc)
				}
				assert.Equal(t, tt.expectRisk, risk, "Risk level mismatch when error expected for %s", tt.desc)
			} else {
				require.NoError(t, err, "Test failed: %s. Unexpected error for path=%q, workDir=%q: %v", tt.desc, tt.path, tt.workDir, err)
				assert.Equal(t, tt.expectRisk, risk, "Test failed: %s. Expected %v but got %v for path=%q, workDir=%q",
					tt.desc, tt.expectRisk, risk, tt.path, tt.workDir)
			}
		})
	}
}
