package security

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzePrivilegeEscalation_BasicSudo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	tests := []struct {
		name    string
		cmdName string
		args    []string
		want    PrivilegeEscalationResult
	}{
		{
			name:    "sudo command",
			cmdName: "sudo",
			args:    []string{"ls", "-la"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSudo,
				RiskLevel:             runnertypes.RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "sudo_command",
				Reason:                "Command uses sudo for privilege escalation",
			},
		},
		{
			name:    "su command",
			cmdName: "su",
			args:    []string{"-", "root"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSudo,
				RiskLevel:             runnertypes.RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "sudo_command",
				Reason:                "Command uses sudo for privilege escalation",
			},
		},
		{
			name:    "absolute path sudo",
			cmdName: "/usr/bin/sudo",
			args:    []string{"id"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSudo,
				RiskLevel:             runnertypes.RiskLevelHigh,
				RequiredPrivileges:    []string{"root"},
				DetectedPattern:       "sudo_command",
				Reason:                "Command uses sudo for privilege escalation",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(context.Background(), tt.cmdName, tt.args)
			require.NoError(t, err)

			assert.Equal(t, tt.want.IsPrivilegeEscalation, result.IsPrivilegeEscalation)
			assert.Equal(t, tt.want.EscalationType, result.EscalationType)
			assert.Equal(t, tt.want.RiskLevel, result.RiskLevel)
			assert.Equal(t, tt.want.RequiredPrivileges, result.RequiredPrivileges)
			assert.Equal(t, tt.want.DetectedPattern, result.DetectedPattern)
			assert.Equal(t, tt.want.Reason, result.Reason)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestAnalyzePrivilegeEscalation_SystemCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	tests := []struct {
		name    string
		cmdName string
		args    []string
		want    PrivilegeEscalationResult
	}{
		{
			name:    "systemctl command",
			cmdName: "systemctl",
			args:    []string{"start", "nginx"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSystemd,
				RiskLevel:             runnertypes.RiskLevelMedium,
				RequiredPrivileges:    []string{"systemd"},
				DetectedPattern:       "systemctl_command",
				Reason:                "Command manages system services",
			},
		},
		{
			name:    "service command",
			cmdName: "service",
			args:    []string{"apache2", "restart"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationService,
				RiskLevel:             runnertypes.RiskLevelMedium,
				RequiredPrivileges:    []string{"service"},
				DetectedPattern:       "service_command",
				Reason:                "Command manages system services",
			},
		},
		{
			name:    "absolute path systemctl",
			cmdName: "/bin/systemctl",
			args:    []string{"status", "docker"},
			want: PrivilegeEscalationResult{
				IsPrivilegeEscalation: true,
				EscalationType:        PrivilegeEscalationSystemd,
				RiskLevel:             runnertypes.RiskLevelMedium,
				RequiredPrivileges:    []string{"systemd"},
				DetectedPattern:       "systemctl_command",
				Reason:                "Command manages system services",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(context.Background(), tt.cmdName, tt.args)
			require.NoError(t, err)

			assert.Equal(t, tt.want.IsPrivilegeEscalation, result.IsPrivilegeEscalation)
			assert.Equal(t, tt.want.EscalationType, result.EscalationType)
			assert.Equal(t, tt.want.RiskLevel, result.RiskLevel)
			assert.Equal(t, tt.want.RequiredPrivileges, result.RequiredPrivileges)
			assert.Equal(t, tt.want.DetectedPattern, result.DetectedPattern)
			assert.Equal(t, tt.want.Reason, result.Reason)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestAnalyzePrivilegeEscalation_NonPrivilegedCommands(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	tests := []struct {
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
			name:    "echo command",
			cmdName: "echo",
			args:    []string{"hello"},
		},
		{
			name:    "cat command",
			cmdName: "cat",
			args:    []string{"/etc/passwd"},
		},
		{
			name:    "absolute path cat",
			cmdName: "/bin/cat",
			args:    []string{"/etc/hosts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.AnalyzePrivilegeEscalation(context.Background(), tt.cmdName, tt.args)
			require.NoError(t, err)

			assert.False(t, result.IsPrivilegeEscalation, "Command should not be detected as privilege escalation")
			assert.Equal(t, runnertypes.RiskLevelNone, result.RiskLevel)
			assert.NotEmpty(t, result.CommandPath)
		})
	}
}

func TestIsPrivilegeEscalationCommand(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	tests := []struct {
		name    string
		cmdName string
		want    bool
	}{
		{
			name:    "sudo command",
			cmdName: "sudo",
			want:    true,
		},
		{
			name:    "su command",
			cmdName: "su",
			want:    true,
		},
		{
			name:    "systemctl command",
			cmdName: "systemctl",
			want:    true,
		},
		{
			name:    "service command",
			cmdName: "service",
			want:    true,
		},
		{
			name:    "absolute sudo",
			cmdName: "/usr/bin/sudo",
			want:    true,
		},
		{
			name:    "ls command",
			cmdName: "ls",
			want:    false,
		},
		{
			name:    "echo command",
			cmdName: "echo",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := analyzer.IsPrivilegeEscalationCommand(tt.cmdName)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGetRequiredPrivileges(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	tests := []struct {
		name    string
		cmdName string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name:    "sudo command",
			cmdName: "sudo",
			args:    []string{"ls", "-la"},
			want:    []string{"root"},
			wantErr: false,
		},
		{
			name:    "systemctl command",
			cmdName: "systemctl",
			args:    []string{"start", "nginx"},
			want:    []string{"systemd"},
			wantErr: false,
		},
		{
			name:    "service command",
			cmdName: "service",
			args:    []string{"apache2", "restart"},
			want:    []string{"service"},
			wantErr: false,
		},
		{
			name:    "non-privileged command",
			cmdName: "ls",
			args:    []string{"-la"},
			want:    []string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := analyzer.GetRequiredPrivileges(tt.cmdName, tt.args)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestPrivilegeEscalationAnalyzer_SymlinkHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	// Test with a command that may not exist but should still be analyzed
	result, err := analyzer.AnalyzePrivilegeEscalation(context.Background(), "/nonexistent/sudo", []string{"ls"})
	require.NoError(t, err)

	// Should detect as sudo even if path doesn't exist
	assert.True(t, result.IsPrivilegeEscalation)
	assert.Equal(t, PrivilegeEscalationSudo, result.EscalationType)
	assert.Equal(t, runnertypes.RiskLevelHigh, result.RiskLevel)
}

// Test helper functions
func TestPrivilegeAnalyzer_HelperMethods(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	analyzer := NewDefaultPrivilegeEscalationAnalyzer(logger)

	t.Run("isSudoCommand", func(t *testing.T) {
		tests := []struct {
			name    string
			cmdPath string
			want    bool
		}{
			{"sudo", "sudo", true},
			{"absolute sudo", "/usr/bin/sudo", true},
			{"su", "su", true},
			{"ls", "ls", false},
			{"cat", "/bin/cat", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := analyzer.isSudoCommand(tt.cmdPath)
				assert.Equal(t, tt.want, result)
			})
		}
	})

	t.Run("isSystemCommand", func(t *testing.T) {
		tests := []struct {
			name    string
			cmdPath string
			want    bool
		}{
			{"systemctl", "systemctl", true},
			{"absolute systemctl", "/bin/systemctl", true},
			{"ls", "ls", false},
			{"sudo", "sudo", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := analyzer.isSystemCommand(tt.cmdPath)
				assert.Equal(t, tt.want, result)
			})
		}
	})

	t.Run("isServiceCommand", func(t *testing.T) {
		tests := []struct {
			name    string
			cmdPath string
			want    bool
		}{
			{"service", "service", true},
			{"absolute service", "/sbin/service", true},
			{"ls", "ls", false},
			{"systemctl", "systemctl", false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := analyzer.isServiceCommand(tt.cmdPath)
				assert.Equal(t, tt.want, result)
			})
		}
	})
}
