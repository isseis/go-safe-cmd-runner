package debug

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestFormatInheritanceAnalysisText_Nil(t *testing.T) {
	result := FormatInheritanceAnalysisText(nil, "test_group")
	assert.Empty(t, result, "Expected empty string for nil analysis")
}

func TestFormatInheritanceAnalysisText_InheritMode(t *testing.T) {
	analysis := &resource.InheritanceAnalysis{
		GlobalEnvImport: []string{"db_host=DB_HOST", "api_key=API_KEY"},
		GlobalAllowlist: []string{"PATH", "HOME"},
		GroupEnvImport:  []string{}, // Empty means inherit
		GroupAllowlist:  []string{},
		InheritanceMode: runnertypes.InheritanceModeInherit,
	}

	result := FormatInheritanceAnalysisText(analysis, "test_group")

	// Print actual output for debugging
	t.Logf("Actual output:\n%s", result)

	// Check for key sections
	assert.Contains(t, result, "----- from_env Inheritance Analysis -----", "Expected header not found in output")
	assert.Contains(t, result, "[Global Level]", "Expected Global Level section not found")
	assert.Contains(t, result, "env_import defined: 2 mappings", "Expected global env_import count not found")
	assert.Contains(t, result, "db_host=DB_HOST", "Expected global mapping not found")
	assert.Contains(t, result, "api_key=API_KEY", "Expected global mapping not found")
	assert.Contains(t, result, "Internal variables created: api_key, db_host", "Expected internal variables not found")
	assert.Contains(t, result, "[Group: test_group]", "Expected Group section not found")
	assert.Contains(t, result, "env_import: Inheriting from Global", "Expected inheritance message not found")
	assert.Contains(t, result, "Inherited variables (2): api_key, db_host", "Expected inherited variables not found")
	assert.Contains(t, result, "----- Allowlist Inheritance -----", "Expected allowlist section not found")
	assert.Contains(t, result, "Inheriting Global env_allowlist", "Expected allowlist inheritance message not found")
	assert.Contains(t, result, "Allowlist (2): PATH, HOME", "Expected allowlist variables not found")
}

func TestFormatInheritanceAnalysisText_ExplicitMode(t *testing.T) {
	analysis := &resource.InheritanceAnalysis{
		GlobalEnvImport:               []string{"db_host=DB_HOST", "api_key=API_KEY"},
		GlobalAllowlist:               []string{"PATH", "HOME", "USER"},
		GroupEnvImport:                []string{"group_var=GROUP_VAR"},
		GroupAllowlist:                []string{"PATH", "HOME"},
		InheritanceMode:               runnertypes.InheritanceModeExplicit,
		RemovedAllowlistVariables:     []string{"USER"},
		UnavailableEnvImportVariables: []string{"api_key", "db_host"},
	}

	result := FormatInheritanceAnalysisText(analysis, "explicit_group")

	// Check for override behavior
	assert.Contains(t, result, "env_import: Overriding Global configuration", "Expected override message not found")
	assert.Contains(t, result, "Group-specific mappings (1):", "Expected group mappings count not found")
	assert.Contains(t, result, "group_var=GROUP_VAR", "Expected group mapping not found")
	assert.Contains(t, result, "Group variables: group_var", "Expected group variables not found")

	// Check for unavailable variables warning
	assert.Contains(t, result, "Warning: Global variables (api_key, db_host) are NOT available", "Expected unavailable variables warning not found")
	assert.Contains(t, result, "These variables will be undefined: %{api_key}, %{db_host}", "Expected undefined variables list not found")

	// Check allowlist section
	assert.Contains(t, result, "Using group-specific env_allowlist", "Expected group allowlist message not found")
	assert.Contains(t, result, "Group allowlist (2): PATH, HOME", "Expected group allowlist not found")
	assert.Contains(t, result, "Removed from Global allowlist: USER", "Expected removed allowlist variables not found")
}

func TestFormatInheritanceAnalysisText_RejectMode(t *testing.T) {
	analysis := &resource.InheritanceAnalysis{
		GlobalEnvImport: []string{"db_host=DB_HOST"},
		GlobalAllowlist: []string{"PATH", "HOME"},
		GroupEnvImport:  []string{},
		GroupAllowlist:  []string{},
		InheritanceMode: runnertypes.InheritanceModeReject,
	}

	result := FormatInheritanceAnalysisText(analysis, "reject_group")

	assert.Contains(t, result, "Rejecting all environment variables", "Expected reject message not found")
	assert.Contains(t, result, "(Group has empty env_allowlist defined, blocking all env inheritance)", "Expected reject explanation not found")
}

func TestFormatInheritanceAnalysisText_EmptyGlobal(t *testing.T) {
	analysis := &resource.InheritanceAnalysis{
		GlobalEnvImport: []string{},
		GlobalAllowlist: []string{},
		GroupEnvImport:  []string{},
		GroupAllowlist:  []string{},
		InheritanceMode: runnertypes.InheritanceModeInherit,
	}

	result := FormatInheritanceAnalysisText(analysis, "empty_group")

	assert.Contains(t, result, "env_import: not defined", "Expected empty global env_import message not found")
	assert.Contains(t, result, "(Global has no env_import defined, so nothing to inherit)", "Expected no inheritance message not found")
	assert.Contains(t, result, "(Global has no env_allowlist defined, so all variables allowed)", "Expected no allowlist message not found")
}

func TestFormatStringSlice(t *testing.T) {
	tests := []struct {
		name         string
		items        []string
		emptyMessage string
		expected     string
	}{
		{
			name:         "empty slice",
			items:        []string{},
			emptyMessage: "not defined",
			expected:     "not defined",
		},
		{
			name:         "single item",
			items:        []string{"item1"},
			emptyMessage: "empty",
			expected:     "item1",
		},
		{
			name:         "multiple items",
			items:        []string{"item1", "item2", "item3"},
			emptyMessage: "empty",
			expected:     "item1, item2, item3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStringSlice(tt.items, tt.emptyMessage)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFinalEnvironmentText_Nil(t *testing.T) {
	result := FormatFinalEnvironmentText(nil)
	assert.Empty(t, result, "Expected empty string for nil environment")
}

func TestFormatFinalEnvironmentText_Empty(t *testing.T) {
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{},
	}

	result := FormatFinalEnvironmentText(env)

	assert.Contains(t, result, "----- Final Process Environment -----", "Expected header not found in output")
	assert.Contains(t, result, "No environment variables set.", "Expected empty message not found")
}

func TestFormatFinalEnvironmentText_WithVariables(t *testing.T) {
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{
			"PATH": {
				Value:  "/usr/bin:/bin",
				Source: "system",
				Masked: false,
			},
			"API_KEY": {
				Value:  "secret123",
				Source: "env_import",
				Masked: true, // Masked because it's a sensitive variable
			},
			"DB_HOST": {
				Value:  "localhost",
				Source: "vars",
				Masked: false,
			},
		},
	}

	result := FormatFinalEnvironmentText(env)

	// Print actual output for debugging
	t.Logf("Actual output:\n%s", result)

	assert.Contains(t, result, "----- Final Process Environment -----", "Expected header not found in output")
	assert.Contains(t, result, "Environment variables (3):", "Expected variable count not found")

	// Check variables are sorted and formatted correctly
	// API_KEY is masked because Masked field is set to true
	assert.Contains(t, result, "API_KEY=[REDACTED]", "Expected API_KEY variable not found")
	assert.Contains(t, result, "(from env_import)", "Expected source info not found")
	assert.Contains(t, result, "DB_HOST=localhost", "Expected DB_HOST variable not found")
	assert.Contains(t, result, "(from vars)", "Expected vars source not found")
	assert.Contains(t, result, "PATH=/usr/bin:/bin", "Expected PATH variable not found")
	assert.Contains(t, result, "(from system)", "Expected system source not found")
}

func TestFormatFinalEnvironmentText_WithMaskedVariables(t *testing.T) {
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{
			"NORMAL_VAR": {
				Value:  "normal_value",
				Source: "system",
				Masked: false,
			},
			"SENSITIVE_VAR": {
				Value:  "", // Value is empty when masked
				Source: "env_import",
				Masked: true,
			},
		},
	}

	result := FormatFinalEnvironmentText(env)

	assert.Contains(t, result, "NORMAL_VAR=normal_value", "Expected normal variable not found")
	assert.Contains(t, result, "SENSITIVE_VAR=[REDACTED]", "Expected masked variable not found")
	assert.Contains(t, result, "(from env_import)", "Expected masked variable source not found")
}

func TestFormatFinalEnvironmentText_WithLongValues(t *testing.T) {
	// Create a value longer than 100 characters
	longValue := strings.Repeat("a", 150)
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{
			"LONG_VAR": {
				Value:  longValue,
				Source: "command",
				Masked: false,
			},
		},
	}

	result := FormatFinalEnvironmentText(env)

	// Verify the FULL value is displayed (no truncation for dry-run verification)
	expectedLine := fmt.Sprintf("LONG_VAR=%s", longValue)

	assert.Contains(t, result, expectedLine, "Expected full value not found")
	assert.NotContains(t, result, "...", "Long values should not be truncated")
	assert.Contains(t, result, "(from command)", "Expected command source not found")
}

func TestFormatFinalEnvironmentText_WithControlCharacters(t *testing.T) {
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{
			"VAR_WITH_NEWLINE": {
				Value:  "value\nwith\nnewlines",
				Source: "command",
				Masked: false,
			},
			"VAR_WITH_TAB": {
				Value:  "value\twith\ttabs",
				Source: "vars",
				Masked: false,
			},
		},
	}

	result := FormatFinalEnvironmentText(env)

	// Verify control characters are escaped
	assert.Contains(t, result, `VAR_WITH_NEWLINE=value\nwith\nnewlines`, "Newlines should be escaped")
	assert.Contains(t, result, `VAR_WITH_TAB=value\twith\ttabs`, "Tabs should be escaped")

	// Verify raw control characters are NOT in output
	assert.NotContains(t, result, "value\nwith\nnewlines", "Raw newlines should not be in output")
	assert.NotContains(t, result, "value\twith\ttabs", "Raw tabs should not be in output")
}
