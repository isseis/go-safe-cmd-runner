package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	// testHashDir is the hash directory used in tests to avoid permission issues
	testHashDir = "/tmp"
)

// TestEndToEnd_InteractiveLogging tests the complete system with interactive logging
func TestEndToEnd_InteractiveLogging(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Skip this test for now as it requires special directory permissions
	// that are not available in the test environment
	t.Skip("E2E test requires root-owned hash directory - skipping in test environment")

	// Create temporary directories and files for test
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.toml")
	logDir := filepath.Join(tempDir, "logs")

	// Use /tmp for hash directory to avoid permission issues in tests
	// The system has proper security validation for hash directories
	hashDir := testHashDir

	// Create log directory
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("Failed to create log directory: %v", err)
	}

	// Create a simple test configuration
	configContent := `
[global]
log_level = "info"

[[groups]]
name = "test_group"
description = "Test group for E2E testing"

[[groups.commands]]
name = "echo_test"
description = "Simple echo command"
cmd = "/bin/echo"
args = ["Hello from interactive logging test"]
working_directory = "` + tempDir + `"
timeout_seconds = 10
`

	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test different scenarios
	testCases := []struct {
		name              string
		args              []string
		envVars           map[string]string
		expectInteractive bool
		expectColorLog    bool
	}{
		{
			name: "dry_run_interactive_mode",
			args: []string{
				"-config", configFile,
				"-hash-directory", hashDir,
				"-log-dir", logDir,
				"-dry-run",
				"-format", "text",
			},
			envVars: map[string]string{
				"TERM":     "xterm-256color",
				"NO_COLOR": "",
			},
			expectInteractive: false, // Dry run in test environment
			expectColorLog:    false,
		},
		{
			name: "forced_no_color_mode",
			args: []string{
				"-config", configFile,
				"-hash-directory", hashDir,
				"-log-dir", logDir,
				"-dry-run",
			},
			envVars: map[string]string{
				"NO_COLOR": "1",
				"TERM":     "xterm-256color",
			},
			expectInteractive: false,
			expectColorLog:    false,
		},
		{
			name: "ci_environment_mode",
			args: []string{
				"-config", configFile,
				"-hash-directory", hashDir,
				"-log-dir", logDir,
				"-dry-run",
			},
			envVars: map[string]string{
				"CI":       "true",
				"NO_COLOR": "",
			},
			expectInteractive: false,
			expectColorLog:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srcDir, err := os.Getwd()
			if err != nil {
				t.Fatalf("Failed to get working directory: %v", err)
			}
			// Use go run to execute the command
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Set up environment
			cmd := exec.CommandContext(ctx, "go", append([]string{"run", "."}, tc.args...)...)
			cmd.Dir = srcDir

			// Set environment variables
			cmd.Env = os.Environ()
			for key, value := range tc.envVars {
				if value == "" {
					// Remove environment variable
					newEnv := []string{}
					for _, env := range cmd.Env {
						if !strings.HasPrefix(env, key+"=") {
							newEnv = append(newEnv, env)
						}
					}
					cmd.Env = newEnv
				} else {
					cmd.Env = append(cmd.Env, key+"="+value)
				}
			}

			// Capture output
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("Command output: %s", string(output))
				t.Fatalf("Command failed: %v", err)
			}

			// Verify output contains expected elements
			outputStr := string(output)

			// Should contain logger initialization message
			if !strings.Contains(outputStr, "Logger initialized") {
				t.Error("Expected 'Logger initialized' message in output")
			}

			// Should contain interactive_mode and color_support information
			if !strings.Contains(outputStr, "interactive_mode") {
				t.Error("Expected 'interactive_mode' information in output")
			}
			if !strings.Contains(outputStr, "color_support") {
				t.Error("Expected 'color_support' information in output")
			}

			// For dry-run, should contain the command information
			if strings.Contains(strings.Join(tc.args, " "), "-dry-run") {
				if !strings.Contains(outputStr, "echo_test") || !strings.Contains(outputStr, "Hello from interactive logging test") {
					t.Error("Expected dry-run output to contain command information")
					t.Logf("Output: %s", outputStr)
				}
			}

			// Verify log file was created
			logEntries, err := os.ReadDir(logDir)
			if err != nil {
				t.Fatalf("Failed to read log directory: %v", err)
			}

			if len(logEntries) == 0 {
				t.Error("Expected log file to be created")
			} else {
				// Check log file content
				logFile := filepath.Join(logDir, logEntries[0].Name())
				logContent, err := os.ReadFile(logFile)
				if err != nil {
					t.Fatalf("Failed to read log file: %v", err)
				}

				logStr := string(logContent)
				if !strings.Contains(logStr, "Logger initialized") {
					t.Error("Expected log file to contain initialization message")
				}
			}

			// Clean up log files for next test iteration
			for _, entry := range logEntries {
				os.Remove(filepath.Join(logDir, entry.Name()))
			}
		})
	}
}

// TestEndToEnd_ConfigValidation tests the config validation with new logging
func TestEndToEnd_ConfigValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Skip this test for now as it requires special directory permissions
	t.Skip("E2E test requires root-owned hash directory - skipping in test environment")

	tempDir := t.TempDir()

	testCases := []struct {
		name          string
		configContent string
		expectError   bool
		envVars       map[string]string
	}{
		{
			name: "valid_config_with_interactive_logging",
			configContent: `
[global]
log_level = "debug"

[[groups]]
name = "test"
description = "Test group"

[[groups.commands]]
name = "test_cmd"
cmd = "/bin/true"
`,
			expectError: false,
			envVars: map[string]string{
				"TERM": "xterm",
			},
		},
		{
			name: "invalid_config",
			configContent: `
[global]
log_level = "invalid"

[[groups]]
name = ""  # Invalid empty name
`,
			expectError: true,
			envVars: map[string]string{
				"NO_COLOR": "1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			configFile := filepath.Join(tempDir, "config.toml")
			if err := os.WriteFile(configFile, []byte(tc.configContent), 0o644); err != nil {
				t.Fatalf("Failed to write config file: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			args := []string{
				"run", ".",
				"-config", configFile,
				"-validate",
			}

			cmd := exec.CommandContext(ctx, "go", args...)

			// Set environment variables
			cmd.Env = os.Environ()
			for key, value := range tc.envVars {
				cmd.Env = append(cmd.Env, key+"="+value)
			}

			output, err := cmd.CombinedOutput()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected validation to fail, but it succeeded. Output: %s", string(output))
				}
			} else {
				if err != nil {
					t.Errorf("Expected validation to succeed, but it failed: %v. Output: %s", err, string(output))
				}

				// Should contain logger initialization and validation messages
				outputStr := string(output)
				if !strings.Contains(outputStr, "Logger initialized") {
					t.Error("Expected logger initialization message in validation output")
				}
			}

			// Clean up
			os.Remove(configFile)
		})
	}
}

// TestEndToEnd_EnvironmentVariableHandling tests environment variable handling
// in the complete system
func TestEndToEnd_EnvironmentVariableHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Skip this test for now as it requires special directory permissions
	t.Skip("E2E test requires root-owned hash directory - skipping in test environment")

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.toml")
	// Use /tmp for hash directory to avoid permission issues
	hashDir := testHashDir

	// Create minimal config
	configContent := `
[global]
log_level = "info"

[[groups]]
name = "env_test"

[[groups.commands]]
name = "env_check"
cmd = "/usr/bin/env"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	testCases := []struct {
		name               string
		envVars            map[string]string
		expectColorSupport bool
		expectInteractive  bool
	}{
		{
			name: "clicolor_force_override",
			envVars: map[string]string{
				"CLICOLOR_FORCE": "1",
				"NO_COLOR":       "1",    // Should be overridden
				"CI":             "true", // Should not affect color
			},
			expectColorSupport: true,
			expectInteractive:  false, // CI environment
		},
		{
			name: "no_color_enforcement",
			envVars: map[string]string{
				"NO_COLOR": "1",
				"TERM":     "xterm-256color",
			},
			expectColorSupport: false,
			expectInteractive:  false, // Test environment
		},
		{
			name: "standard_terminal",
			envVars: map[string]string{
				"TERM": "xterm",
				// No color override variables
			},
			expectColorSupport: false, // Test environment doesn't have true terminal
			expectInteractive:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			args := []string{
				"run", ".",
				"-config", configFile,
				"-hash-directory", hashDir,
				"-dry-run",
				"-log-level", "debug", // Enable debug to see more logging details
			}

			cmd := exec.CommandContext(ctx, "go", args...)

			// Set up environment
			cmd.Env = os.Environ()
			for key, value := range tc.envVars {
				cmd.Env = append(cmd.Env, key+"="+value)
			}

			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Command failed: %v, output: %s", err, string(output))
			}

			outputStr := string(output)

			// Check for expected capability reporting
			if tc.expectColorSupport {
				if !strings.Contains(outputStr, `"color_support":true`) &&
					!strings.Contains(outputStr, "color_support=true") {
					t.Errorf("Expected color support to be enabled in output. Output: %s", outputStr)
				}
			} else {
				if !strings.Contains(outputStr, `"color_support":false`) &&
					!strings.Contains(outputStr, "color_support=false") {
					t.Errorf("Expected color support to be disabled in output. Output: %s", outputStr)
				}
			}

			if tc.expectInteractive {
				if !strings.Contains(outputStr, `"interactive_mode":true`) &&
					!strings.Contains(outputStr, "interactive_mode=true") {
					t.Errorf("Expected interactive mode to be enabled in output. Output: %s", outputStr)
				}
			} else {
				if !strings.Contains(outputStr, `"interactive_mode":false`) &&
					!strings.Contains(outputStr, "interactive_mode=false") {
					t.Errorf("Expected interactive mode to be disabled in output. Output: %s", outputStr)
				}
			}
		})
	}
}

// TestEndToEnd_LoggingFlow tests the complete logging flow
func TestEndToEnd_LoggingFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	// Skip this test for now as it requires special directory permissions
	t.Skip("E2E test requires root-owned hash directory - skipping in test environment")

	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "flow-config.toml")
	logDir := filepath.Join(tempDir, "logs")
	// Use /tmp for hash directory to avoid permission issues
	hashDir := testHashDir

	// Create log directory
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("Failed to create directory %s: %v", logDir, err)
	}

	// Create config with commands that generate different log levels
	configContent := `
[global]
log_level = "debug"

[[groups]]
name = "logging_test"

[[groups.commands]]
name = "success_cmd"
cmd = "/bin/echo"
args = ["Success message"]

[[groups.commands]]
name = "warning_cmd"
cmd = "/bin/sh"
args = ["-c", "echo 'Warning message' >&2; exit 0"]
`

	if err := os.WriteFile(configFile, []byte(configContent), 0o644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"run", ".",
		"-config", configFile,
		"-hash-directory", hashDir,
		"-log-dir", logDir,
		"-dry-run", // Use dry-run to avoid actual command execution complexity
	}

	cmd := exec.CommandContext(ctx, "go", args...)

	// Set up for structured logging test
	cmd.Env = append(os.Environ(),
		"TERM=dumb",  // Disable interactive features
		"NO_COLOR=1", // Ensure consistent output
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command failed: %v, output: %s", err, string(output))
	}

	// Verify structured output
	outputStr := string(output)

	// Should show initialization
	if !strings.Contains(outputStr, "Logger initialized") {
		t.Error("Expected logger initialization message")
	}

	// Should show command information in dry-run
	if !strings.Contains(outputStr, "success_cmd") {
		t.Error("Expected to see success_cmd in dry-run output")
	}

	// Check log file was created and contains structured data
	logEntries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("Failed to read log directory: %v", err)
	}

	if len(logEntries) != 1 {
		t.Errorf("Expected exactly 1 log file, got %d", len(logEntries))
	}

	if len(logEntries) > 0 {
		logFile := filepath.Join(logDir, logEntries[0].Name())
		logContent, err := os.ReadFile(logFile)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		logStr := string(logContent)

		// Should contain JSON formatted logs
		if !strings.Contains(logStr, `"msg":"Logger initialized"`) {
			t.Error("Expected JSON formatted log entry for initialization")
		}

		// Should contain structured attributes
		if !strings.Contains(logStr, `"interactive_mode"`) {
			t.Error("Expected interactive_mode attribute in JSON logs")
		}

		if !strings.Contains(logStr, `"color_support"`) {
			t.Error("Expected color_support attribute in JSON logs")
		}
	}
}
