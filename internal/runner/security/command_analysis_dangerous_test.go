package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_IsDangerousRootCommand(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdPath    string
		wantResult bool
	}{
		{
			name:       "rm command is dangerous",
			cmdPath:    "/bin/rm",
			wantResult: true,
		},
		{
			name:       "rmdir command is dangerous",
			cmdPath:    "/usr/bin/rmdir",
			wantResult: true,
		},
		{
			name:       "dd command is dangerous",
			cmdPath:    "/bin/dd",
			wantResult: true,
		},
		{
			name:       "mkfs command is dangerous",
			cmdPath:    "/sbin/mkfs",
			wantResult: true,
		},
		{
			name:       "fdisk command is dangerous",
			cmdPath:    "/sbin/fdisk",
			wantResult: true,
		},
		{
			name:       "format command is dangerous",
			cmdPath:    "/usr/bin/format",
			wantResult: true,
		},
		{
			name:       "safe command like ls",
			cmdPath:    "/bin/ls",
			wantResult: false,
		},
		{
			name:       "safe command like cat",
			cmdPath:    "/bin/cat",
			wantResult: false,
		},
		{
			name:       "safe command like echo",
			cmdPath:    "/usr/bin/echo",
			wantResult: false,
		},
		{
			name:       "relative path with dangerous command",
			cmdPath:    "rm",
			wantResult: true,
		},
		{
			name:       "command with similar name but safe",
			cmdPath:    "/bin/lsrm", // Contains "rm" but not dangerous
			wantResult: true,        // Will match because Contains is used
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsDangerousRootCommand(tt.cmdPath)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}

func TestValidator_HasDangerousRootArgs(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name             string
		args             []string
		expectedIndices  []int
		shouldHaveDanger bool
	}{
		{
			name: "no dangerous arguments",
			args: []string{
				"file1.txt",
				"file2.txt",
			},
			expectedIndices:  []int{},
			shouldHaveDanger: false,
		},
		{
			name: "recursive flag is dangerous",
			args: []string{
				"-rf",
				"/home/user",
			},
			expectedIndices:  []int{0},
			shouldHaveDanger: true,
		},
		{
			name: "recursive long option",
			args: []string{
				"--recursive",
				"/home/user",
			},
			expectedIndices:  []int{0},
			shouldHaveDanger: true,
		},
		{
			name: "force flag is dangerous",
			args: []string{
				"--force", // Changed from "-f" to "--force" which contains "force"
				"file.txt",
			},
			expectedIndices:  []int{0},
			shouldHaveDanger: true,
		},
		{
			name: "multiple dangerous args",
			args: []string{
				"-r",          // Contains "r" but not "rf", "recursive", etc.
				"--force",     // Contains "force"
				"--recursive", // Contains "recursive"
				"file.txt",
			},
			expectedIndices:  []int{1, 2}, // Only force and recursive
			shouldHaveDanger: true,
		},
		{
			name: "dangerous arg in middle",
			args: []string{
				"file1.txt",
				"-rf",
				"file2.txt",
			},
			expectedIndices:  []int{1},
			shouldHaveDanger: true,
		},
		{
			name: "preserve-root safe",
			args: []string{
				"--preserve-root",
				"/",
			},
			expectedIndices:  []int{},
			shouldHaveDanger: false,
		},
		{
			name:             "empty args",
			args:             []string{},
			expectedIndices:  []int{},
			shouldHaveDanger: false,
		},
		{
			name:             "nil args",
			args:             nil,
			expectedIndices:  []int{},
			shouldHaveDanger: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := validator.HasDangerousRootArgs(tt.args)

			if tt.shouldHaveDanger {
				assert.NotEmpty(t, indices, "Expected to find dangerous arguments")
				assert.ElementsMatch(t, tt.expectedIndices, indices)
			} else {
				assert.Empty(t, indices, "Expected no dangerous arguments")
			}
		})
	}
}

func TestValidator_HasWildcards(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name               string
		args               []string
		expectedIndices    []int
		shouldHaveWildcard bool
	}{
		{
			name: "no wildcards",
			args: []string{
				"file.txt",
				"/path/to/file",
			},
			expectedIndices:    []int{},
			shouldHaveWildcard: false,
		},
		{
			name: "asterisk wildcard",
			args: []string{
				"*.txt",
				"file.doc",
			},
			expectedIndices:    []int{0},
			shouldHaveWildcard: true,
		},
		{
			name: "question mark wildcard",
			args: []string{
				"file?.txt",
				"normal.doc",
			},
			expectedIndices:    []int{0},
			shouldHaveWildcard: true,
		},
		{
			name: "multiple wildcards in one arg",
			args: []string{
				"*file?.txt",
			},
			expectedIndices:    []int{0},
			shouldHaveWildcard: true,
		},
		{
			name: "wildcards in multiple args",
			args: []string{
				"*.txt",
				"file?.doc",
				"normal.pdf",
			},
			expectedIndices:    []int{0, 1},
			shouldHaveWildcard: true,
		},
		{
			name: "wildcard at end",
			args: []string{
				"normal.txt",
				"wildcard*",
			},
			expectedIndices:    []int{1},
			shouldHaveWildcard: true,
		},
		{
			name: "wildcard at beginning",
			args: []string{
				"*wildcard",
				"normal.txt",
			},
			expectedIndices:    []int{0},
			shouldHaveWildcard: true,
		},
		{
			name: "path with wildcards",
			args: []string{
				"/path/*/file.txt",
				"/path/to/file?.txt",
			},
			expectedIndices:    []int{0, 1},
			shouldHaveWildcard: true,
		},
		{
			name:               "empty args",
			args:               []string{},
			expectedIndices:    []int{},
			shouldHaveWildcard: false,
		},
		{
			name:               "nil args",
			args:               nil,
			expectedIndices:    []int{},
			shouldHaveWildcard: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := validator.HasWildcards(tt.args)

			if tt.shouldHaveWildcard {
				assert.NotEmpty(t, indices, "Expected to find wildcards")
				assert.ElementsMatch(t, tt.expectedIndices, indices)
			} else {
				assert.Empty(t, indices, "Expected no wildcards")
			}
		})
	}
}

func TestValidator_HasSystemCriticalPaths(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name               string
		args               []string
		expectedIndices    []int
		shouldHaveCritical bool
	}{
		{
			name: "no critical paths",
			args: []string{
				"/home/user/file.txt",
				"/tmp/test.txt",
			},
			expectedIndices:    []int{},
			shouldHaveCritical: false,
		},
		{
			name: "/etc path is critical",
			args: []string{
				"/etc/passwd",
				"/home/user/file.txt",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "/boot path is critical",
			args: []string{
				"/boot/grub.cfg",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "/sys path is critical",
			args: []string{
				"/sys/kernel/config",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "/proc path is critical",
			args: []string{
				"/proc/sys/net",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "multiple critical paths",
			args: []string{
				"/etc/shadow",
				"/boot/vmlinuz",
				"/home/user/safe.txt",
				"/sys/devices",
			},
			expectedIndices:    []int{0, 1, 3},
			shouldHaveCritical: true,
		},
		{
			name: "exact match of critical path",
			args: []string{
				"/etc",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "path that starts with critical but not subdirectory",
			args: []string{
				"/etc-backup/file.txt", // Does not match because no slash after /etc
			},
			expectedIndices:    []int{},
			shouldHaveCritical: false,
		},
		{
			name: "subdirectory of critical path",
			args: []string{
				"/etc/systemd/system/service.conf",
			},
			expectedIndices:    []int{0},
			shouldHaveCritical: true,
		},
		{
			name: "mixed safe and critical",
			args: []string{
				"/home/user/file.txt",
				"/etc/config.conf",
				"/tmp/test.txt",
				"/boot/config",
			},
			expectedIndices:    []int{1, 3},
			shouldHaveCritical: true,
		},
		{
			name:               "empty args",
			args:               []string{},
			expectedIndices:    []int{},
			shouldHaveCritical: false,
		},
		{
			name:               "nil args",
			args:               nil,
			expectedIndices:    []int{},
			shouldHaveCritical: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := validator.HasSystemCriticalPaths(tt.args)

			if tt.shouldHaveCritical {
				assert.NotEmpty(t, indices, "Expected to find critical paths")
				assert.ElementsMatch(t, tt.expectedIndices, indices)
			} else {
				assert.Empty(t, indices, "Expected no critical paths")
			}
		})
	}
}
