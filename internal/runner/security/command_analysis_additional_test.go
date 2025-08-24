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
			name:     "apt install vim",
			cmd:      "apt",
			args:     []string{"install", "vim"},
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
			name:     "npm install express",
			cmd:      "npm",
			args:     []string{"install", "express"},
			expected: true,
		},
		{
			name:     "npm install package",
			cmd:      "npm",
			args:     []string{"install", "package"},
			expected: true,
		},
		{
			name:     "mount sdb1",
			cmd:      "mount",
			args:     []string{"/dev/sdb1", "/mnt"},
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
			name:     "safe apt list installed",
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
			result := IsSystemModification(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("IsSystemModification(%q, %v) = %v, want %v", tt.cmd, tt.args, result, tt.expected)
			}
		})
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
			result, _ := IsNetworkOperation(tt.cmd, tt.args)
			if result != tt.expected {
				t.Errorf("IsNetworkOperation(%q, %v) = %v, want %v", tt.cmd, tt.args, result, tt.expected)
			}
		})
	}
}
