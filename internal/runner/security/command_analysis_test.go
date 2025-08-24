package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeCommandSecurity_SetuidSetgid(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	t.Run("normal executable without setuid/setgid", func(t *testing.T) {
		// Create a normal executable
		normalExec := filepath.Join(tmpDir, "normal_exec")
		err := os.WriteFile(normalExec, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		risk, pattern, reason, err := AnalyzeCommandSecurity(normalExec, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})

	t.Run("executable with setuid bit", func(t *testing.T) {
		// Create an executable file with setuid bit
		setuidExec := filepath.Join(tmpDir, "setuid_exec")
		err := os.WriteFile(setuidExec, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set the setuid bit (requires the file to be executable first)
		err = os.Chmod(setuidExec, 0o755|os.ModeSetuid) // setuid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setuid bit is actually set
		fileInfo, err := os.Stat(setuidExec)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")

		risk, pattern, reason, err := AnalyzeCommandSecurity(setuidExec, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelHigh, risk)
		assert.Equal(t, setuidExec, pattern)
		assert.Equal(t, "Executable has setuid or setgid bit set", reason)
	})

	t.Run("executable with setgid bit", func(t *testing.T) {
		// Create an executable file with setgid bit
		setgidExec := filepath.Join(tmpDir, "setgid_exec")
		err := os.WriteFile(setgidExec, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set the setgid bit (requires the file to be executable first)
		err = os.Chmod(setgidExec, 0o755|os.ModeSetgid) // setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setgid bit is actually set
		fileInfo, err := os.Stat(setgidExec)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")

		risk, pattern, reason, err := AnalyzeCommandSecurity(setgidExec, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelHigh, risk)
		assert.Equal(t, setgidExec, pattern)
		assert.Equal(t, "Executable has setuid or setgid bit set", reason)
	})

	t.Run("executable with both setuid and setgid bits", func(t *testing.T) {
		// Create an executable file with both setuid and setgid bits
		setuidSetgidExec := filepath.Join(tmpDir, "setuid_setgid_exec")
		err := os.WriteFile(setuidSetgidExec, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set both setuid and setgid bits
		err = os.Chmod(setuidSetgidExec, 0o755|os.ModeSetuid|os.ModeSetgid) // setuid+setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify both bits are actually set
		fileInfo, err := os.Stat(setuidSetgidExec)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")

		risk, pattern, reason, err := AnalyzeCommandSecurity(setuidSetgidExec, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelHigh, risk)
		assert.Equal(t, setuidSetgidExec, pattern)
		assert.Equal(t, "Executable has setuid or setgid bit set", reason)
	})

	t.Run("non-executable file with setuid bit", func(t *testing.T) {
		// Create a non-executable file with setuid bit (should not be detected as risky)
		nonExecFile := filepath.Join(tmpDir, "non_exec_setuid")
		err := os.WriteFile(nonExecFile, []byte("not executable"), 0o644)
		require.NoError(t, err)

		// Set the setuid bit on non-executable file
		err = os.Chmod(nonExecFile, 0o644|os.ModeSetuid) // setuid + rw-r--r--
		require.NoError(t, err)

		// Verify the setuid bit is set but file is not executable
		fileInfo, err := os.Stat(nonExecFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")
		assert.False(t, fileInfo.Mode()&0o111 != 0, "file should not be executable")

		// The function should still detect setuid bit regardless of executable status
		// because the security risk comes from the setuid bit itself
		risk, pattern, reason, err := AnalyzeCommandSecurity(nonExecFile, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelHigh, risk)
		assert.Equal(t, nonExecFile, pattern)
		assert.Equal(t, "Executable has setuid or setgid bit set", reason)
	})

	t.Run("directory with setgid bit", func(t *testing.T) {
		// Create a directory with setgid bit (should not be detected as risky)
		setgidDir := filepath.Join(tmpDir, "setgid_dir")
		err := os.Mkdir(setgidDir, 0o755)
		require.NoError(t, err)

		// Set the setgid bit on directory
		err = os.Chmod(setgidDir, 0o755|os.ModeSetgid) // setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setgid bit is set and it's a directory
		fileInfo, err := os.Stat(setgidDir)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")
		assert.True(t, fileInfo.IsDir(), "should be a directory")

		// The function should not detect directories as risky even with setgid
		// because hasSetuidOrSetgidBit only checks regular files
		risk, pattern, reason, err := AnalyzeCommandSecurity(setgidDir, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})

	t.Run("non-existent file", func(t *testing.T) {
		// Test with non-existent file - should not cause panic and fallback gracefully
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		risk, pattern, reason, err := AnalyzeCommandSecurity(nonExistentFile, []string{})
		require.NoError(t, err)
		assert.Equal(t, RiskLevelNone, risk)
		assert.Empty(t, pattern)
		assert.Empty(t, reason)
	})

	t.Run("relative path should return error", func(t *testing.T) {
		// Test with relative path - should return error
		_, _, _, err := AnalyzeCommandSecurity("relative/path", []string{})
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPath)
		assert.Contains(t, err.Error(), "path must be absolute")
	})

	t.Run("integration test with real setuid binary", func(t *testing.T) {
		// Check if passwd command exists and has setuid bit (common on most Unix systems)
		if fileInfo, err := os.Stat(passwdPath); err == nil && fileInfo.Mode()&os.ModeSetuid != 0 {
			risk, pattern, reason, err := AnalyzeCommandSecurity(passwdPath, []string{})
			require.NoError(t, err)
			assert.Equal(t, RiskLevelHigh, risk)
			assert.Equal(t, passwdPath, pattern)
			assert.Equal(t, "Executable has setuid or setgid bit set", reason)
		} else {
			t.Skip("No setuid passwd binary found for integration test")
		}
	})
}

func TestContainsSSHStyleAddress(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		// Valid SSH-style addresses with user@host:path
		{
			name:     "ssh style user@host:path",
			args:     []string{"user@example.com:/path/to/file"},
			expected: true,
		},
		{
			name:     "ssh style with complex path",
			args:     []string{"root@server.example.com:/home/user/documents/file.txt"},
			expected: true,
		},
		{
			name:     "ssh style with home directory",
			args:     []string{"user@host:~/backup"},
			expected: true,
		},
		{
			name:     "multiple args with ssh style",
			args:     []string{"-v", "user@host:/remote/path", "./local/"},
			expected: true,
		},

		// Valid host:path addresses without user@
		{
			name:     "host:path with forward slash",
			args:     []string{"server:/path/to/file"},
			expected: true,
		},
		{
			name:     "host:path with home directory",
			args:     []string{"host:~/documents"},
			expected: true,
		},
		{
			name:     "host:path in rsync command",
			args:     []string{"-av", "backup-server:/data/backup/", "./local-backup/"},
			expected: true,
		},

		// Invalid cases that should NOT be detected as SSH-style
		{
			name:     "email address only",
			args:     []string{"user@example.com"},
			expected: false,
		},
		{
			name:     "email in echo command",
			args:     []string{"echo", "Contact user@example.com for support"},
			expected: false,
		},
		{
			name:     "time format",
			args:     []string{"12:30:45"},
			expected: false,
		},
		{
			name:     "port specification",
			args:     []string{"localhost:8080"},
			expected: false,
		},
		{
			name:     "ratio or mathematical expression",
			args:     []string{"3:2"},
			expected: false,
		},
		{
			name:     "grep pattern with @",
			args:     []string{"grep", "@", "file.txt"},
			expected: false,
		},
		{
			name:     "at symbol in middle of word",
			args:     []string{"some@text:word"},
			expected: false, // This is ambiguous but we consider it invalid since no path indicators
		},

		// Edge cases
		{
			name:     "empty args",
			args:     []string{},
			expected: false,
		},
		{
			name:     "args with only @",
			args:     []string{"@"},
			expected: false,
		},
		{
			name:     "args with only :",
			args:     []string{":"},
			expected: false,
		},
		{
			name:     "malformed user@host: (missing path)",
			args:     []string{"user@host:"},
			expected: false,
		},
		{
			name:     "malformed @host:path (missing user)",
			args:     []string{"@host:/path"},
			expected: false,
		},
		{
			name:     "colon before at symbol",
			args:     []string{"path:user@host"},
			expected: false,
		},

		// More realistic examples
		{
			name:     "scp source to destination",
			args:     []string{"user@remote:/home/user/file.txt", "./local-file.txt"},
			expected: true,
		},
		{
			name:     "rsync with exclude pattern",
			args:     []string{"-av", "--exclude=*.tmp", "backup@server:/data/", "./backup/"},
			expected: true,
		},
		{
			name:     "git clone with ssh",
			args:     []string{"clone", "git@github.com:user/repo.git"},
			expected: true, // This is actually a valid SSH-style Git repository address
		},
		{
			name:     "mixed valid and invalid",
			args:     []string{"echo", "user@example.com", "user@host:/path"},
			expected: true, // Should detect the SSH-style address despite email presence
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsSSHStyleAddress(tt.args)
			if result != tt.expected {
				t.Errorf("containsSSHStyleAddress(%v) = %v, expected %v", tt.args, result, tt.expected)
			}
		})
	}
}

func TestIsNetworkOperation(t *testing.T) {
	tests := []struct {
		name         string
		cmdName      string
		args         []string
		expectedNet  bool
		expectedRisk bool
		description  string
	}{
		// Always network commands
		{
			name:         "curl command",
			cmdName:      "curl",
			args:         []string{"https://example.com"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "curl is always a network command",
		},
		{
			name:         "wget command",
			cmdName:      "wget",
			args:         []string{"https://example.com/file.zip"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "wget is always a network command",
		},
		{
			name:         "ssh command",
			cmdName:      "ssh",
			args:         []string{"user@host"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "ssh is always a network command",
		},

		// Conditional network commands with network arguments
		{
			name:         "rsync with ssh-style address",
			cmdName:      "rsync",
			args:         []string{"-av", "user@host:/remote/", "./local/"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "rsync with SSH-style address should be detected as network",
		},
		{
			name:         "rsync with URL",
			cmdName:      "rsync",
			args:         []string{"rsync://host/module/path", "./local/"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "rsync with URL should be detected as network",
		},
		{
			name:         "git with https URL",
			cmdName:      "git",
			args:         []string{"clone", "https://github.com/user/repo.git"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "git with HTTPS URL should be detected as network",
		},

		// Conditional network commands without network arguments
		{
			name:         "rsync local only",
			cmdName:      "rsync",
			args:         []string{"-av", "./source/", "./destination/"},
			expectedNet:  false,
			expectedRisk: false,
			description:  "rsync with only local paths should not be detected as network",
		},
		{
			name:         "git local operation",
			cmdName:      "git",
			args:         []string{"status"},
			expectedNet:  false,
			expectedRisk: false,
			description:  "git local operation should not be detected as network",
		},

		// Non-network commands
		{
			name:         "ls command",
			cmdName:      "ls",
			args:         []string{"-la"},
			expectedNet:  false,
			expectedRisk: false,
			description:  "ls should not be detected as network",
		},
		{
			name:         "echo with email",
			cmdName:      "echo",
			args:         []string{"Contact user@example.com"},
			expectedNet:  false,
			expectedRisk: false,
			description:  "echo with email should not be detected as network",
		},

		// Edge cases
		{
			name:         "any command with URL",
			cmdName:      "somecommand",
			args:         []string{"https://example.com"},
			expectedNet:  true,
			expectedRisk: false,
			description:  "any command with URL should be detected as network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isNet, isRisk := IsNetworkOperation(tt.cmdName, tt.args)
			if isNet != tt.expectedNet {
				t.Errorf("IsNetworkOperation(%s, %v) network detection = %v, expected %v. %s",
					tt.cmdName, tt.args, isNet, tt.expectedNet, tt.description)
			}
			if isRisk != tt.expectedRisk {
				t.Errorf("IsNetworkOperation(%s, %v) risk detection = %v, expected %v. %s",
					tt.cmdName, tt.args, isRisk, tt.expectedRisk, tt.description)
			}
		})
	}
}

func TestHasSetuidOrSetgidBit_Detailed(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()

	t.Run("normal file without setuid/setgid", func(t *testing.T) {
		normalFile := filepath.Join(tmpDir, "normal")
		err := os.WriteFile(normalFile, []byte("test content"), 0o644)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(normalFile)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("executable file without setuid/setgid", func(t *testing.T) {
		execFile := filepath.Join(tmpDir, "normal_exec")
		err := os.WriteFile(execFile, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(execFile)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("file with setuid bit", func(t *testing.T) {
		setuidFile := filepath.Join(tmpDir, "setuid_file")
		err := os.WriteFile(setuidFile, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set the setuid bit
		err = os.Chmod(setuidFile, 0o755|os.ModeSetuid) // setuid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setuid bit is actually set
		fileInfo, err := os.Stat(setuidFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(setuidFile)
		assert.NoError(t, err)
		assert.True(t, hasSetuidOrSetgid)
	})

	t.Run("file with setgid bit", func(t *testing.T) {
		setgidFile := filepath.Join(tmpDir, "setgid_file")
		err := os.WriteFile(setgidFile, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set the setgid bit
		err = os.Chmod(setgidFile, 0o755|os.ModeSetgid) // setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setgid bit is actually set
		fileInfo, err := os.Stat(setgidFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(setgidFile)
		assert.NoError(t, err)
		assert.True(t, hasSetuidOrSetgid)
	})

	t.Run("file with both setuid and setgid bits", func(t *testing.T) {
		bothBitsFile := filepath.Join(tmpDir, "both_bits_file")
		err := os.WriteFile(bothBitsFile, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)

		// Set both setuid and setgid bits
		err = os.Chmod(bothBitsFile, 0o755|os.ModeSetuid|os.ModeSetgid) // setuid+setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify both bits are actually set
		fileInfo, err := os.Stat(bothBitsFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(bothBitsFile)
		assert.NoError(t, err)
		assert.True(t, hasSetuidOrSetgid)
	})

	t.Run("non-executable file with setuid bit", func(t *testing.T) {
		nonExecSetuidFile := filepath.Join(tmpDir, "non_exec_setuid")
		err := os.WriteFile(nonExecSetuidFile, []byte("not executable"), 0o644)
		require.NoError(t, err)

		// Set the setuid bit on non-executable file
		err = os.Chmod(nonExecSetuidFile, 0o644|os.ModeSetuid) // setuid + rw-r--r--
		require.NoError(t, err)

		// Verify the setuid bit is set but file is not executable
		fileInfo, err := os.Stat(nonExecSetuidFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")
		assert.False(t, fileInfo.Mode()&0o111 != 0, "file should not be executable")

		// Function should still detect setuid bit regardless of executable status
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(nonExecSetuidFile)
		assert.NoError(t, err)
		assert.True(t, hasSetuidOrSetgid)
	})

	t.Run("directory with setgid bit", func(t *testing.T) {
		setgidDir := filepath.Join(tmpDir, "setgid_dir")
		err := os.Mkdir(setgidDir, 0o755)
		require.NoError(t, err)

		// Set the setgid bit on directory
		err = os.Chmod(setgidDir, 0o755|os.ModeSetgid) // setgid + rwxr-xr-x
		require.NoError(t, err)

		// Verify the setgid bit is set and it's a directory
		fileInfo, err := os.Stat(setgidDir)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetgid != 0, "setgid bit should be set")
		assert.True(t, fileInfo.IsDir(), "should be a directory")

		// Function should return false for directories (not regular files)
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(setgidDir)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("symbolic link to setuid file", func(t *testing.T) {
		// Create a setuid file
		setuidFile := filepath.Join(tmpDir, "original_setuid")
		err := os.WriteFile(setuidFile, []byte("#!/bin/bash\necho test"), 0o755)
		require.NoError(t, err)
		err = os.Chmod(setuidFile, 0o755|os.ModeSetuid) // setuid
		require.NoError(t, err)

		// Create a symbolic link to the setuid file
		symlinkFile := filepath.Join(tmpDir, "symlink_to_setuid")
		err = os.Symlink(setuidFile, symlinkFile)
		require.NoError(t, err)

		// Function should follow the symlink and detect setuid bit
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(symlinkFile)
		assert.NoError(t, err)
		assert.True(t, hasSetuidOrSetgid)
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(nonExistentFile)
		assert.Error(t, err, "should return error for non-existent file")
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("integration test with real setuid binary", func(t *testing.T) {
		// Check if passwd command exists and has setuid bit (common on most Unix systems)
		if fileInfo, err := os.Stat(passwdPath); err == nil && fileInfo.Mode()&os.ModeSetuid != 0 {
			hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(passwdPath)
			assert.NoError(t, err)
			assert.True(t, hasSetuidOrSetgid)
		} else {
			t.Skip("No setuid passwd binary found for integration test")
		}
	})

	t.Run("integration test with real setgid binary", func(t *testing.T) {
		// Check for common setgid binaries
		possibleSetgidPaths := []string{
			"/usr/bin/write",
			"/usr/bin/wall",
			"/usr/bin/expiry",
		}

		for _, path := range possibleSetgidPaths {
			if fileInfo, err := os.Stat(path); err == nil && fileInfo.Mode()&os.ModeSetgid != 0 {
				hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(path)
				assert.NoError(t, err)
				assert.True(t, hasSetuidOrSetgid)
				return // Found one, test passed
			}
		}
		t.Skip("No setgid binary found for integration test")
	})
}
