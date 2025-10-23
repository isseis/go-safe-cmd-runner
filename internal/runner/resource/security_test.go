//go:build test
// +build test

package resource

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestSecurityAnalysis verifies that security analysis properly identifies risks
func TestSecurityAnalysis(t *testing.T) {
	tests := []struct {
		name            string
		spec            runnertypes.CommandSpec
		expectRisk      bool
		expectedPattern string // Expected pattern found in security analysis
	}{
		{
			name: "dangerous command - rm with wildcards",
			spec: runnertypes.CommandSpec{
				Name:        "dangerous-rm",
				Description: "Dangerous rm command",
				Cmd:         "rm",
				Args:        []string{"-rf", "/tmp/*"},
			},
			expectRisk:      true,
			expectedPattern: "rm -rf", // This pattern is in the security analysis
		},
		{
			name: "sudo rm command with user specification",
			spec: runnertypes.CommandSpec{
				Name:        "sudo-rm-command",
				Description: "Command requiring sudo with rm",
				Cmd:         "sudo",
				Args:        []string{"rm", "-rf", "/tmp/files"},
				RunAsUser:   "root",
			},
			expectRisk:      true,
			expectedPattern: "Privileged", // Should detect privileged file removal
		},
		{
			name: "network command - curl to external",
			spec: runnertypes.CommandSpec{
				Name:        "external-curl",
				Description: "External network request",
				Cmd:         "curl",
				Args:        []string{"https://external-api.example.com/data"},
			},
			expectRisk:      true,
			expectedPattern: "curl", // curl is a medium risk pattern
		},
		{
			name: "safe command - simple echo",
			spec: runnertypes.CommandSpec{
				Name:        "safe-echo",
				Description: "Safe echo command",
				Cmd:         "echo",
				Args:        []string{"hello", "world"},
			},
			expectRisk: true, // Now expects risk due to directory-based assessment
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			dryRunOpts := &DryRunOptions{
				DetailLevel:   DetailLevelDetailed,
				OutputFormat:  OutputFormatText,
				ShowSensitive: false,
				VerifyFiles:   true,
			}

			mockPathResolver := &MockPathResolver{}
			setupStandardCommandPaths(mockPathResolver) // fallback
			manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
			require.NoError(t, err)
			if err != nil {
				t.Fatalf("Failed to create DryRunResourceManager: %v", err)
			}
			require.NotNil(t, manager)

			group := &runnertypes.GroupSpec{
				Name:        "security-test-group",
				Description: "Security test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"TEST_VAR": "test_value",
			}

			// Execute the command
			cmd := createRuntimeCommand(&tt.spec)
			result, err := manager.ExecuteCommand(ctx, cmd, group, envVars)
			assert.NoError(t, err)
			assert.NotNil(t, result)

			// Get dry-run results
			dryRunResult := manager.GetDryRunResults()
			require.NotNil(t, dryRunResult)

			// Verify the resource analysis was captured
			require.Len(t, dryRunResult.ResourceAnalyses, 1, "should have one resource analysis")
			analysis := dryRunResult.ResourceAnalyses[0]

			// Verify basic analysis properties
			assert.Equal(t, ResourceTypeCommand, analysis.Type)
			assert.Equal(t, OperationExecute, analysis.Operation)

			// Verify security analysis results
			if tt.expectRisk {
				// Should have detected security risk
				assert.NotEmpty(t, analysis.Impact.SecurityRisk, "should have detected security risk")

				if tt.expectedPattern != "" {
					// Should contain expected pattern in description
					assert.Contains(t, analysis.Impact.Description, tt.expectedPattern,
						"security analysis description should contain expected pattern")
				}
			} else {
				// Should not have detected security risk
				assert.Empty(t, analysis.Impact.SecurityRisk, "should not have detected security risk for safe command")
			}
		})
	}
}

// TestPrivilegeEscalationDetection tests detection of privilege escalation patterns
func TestPrivilegeEscalationDetection(t *testing.T) {
	tests := []struct {
		name                  string
		spec                  runnertypes.CommandSpec
		expectPrivilegeChange bool
	}{
		{
			name: "sudo rm command",
			spec: runnertypes.CommandSpec{
				Name:        "sudo-rm-test",
				Description: "Sudo rm test command",
				Cmd:         "sudo",
				Args:        []string{"rm", "-rf", "/tmp/test"},
				RunAsUser:   "root",
			},
			expectPrivilegeChange: true,
		},
		{
			name: "normal command",
			spec: runnertypes.CommandSpec{
				Name:        "normal-test",
				Description: "Normal command test",
				Cmd:         "ls",
				Args:        []string{"-la"},
			},
			expectPrivilegeChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			dryRunOpts := &DryRunOptions{
				DetailLevel:   DetailLevelDetailed,
				OutputFormat:  OutputFormatText,
				ShowSensitive: false,
				VerifyFiles:   true,
			}

			mockPathResolver := &MockPathResolver{}
			setupStandardCommandPaths(mockPathResolver) // fallback
			manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
			require.NoError(t, err)
			if err != nil {
				t.Fatalf("Failed to create DryRunResourceManager: %v", err)
			}
			require.NotNil(t, manager)

			group := &runnertypes.GroupSpec{
				Name:        "privilege-test-group",
				Description: "Privilege test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"USER": "testuser",
			}

			// Execute the command
			cmd := createRuntimeCommand(&tt.spec)
			_, err = manager.ExecuteCommand(ctx, cmd, group, envVars)
			assert.NoError(t, err)

			// Get dry-run results
			dryRunResult := manager.GetDryRunResults()
			require.NotNil(t, dryRunResult)

			// Verify the resource analysis was captured
			require.Len(t, dryRunResult.ResourceAnalyses, 1, "should have one resource analysis")
			analysis := dryRunResult.ResourceAnalyses[0]

			// Verify privilege escalation detection
			if tt.expectPrivilegeChange {
				// Should have detected security risk for privileged command
				assert.NotEmpty(t, analysis.Impact.SecurityRisk, "should have detected security risk for privileged command")
				assert.Contains(t, analysis.Impact.Description, "Privileged",
					"should mention privilege requirement in description")
			} else if analysis.Impact.Description != "" {
				// Normal commands may still have some security analysis but shouldn't mention privilege
				assert.NotContains(t, analysis.Impact.Description, "Privileged",
					"should not mention privilege for normal command")
			}
		})
	}
}

// TestCommandSecurityAnalysis tests that the security analysis function is called correctly
func TestCommandSecurityAnalysis(t *testing.T) {
	ctx := context.Background()

	// Test that we can directly verify the security analysis function
	opts := &security.AnalysisOptions{
		VerifyStandardPaths: false,
		HashDir:             "",
	}
	riskLevel, pattern, reason, err := security.AnalyzeCommandSecurity("/bin/rm", []string{"-rf", "/tmp/*"}, opts)
	require.NoError(t, err)

	// Verify direct security analysis works
	assert.Equal(t, runnertypes.RiskLevelHigh, riskLevel, "should detect high risk for rm -rf")
	assert.Contains(t, pattern, "rm -rf", "should identify rm -rf pattern")
	assert.NotEmpty(t, reason, "should provide reason for risk")

	// Test through dry-run manager
	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	mockPathResolver.On("ResolvePath", mock.Anything).Return("/usr/bin/unknown", nil) // fallback
	manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
	require.NoError(t, err)
	if err != nil {
		t.Fatalf("Failed to create DryRunResourceManager: %v", err)
	}
	require.NotNil(t, manager)

	group := &runnertypes.GroupSpec{
		Name:        "security-test-group",
		Description: "Security test group",
		Priority:    1,
	}

	cmd := createRuntimeCommand(&runnertypes.CommandSpec{
		Name:        "dangerous-rm",
		Description: "Dangerous rm command",
		Cmd:         "rm",
		Args:        []string{"-rf", "/tmp/*"},
	})

	// Execute the command
	_, err = manager.ExecuteCommand(ctx, cmd, group, map[string]string{})
	assert.NoError(t, err)

	// Get dry-run results and verify security analysis was applied
	dryRunResult := manager.GetDryRunResults()
	require.NotNil(t, dryRunResult)
	require.Len(t, dryRunResult.ResourceAnalyses, 1)

	analysis := dryRunResult.ResourceAnalyses[0]
	assert.NotEmpty(t, analysis.Impact.SecurityRisk, "should have security risk")
	assert.Contains(t, analysis.Impact.Description, "WARNING", "should contain security warning")
}

// TestSecurityAnalysisIntegration tests the overall security analysis integration
func TestSecurityAnalysisIntegration(t *testing.T) {
	ctx := context.Background()

	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	mockPathResolver := &MockPathResolver{}
	setupStandardCommandPaths(mockPathResolver)
	manager, err := NewDryRunResourceManager(nil, nil, mockPathResolver, dryRunOpts)
	require.NoError(t, err)
	if err != nil {
		t.Fatalf("Failed to create DryRunResourceManager: %v", err)
	}
	require.NotNil(t, manager)

	group := &runnertypes.GroupSpec{
		Name:        "security-integration-test",
		Description: "Security integration test group",
		Priority:    1,
	}

	// Test multiple commands with different risk levels
	commandSpecs := []runnertypes.CommandSpec{
		{
			Name: "high-risk",
			Cmd:  "rm",
			Args: []string{"-rf", "/"},
		},
		{
			Name: "medium-risk",
			Cmd:  "curl",
			Args: []string{"https://example.com"},
		},
		{
			Name: "safe",
			Cmd:  "echo",
			Args: []string{"hello"},
		},
	}

	var analyses []ResourceAnalysis
	for _, cmdSpec := range commandSpecs {
		cmd := createRuntimeCommand(&cmdSpec)
		_, err := manager.ExecuteCommand(ctx, cmd, group, map[string]string{})
		assert.NoError(t, err)

		result := manager.GetDryRunResults()
		require.NotNil(t, result)

		// Get the latest analysis
		if len(result.ResourceAnalyses) > 0 {
			analyses = append(analyses, result.ResourceAnalyses[len(result.ResourceAnalyses)-1])
		}
	}

	// Verify we captured analyses for all commands
	assert.Len(t, analyses, 3, "should have analyses for all commands")

	// Verify high-risk command has security risk
	highRiskAnalysis := analyses[0]
	assert.NotEmpty(t, highRiskAnalysis.Impact.SecurityRisk, "high-risk command should have security risk")

	// Verify medium-risk command has security risk
	mediumRiskAnalysis := analyses[1]
	assert.NotEmpty(t, mediumRiskAnalysis.Impact.SecurityRisk, "medium-risk command should have security risk")

	// Safe command may or may not have security info, but should not fail
	safeAnalysis := analyses[2]
	assert.Equal(t, ResourceTypeCommand, safeAnalysis.Type, "safe command should still be analyzed")
}
