//go:build test

package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "simple alphanumeric",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "path with slashes",
			input:    "/usr/bin/echo",
			expected: "/usr/bin/echo",
		},
		{
			name:     "string with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "string with single quote",
			input:    "it's",
			expected: "'it'\\''s'",
		},
		{
			name:     "string with special characters",
			input:    "hello $USER",
			expected: "'hello $USER'",
		},
		{
			name:     "string with newline",
			input:    "line1\nline2",
			expected: "'line1\nline2'",
		},
		{
			name:     "string with tab",
			input:    "col1\tcol2",
			expected: "'col1\tcol2'",
		},
		{
			name:     "complex path",
			input:    "/usr/local/bin/my-app_v2.0",
			expected: "/usr/local/bin/my-app_v2.0",
		},
		{
			name:     "string with multiple single quotes",
			input:    "it's a 'test'",
			expected: "'it'\\''s a '\\''test'\\'''",
		},
		{
			name:     "string with semicolon",
			input:    "cmd; rm -rf /",
			expected: "'cmd; rm -rf /'",
		},
		{
			name:     "string with pipe",
			input:    "cat file | grep pattern",
			expected: "'cat file | grep pattern'",
		},
		{
			name:     "string with redirect",
			input:    "echo hello > file",
			expected: "'echo hello > file'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellEscape(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatCommandForLog(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		args     []string
		expected string
	}{
		{
			name:     "simple command without args",
			path:     "/bin/ls",
			args:     []string{},
			expected: "/bin/ls",
		},
		{
			name:     "command with simple args",
			path:     "/bin/echo",
			args:     []string{"hello", "world"},
			expected: "/bin/echo hello world",
		},
		{
			name:     "command with args containing spaces",
			path:     "/bin/echo",
			args:     []string{"hello world", "foo bar"},
			expected: "/bin/echo 'hello world' 'foo bar'",
		},
		{
			name:     "command with args containing special characters",
			path:     "/bin/sh",
			args:     []string{"-c", "echo $HOME"},
			expected: "/bin/sh -c 'echo $HOME'",
		},
		{
			name:     "complex command with mixed args",
			path:     "/usr/bin/grep",
			args:     []string{"-r", "pattern with spaces", "/path/to/dir"},
			expected: "/usr/bin/grep -r 'pattern with spaces' /path/to/dir",
		},
		{
			name:     "command with single quote in arg",
			path:     "/bin/echo",
			args:     []string{"it's a test"},
			expected: "/bin/echo 'it'\\''s a test'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCommandForLog(tt.path, tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}
