//go:build test
// +build test

package security

import (
	"fmt"
	"testing"
)

// IsVariableValueSafe validates that a variable value contains no dangerous patterns
// This is a global convenience function that creates a default validator to check variable values
func IsVariableValueSafe(name, value string) error {
	validator, err := NewValidator(nil) // Use default config
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	return validator.ValidateEnvironmentValue(name, value)
}

// cmdNameSet resolves a command name to its name set, for tests that exercise the
// name-set-taking classifiers (IsArbitraryCodeExecutionRunner, ResolveProfile,
// IsDestructiveFileOperation, ...) with a command string.
func cmdNameSet(cmd string) map[string]struct{} {
	names, _ := extractAllCommandNames(cmd)
	return names
}

// SetCoreutilsDirForTest overrides the package-level coreutilsDir variable for the
// duration of a test, restoring it automatically via t.Cleanup.
// Tests that call this must not use t.Parallel() because coreutilsDir is a
// package-global variable shared across goroutines in the same test binary.
func SetCoreutilsDirForTest(t *testing.T, dir string) {
	t.Helper()
	original := coreutilsDir
	coreutilsDir = dir
	t.Cleanup(func() { coreutilsDir = original })
}
