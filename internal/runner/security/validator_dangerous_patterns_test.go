package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateDangerousRootPatterns tests the validation logic for DangerousRootPatterns
func TestValidateDangerousRootPatterns(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid patterns - simple command names",
			patterns:    []string{"rm", "dd", "mkfs", "fdisk"},
			expectError: false,
		},
		{
			name:        "valid patterns - with hyphens",
			patterns:    []string{"apt-get", "update-rc.d"},
			expectError: false,
		},
		{
			name:        "invalid pattern - empty string",
			patterns:    []string{"rm", ""},
			expectError: true,
			errorMsg:    "contains empty pattern",
		},
		{
			name:        "invalid pattern - contains path separator slash",
			patterns:    []string{"/bin/rm"},
			expectError: true,
			errorMsg:    "contains path separator",
		},
		{
			name:        "invalid pattern - contains path separator backslash",
			patterns:    []string{"bin\\rm"},
			expectError: true,
			errorMsg:    "contains path separator",
		},
		{
			name:        "invalid pattern - contains asterisk wildcard",
			patterns:    []string{"rm*"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains question mark wildcard",
			patterns:    []string{"rm?"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains regex brackets",
			patterns:    []string{"[rm]"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains regex braces",
			patterns:    []string{"{rm}"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains regex caret",
			patterns:    []string{"^rm"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains regex dollar",
			patterns:    []string{"rm$"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "valid pattern - contains dot (common in commands)",
			patterns:    []string{"update-rc.d"},
			expectError: false,
		},
		{
			name:        "invalid pattern - contains regex pipe",
			patterns:    []string{"rm|dd"},
			expectError: true,
			errorMsg:    "contains wildcard/regex characters",
		},
		{
			name:        "invalid pattern - contains uppercase",
			patterns:    []string{"RM"},
			expectError: true,
			errorMsg:    "contains uppercase",
		},
		{
			name:        "invalid pattern - mixed case",
			patterns:    []string{"Rm"},
			expectError: true,
			errorMsg:    "contains uppercase",
		},
		{
			name:        "edge case - empty list",
			patterns:    []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDangerousRootPatterns(tt.patterns)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestNewValidator_DangerousRootPatternsValidation tests that NewValidator rejects invalid DangerousRootPatterns
func TestNewValidator_DangerousRootPatternsValidation(t *testing.T) {
	tests := []struct {
		name        string
		patterns    []string
		expectError bool
	}{
		{
			name:        "valid patterns accepted",
			patterns:    []string{"rm", "dd", "mkfs"},
			expectError: false,
		},
		{
			name:        "invalid pattern rejected - contains path",
			patterns:    []string{"/bin/rm"},
			expectError: true,
		},
		{
			name:        "invalid pattern rejected - contains wildcard",
			patterns:    []string{"rm*"},
			expectError: true,
		},
		{
			name:        "invalid pattern rejected - uppercase",
			patterns:    []string{"RM"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.DangerousRootPatterns = tt.patterns

			validator, err := NewValidator(config)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, validator)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, validator)
			}
		})
	}
}

// TestIsDangerousRootCommand_EdgeCases tests edge cases for exact matching behavior
func TestIsDangerousRootCommand_EdgeCases(t *testing.T) {
	config := DefaultConfig()
	config.DangerousRootPatterns = []string{"rm", "dd"}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdPath    string
		wantResult bool
	}{
		{
			name:       "exact match - rm",
			cmdPath:    "/bin/rm",
			wantResult: true,
		},
		{
			name:       "exact match - dd",
			cmdPath:    "/usr/bin/dd",
			wantResult: true,
		},
		{
			name:       "no match - similar name lsrm",
			cmdPath:    "/bin/lsrm",
			wantResult: false,
		},
		{
			name:       "no match - similar name rmdir",
			cmdPath:    "/bin/rmdir",
			wantResult: false,
		},
		{
			name:       "no match - prefix match rmd",
			cmdPath:    "/bin/rmd",
			wantResult: false,
		},
		{
			name:       "no match - suffix match xrm",
			cmdPath:    "/bin/xrm",
			wantResult: false,
		},
		{
			name:       "case insensitive match - RM",
			cmdPath:    "/bin/RM",
			wantResult: true,
		},
		{
			name:       "case insensitive match - Rm",
			cmdPath:    "/bin/Rm",
			wantResult: true,
		},
		{
			name:       "no match - safe command",
			cmdPath:    "/usr/bin/ls",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.IsDangerousRootCommand(tt.cmdPath)
			assert.Equal(t, tt.wantResult, result)
		})
	}
}
