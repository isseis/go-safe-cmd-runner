package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_IsDangerousPrivilegedCommand(t *testing.T) {
	validator, err := NewValidator(nil)
	assert.NoError(t, err)

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
	assert.NoError(t, err)

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

func TestValidator_HasShellMetacharacters(t *testing.T) {
	validator, err := NewValidator(nil)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		{
			name:     "no metacharacters",
			args:     []string{"hello", "world"},
			expected: false,
		},
		{
			name:     "semicolon metacharacter",
			args:     []string{"echo hello", "; rm -rf /"},
			expected: true,
		},
		{
			name:     "pipe metacharacter",
			args:     []string{"cat file", "| grep test"},
			expected: true,
		},
		{
			name:     "dollar sign for variable",
			args:     []string{"echo $HOME"},
			expected: true,
		},
		{
			name:     "backtick for command substitution",
			args:     []string{"echo `date`"},
			expected: true,
		},
		{
			name:     "redirect metacharacter",
			args:     []string{"echo hello", "> /tmp/file"},
			expected: true,
		},
		{
			name:     "wildcard metacharacter",
			args:     []string{"ls *.txt"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.HasShellMetacharacters(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_IsRelativePath(t *testing.T) {
	validator, err := NewValidator(nil)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "absolute path",
			path:     "/usr/bin/ls",
			expected: false,
		},
		{
			name:     "relative path",
			path:     "bin/ls",
			expected: true,
		},
		{
			name:     "relative path with dot",
			path:     "./bin/ls",
			expected: true,
		},
		{
			name:     "relative path with double dot",
			path:     "../bin/ls",
			expected: true,
		},
		{
			name:     "just filename",
			path:     "ls",
			expected: true,
		},
		{
			name:     "empty path",
			path:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsRelativePath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_CustomConfig(t *testing.T) {
	config := &Config{
		DangerousPrivilegedCommands: []string{"/custom/dangerous"},
		ShellCommands:               []string{"/custom/shell"},
		ShellMetacharacters:         []string{"@", "#"},
	}

	validator, err := NewValidator(config)
	assert.NoError(t, err)

	// Test custom dangerous command
	assert.True(t, validator.IsDangerousPrivilegedCommand("/custom/dangerous"))
	assert.False(t, validator.IsDangerousPrivilegedCommand("/bin/bash")) // Not in custom list

	// Test custom shell command
	assert.True(t, validator.IsShellCommand("/custom/shell"))
	assert.False(t, validator.IsShellCommand("/bin/bash")) // Not in custom list

	// Test custom metacharacters
	assert.True(t, validator.HasShellMetacharacters([]string{"test@example"}))
	assert.True(t, validator.HasShellMetacharacters([]string{"test#hash"}))
	assert.False(t, validator.HasShellMetacharacters([]string{"test;semicolon"})) // Not in custom list
}
