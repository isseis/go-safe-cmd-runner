package security

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const passwdPath = "/usr/bin/passwd"

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
		{
			name:     "ssh style with relative path",
			args:     []string{"user@host:file.txt"},
			expected: true,
		},
		{
			name:     "ssh style with bare directory name",
			args:     []string{"user@host:backup"},
			expected: true,
		},
		{
			name:     "scp with relative path",
			args:     []string{"root@server:backup.tar.gz", "./local/"},
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
			name:     "text with colon and path (not SSH)",
			args:     []string{"Current working directory: /tmp/test"},
			expected: false, // Space after colon indicates this is not SSH-style
		},
		{
			name:     "label with path (not SSH)",
			args:     []string{"Output path: ~/documents/file.txt"},
			expected: false, // Space after colon indicates this is not SSH-style
		},
		{
			name:     "user@host with space after colon (not SSH)",
			args:     []string{"user@host: /tmp/test"},
			expected: false, // Space after colon indicates this is not SSH-style
		},
		{
			name:     "user@host with tab after colon (not SSH)",
			args:     []string{"user@host:\t/tmp/test"},
			expected: false, // Tab after colon indicates this is not SSH-style
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
			expected: true, // Matches user@host:path pattern (relative path)
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
			assert.Equal(t, tt.expected, result, "containsSSHStyleAddress(%v)", tt.args)
		})
	}
}

func TestIsNetworkOperation(t *testing.T) {
	tests := []struct {
		name        string
		cmdName     string
		args        []string
		expectedNet bool
		description string
	}{
		// Always network commands
		{
			name:        "curl command",
			cmdName:     "curl",
			args:        []string{"https://example.com"},
			expectedNet: true,
			description: "curl is always a network command",
		},
		{
			name:        "wget command",
			cmdName:     "wget",
			args:        []string{"https://example.com/file.zip"},
			expectedNet: true,
			description: "wget is always a network command",
		},
		{
			name:        "ssh command",
			cmdName:     "ssh",
			args:        []string{"user@host"},
			expectedNet: true,
			description: "ssh is always a network command",
		},

		// Conditional network commands with network arguments
		{
			name:        "rsync with ssh-style address",
			cmdName:     "rsync",
			args:        []string{"-av", "user@host:/remote/", "./local/"},
			expectedNet: true,
			description: "rsync with SSH-style address should be detected as network",
		},
		{
			name:        "rsync with URL",
			cmdName:     "rsync",
			args:        []string{"rsync://host/module/path", "./local/"},
			expectedNet: true,
			description: "rsync with URL should be detected as network",
		},
		{
			name:        "git with https URL",
			cmdName:     "git",
			args:        []string{"clone", "https://github.com/user/repo.git"},
			expectedNet: true,
			description: "git with HTTPS URL should be detected as network",
		},

		// Conditional network commands without network arguments
		{
			name:        "rsync local only",
			cmdName:     "rsync",
			args:        []string{"-av", "./source/", "./destination/"},
			expectedNet: false,
			description: "rsync with only local paths should not be detected as network",
		},
		{
			name:        "git local operation",
			cmdName:     "git",
			args:        []string{"status"},
			expectedNet: false,
			description: "git local operation should not be detected as network",
		},
		{
			name:        "git fetch without URL",
			cmdName:     "git",
			args:        []string{"fetch"},
			expectedNet: true,
			description: "git fetch should be detected as network even without URL",
		},
		{
			name:        "git pull without URL",
			cmdName:     "git",
			args:        []string{"pull"},
			expectedNet: true,
			description: "git pull should be detected as network even without URL",
		},
		{
			name:        "git push without URL",
			cmdName:     "git",
			args:        []string{"push"},
			expectedNet: true,
			description: "git push should be detected as network even without URL",
		},
		{
			name:        "git clone with https URL",
			cmdName:     "git",
			args:        []string{"clone", "https://github.com/user/repo.git"},
			expectedNet: true,
			description: "git clone with URL should be detected as network",
		},
		{
			name:        "git remote update",
			cmdName:     "git",
			args:        []string{"remote", "update"},
			expectedNet: true,
			description: "git remote update should be detected as network",
		},
		{
			name:        "git fetch with options before subcommand",
			cmdName:     "git",
			args:        []string{"--no-pager", "fetch"},
			expectedNet: true,
			description: "git fetch with options should be detected as network",
		},
		{
			name:        "git pull with multiple options",
			cmdName:     "git",
			args:        []string{"--no-pager", "-c", "color.ui=false", "pull"},
			expectedNet: true,
			description: "git pull with multiple options should be detected as network",
		},
		{
			name:        "git push with options",
			cmdName:     "git",
			args:        []string{"-v", "push", "origin", "main"},
			expectedNet: true,
			description: "git push with options should be detected as network",
		},
		{
			name:        "git status with options (local operation)",
			cmdName:     "git",
			args:        []string{"--no-pager", "status"},
			expectedNet: false,
			description: "git status with options should not be detected as network",
		},

		// Non-network commands
		{
			name:        "ls command",
			cmdName:     "ls",
			args:        []string{"-la"},
			expectedNet: false,
			description: "ls should not be detected as network",
		},
		{
			name:        "echo with email",
			cmdName:     "echo",
			args:        []string{"Contact user@example.com"},
			expectedNet: false,
			description: "echo with email should not be detected as network",
		},

		// Edge cases: an unprofiled command is no longer classified as a network
		// operation from its arguments alone (consistent with the dry-run path).
		{
			name:        "unprofiled command with URL is not network",
			cmdName:     "somecommand",
			args:        []string{"https://example.com"},
			expectedNet: false,
			description: "an unprofiled command is not a network operation by argument alone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileNetwork(tt.cmdName, tt.args)
			assert.Equal(t, tt.expectedNet, got, "profileNetwork(%s, %v). %s",
				tt.cmdName, tt.args, tt.description)
		})
	}
}

func TestHasSetuidOrSetgidBit_Detailed(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := tu.SafeTempDir(t)

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
		// On macOS, non-root users may not be able to set setgid bit
		fileInfo, err := os.Stat(setgidFile)
		require.NoError(t, err)
		if fileInfo.Mode()&os.ModeSetgid == 0 {
			t.Skip("Skipping: OS silently ignored setgid bit (non-root on macOS)")
		}

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
		// On macOS, non-root users may not be able to set setgid bit
		fileInfo, err := os.Stat(bothBitsFile)
		require.NoError(t, err)
		assert.True(t, fileInfo.Mode()&os.ModeSetuid != 0, "setuid bit should be set")
		if fileInfo.Mode()&os.ModeSetgid == 0 {
			t.Skip("Skipping: OS silently ignored setgid bit (non-root on macOS)")
		}

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
		// On macOS, non-root users may not be able to set setgid bit
		fileInfo, err := os.Stat(setgidDir)
		require.NoError(t, err)
		if fileInfo.Mode()&os.ModeSetgid == 0 {
			t.Skip("Skipping: OS silently ignored setgid bit (non-root on macOS)")
		}
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

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		command  []string
		pattern  []string
		expected bool
	}{
		// Command name exact matching tests
		{
			name:     "exact command match",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "command name mismatch",
			command:  []string{"ls", "-la"},
			pattern:  []string{"rm", "-la"},
			expected: false,
		},
		{
			name:     "pattern longer than command",
			command:  []string{"rm"},
			pattern:  []string{"rm", "-rf", "/tmp"},
			expected: false,
		},

		// Regular argument exact matching tests
		{
			name:     "regular argument exact match",
			command:  []string{"chmod", "777", "/tmp/file"},
			pattern:  []string{"chmod", "777"},
			expected: true,
		},
		{
			name:     "regular argument mismatch",
			command:  []string{"chmod", "755", "/tmp/file"},
			pattern:  []string{"chmod", "777"},
			expected: false,
		},

		// Key-value pattern prefix matching tests (ending with "=")
		{
			name:     "dd if= pattern match",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/file"},
			pattern:  []string{"dd", "if="},
			expected: true,
		},
		{
			name:     "dd of= pattern match",
			command:  []string{"dd", "if=/dev/zero", "of=/dev/sda"},
			pattern:  []string{"dd", "of="},
			expected: true,
		},
		{
			name:     "dd if= pattern with specific value",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/file"},
			pattern:  []string{"dd", "if=/dev/kmsg"},
			expected: false, // exact match required for non-ending-with-"=" patterns
		},
		{
			name:     "key-value pattern without = in command",
			command:  []string{"dd", "input", "output"},
			pattern:  []string{"dd", "if="},
			expected: false,
		},
		{
			name:     "pattern with = at command name (index 0) - should use exact match",
			command:  []string{"test=value", "arg"},
			pattern:  []string{"test=", "arg"},
			expected: false, // command names require exact match
		},

		// Edge cases - empty command is a programming error and should not occur
		// {
		// 	name:     "empty command and pattern",
		// 	command:  []string{},
		// 	pattern:  []string{},
		// 	expected: true,
		// },
		{
			name:     "empty args pattern with command",
			command:  []string{"ls", "-r"},
			pattern:  []string{"ls"},
			expected: true,
		},
		{
			name:     "pattern with = but no = in command arg",
			command:  []string{"myapp", "config", "value"},
			pattern:  []string{"myapp", "config="},
			expected: false,
		},
		{
			name:     "complex dd command matching",
			command:  []string{"dd", "if=/dev/zero", "of=/tmp/test", "bs=1M", "count=10"},
			pattern:  []string{"dd", "if="},
			expected: true,
		},
		{
			name:     "multiple key-value patterns",
			command:  []string{"rsync", "src=/home", "dst=/backup", "opts=archive"},
			pattern:  []string{"rsync", "src=", "dst="},
			expected: true,
		},
		{
			name:     "mixed exact and prefix patterns",
			command:  []string{"mount", "-t", "ext4", "device=/dev/sdb1", "/mnt"},
			pattern:  []string{"mount", "-t", "ext4", "device="},
			expected: true,
		},

		// Additional test cases for thorough coverage
		{
			name:     "pattern ending with = but no equals in command",
			command:  []string{"cmd", "argument"},
			pattern:  []string{"cmd", "arg="},
			expected: false,
		},
		{
			name:     "argument with equals but different prefix",
			command:  []string{"dd", "if=/dev/sda", "bs=1M"},
			pattern:  []string{"dd", "of="},
			expected: false,
		},
		{
			name:     "exact match for command with equals sign",
			command:  []string{"export", "PATH=/usr/bin"},
			pattern:  []string{"export", "PATH=/usr/bin"},
			expected: true,
		},

		// Full path matching tests
		{
			name:     "full path command matches filename pattern",
			command:  []string{"/bin/rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "full path command matches full path pattern",
			command:  []string{"/bin/rm", "-rf", "/tmp"},
			pattern:  []string{"/bin/rm", "-rf"},
			expected: true,
		},
		{
			name:     "filename command matches filename pattern",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"rm", "-rf"},
			expected: true,
		},
		{
			name:     "filename command does not match full path pattern",
			command:  []string{"rm", "-rf", "/tmp"},
			pattern:  []string{"/bin/rm", "-rf"},
			expected: false,
		},
		{
			name:     "complex full path with filename pattern",
			command:  []string{"/usr/local/bin/custom-tool", "arg1"},
			pattern:  []string{"custom-tool", "arg1"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdName := ""
			cmdArgs := []string{}
			if len(tt.command) > 0 {
				cmdName = tt.command[0]
				cmdArgs = tt.command[1:]
			}
			result := matchesPattern(cmdNameSet(cmdName), cmdArgs, tt.pattern)
			assert.Equal(t, tt.expected, result, "matchesPattern(%s, %v, %v) should return %v", cmdName, cmdArgs, tt.pattern, tt.expected)
		})
	}
}

func TestExtractAllCommandNames(t *testing.T) {
	t.Run("simple filename", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames("echo")
		expected := map[string]struct{}{"echo": {}}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("full path", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames("/bin/echo")
		// /bin/echo may be a symlink on some systems (e.g., -> /usr/lib/cargo/bin/coreutils/echo
		// on Ubuntu 26.04+). Mirror extractAllCommandNames's own multi-level resolution loop to
		// build the expected set, so the test stays consistent with the implementation even when
		// the symlink chain has more than one hop.
		expected := map[string]struct{}{"/bin/echo": {}, "echo": {}}
		current := "/bin/echo"
		for range MaxSymlinkDepth {
			target, err := os.Readlink(current)
			if err != nil {
				break
			}
			if !filepath.IsAbs(target) {
				current = filepath.Join(filepath.Dir(current), target)
			} else {
				current = target
			}
			expected[current] = struct{}{}
			expected[filepath.Base(current)] = struct{}{}
		}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("non-existent file", func(t *testing.T) {
		// Test with a path that doesn't exist - should not crash
		names, exceededDepth := extractAllCommandNames("/non/existent/path/cmd")
		expected := map[string]struct{}{"/non/existent/path/cmd": {}, "cmd": {}}
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})

	t.Run("empty command name", func(t *testing.T) {
		// Test error case: empty command name should return empty map
		names, exceededDepth := extractAllCommandNames("")
		expected := make(map[string]struct{})
		assert.Equal(t, expected, names)
		assert.False(t, exceededDepth)
	})
}

func TestExtractAllCommandNamesWithSymlinks(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := tu.SafeTempDir(t)

	// Create the actual executable
	actualCmd := tmpDir + "/actual_echo"
	f, err := os.Create(actualCmd)
	require.NoError(t, err)
	f.Close()

	// Create first level symlink
	symlink1 := tmpDir + "/echo_link"
	err = os.Symlink(actualCmd, symlink1)
	require.NoError(t, err)

	// Create second level symlink (multi-level)
	symlink2 := tmpDir + "/echo_link2"
	err = os.Symlink(symlink1, symlink2)
	require.NoError(t, err)

	t.Run("single level symlink", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames(symlink1)

		// Should contain: original symlink name, base name, target, target base name
		assert.Contains(t, names, symlink1)
		assert.Contains(t, names, "echo_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("multi-level symlink", func(t *testing.T) {
		names, exceededDepth := extractAllCommandNames(symlink2)

		// Should contain all names in the chain
		assert.Contains(t, names, symlink2)
		assert.Contains(t, names, "echo_link2")
		assert.Contains(t, names, symlink1)
		assert.Contains(t, names, "echo_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("relative symlink", func(t *testing.T) {
		// Create a relative symlink
		relSymlink := tmpDir + "/rel_link"
		err = os.Symlink("actual_echo", relSymlink)
		require.NoError(t, err)

		names, exceededDepth := extractAllCommandNames(relSymlink)
		assert.Contains(t, names, relSymlink)
		assert.Contains(t, names, "rel_link")
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth)
	})

	t.Run("exceeds max symlink depth", func(t *testing.T) {
		// Create a chain that exceeds MaxSymlinkDepth (40)
		// For testing, we'll create a smaller chain and mock the depth check
		chainStart := tmpDir + "/deep_start"
		current := chainStart

		// Create a chain of 5 symlinks for testing (simulating deep chain)
		for i := range 5 {
			next := fmt.Sprintf("%s/link_%d", tmpDir, i)
			if i == 4 {
				// Last link points to actual file
				err = os.Symlink(actualCmd, current)
			} else {
				err = os.Symlink(next, current)
			}
			require.NoError(t, err)
			current = next
		}

		names, exceededDepth := extractAllCommandNames(chainStart)

		// Should contain the original link and some resolved names
		assert.Contains(t, names, chainStart)
		assert.Contains(t, names, "deep_start")

		// Should contain the final target if chain is within limit
		assert.Contains(t, names, actualCmd)
		assert.Contains(t, names, "actual_echo")
		assert.False(t, exceededDepth, "Chain should be within depth limit")
	})
}

// TestExtractAllCommandNamesBareNameIgnoresCWD verifies that a bare command name
// (no path separator) is resolved by its name alone and never through the
// filesystem, so a same-named entry in the working directory cannot influence
// name-based classification. PATH resolution happens at exec time, not here.
func TestExtractAllCommandNamesBareNameIgnoresCWD(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	// A working-directory symlink whose name collides with a command and points at
	// a privilege binary. Without the bare-name guard, extractAllCommandNames("rm")
	// would follow this and add "sudo" to the name set.
	require.NoError(t, os.Symlink("/usr/bin/sudo", tmpDir+"/rm"))

	cwd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	require.NoError(t, os.Chdir(tmpDir))

	names, exceededDepth := extractAllCommandNames("rm")
	assert.False(t, exceededDepth)
	assert.Contains(t, names, "rm", "the bare name itself is always present")
	assert.NotContains(t, names, "sudo", "a CWD symlink must not influence bare-name resolution")
	assert.Len(t, names, 1, "a bare name resolves to only itself")
}

func TestHasSetuidOrSetgidBit(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := tu.SafeTempDir(t)

	t.Run("normal file", func(t *testing.T) {
		normalFile := filepath.Join(tmpDir, "normal")
		err := os.WriteFile(normalFile, []byte("test"), 0o644)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(normalFile)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	// Integration test with real setuid binary
	t.Run("real setuid binary", func(t *testing.T) {
		// Check if passwd command exists and has setuid bit
		if fileInfo, err := os.Stat(passwdPath); err == nil && fileInfo.Mode()&os.ModeSetuid != 0 {
			hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(passwdPath)
			assert.NoError(t, err)
			assert.True(t, hasSetuidOrSetgid)
		} else {
			t.Skip("No setuid passwd binary found for integration test")
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := filepath.Join(tmpDir, "testdir")
		err := os.Mkdir(dir, 0o755)
		require.NoError(t, err)

		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit(dir)
		assert.NoError(t, err)
		assert.False(t, hasSetuidOrSetgid) // directories are not regular files
	})

	t.Run("non-existent file", func(t *testing.T) {
		hasSetuidOrSetgid, err := hasSetuidOrSetgidBit("/non/existent/file")
		assert.Error(t, err)
		assert.False(t, hasSetuidOrSetgid)
	})

	t.Run("relative path - command in PATH", func(t *testing.T) {
		_, err := hasSetuidOrSetgidBit("echo")
		assert.Error(t, err)
	})
}

func TestIsDestructiveFileOperation(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected bool
	}{
		{
			name:     "rm command",
			cmd:      "rm",
			args:     []string{"file.txt"},
			expected: true,
		},
		{
			name:     "rm with force flag",
			cmd:      "rm",
			args:     []string{"-rf", "/tmp/test"},
			expected: true,
		},
		{
			name:     "rmdir command",
			cmd:      "rmdir",
			args:     []string{"directory"},
			expected: true,
		},
		{
			name:     "unlink command",
			cmd:      "unlink",
			args:     []string{"/tmp/file"},
			expected: true,
		},
		{
			name:     "shred command",
			cmd:      "shred",
			args:     []string{"-u", "file.txt"},
			expected: true,
		},
		{
			name:     "find with delete",
			cmd:      "find",
			args:     []string{".", "-name", "*.tmp", "-delete"},
			expected: true,
		},
		{
			name:     "find with exec rm",
			cmd:      "find",
			args:     []string{".", "-name", "*.tmp", "-exec", "rm", "{}", ";"},
			expected: true,
		},
		{
			name:     "find with exec shred (destructive)",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.tmp", "-exec", "shred", "-u", "{}", ";"},
			expected: true,
		},
		{
			name:     "find with exec stat (safe)",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.log", "-exec", "stat", "{}", ";"},
			expected: false,
		},
		{
			name:     "find with exec cat (safe)",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.log", "-exec", "cat", "{}", ";"},
			expected: false,
		},
		{
			name:     "rsync with delete",
			cmd:      "rsync",
			args:     []string{"-av", "--delete", "src/", "dst/"},
			expected: true,
		},
		{
			name:     "rsync with delete-before",
			cmd:      "rsync",
			args:     []string{"-av", "--delete-before", "src/", "dst/"},
			expected: true,
		},
		{
			name:     "rsync with delete-after",
			cmd:      "rsync",
			args:     []string{"-av", "--delete-after", "src/", "dst/"},
			expected: true,
		},
		{
			name:     "dd command",
			cmd:      "dd",
			args:     []string{"if=/dev/zero", "of=/tmp/test", "bs=1M", "count=10"},
			expected: true,
		},
		{
			name:     "safe ls",
			cmd:      "ls",
			args:     []string{"-la"},
			expected: false,
		},
		{
			name:     "safe find",
			cmd:      "find",
			args:     []string{".", "-name", "*.txt"},
			expected: false,
		},
		{
			name:     "safe rsync",
			cmd:      "rsync",
			args:     []string{"-av", "src/", "dst/"},
			expected: false,
		},
		{
			name:     "safe cat",
			cmd:      "cat",
			args:     []string{"file.txt"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDestructiveFileOperation(cmdNameSet(tt.cmd), tt.args)
			assert.Equal(t, tt.expected, result, "IsDestructiveFileOperation(%q, %v)", tt.cmd, tt.args)
		})
	}
}

// TestIsDestructive_AbsolutePath verifies that a destructive command given as a
// resolved absolute path (e.g. /usr/bin/rm) is detected, not only its basename.
func TestIsDestructive_AbsolutePath(t *testing.T) {
	assert.True(t, IsDestructiveFileOperation(cmdNameSet("/usr/bin/rm"), []string{"file"}))
	assert.True(t, IsDestructiveFileOperation(cmdNameSet("/bin/rm"), []string{"-rf", "/tmp/x"}))
	assert.True(t, IsDestructiveFileOperation(cmdNameSet("/usr/bin/shred"), []string{"f"}))
}

// TestIsDestructive_NoSubstringMatch verifies that a command whose basename
// merely contains a destructive name as a substring is not matched.
func TestIsDestructive_NoSubstringMatch(t *testing.T) {
	assert.False(t, IsDestructiveFileOperation(cmdNameSet("/usr/bin/lsrm"), []string{"x"}))
	assert.False(t, IsDestructiveFileOperation(cmdNameSet("/usr/bin/rmate"), nil))
}

// TestIsDestructive_BasenameBackwardCompat verifies that a bare basename is
// still detected (the path-resolution addition does not lose basename detection).
func TestIsDestructive_BasenameBackwardCompat(t *testing.T) {
	assert.True(t, IsDestructiveFileOperation(cmdNameSet("rm"), []string{"file"}))
	assert.True(t, IsDestructiveFileOperation(cmdNameSet("dd"), []string{"if=/dev/zero"}))
}

// TestSystemModificationRisk verifies the name-only fixed-level classification:
// package managers and service/init management are High regardless of arguments
// (queries are treated identically to installs), the remaining name-matched
// commands are Medium, and anything else (or a name appearing only as an argument
// value) is Unknown. Matching is by basename and resolved symlinks.
func TestSystemModificationRisk(t *testing.T) {
	tests := []struct {
		name  string
		names map[string]struct{}
		want  runnertypes.RiskLevel
	}{
		// Package managers -> High, install and query alike.
		{"apt", cmdNameSet("apt"), runnertypes.RiskLevelHigh},
		{"apt-get", cmdNameSet("apt-get"), runnertypes.RiskLevelHigh},
		{"yum", cmdNameSet("yum"), runnertypes.RiskLevelHigh},
		{"dnf", cmdNameSet("dnf"), runnertypes.RiskLevelHigh},
		{"zypper", cmdNameSet("zypper"), runnertypes.RiskLevelHigh},
		{"pacman", cmdNameSet("pacman"), runnertypes.RiskLevelHigh},
		{"brew", cmdNameSet("brew"), runnertypes.RiskLevelHigh},
		{"pip", cmdNameSet("pip"), runnertypes.RiskLevelHigh},
		{"npm", cmdNameSet("npm"), runnertypes.RiskLevelHigh},
		{"yarn", cmdNameSet("yarn"), runnertypes.RiskLevelHigh},
		{"dpkg", cmdNameSet("dpkg"), runnertypes.RiskLevelHigh},
		{"rpm", cmdNameSet("rpm"), runnertypes.RiskLevelHigh},
		// Service / init management -> High.
		{"systemctl", cmdNameSet("systemctl"), runnertypes.RiskLevelHigh},
		{"service", cmdNameSet("service"), runnertypes.RiskLevelHigh},
		// Medium name-matched commands stay Medium.
		{"mount", cmdNameSet("mount"), runnertypes.RiskLevelMedium},
		{"umount", cmdNameSet("umount"), runnertypes.RiskLevelMedium},
		{"fdisk", cmdNameSet("fdisk"), runnertypes.RiskLevelMedium},
		{"parted", cmdNameSet("parted"), runnertypes.RiskLevelMedium},
		{"mkfs", cmdNameSet("mkfs"), runnertypes.RiskLevelMedium},
		{"fsck", cmdNameSet("fsck"), runnertypes.RiskLevelMedium},
		{"crontab", cmdNameSet("crontab"), runnertypes.RiskLevelMedium},
		{"at", cmdNameSet("at"), runnertypes.RiskLevelMedium},
		{"batch", cmdNameSet("batch"), runnertypes.RiskLevelMedium},
		{"chkconfig", cmdNameSet("chkconfig"), runnertypes.RiskLevelMedium},
		{"update-rc.d", cmdNameSet("update-rc.d"), runnertypes.RiskLevelMedium},
		// Non-matching names -> Unknown. Because the function takes only the
		// resolved name set, a pm name that appears only as an argument value (e.g.
		// "echo rpm") can never reach this dimension; that guarantee is structural,
		// so these cases just confirm an unrelated command yields Unknown.
		{"echo", cmdNameSet("echo"), runnertypes.RiskLevelUnknown},
		{"ls", cmdNameSet("ls"), runnertypes.RiskLevelUnknown},
		// symlink / absolute path resolution.
		{"/usr/sbin/systemctl absolute", cmdNameSet("/usr/sbin/systemctl"), runnertypes.RiskLevelHigh},
		// A substring match must not be treated as systemctl.
		{"systemctl-helper not matched", cmdNameSet("systemctl-helper"), runnertypes.RiskLevelUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SystemModificationRisk(tt.names))
		})
	}
}

// TestFindExecAllActions verifies that find's exec-style actions
// (-exec/-execdir/-ok/-okdir) are all covered, and the target command is matched
// by basename including absolute and coreutils-directory paths.
func TestFindExecAllActions(t *testing.T) {
	for _, action := range []string{"-exec", "-execdir", "-ok", "-okdir"} {
		assert.Truef(t, IsDestructiveFileOperation(cmdNameSet("find"),
			[]string{".", action, "/usr/bin/rm", "{}", ";"}),
			"find %s /usr/bin/rm should be destructive", action)
		assert.Falsef(t, IsDestructiveFileOperation(cmdNameSet("find"),
			[]string{".", action, "/usr/bin/stat", "{}", ";"}),
			"find %s /usr/bin/stat should be safe", action)
	}
}

func TestIsNetworkOperation_FromEvaluatorTests(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected bool
	}{
		{
			name:     "wget with URL",
			cmd:      "wget",
			args:     []string{"https://example.com/file.txt"},
			expected: true,
		},
		{
			name:     "curl with URL",
			cmd:      "curl",
			args:     []string{"-O", "https://example.com/file.txt"},
			expected: true,
		},
		{
			name:     "ssh command",
			cmd:      "ssh",
			args:     []string{"user@host"},
			expected: true,
		},
		{
			name:     "rsync with remote",
			cmd:      "rsync",
			args:     []string{"-av", "user@host:/path/", "local/"},
			expected: true,
		},
		{
			name:     "git with URL",
			cmd:      "git",
			args:     []string{"clone", "https://github.com/user/repo.git"},
			expected: true,
		},
		{
			name:     "unprofiled command with http URL in args is not network",
			cmd:      "myapp",
			args:     []string{"--url", "http://api.example.com"},
			expected: false,
		},
		{
			name:     "safe local git",
			cmd:      "git",
			args:     []string{"status"},
			expected: false,
		},
		{
			name:     "safe local rsync",
			cmd:      "rsync",
			args:     []string{"-av", "src/", "dst/"},
			expected: false,
		},
		{
			name:     "safe command",
			cmd:      "ls",
			args:     []string{"-la"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profileNetwork(tt.cmd, tt.args)
			assert.Equal(t, tt.expected, got, "profileNetwork(%q, %v)", tt.cmd, tt.args)
		})
	}
}

func TestValidator_IsDangerousPrivilegedCommand(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name     string
		cmdPath  string
		expected bool
	}{
		{
			name:     "shell command is dangerous",
			cmdPath:  "/bin/bash",
			expected: true,
		},
		{
			name:     "sudo is dangerous",
			cmdPath:  "/usr/bin/sudo",
			expected: true,
		},
		{
			name:     "rm is dangerous",
			cmdPath:  "/bin/rm",
			expected: true,
		},
		{
			name:     "safe command like ls",
			cmdPath:  "/bin/ls",
			expected: false,
		},
		{
			name:     "echo is safe",
			cmdPath:  "/bin/echo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsDangerousPrivilegedCommand(tt.cmdPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_IsShellCommand(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name     string
		cmdPath  string
		expected bool
	}{
		{
			name:     "bash is shell command",
			cmdPath:  "/bin/bash",
			expected: true,
		},
		{
			name:     "sh is shell command",
			cmdPath:  "/bin/sh",
			expected: true,
		},
		{
			name:     "zsh is shell command",
			cmdPath:  "/bin/zsh",
			expected: true,
		},
		{
			name:     "fish is shell command",
			cmdPath:  "/bin/fish",
			expected: true,
		},
		{
			name:     "ls is not shell command",
			cmdPath:  "/bin/ls",
			expected: false,
		},
		{
			name:     "sudo is not shell command",
			cmdPath:  "/usr/bin/sudo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsShellCommand(tt.cmdPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommandRiskProfiles_Completeness(t *testing.T) {
	// Ensure all defined profiles are valid
	for cmdName, profile := range commandRiskProfiles {
		t.Run("validate_"+cmdName, func(t *testing.T) {
			assert.NotEmpty(t, cmdName, "command name should not be empty")
			assert.True(t, profile.BaseRiskLevel() > runnertypes.RiskLevelUnknown && profile.BaseRiskLevel() <= runnertypes.RiskLevelCritical,
				"risk level should be valid: %d", profile.BaseRiskLevel())
			assert.NotEmpty(t, profile.GetRiskReasons(), "reasons should not be empty")
			assert.True(t, profile.NetworkType >= NetworkTypeNone && profile.NetworkType <= NetworkTypeConditional,
				"network type should be valid: %d", profile.NetworkType)
		})
	}
}

func TestCommandRiskProfiles_PrivilegeEscalation(t *testing.T) {
	// Test that all privilege escalation commands are properly flagged
	privilegeCommands := []string{"sudo", "su", "doas"}
	for _, cmd := range privilegeCommands {
		t.Run(cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists, "privilege command %s should exist in profiles", cmd)
			if exists {
				assert.True(t, profile.IsPrivilege(), "command %s should be marked as privilege escalation", cmd)
				assert.Equal(t, runnertypes.RiskLevelCritical, profile.BaseRiskLevel(), "privilege command %s should have critical risk", cmd)
			}
		})
	}
}

func TestCommandRiskProfiles_NetworkCommands(t *testing.T) {
	testCases := []struct {
		name        string
		cmd         string
		networkType NetworkOperationType
	}{
		{"curl is always network", "curl", NetworkTypeAlways},
		{"wget is always network", "wget", NetworkTypeAlways},
		{"ssh is always network", "ssh", NetworkTypeAlways},
		{"scp is always network", "scp", NetworkTypeAlways},
		{"nc is always network", "nc", NetworkTypeAlways},
		{"netcat is always network", "netcat", NetworkTypeAlways},
		{"telnet is always network", "telnet", NetworkTypeAlways},
		{"rsync is conditional network", "rsync", NetworkTypeConditional},
		{"git is conditional network", "git", NetworkTypeConditional},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile, exists := commandRiskProfiles[tc.cmd]
			assert.True(t, exists, "network command %s should exist in profiles", tc.cmd)
			if exists {
				assert.Equal(t, tc.networkType, profile.NetworkType, "command %s should have correct network type", tc.cmd)
			}
		})
	}
}

// TestCommandRiskProfiles_AdditionalInterpreters verifies that newly added language
// interpreters and runtimes are registered as NetworkTypeAlways.
func TestCommandRiskProfiles_AdditionalInterpreters(t *testing.T) {
	testCases := []struct {
		name string
		cmd  string
	}{
		{"luajit is always network", "luajit"},
		{"tclsh is always network", "tclsh"},
		{"R is always network", "R"},
		{"julia is always network", "julia"},
		{"guile is always network", "guile"},
		{"erl is always network", "erl"},
		{"elixir is always network", "elixir"},
		{"java is always network", "java"},
		{"groovy is always network", "groovy"},
		{"scala is always network", "scala"},
		{"dotnet is always network", "dotnet"},
		{"pwsh is always network", "pwsh"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile, exists := commandRiskProfiles[tc.cmd]
			assert.True(t, exists, "command %s should exist in profiles", tc.cmd)
			if exists {
				assert.Equal(t, NetworkTypeAlways, profile.NetworkType,
					"command %s should have NetworkTypeAlways", tc.cmd)
			}
		})
	}
}

// TestAllProfilesAreValid verifies that all command profiles pass validation
func TestAllProfilesAreValid(t *testing.T) {
	for _, def := range commandProfileDefinitions {
		err := def.Profile().Validate()
		assert.NoError(t, err, "Profile for commands %v should be valid", def.Commands())
	}
}

// TestAllProfilesHaveReasons verifies that all profiles with non-Unknown risk level have reasons
func TestAllProfilesHaveReasons(t *testing.T) {
	for _, def := range commandProfileDefinitions {
		profile := def.Profile()
		baseRisk := profile.BaseRiskLevel()
		reasons := profile.GetRiskReasons()

		// Only profiles with risk level > Unknown should have reasons
		if baseRisk > runnertypes.RiskLevelUnknown {
			assert.NotEmpty(t, reasons,
				"Profile for commands %v has risk level %v but no reasons",
				def.Commands(), baseRisk)
		}
	}
}

// TestMigration_RiskLevelConsistency verifies that migrated profiles maintain expected risk levels
func TestMigration_RiskLevelConsistency(t *testing.T) {
	tests := []struct {
		command      string
		expectedRisk runnertypes.RiskLevel
	}{
		// Privilege escalation - Critical
		{"sudo", runnertypes.RiskLevelCritical},
		{"su", runnertypes.RiskLevelCritical},
		{"doas", runnertypes.RiskLevelCritical},

		// System modification - High
		{"systemctl", runnertypes.RiskLevelHigh},
		{"service", runnertypes.RiskLevelHigh},

		// Destructive operations
		{"rm", runnertypes.RiskLevelHigh},
		{"dd", runnertypes.RiskLevelHigh},

		// AI services - High
		{"claude", runnertypes.RiskLevelHigh},
		{"gemini", runnertypes.RiskLevelHigh},
		{"chatgpt", runnertypes.RiskLevelHigh},
		{"gpt", runnertypes.RiskLevelHigh},
		{"openai", runnertypes.RiskLevelHigh},
		{"anthropic", runnertypes.RiskLevelHigh},

		// Network commands (always) - Medium
		{"curl", runnertypes.RiskLevelMedium},
		{"wget", runnertypes.RiskLevelMedium},
		{"nc", runnertypes.RiskLevelMedium},
		{"netcat", runnertypes.RiskLevelMedium},
		{"telnet", runnertypes.RiskLevelMedium},
		{"ssh", runnertypes.RiskLevelMedium},
		{"scp", runnertypes.RiskLevelMedium},
		{"aws", runnertypes.RiskLevelMedium},

		// Network commands (conditional) - Medium (changed from Low)
		{"git", runnertypes.RiskLevelMedium},
		{"rsync", runnertypes.RiskLevelMedium},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			profile, exists := commandRiskProfiles[tt.command]
			assert.True(t, exists, "Command %s should exist in profiles", tt.command)
			if exists {
				assert.Equal(t, tt.expectedRisk, profile.BaseRiskLevel(),
					"Risk level mismatch for command %s", tt.command)
			}
		})
	}
}

// TestMigration_NetworkTypeConsistency verifies that network types are correctly migrated
func TestMigration_NetworkTypeConsistency(t *testing.T) {
	alwaysNetwork := []string{
		"curl", "wget", "nc", "netcat", "telnet", "ssh", "scp", "aws",
		"claude", "gemini", "chatgpt", "gpt", "openai", "anthropic",
		// Shells - can execute arbitrary network commands
		"bash", "sh", "dash", "zsh", "ksh", "csh", "tcsh", "fish",
		// Script interpreters - have built-in network capabilities
		"node", "nodejs", "deno", "bun", "php",
		// Additional language runtimes
		"lua", "luajit", "tclsh", "R", "Rscript", "julia",
		"guile", "elixir", "iex", "erl", "erlc", "escript",
		"java", "javaw", "groovy", "kotlin", "scala",
		"dotnet", "mono", "pwsh", "powershell",
	}
	conditionalNetwork := []string{"git", "rsync"}
	noneNetwork := []string{"sudo", "su", "doas", "systemctl", "service", "rm", "dd"}

	for _, cmd := range alwaysNetwork {
		t.Run("AlwaysNetwork_"+cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists)
			assert.Equal(t, NetworkTypeAlways, profile.NetworkType)
		})
	}

	for _, cmd := range conditionalNetwork {
		t.Run("ConditionalNetwork_"+cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists)
			assert.Equal(t, NetworkTypeConditional, profile.NetworkType)
		})
	}

	for _, cmd := range noneNetwork {
		t.Run("NoneNetwork_"+cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists)
			assert.Equal(t, NetworkTypeNone, profile.NetworkType)
		})
	}
}

// TestMigration_NetworkSubcommandsConsistency verifies network subcommands are correctly migrated
func TestMigration_NetworkSubcommandsConsistency(t *testing.T) {
	tests := []struct {
		command     string
		subcommands []string
	}{
		{"git", []string{"clone", "fetch", "pull", "push", "remote"}},
		{"rsync", nil}, // nil - uses argument-based detection
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			profile, exists := commandRiskProfiles[tt.command]
			assert.True(t, exists)
			if exists {
				if tt.subcommands == nil {
					assert.Empty(t, profile.NetworkSubcommands,
						"Network subcommands should be empty for command %s", tt.command)
				} else {
					assert.Equal(t, tt.subcommands, profile.NetworkSubcommands,
						"Network subcommands mismatch for command %s", tt.command)
				}
			}
		})
	}
}

// TestMigration_IsPrivilegeConsistency verifies IsPrivilege flag is correctly set
func TestMigration_IsPrivilegeConsistency(t *testing.T) {
	privilegeCommands := []string{"sudo", "su", "doas"}
	nonPrivilegeCommands := []string{"rm", "dd", "curl", "git", "systemctl"}

	for _, cmd := range privilegeCommands {
		t.Run("Privilege_"+cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists)
			assert.True(t, profile.IsPrivilege(), "Command %s should have IsPrivilege=true", cmd)
		})
	}

	for _, cmd := range nonPrivilegeCommands {
		t.Run("NonPrivilege_"+cmd, func(t *testing.T) {
			profile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists)
			assert.False(t, profile.IsPrivilege(), "Command %s should have IsPrivilege=false", cmd)
		})
	}
}

// TestMigration_MultipleRiskFactors verifies AI service commands have multiple risk factors
func TestMigration_MultipleRiskFactors(t *testing.T) {
	aiCommands := []string{"claude", "gemini", "chatgpt", "gpt", "openai", "anthropic"}

	for _, cmd := range aiCommands {
		t.Run(cmd, func(t *testing.T) {
			newProfile, exists := commandRiskProfiles[cmd]
			assert.True(t, exists, "Command %s should exist in new profiles", cmd)
			if !exists {
				return
			}

			// Should have both NetworkRisk and DataExfilRisk
			assert.Equal(t, runnertypes.RiskLevelHigh, newProfile.NetworkRisk.Level,
				"Command %s should have High NetworkRisk", cmd)
			assert.Equal(t, runnertypes.RiskLevelHigh, newProfile.DataExfilRisk.Level,
				"Command %s should have High DataExfilRisk", cmd)

			// Should have multiple reasons
			reasons := newProfile.GetRiskReasons()
			assert.GreaterOrEqual(t, len(reasons), 2,
				"Command %s should have at least 2 risk reasons", cmd)
		})
	}
}

// TestProfileNetworkApplies_Conditional tests profile-based network detection.
// Network classification now comes only from the command's profile (Always or
// Conditional + subcommand/argument); unprofiled commands are not network
// operations regardless of their arguments.
func TestProfileNetworkApplies_Conditional(t *testing.T) {
	tests := []struct {
		name          string
		cmdName       string
		args          []string
		expectNetwork bool
	}{
		{
			name:          "profile command curl",
			cmdName:       "curl",
			args:          []string{"http://example.com"},
			expectNetwork: true,
		},
		{
			name:          "profile command git without network subcommand",
			cmdName:       "git",
			args:          []string{"status"},
			expectNetwork: false,
		},
		{
			name:          "profile command git with network subcommand",
			cmdName:       "git",
			args:          []string{"fetch", "origin"},
			expectNetwork: true,
		},
		{
			name:          "unprofiled command without URL is not network",
			cmdName:       "/bin/ls",
			args:          []string{"-la"},
			expectNetwork: false,
		},
		{
			name:          "unprofiled command with URL is not network",
			cmdName:       "/bin/ls",
			args:          []string{"http://example.com"},
			expectNetwork: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := profileNetwork(tc.cmdName, tc.args)
			assert.Equal(t, tc.expectNetwork, got, "profileNetwork mismatch")
		})
	}
}

// TestFormatDetectedSymbols tests the formatDetectedSymbols helper function.
func TestFormatDetectedSymbols(t *testing.T) {
	tests := []struct {
		name     string
		symbols  []binaryanalyzer.DetectedSymbol
		expected string
	}{
		{
			name:     "empty symbols",
			symbols:  []binaryanalyzer.DetectedSymbol{},
			expected: "[]",
		},
		{
			name:     "nil symbols",
			symbols:  nil,
			expected: "[]",
		},
		{
			name: "single symbol",
			symbols: []binaryanalyzer.DetectedSymbol{
				{Name: "socket", Category: "socket"},
			},
			expected: "[socket(socket)]",
		},
		{
			name: "multiple symbols",
			symbols: []binaryanalyzer.DetectedSymbol{
				{Name: "socket", Category: "socket"},
				{Name: "connect", Category: "socket"},
				{Name: "getaddrinfo", Category: "dns"},
			},
			expected: "[socket(socket), connect(socket), getaddrinfo(dns)]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatDetectedSymbols(tc.symbols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestNewNetworkAnalyzer tests the creation of NetworkAnalyzer.
func TestNewNetworkAnalyzer(t *testing.T) {
	t.Run("creates analyzer", func(t *testing.T) {
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, nil)
		assert.NotNil(t, analyzer)
		assert.False(t, analyzer.AnalysisEnabled(), "nil store means analysis disabled")

		// With analysis disabled, Classify is fail-closed: Uncertain, never Clean.
		res, err := analyzer.Classify("/usr/bin/unknowncmd", "sha256:dummy")
		require.NoError(t, err)
		assert.Equal(t, risktypes.BinaryAnalysisUncertain, res.Class, "nil store must classify as Uncertain")
		assert.Contains(t, res.ReasonCodes, risktypes.ReasonAnalysisDisabled)
	})
}

// stubRecordStore is a test double for RecordStore.
type stubRecordStore struct {
	record *fileanalysis.Record
	err    error
}

func (s *stubRecordStore) LoadRecord(_ string) (*fileanalysis.Record, error) {
	return s.record, s.err
}

// callTrackingRecordStore records whether LoadRecord was called.
type callTrackingRecordStore struct {
	called *bool
	record *fileanalysis.Record
	err    error
}

func (s *callTrackingRecordStore) LoadRecord(_ string) (*fileanalysis.Record, error) {
	*s.called = true
	return s.record, s.err
}

// TestIsNetworkViaBinaryAnalysis_AnalysisStore tests the record store path in analyzeBinarySignals.
func TestIsNetworkViaBinaryAnalysis_AnalysisStore(t *testing.T) {
	const cmdPath = "/usr/bin/curl"
	const contentHash = "sha256:abc123"

	t.Run("record.SymbolAnalysis=[socket] → NetworkDetected", func(t *testing.T) {
		store := &stubRecordStore{
			record: &fileanalysis.Record{
				ContentHash: contentHash,
				SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
					DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
				},
			},
		}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.True(t, isNet, "expected network detected from record")
		assert.False(t, isHigh, "expected not high risk (no dlopen)")
	})

	t.Run("record.SymbolAnalysis.DetectedSymbols=nil → NoNetworkSymbols", func(t *testing.T) {
		store := &stubRecordStore{
			record: &fileanalysis.Record{
				ContentHash: contentHash,
				SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
					DetectedSymbols:    nil,
					DynamicLoadSymbols: nil,
				},
			},
		}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.False(t, isNet, "expected no network from record")
		assert.False(t, isHigh, "expected not high risk")
	})

	t.Run("record.SymbolAnalysis.DynamicLoadSymbols=[dlopen] → isHighRisk=true", func(t *testing.T) {
		store := &stubRecordStore{
			record: &fileanalysis.Record{
				ContentHash: contentHash,
				SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
					DynamicLoadSymbols: []fileanalysis.DetectedSymbol{{Name: "dlopen"}},
				},
			},
		}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.False(t, isNet, "expected no network (DetectedSymbols=nil)")
		assert.True(t, isHigh, "expected high risk from dlopen in record")
	})

	t.Run("record.SymbolAnalysis=nil → false, false (static binary)", func(t *testing.T) {
		store := &stubRecordStore{
			record: &fileanalysis.Record{
				ContentHash: contentHash,
			},
		}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.False(t, isNet, "static binary with no svc should return false")
		assert.False(t, isHigh, "static binary with no svc should return false")
	})

	t.Run("SchemaVersionMismatchError → high risk", func(t *testing.T) {
		schemaErr := &fileanalysis.SchemaVersionMismatchError{Expected: fileanalysis.CurrentSchemaVersion, Actual: fileanalysis.CurrentSchemaVersion - 1}
		store := &stubRecordStore{err: schemaErr}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.True(t, isNet, "schema mismatch must return isNetwork=true as safety measure")
		assert.True(t, isHigh, "schema mismatch must return high risk")
	})

	t.Run("nil store → uncertain (fail-closed)", func(t *testing.T) {
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, nil)
		res, err := analyzer.Classify(cmdPath, contentHash)
		require.NoError(t, err)
		assert.Equal(t, risktypes.BinaryAnalysisUncertain, res.Class,
			"analysis disabled must be uncertain, not fail-open")
		assert.Contains(t, res.ReasonCodes, risktypes.ReasonAnalysisDisabled)
	})

	t.Run("empty contentHash → high risk (fail-closed)", func(t *testing.T) {
		storeCalled := false
		store := &callTrackingRecordStore{called: &storeCalled}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, "")
		require.NoError(t, err)
		assert.False(t, storeCalled, "store must not be called when contentHash is empty")
		assert.True(t, isNet, "unverified binary must be treated as high risk (fail-closed)")
		assert.True(t, isHigh, "unverified binary must be treated as high risk (fail-closed)")
	})

	t.Run("ErrRecordNotFound → true, true (fail-closed)", func(t *testing.T) {
		store := &stubRecordStore{err: fileanalysis.ErrRecordNotFound}
		analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)
		isNet, isHigh, err := analyzer.analyzeBinarySignals(cmdPath, contentHash)
		require.NoError(t, err)
		assert.True(t, isNet, "missing analysis record must be treated as high risk (fail-closed)")
		assert.True(t, isHigh, "missing analysis record must be treated as high risk (fail-closed)")
	})
}

// TestNetworkSymbolAnalysisStore_RecordToRunner tests the record→runner analysis flow.
// Verifies that NetworkAnalyzer correctly reads SymbolAnalysis from a preloaded Record.
func TestNetworkSymbolAnalysisStore_RecordToRunner(t *testing.T) {
	const cmdPath = "/usr/bin/fake-curl"
	const fakeHash = "sha256:deadbeef"

	store := &stubRecordStore{
		record: &fileanalysis.Record{
			ContentHash: fakeHash,
			SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
				DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
			},
		},
	}
	analyzer := newNetworkAnalyzerWithStore(runtime.GOOS, store)

	isNet, _, err := analyzer.analyzeBinarySignals(cmdPath, fakeHash)
	require.NoError(t, err)

	assert.True(t, isNet, "record with socket symbol should report network detected")
}
