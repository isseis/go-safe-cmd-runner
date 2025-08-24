package security

import "testing"

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
			name:     "rmdir command",
			cmd:      "rmdir",
			args:     []string{"directory"},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsDestructiveFileOperation(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("IsDestructiveFileOperation(%q, %v) = %v, want %v", tt.cmd, tt.args, result, tt.expected)
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
			name:     "mount command",
			cmd:      "mount",
			args:     []string{"/dev/sda1", "/mnt"},
			expected: true,
		},
		{
			name:     "apt install",
			cmd:      "apt",
			args:     []string{"install", "nginx"},
			expected: true,
		},
		{
			name:     "apt remove",
			cmd:      "apt",
			args:     []string{"remove", "nginx"},
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
			args:     []string{"install", "express"},
			expected: true,
		},
		{
			name:     "crontab edit",
			cmd:      "crontab",
			args:     []string{"-e"},
			expected: true,
		},
		{
			name:     "safe apt list",
			cmd:      "apt",
			args:     []string{"list"},
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
			result := IsSystemModification(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("IsSystemModification(%q, %v) = %v, want %v", tt.cmd, tt.args, result, tt.expected)
			}
		})
	}
}
