package debug

import (
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestFormatInheritanceAnalysisText_Nil(t *testing.T) {
	result := FormatInheritanceAnalysisText(nil, "test_group")
	if result != "" {
		t.Errorf("Expected empty string for nil analysis, got %q", result)
	}
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
	if !strings.Contains(result, "===== from_env Inheritance Analysis =====") {
		t.Error("Expected header not found in output")
	}

	if !strings.Contains(result, "[Global Level]") {
		t.Error("Expected Global Level section not found")
	}

	if !strings.Contains(result, "env_import defined: 2 mappings") {
		t.Error("Expected global env_import count not found")
	}

	if !strings.Contains(result, "db_host=DB_HOST") {
		t.Error("Expected global mapping not found")
	}

	if !strings.Contains(result, "api_key=API_KEY") {
		t.Error("Expected global mapping not found")
	}

	if !strings.Contains(result, "Internal variables created: api_key, db_host") {
		t.Error("Expected internal variables not found")
	}

	if !strings.Contains(result, "[Group: test_group]") {
		t.Error("Expected Group section not found")
	}

	if !strings.Contains(result, "env_import: Inheriting from Global") {
		t.Error("Expected inheritance message not found")
	}

	if !strings.Contains(result, "Inherited variables (2): api_key, db_host") {
		t.Error("Expected inherited variables not found")
	}

	if !strings.Contains(result, "[Allowlist Inheritance]") {
		t.Error("Expected allowlist section not found")
	}

	if !strings.Contains(result, "Inheriting Global env_allowlist") {
		t.Error("Expected allowlist inheritance message not found")
	}

	if !strings.Contains(result, "Allowlist (2): PATH, HOME") {
		t.Error("Expected allowlist variables not found")
	}
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
	if !strings.Contains(result, "env_import: Overriding Global configuration") {
		t.Error("Expected override message not found")
	}

	if !strings.Contains(result, "Group-specific mappings (1):") {
		t.Error("Expected group mappings count not found")
	}

	if !strings.Contains(result, "group_var=GROUP_VAR") {
		t.Error("Expected group mapping not found")
	}

	if !strings.Contains(result, "Group variables: group_var") {
		t.Error("Expected group variables not found")
	}

	// Check for unavailable variables warning
	if !strings.Contains(result, "Warning: Global variables (api_key, db_host) are NOT available") {
		t.Error("Expected unavailable variables warning not found")
	}

	if !strings.Contains(result, "These variables will be undefined: %{api_key}, %{db_host}") {
		t.Error("Expected undefined variables list not found")
	}

	// Check allowlist section
	if !strings.Contains(result, "Using group-specific env_allowlist") {
		t.Error("Expected group allowlist message not found")
	}

	if !strings.Contains(result, "Group allowlist (2): PATH, HOME") {
		t.Error("Expected group allowlist not found")
	}

	if !strings.Contains(result, "Removed from Global allowlist: USER") {
		t.Error("Expected removed allowlist variables not found")
	}
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

	if !strings.Contains(result, "Rejecting all environment variables") {
		t.Error("Expected reject message not found")
	}

	if !strings.Contains(result, "(Group has empty env_allowlist defined, blocking all env inheritance)") {
		t.Error("Expected reject explanation not found")
	}
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

	if !strings.Contains(result, "env_import: not defined") {
		t.Error("Expected empty global env_import message not found")
	}

	if !strings.Contains(result, "(Global has no env_import defined, so nothing to inherit)") {
		t.Error("Expected no inheritance message not found")
	}

	if !strings.Contains(result, "(Global has no env_allowlist defined, so all variables allowed)") {
		t.Error("Expected no allowlist message not found")
	}
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
			if result != tt.expected {
				t.Errorf("formatStringSlice() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatGroupField(t *testing.T) {
	result := formatGroupField("test_field", 5)
	expected := "  test_field (5):"
	if result != expected {
		t.Errorf("formatGroupField() = %q, want %q", result, expected)
	}
}

func TestFormatFinalEnvironmentText_Nil(t *testing.T) {
	result := FormatFinalEnvironmentText(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil environment, got %q", result)
	}
}

func TestFormatFinalEnvironmentText_Empty(t *testing.T) {
	env := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{},
	}

	result := FormatFinalEnvironmentText(env)

	if !strings.Contains(result, "===== Final Process Environment =====") {
		t.Error("Expected header not found in output")
	}

	if !strings.Contains(result, "No environment variables set.") {
		t.Error("Expected empty message not found")
	}
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

	if !strings.Contains(result, "===== Final Process Environment =====") {
		t.Error("Expected header not found in output")
	}

	if !strings.Contains(result, "Environment variables (3):") {
		t.Error("Expected variable count not found")
	}

	// Check variables are sorted and formatted correctly
	// API_KEY is masked because Masked field is set to true
	if !strings.Contains(result, "API_KEY=[REDACTED]") {
		t.Error("Expected API_KEY variable not found")
	}

	if !strings.Contains(result, "(from env_import)") {
		t.Error("Expected source info not found")
	}

	if !strings.Contains(result, "DB_HOST=localhost") {
		t.Error("Expected DB_HOST variable not found")
	}

	if !strings.Contains(result, "(from vars)") {
		t.Error("Expected vars source not found")
	}

	if !strings.Contains(result, "PATH=/usr/bin:/bin") {
		t.Error("Expected PATH variable not found")
	}

	if !strings.Contains(result, "(from system)") {
		t.Error("Expected system source not found")
	}
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

	if !strings.Contains(result, "NORMAL_VAR=normal_value") {
		t.Error("Expected normal variable not found")
	}

	if !strings.Contains(result, "SENSITIVE_VAR=[REDACTED]") {
		t.Error("Expected masked variable not found")
	}

	if !strings.Contains(result, "(from env_import)") {
		t.Error("Expected masked variable source not found")
	}
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

	if !strings.Contains(result, expectedLine) {
		t.Errorf("Expected full value not found. Got: %s", result)
	}

	// Verify no ellipsis is present
	if strings.Contains(result, "...") {
		t.Error("Long values should not be truncated")
	}

	if !strings.Contains(result, "(from command)") {
		t.Error("Expected command source not found")
	}
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
	if !strings.Contains(result, `VAR_WITH_NEWLINE=value\nwith\nnewlines`) {
		t.Errorf("Newlines should be escaped. Got: %s", result)
	}
	if !strings.Contains(result, `VAR_WITH_TAB=value\twith\ttabs`) {
		t.Errorf("Tabs should be escaped. Got: %s", result)
	}

	// Verify raw control characters are NOT in output
	if strings.Contains(result, "value\nwith\nnewlines") {
		t.Error("Raw newlines should not be in output")
	}
	if strings.Contains(result, "value\twith\ttabs") {
		t.Error("Raw tabs should not be in output")
	}
}

// TestFormatConsistency_InheritanceAnalysis compares the output between
// the existing PrintFromEnvInheritance and the new FormatInheritanceAnalysisText
func TestFormatConsistency_InheritanceAnalysis(t *testing.T) {
	// Create test data
	global := &runnertypes.GlobalSpec{
		EnvImport:  []string{"db_host=DB_HOST", "api_key=API_KEY"},
		EnvAllowed: []string{"PATH", "HOME"},
	}

	runtimeGroup := &runnertypes.RuntimeGroup{
		Spec: &runnertypes.GroupSpec{
			Name:       "test_group",
			EnvImport:  []string{}, // Empty means inherit
			EnvAllowed: []string{},
		},
		EnvAllowlistInheritanceMode: runnertypes.InheritanceModeInherit,
	}

	// Get output from existing function
	var existingBuf strings.Builder
	PrintFromEnvInheritance(&existingBuf, global, runtimeGroup)
	existingOutput := existingBuf.String()

	// Create corresponding InheritanceAnalysis
	analysis := &resource.InheritanceAnalysis{
		GlobalEnvImport: global.EnvImport,
		GlobalAllowlist: global.EnvAllowed,
		GroupEnvImport:  runtimeGroup.Spec.EnvImport,
		GroupAllowlist:  runtimeGroup.Spec.EnvAllowed,
		InheritanceMode: runtimeGroup.EnvAllowlistInheritanceMode,
	}

	// Get output from new function with the actual group name
	newOutput := FormatInheritanceAnalysisText(analysis, runtimeGroup.Spec.Name)

	// Compare outputs - they should be identical
	if existingOutput != newOutput {
		t.Errorf("Output mismatch!\nExisting:\n%s\nNew:\n%s", existingOutput, newOutput)
	}
}

// TestFormatConsistency_FinalEnvironment compares the output between
// the existing PrintFinalEnvironment and the new FormatFinalEnvironmentText
func TestFormatConsistency_FinalEnvironment(t *testing.T) {
	// Create test environment map
	envMap := map[string]executor.EnvVar{
		"PATH": {
			Value:  "/usr/bin:/bin",
			Origin: "system",
		},
		"API_KEY": {
			Value:  "secret123",
			Origin: "env_import",
		},
	}

	// Get output from existing function
	var existingBuf strings.Builder
	PrintFinalEnvironment(&existingBuf, envMap, false) // showSensitive = false
	existingOutput := existingBuf.String()

	// Create corresponding FinalEnvironment
	// API_KEY should be masked to match PrintFinalEnvironment behavior with showSensitive=false
	finalEnv := &resource.FinalEnvironment{
		Variables: map[string]resource.EnvironmentVariable{
			"PATH": {
				Value:  "/usr/bin:/bin",
				Source: "system",
				Masked: false,
			},
			"API_KEY": {
				Value:  "secret123",
				Source: "env_import",
				Masked: true, // Masked to match showSensitive=false behavior
			},
		},
	}

	// Get output from new function
	newOutput := FormatFinalEnvironmentText(finalEnv)

	// Compare outputs - they should be identical
	if existingOutput != newOutput {
		t.Errorf("Output mismatch!\nExisting:\n%s\nNew:\n%s", existingOutput, newOutput)
	}
}
