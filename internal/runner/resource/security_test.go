package resource

import (
	"context"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecurityAnalysis verifies that security analysis properly identifies risks
func TestSecurityAnalysis(t *testing.T) {
	tests := []struct {
		name            string
		command         runnertypes.Command
		expectRisk      bool
		expectedPattern string // Expected pattern found in security analysis
	}{
		{
			name: "dangerous command - rm with wildcards",
			command: runnertypes.Command{
				Name:        "dangerous-rm",
				Description: "Dangerous rm command",
				Cmd:         "rm",
				Args:        []string{"-rf", "/tmp/*"},
			},
			expectRisk:      true,
			expectedPattern: "rm -rf", // This pattern is in the security analysis
		},
		{
			name: "privileged command - sudo usage",
			command: runnertypes.Command{
				Name:        "sudo-command",
				Description: "Command requiring sudo",
				Cmd:         "sudo",
				Args:        []string{"systemctl", "restart", "nginx"},
				Privileged:  true,
			},
			expectRisk:      true,
			expectedPattern: "PRIVILEGE", // Should detect privilege escalation
		},
		{
			name: "network command - curl to external",
			command: runnertypes.Command{
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
			command: runnertypes.Command{
				Name:        "safe-echo",
				Description: "Safe echo command",
				Cmd:         "echo",
				Args:        []string{"hello", "world"},
			},
			expectRisk: false,
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

			manager := NewDryRunResourceManager(nil, nil, dryRunOpts)
			require.NotNil(t, manager)

			group := &runnertypes.CommandGroup{
				Name:        "security-test-group",
				Description: "Security test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"TEST_VAR": "test_value",
			}

			// Execute the command
			result, err := manager.ExecuteCommand(ctx, tt.command, group, envVars)
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
		command               runnertypes.Command
		expectPrivilegeChange bool
	}{
		{
			name: "sudo command",
			command: runnertypes.Command{
				Name:        "sudo-test",
				Description: "Sudo test command",
				Cmd:         "sudo",
				Args:        []string{"apt", "update"},
				Privileged:  true,
			},
			expectPrivilegeChange: true,
		},
		{
			name: "normal command",
			command: runnertypes.Command{
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

			manager := NewDryRunResourceManager(nil, nil, dryRunOpts)
			require.NotNil(t, manager)

			group := &runnertypes.CommandGroup{
				Name:        "privilege-test-group",
				Description: "Privilege test group",
				Priority:    1,
			}

			envVars := map[string]string{
				"USER": "testuser",
			}

			// Execute the command
			_, err := manager.ExecuteCommand(ctx, tt.command, group, envVars)
			assert.NoError(t, err)

			// Get dry-run results
			dryRunResult := manager.GetDryRunResults()
			require.NotNil(t, dryRunResult)

			// Verify the resource analysis was captured
			require.Len(t, dryRunResult.ResourceAnalyses, 1, "should have one resource analysis")
			analysis := dryRunResult.ResourceAnalyses[0]

			// Verify privilege escalation detection
			if tt.expectPrivilegeChange {
				// Should have detected privilege requirement
				assert.NotEmpty(t, analysis.Impact.SecurityRisk, "should have detected security risk for privileged command")
				assert.Contains(t, analysis.Impact.Description, "PRIVILEGE",
					"should mention privilege requirement in description")
			} else if analysis.Impact.Description != "" {
				// Normal commands may still have some security analysis but shouldn't mention privilege
				assert.NotContains(t, analysis.Impact.Description, "PRIVILEGE",
					"should not mention privilege for normal command")
			}
		})
	}
}

// TestCommandSecurityAnalysis tests that the security analysis function is called correctly
func TestCommandSecurityAnalysis(t *testing.T) {
	ctx := context.Background()

	// Test that we can directly verify the security analysis function
	riskLevel, pattern, reason := security.AnalyzeCommandSecurity("rm", []string{"-rf", "/tmp/*"})

	// Verify direct security analysis works
	assert.Equal(t, security.RiskLevelHigh, riskLevel, "should detect high risk for rm -rf")
	assert.Contains(t, pattern, "rm -rf", "should identify rm -rf pattern")
	assert.NotEmpty(t, reason, "should provide reason for risk")

	// Test through dry-run manager
	dryRunOpts := &DryRunOptions{
		DetailLevel:   DetailLevelDetailed,
		OutputFormat:  OutputFormatText,
		ShowSensitive: false,
		VerifyFiles:   true,
	}

	manager := NewDryRunResourceManager(nil, nil, dryRunOpts)
	require.NotNil(t, manager)

	group := &runnertypes.CommandGroup{
		Name:        "security-test-group",
		Description: "Security test group",
		Priority:    1,
	}

	command := runnertypes.Command{
		Name:        "dangerous-rm",
		Description: "Dangerous rm command",
		Cmd:         "rm",
		Args:        []string{"-rf", "/tmp/*"},
	}

	// Execute the command
	_, err := manager.ExecuteCommand(ctx, command, group, map[string]string{})
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

	manager := NewDryRunResourceManager(nil, nil, dryRunOpts)
	require.NotNil(t, manager)

	group := &runnertypes.CommandGroup{
		Name:        "security-integration-test",
		Description: "Security integration test group",
		Priority:    1,
	}

	// Test multiple commands with different risk levels
	commands := []runnertypes.Command{
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
	for _, cmd := range commands {
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
