//go:build test

package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestArbitraryCodeExecutionRunner_Names verifies the F-015 classifier matches
// shells, interpreters, and build/task runners by basename, on both bare names
// and absolute paths, while rejecting substring look-alikes.
func TestArbitraryCodeExecutionRunner_Names(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"bash", true},
		{"sh", true},
		{"python3", true},
		{"node", true},
		{"make", true},
		{"/usr/bin/bash", true},
		{"/opt/python/python", true},
		{"/usr/bin/make", true},
		// Substring look-alikes must not match.
		{"/usr/bin/makebelieve", false},
		{"/usr/bin/bashful", false},
		{"shred", false},
		{"echo", false},
		{"/usr/bin/ls", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			assert.Equal(t, tt.want, IsArbitraryCodeExecutionRunner(tt.cmd))
		})
	}
}

// TestArbitraryCodeExecutionRunner_Symlink verifies that a symlink whose target
// basename is an interpreter is classified as an arbitrary-code runner, so an
// alias cannot hide a shell/interpreter.
func TestArbitraryCodeExecutionRunner_Symlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "python")
	require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755))
	link := filepath.Join(tmp, "myinterp")
	require.NoError(t, os.Symlink(target, link))

	assert.True(t, IsArbitraryCodeExecutionRunner(link),
		"a symlink to an interpreter must be classified as an arbitrary-code runner")
}
