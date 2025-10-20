//go:build test
// +build test

package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutionModeString(t *testing.T) {
	tests := []struct {
		mode     ExecutionMode
		expected string
	}{
		{ExecutionModeNormal, "normal"},
		{ExecutionModeDryRun, "dry-run"},
		{ExecutionMode(999), "unknown"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, test.mode.String())
	}
}

func TestExecutionResult(t *testing.T) {
	result := &ExecutionResult{
		ExitCode: 0,
		Stdout:   "test output",
		Stderr:   "",
		Duration: 100,
		DryRun:   false,
	}

	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "test output", result.Stdout)
	assert.Equal(t, int64(100), result.Duration)
	assert.False(t, result.DryRun)
}

func TestExecutionResultDryRun(t *testing.T) {
	result := &ExecutionResult{
		ExitCode: 0,
		Stdout:   "simulated output",
		Stderr:   "",
		Duration: 10,
		DryRun:   true,
	}

	assert.True(t, result.DryRun)
	assert.Equal(t, "simulated output", result.Stdout)
}
