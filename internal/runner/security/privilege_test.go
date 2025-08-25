package security

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDefaultPrivilegeEscalationAnalyzer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	assert.NotNil(t, analyzer)
	assert.NotNil(t, analyzer.logger)
	assert.NotNil(t, analyzer.commandChecks)

	// Check default commands are set
	assert.Contains(t, analyzer.commandChecks, "sudo")
	assert.Contains(t, analyzer.commandChecks, "su")
	assert.Contains(t, analyzer.commandChecks, "doas")
	assert.Contains(t, analyzer.commandChecks, "systemctl")
	assert.Contains(t, analyzer.commandChecks, "service")

	// Check command properties
	sudoInfo := analyzer.commandChecks["sudo"]
	assert.Equal(t, PrivilegeEscalationTypeSudo, sudoInfo.EscalationType)
	assert.Equal(t, []string{"root"}, sudoInfo.RequiredPrivileges)

	systemctlInfo := analyzer.commandChecks["systemctl"]
	assert.Equal(t, PrivilegeEscalationTypeSystemd, systemctlInfo.EscalationType)
	assert.Equal(t, []string{"systemd"}, systemctlInfo.RequiredPrivileges)
}

func TestAnalyzePrivilegeEscalation_BasicSudo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)
	ctx := context.Background()

	testCases := []struct {
		name     string
		cmdName  string
		args     []string
		expected *PrivilegeEscalationResult
	}{
		{
			name:    "sudo command",
			cmdName: "sudo",
			args:    []string{"ls", "-la"},
			expected: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSudo,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "sudo",
				Reason:                "Command requires root privileges for execution",
			},
		},
		{
			name:    "su command",
			cmdName: "su",
			args:    []string{"-", "root"},
			expected: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSu,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "su",
				Reason:                "Command requires root privileges for execution",
			},
		},
		{
			name:    "doas command",
			cmdName: "doas",
			args:    []string{"ls"},
			expected: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSudo,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "doas",
				Reason:                "Command requires root privileges for execution",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(ctx, tc.cmdName, tc.args)
			require.NoError(t, err)

			assert.Equal(t, tc.expected.IsPrivilegeEscalation, result.IsPrivilegeEscalation)
			assert.Equal(t, tc.expected.EscalationType, result.EscalationType)
			assert.Equal(t, tc.expected.RequiredPrivileges, result.RequiredPrivileges)
			assert.Equal(t, tc.expected.DetectedPattern, result.DetectedPattern)
			assert.Equal(t, tc.expected.Reason, result.Reason)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestAnalyzePrivilegeEscalation_SystemCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)
	ctx := context.Background()

	testCases := []struct {
		name     string
		cmdName  string
		args     []string
		expected *PrivilegeEscalationResult
	}{
		{
			name:    "systemctl command",
			cmdName: "systemctl",
			args:    []string{"start", "nginx"},
			expected: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeSystemd,
				RequiredPrivileges:    []string{"systemd"},
				DetectedPattern:       "systemctl",
				Reason:                "Command can control system services",
			},
		},
		{
			name:    "service command",
			cmdName: "service",
			args:    []string{"apache2", "restart"},
			expected: &PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationTypeService,
				RequiredPrivileges:    []string{"service"},
				DetectedPattern:       "service",
				Reason:                "Command can control system services",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(ctx, tc.cmdName, tc.args)
			require.NoError(t, err)

			assert.Equal(t, tc.expected.IsPrivilegeEscalation, result.IsPrivilegeEscalation)
			assert.Equal(t, tc.expected.EscalationType, result.EscalationType)
			assert.Equal(t, tc.expected.RequiredPrivileges, result.RequiredPrivileges)
			assert.Equal(t, tc.expected.DetectedPattern, result.DetectedPattern)
			assert.Equal(t, tc.expected.Reason, result.Reason)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestAnalyzePrivilegeEscalation_NonPrivilegedCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)
	ctx := context.Background()

	testCases := []struct {
		name    string
		cmdName string
		args    []string
	}{
		{
			name:    "ls command",
			cmdName: "ls",
			args:    []string{"-la"},
		},
		{
			name:    "cat command",
			cmdName: "cat",
			args:    []string{"/etc/passwd"},
		},
		{
			name:    "echo command",
			cmdName: "echo",
			args:    []string{"hello world"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(ctx, tc.cmdName, tc.args)
			require.NoError(t, err)

			assert.False(t, result.IsPrivilegeEscalation)
			assert.Equal(t, PrivilegeEscalationType(""), result.EscalationType)
			assert.Equal(t, RiskLevelNone, result.RiskLevel)
			assert.Empty(t, result.RequiredPrivileges)
			assert.Equal(t, "", result.DetectedPattern)
			assert.Equal(t, "", result.Reason)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestAnalyzePrivilegeEscalation_SymlinkHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)
	ctx := context.Background()

	// Create a temporary directory for the symlink test
	tempDir := t.TempDir()
	symlinkPath := tempDir + "/symlink_to_sudo"
	sudoPath := "/usr/bin/sudo"

	// Create a symlink to the sudo binary
	err := os.Symlink(sudoPath, symlinkPath)
	require.NoError(t, err, "Failed to create symlink")

	// Test with the symlink path
	result, err := analyzer.AnalyzePrivilegeEscalation(ctx, symlinkPath, []string{"ls"})
	require.NoError(t, err)

	assert.True(t, result.IsPrivilegeEscalation)
	assert.Equal(t, PrivilegeEscalationTypeSudo, result.EscalationType)
	assert.Equal(t, sudoPath, result.CommandPath)

	// Test with absolute path
	result, err = analyzer.AnalyzePrivilegeEscalation(ctx, sudoPath, []string{"ls"})
	require.NoError(t, err)

	assert.True(t, result.IsPrivilegeEscalation)
	assert.Equal(t, PrivilegeEscalationTypeSudo, result.EscalationType)
	assert.Equal(t, sudoPath, result.CommandPath)
}

func TestGetRequiredPrivileges(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	testCases := []struct {
		name     string
		cmdName  string
		args     []string
		expected []string
	}{
		{
			name:     "sudo command",
			cmdName:  "sudo",
			args:     []string{"ls"},
			expected: []string{"root"},
		},
		{
			name:     "su command",
			cmdName:  "su",
			args:     []string{"-", "root"},
			expected: []string{"root"},
		},
		{
			name:     "systemctl command",
			cmdName:  "systemctl",
			args:     []string{"start", "nginx"},
			expected: []string{"systemd"},
		},
		{
			name:     "service command",
			cmdName:  "service",
			args:     []string{"apache2", "start"},
			expected: []string{"service"},
		},
		{
			name:     "non-privileged command",
			cmdName:  "ls",
			args:     []string{"-la"},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := analyzer.GetRequiredPrivileges(tc.cmdName, tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
