package risk

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

func TestStandardEvaluator_EvaluateRisk(t *testing.T) {
	evaluator := NewStandardEvaluator()

	tests := []struct {
		name     string
		cmd      *runnertypes.Command
		expected runnertypes.RiskLevel
	}{
		{
			name: "privilege escalation command - sudo",
			cmd: &runnertypes.Command{
				Cmd:  "sudo",
				Args: []string{"ls", "/root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "privilege escalation command - su",
			cmd: &runnertypes.Command{
				Cmd:  "su",
				Args: []string{"root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "privilege escalation command - doas",
			cmd: &runnertypes.Command{
				Cmd:  "doas",
				Args: []string{"ls", "/root"},
			},
			expected: runnertypes.RiskLevelCritical,
		},
		{
			name: "destructive file operation - rm",
			cmd: &runnertypes.Command{
				Cmd:  "rm",
				Args: []string{"-rf", "/tmp/files"},
			},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name: "destructive file operation - find with delete",
			cmd: &runnertypes.Command{
				Cmd:  "find",
				Args: []string{"/tmp", "-name", "*.tmp", "-delete"},
			},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name: "destructive file operation - find with exec rm",
			cmd: &runnertypes.Command{
				Cmd:  "find",
				Args: []string{"/tmp", "-name", "*.log", "-exec", "rm", "{}", ";"},
			},
			expected: runnertypes.RiskLevelHigh,
		},
		{
			name: "safe file operation - find with exec stat",
			cmd: &runnertypes.Command{
				Cmd:  "find",
				Args: []string{"/tmp", "-name", "*.log", "-exec", "stat", "{}", ";"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "network operation - wget",
			cmd: &runnertypes.Command{
				Cmd:  "wget",
				Args: []string{"https://example.com/file.txt"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "network operation - curl",
			cmd: &runnertypes.Command{
				Cmd:  "curl",
				Args: []string{"-O", "https://example.com/file.txt"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "system modification - systemctl",
			cmd: &runnertypes.Command{
				Cmd:  "systemctl",
				Args: []string{"restart", "nginx"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "package installation - apt install",
			cmd: &runnertypes.Command{
				Cmd:  "apt",
				Args: []string{"install", "vim"},
			},
			expected: runnertypes.RiskLevelMedium,
		},
		{
			name: "safe package query - apt list",
			cmd: &runnertypes.Command{
				Cmd:  "apt",
				Args: []string{"list", "--installed"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - echo",
			cmd: &runnertypes.Command{
				Cmd:  "echo",
				Args: []string{"Hello, World!"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - ls",
			cmd: &runnertypes.Command{
				Cmd:  "ls",
				Args: []string{"-la", "/home"},
			},
			expected: runnertypes.RiskLevelLow,
		},
		{
			name: "safe command - cat",
			cmd: &runnertypes.Command{
				Cmd:  "cat",
				Args: []string{"/etc/passwd"},
			},
			expected: runnertypes.RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateRisk(tt.cmd)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
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
			args:     []string{"-rf", "/tmp/test"},
			expected: true,
		},
		{
			name:     "rmdir command",
			cmd:      "rmdir",
			args:     []string{"/tmp/empty"},
			expected: true,
		},
		{
			name:     "find with delete",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.tmp", "-delete"},
			expected: true,
		},
		{
			name:     "find with exec rm (destructive)",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.log", "-exec", "rm", "{}", ";"},
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
			name:     "find with exec shred (destructive)",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.tmp", "-exec", "shred", "-u", "{}", ";"},
			expected: true,
		},
		{
			name:     "rsync with delete",
			cmd:      "rsync",
			args:     []string{"-av", "--delete", "src/", "dst/"},
			expected: true,
		},
		{
			name:     "safe find",
			cmd:      "find",
			args:     []string{"/tmp", "-name", "*.log"},
			expected: false,
		},
		{
			name:     "safe rsync",
			cmd:      "rsync",
			args:     []string{"-av", "src/", "dst/"},
			expected: false,
		},
		{
			name:     "safe ls",
			cmd:      "ls",
			args:     []string{"-la"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := security.IsDestructiveFileOperation(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsNetworkOperation(t *testing.T) {
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
			name:     "command with http URL in args",
			cmd:      "myapp",
			args:     []string{"--url", "http://api.example.com"},
			expected: true,
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
			result, _ := security.IsNetworkOperation(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsSystemModification(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		args     []string
		expected bool
	}{
		{
			name:     "systemctl command",
			cmd:      "systemctl",
			args:     []string{"restart", "nginx"},
			expected: true,
		},
		{
			name:     "apt install",
			cmd:      "apt",
			args:     []string{"install", "vim"},
			expected: true,
		},
		{
			name:     "yum update",
			cmd:      "yum",
			args:     []string{"update"},
			expected: true,
		},
		{
			name:     "npm install",
			cmd:      "npm",
			args:     []string{"install", "package"},
			expected: true,
		},
		{
			name:     "mount command",
			cmd:      "mount",
			args:     []string{"/dev/sdb1", "/mnt"},
			expected: true,
		},
		{
			name:     "safe apt list",
			cmd:      "apt",
			args:     []string{"list", "--installed"},
			expected: false,
		},
		{
			name:     "safe npm list",
			cmd:      "npm",
			args:     []string{"list"},
			expected: false,
		},
		{
			name:     "safe command",
			cmd:      "echo",
			args:     []string{"hello"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := security.IsSystemModification(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
