//go:build test

package config

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandGlobal_AutoVarsGenerated(t *testing.T) {
	// Test that ExpandGlobal generates __runner_datetime and __runner_pid
	timeout := int32(3600)
	spec := &runnertypes.GlobalSpec{
		Timeout:    &timeout,
		EnvAllowed: []string{"PATH"},
		Vars:       nil,
	}

	runtime, err := ExpandGlobal(spec)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	// Check that __runner_datetime is present
	datetime, ok := runtime.ExpandedVars[variable.DatetimeKey()]
	assert.True(t, ok, "__runner_datetime should be present in ExpandedVars")
	assert.NotEmpty(t, datetime, "__runner_datetime should not be empty")

	// Check datetime format: YYYYMMDDHHmmSS.msec
	matched, err := regexp.MatchString(`^\d{14}\.\d{3}$`, datetime)
	require.NoError(t, err)
	assert.True(t, matched, "Datetime should match format YYYYMMDDHHmmSS.msec, got: %s", datetime)

	// Check that __runner_pid is present
	pid, ok := runtime.ExpandedVars[variable.PIDKey()]
	assert.True(t, ok, "__runner_pid should be present in ExpandedVars")
	assert.Equal(t, fmt.Sprintf("%d", os.Getpid()), pid)

	// Check PID format: should be numeric
	matched, err = regexp.MatchString(`^\d+$`, pid)
	require.NoError(t, err)
	assert.True(t, matched, "PID should be numeric, got: %s", pid)
}

func TestExpandGlobal_AutoVarsReservedPrefix(t *testing.T) {
	// Test that user-defined vars cannot use reserved prefix __runner_
	timeout := int32(3600)
	spec := &runnertypes.GlobalSpec{
		Timeout: &timeout,
		Vars: map[string]any{
			"__runner_datetime": "user_value",
		},
	}

	// Should fail because __runner_ prefix is reserved
	_, err := ExpandGlobal(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved variable prefix")
	assert.Contains(t, err.Error(), "__runner_datetime")
}

func TestExpandGlobal_AutoVarsAvailableForVarsExpansion(t *testing.T) {
	// Test that auto variables can be used in vars expansion
	timeout := int32(3600)
	spec := &runnertypes.GlobalSpec{
		Timeout: &timeout,
		Vars: map[string]any{
			"Output_file": "/tmp/backup-%{__runner_datetime}.tar.gz",
			"Lock_file":   "/var/run/myapp-%{__runner_pid}.lock",
		},
	}

	runtime, err := ExpandGlobal(spec)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	// Check that auto variables were expanded in user vars
	outputFile := runtime.ExpandedVars["Output_file"]
	lockFile := runtime.ExpandedVars["Lock_file"]

	// Should contain expanded values, not the template
	assert.NotContains(t, outputFile, "%{__runner_datetime}")
	assert.Contains(t, outputFile, "/tmp/backup-")
	assert.Contains(t, outputFile, ".tar.gz")

	assert.NotContains(t, lockFile, "%{__runner_pid}")
	assert.Contains(t, lockFile, "/var/run/myapp-")
	assert.Contains(t, lockFile, ".lock")

	// Verify the actual expanded values
	datetime := runtime.ExpandedVars[variable.DatetimeKey()]
	pid := runtime.ExpandedVars[variable.PIDKey()]

	expectedOutput := fmt.Sprintf("/tmp/backup-%s.tar.gz", datetime)
	expectedLock := fmt.Sprintf("/var/run/myapp-%s.lock", pid)

	assert.Equal(t, expectedOutput, outputFile)
	assert.Equal(t, expectedLock, lockFile)
}

func TestExpandGlobal_AutoVarsConsistentAcrossExpansions(t *testing.T) {
	// Test that auto variables have consistent values within a single ExpandGlobal call
	timeout := int32(3600)
	spec := &runnertypes.GlobalSpec{
		Timeout: &timeout,
		Vars: map[string]any{
			"File1": "/tmp/file1-%{__runner_datetime}.log",
			"File2": "/tmp/file2-%{__runner_datetime}.log",
			"Lock1": "/tmp/lock1-%{__runner_pid}.pid",
			"Lock2": "/tmp/lock2-%{__runner_pid}.pid",
		},
	}

	runtime, err := ExpandGlobal(spec)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	file1 := runtime.ExpandedVars["File1"]
	file2 := runtime.ExpandedVars["File2"]
	lock1 := runtime.ExpandedVars["Lock1"]
	lock2 := runtime.ExpandedVars["Lock2"]

	// Extract datetime from file1 and file2
	re := regexp.MustCompile(`/tmp/file\d+-(\d{14}\.\d{3})\.log`)
	matches1 := re.FindStringSubmatch(file1)
	matches2 := re.FindStringSubmatch(file2)
	require.Len(t, matches1, 2, "file1 should match pattern")
	require.Len(t, matches2, 2, "file2 should match pattern")

	// Both should have the same datetime
	assert.Equal(t, matches1[1], matches2[1], "Both files should have the same datetime")

	// Extract PID from lock1 and lock2
	rePID := regexp.MustCompile(`/tmp/lock\d+-(\d+)\.pid`)
	matchesPID1 := rePID.FindStringSubmatch(lock1)
	matchesPID2 := rePID.FindStringSubmatch(lock2)
	require.Len(t, matchesPID1, 2, "lock1 should match pattern")
	require.Len(t, matchesPID2, 2, "lock2 should match pattern")

	// Both should have the same PID
	assert.Equal(t, matchesPID1[1], matchesPID2[1], "Both locks should have the same PID")
}

func TestExpandGlobal_AutoVarsWithEnvImport(t *testing.T) {
	// Test that auto variables work together with env_import
	t.Setenv("TEST_VAR", "test_value")

	timeout := int32(3600)
	spec := &runnertypes.GlobalSpec{
		Timeout:    &timeout,
		EnvAllowed: []string{"TEST_VAR"},
		EnvImport:  []string{"TEST_VAR=TEST_VAR"}, // Correct format: internal_name=SYSTEM_VAR (global vars must be uppercase)
		Vars: map[string]any{
			"Combined": "%{TEST_VAR}-%{__runner_datetime}",
		},
	}

	runtime, err := ExpandGlobal(spec)
	require.NoError(t, err)
	require.NotNil(t, runtime)

	// Check that both env import and auto vars work
	combined := runtime.ExpandedVars["Combined"]
	assert.Contains(t, combined, "test_value-")
	assert.NotContains(t, combined, "%{TEST_VAR}")
	assert.NotContains(t, combined, "%{__runner_datetime}")

	// Verify the actual expanded value
	datetime := runtime.ExpandedVars[variable.DatetimeKey()]
	expectedCombined := fmt.Sprintf("test_value-%s", datetime)
	assert.Equal(t, expectedCombined, combined)
}
