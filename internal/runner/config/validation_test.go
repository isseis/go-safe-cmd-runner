package config_test

import (
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestValidatePrivilegedCommands(t *testing.T) {
	tests := []struct {
		name     string
		config   *runnertypes.Config
		expected []config.ValidationWarning
	}{
		{
			name: "safe privileged command",
			config: &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "safe_group",
						Commands: []runnertypes.Command{
							{
								Name:       "safe_cmd",
								Cmd:        "/usr/bin/id",
								Args:       []string{"-u"},
								Privileged: true,
							},
						},
					},
				},
			},
			expected: []config.ValidationWarning{},
		},
		{
			name: "dangerous privileged commands",
			config: &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "dangerous_group",
						Commands: []runnertypes.Command{
							{
								Name:       "shell_cmd",
								Cmd:        "/bin/bash",
								Args:       []string{"-c", "echo test"},
								Privileged: true,
							},
							{
								Name:       "su_cmd",
								Cmd:        "/bin/su",
								Args:       []string{"root"},
								Privileged: true,
							},
						},
					},
				},
			},
			expected: []config.ValidationWarning{
				{
					Type:       "security",
					Location:   "groups[dangerous_group].commands[shell_cmd]",
					Message:    "Privileged command uses potentially dangerous path: /bin/bash",
					Suggestion: "Consider using a safer alternative or additional validation",
				},
				{
					Type:       "security",
					Location:   "groups[dangerous_group].commands[shell_cmd]",
					Message:    "Privileged shell commands require extra caution",
					Suggestion: "Avoid using shell commands with privileges or implement strict argument validation",
				},
				{
					Type:       "security",
					Location:   "groups[dangerous_group].commands[su_cmd]",
					Message:    "Privileged command uses potentially dangerous path: /bin/su",
					Suggestion: "Consider using a safer alternative or additional validation",
				},
			},
		},
		{
			name: "commands with shell metacharacters",
			config: &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "meta_group",
						Commands: []runnertypes.Command{
							{
								Name:       "pipe_cmd",
								Cmd:        "/usr/bin/echo",
								Args:       []string{"test", "|", "grep", "something"},
								Privileged: true,
							},
							{
								Name:       "variable_cmd",
								Cmd:        "/usr/bin/echo",
								Args:       []string{"$HOME"},
								Privileged: true,
							},
						},
					},
				},
			},
			expected: []config.ValidationWarning{
				{
					Type:       "security",
					Location:   "groups[meta_group].commands[pipe_cmd].args",
					Message:    "Command arguments contain shell metacharacters - ensure proper escaping",
					Suggestion: "Use absolute paths and avoid shell metacharacters in arguments",
				},
				{
					Type:       "security",
					Location:   "groups[meta_group].commands[variable_cmd].args",
					Message:    "Command arguments contain shell metacharacters - ensure proper escaping",
					Suggestion: "Use absolute paths and avoid shell metacharacters in arguments",
				},
			},
		},
		{
			name: "relative path command",
			config: &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "relative_group",
						Commands: []runnertypes.Command{
							{
								Name:       "relative_cmd",
								Cmd:        "echo",
								Args:       []string{"test"},
								Privileged: true,
							},
						},
					},
				},
			},
			expected: []config.ValidationWarning{
				{
					Type:       "security",
					Location:   "groups[relative_group].commands[relative_cmd].cmd",
					Message:    "Privileged command uses relative path - consider using absolute path for security",
					Suggestion: "Use absolute path to prevent PATH-based attacks",
				},
			},
		},
		{
			name: "non-privileged commands ignored",
			config: &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "normal_group",
						Commands: []runnertypes.Command{
							{
								Name:       "normal_shell",
								Cmd:        "/bin/bash",
								Args:       []string{"-c", "echo test"},
								Privileged: false, // Not privileged
							},
						},
					},
				},
			},
			expected: []config.ValidationWarning{}, // No warnings for non-privileged commands
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := config.ValidatePrivilegedCommands(tt.config)

			assert.Equal(t, len(tt.expected), len(warnings), "Number of warnings should match")

			for i, expected := range tt.expected {
				if i < len(warnings) {
					assert.Equal(t, expected.Type, warnings[i].Type)
					assert.Equal(t, expected.Location, warnings[i].Location)
					assert.Equal(t, expected.Message, warnings[i].Message)
					assert.Equal(t, expected.Suggestion, warnings[i].Suggestion)
				}
			}
		})
	}
}

func TestIsDangerousCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmdPath  string
		expected bool
	}{
		{"bash shell", "/bin/bash", true},
		{"su command", "/bin/su", true},
		{"sudo command", "/usr/bin/sudo", true},
		{"rm command", "/bin/rm", true},
		{"dd command", "/bin/dd", true},
		{"safe command", "/usr/bin/id", false},
		{"safe command", "/usr/bin/date", false},
		{"custom script", "/opt/myapp/script.sh", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to test the internal function through the public API
			// by creating a minimal config and checking if warnings are generated
			cfg := &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Commands: []runnertypes.Command{
							{
								Name:       "test_cmd",
								Cmd:        tt.cmdPath,
								Privileged: true,
							},
						},
					},
				},
			}

			warnings := config.ValidatePrivilegedCommands(cfg)
			hasDangerousWarning := false
			for _, warning := range warnings {
				if warning.Type == "security" &&
					warning.Message == "Privileged command uses potentially dangerous path: "+tt.cmdPath {
					hasDangerousWarning = true
					break
				}
			}

			assert.Equal(t, tt.expected, hasDangerousWarning)
		})
	}
}

func TestHasShellMetacharacters(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{"no metacharacters", []string{"arg1", "arg2"}, false},
		{"pipe character", []string{"test", "|", "grep"}, true},
		{"variable expansion", []string{"$HOME"}, true},
		{"command substitution", []string{"$(date)"}, true},
		{"redirection", []string{"output", ">", "file.txt"}, true},
		{"wildcard", []string{"*.txt"}, true},
		{"background", []string{"command", "&"}, true},
		{"safe special chars", []string{"file-name", "file_name", "file.txt"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test through public API
			cfg := &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Commands: []runnertypes.Command{
							{
								Name:       "test_cmd",
								Cmd:        "/usr/bin/echo",
								Args:       tt.args,
								Privileged: true,
							},
						},
					},
				},
			}

			warnings := config.ValidatePrivilegedCommands(cfg)
			hasMetacharWarning := false
			for _, warning := range warnings {
				if warning.Type == "security" &&
					warning.Message == "Command arguments contain shell metacharacters - ensure proper escaping" {
					hasMetacharWarning = true
					break
				}
			}

			assert.Equal(t, tt.expected, hasMetacharWarning)
		})
	}
}

// TestValidatorSingleton_ThreadSafety tests thread safety of validator access
func TestValidatorSingleton_ThreadSafety(t *testing.T) {
	const numGoroutines = 100
	var wg sync.WaitGroup
	results := make([]bool, numGoroutines)

	// Launch multiple goroutines that try to access validation functions concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			// Test validation through the public API to ensure thread safety
			cfg := &runnertypes.Config{
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Commands: []runnertypes.Command{
							{
								Name:       "test_cmd",
								Cmd:        "/bin/bash",
								Privileged: true,
							},
						},
					},
				},
			}
			warnings := config.ValidatePrivilegedCommands(cfg)
			// /bin/bash should generate warnings
			results[index] = len(warnings) > 0
		}(i)
	}

	wg.Wait()

	// All results should be consistent
	for i, result := range results {
		assert.True(t, result, "Result at index %d should be true", i)
	}
}

// TestValidatorSingleton_ConcurrentValidation tests concurrent validation calls
func TestValidatorSingleton_ConcurrentValidation(t *testing.T) {
	const numGoroutines = 50
	var wg sync.WaitGroup

	// Launch multiple goroutines that perform concurrent validation
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Test different validation scenarios concurrently
			configs := []*runnertypes.Config{
				{
					Groups: []runnertypes.CommandGroup{
						{
							Name: "dangerous_group",
							Commands: []runnertypes.Command{
								{Name: "bash_cmd", Cmd: "/bin/bash", Privileged: true},
							},
						},
					},
				},
				{
					Groups: []runnertypes.CommandGroup{
						{
							Name: "shell_meta_group",
							Commands: []runnertypes.Command{
								{Name: "meta_cmd", Cmd: "/usr/bin/echo", Args: []string{"test", "|", "grep"}, Privileged: true},
							},
						},
					},
				},
				{
					Groups: []runnertypes.CommandGroup{
						{
							Name: "relative_group",
							Commands: []runnertypes.Command{
								{Name: "relative_cmd", Cmd: "echo", Privileged: true},
							},
						},
					},
				},
			}

			for _, cfg := range configs {
				warnings := config.ValidatePrivilegedCommands(cfg)
				// Each config should generate warnings
				assert.Greater(t, len(warnings), 0, "Should generate warnings")
			}
		}()
	}

	wg.Wait()
	// Test passes if no race conditions occur
}
