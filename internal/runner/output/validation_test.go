package output

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/require"
)

func TestConfigValidator_ValidateGlobalConfig(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name        string
		config      *runnertypes.GlobalConfig
		expectError bool
		expectedMax int64
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "negative max output size",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: -1,
			},
			expectError: true,
		},
		{
			name: "zero max output size (should be invalid)",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: 0,
			},
			expectError: true,
		},
		{
			name: "exceeds absolute maximum",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: AbsoluteMaxOutputSize + 1,
			},
			expectError: true,
		},
		{
			name: "valid max output size",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: 5 * 1024 * 1024, // 5MB
			},
			expectError: false,
			expectedMax: 5 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateGlobalConfig(tt.config)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectedMax > 0 {
					require.Equal(t, tt.expectedMax, tt.config.MaxOutputSize)
				}
			}
		})
	}
}

func TestConfigValidator_ValidateCommand(t *testing.T) {
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	tests := []struct {
		name        string
		command     *runnertypes.Command
		expectError bool
	}{
		{
			name:        "nil command",
			command:     nil,
			expectError: true,
		},
		{
			name: "command without output",
			command: &runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
			},
			expectError: false,
		},
		{
			name: "valid command with output",
			command: &runnertypes.Command{
				Name:   "test",
				Cmd:    "echo",
				Output: "output.txt",
			},
			expectError: false,
		},
		{
			name: "command with path traversal",
			command: &runnertypes.Command{
				Name:   "test",
				Cmd:    "echo",
				Output: "../../../etc/passwd",
			},
			expectError: true,
		},
		{
			name: "command with system directory output",
			command: &runnertypes.Command{
				Name:   "test",
				Cmd:    "echo",
				Output: "/etc/malicious.txt",
			},
			expectError: true,
		},
		{
			name: "command with executable extension",
			command: &runnertypes.Command{
				Name:   "test",
				Cmd:    "echo",
				Output: "malicious.sh",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.command, globalConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidator_ValidateCommands(t *testing.T) {
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	tests := []struct {
		name        string
		commands    []runnertypes.Command
		expectError bool
	}{
		{
			name:        "empty commands",
			commands:    []runnertypes.Command{},
			expectError: false,
		},
		{
			name: "valid commands",
			commands: []runnertypes.Command{
				{Name: "cmd1", Cmd: "echo", Output: "output1.txt"},
				{Name: "cmd2", Cmd: "echo", Output: "output2.txt"},
			},
			expectError: false,
		},
		{
			name: "conflicting output paths",
			commands: []runnertypes.Command{
				{Name: "cmd1", Cmd: "echo", Output: "output.txt"},
				{Name: "cmd2", Cmd: "echo", Output: "output.txt"},
			},
			expectError: true,
		},
		{
			name: "invalid command in slice",
			commands: []runnertypes.Command{
				{Name: "valid", Cmd: "echo", Output: "valid.txt"},
				{Name: "invalid", Cmd: "echo", Output: "../../../etc/passwd"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommands(tt.commands, globalConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConfigValidator_ValidateCommands_LoopVariablePointerFix tests that the fix for the loop variable pointer issue
// works correctly. Previously, all entries in outputPaths map pointed to the last command due to storing &cmd.
// This test verifies that conflict detection reports the correct command names.
func TestConfigValidator_ValidateCommands_LoopVariablePointerFix(t *testing.T) {
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	// Create commands where multiple commands have the same output path
	// This tests that the error message correctly identifies which commands conflict
	commands := []runnertypes.Command{
		{Name: "first_command", Cmd: "echo", Output: "different_output1.txt"},
		{Name: "second_command", Cmd: "echo", Output: "different_output2.txt"},
		{Name: "third_command", Cmd: "echo", Output: "conflicting_output.txt"}, // First to use this path
		{Name: "fourth_command", Cmd: "echo", Output: "different_output3.txt"},
		{Name: "fifth_command", Cmd: "echo", Output: "conflicting_output.txt"}, // Conflicts with third_command
	}

	err := validator.ValidateCommands(commands, globalConfig)

	// Should get an error about path conflict
	require.Error(t, err)

	// The error message should mention the correct command names: "third_command" and "fifth_command"
	// Before the fix, it would incorrectly report "fifth_command" and "fifth_command" because
	// all pointers pointed to the last command in the loop
	require.Contains(t, err.Error(), "third_command")
	require.Contains(t, err.Error(), "fifth_command")
	require.Contains(t, err.Error(), "conflicting_output.txt")
	require.Contains(t, err.Error(), "output path conflict")

	// Make sure it doesn't contain other command names that shouldn't be in the conflict
	require.NotContains(t, err.Error(), "first_command")
	require.NotContains(t, err.Error(), "second_command")
	require.NotContains(t, err.Error(), "fourth_command")
}

// TestConfigValidator_ValidateCommands_MultipleConflictDetection tests that we can detect
// multiple different path conflicts correctly, ensuring each conflict reports the right commands.
func TestConfigValidator_ValidateCommands_MultipleConflictDetection(t *testing.T) {
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	// Test with multiple conflicting output paths to ensure the fix works in complex scenarios
	commands := []runnertypes.Command{
		{Name: "cmd_a", Cmd: "echo", Output: "output_1.txt"},
		{Name: "cmd_b", Cmd: "echo", Output: "output_2.txt"},
		{Name: "cmd_c", Cmd: "echo", Output: "output_1.txt"}, // Conflicts with cmd_a
	}

	err := validator.ValidateCommands(commands, globalConfig)

	// Should detect the conflict between cmd_a and cmd_c
	require.Error(t, err)
	require.Contains(t, err.Error(), "cmd_a")
	require.Contains(t, err.Error(), "cmd_c")
	require.Contains(t, err.Error(), "output_1.txt")

	// Should not mention cmd_b as it has a unique output path
	require.NotContains(t, err.Error(), "cmd_b")
}

func TestConfigValidator_ValidateConfigFile(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name        string
		config      *runnertypes.Config
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "valid config",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					MaxOutputSize: DefaultMaxOutputSize,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test",
						Commands: []runnertypes.Command{
							{Name: "cmd1", Cmd: "echo", Output: "output.txt"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid global config",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					MaxOutputSize: -1,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfigFile(tt.config)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidator_validateOutputPath(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "empty path",
			path:        "",
			expectError: false,
		},
		{
			name:        "valid relative path",
			path:        "output.txt",
			expectError: false,
		},
		{
			name:        "valid nested path",
			path:        "logs/output.txt",
			expectError: false,
		},
		{
			name:        "path traversal with ../",
			path:        "../sensitive.txt",
			expectError: true,
		},
		{
			name:        "nested path traversal",
			path:        "logs/../../etc/passwd",
			expectError: true,
		},
		{
			name:        "system directory /etc",
			path:        "/etc/malicious.txt",
			expectError: true,
		},
		{
			name:        "system directory /root",
			path:        "/root/malicious.txt",
			expectError: true,
		},
		{
			name:        "executable extension .sh",
			path:        "script.sh",
			expectError: true,
		},
		{
			name:        "executable extension .exe",
			path:        "malware.exe",
			expectError: true,
		},
		{
			name:        "tmp path (medium risk, blocked by default low risk limit)",
			path:        "/tmp/output.txt",
			expectError: true, // /tmp is medium risk, blocked by default low risk limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateOutputPath(tt.path)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfigValidator_AssessSecurityRisk(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name         string
		outputPath   string
		expectedRisk runnertypes.RiskLevel
	}{
		{
			name:         "empty path",
			outputPath:   "",
			expectedRisk: runnertypes.RiskLevelHigh,
		},
		{
			name:         "relative safe path",
			outputPath:   "output.txt",
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name:         "relative path with traversal",
			outputPath:   "../sensitive.txt",
			expectedRisk: runnertypes.RiskLevelHigh,
		},
		{
			name:         "system directory /etc",
			outputPath:   "/etc/config.txt",
			expectedRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:         "system directory /root",
			outputPath:   "/root/file.txt",
			expectedRisk: runnertypes.RiskLevelCritical,
		},
		{
			name:         "tmp directory",
			outputPath:   "/tmp/output.txt",
			expectedRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:         "other absolute path",
			outputPath:   "/home/user/output.txt",
			expectedRisk: runnertypes.RiskLevelHigh,
		},
		{
			name:         "suspicious file pattern",
			outputPath:   "passwd_backup.txt",
			expectedRisk: runnertypes.RiskLevelHigh,
		},
		{
			name:         "ssh key pattern",
			outputPath:   "id_rsa_backup",
			expectedRisk: runnertypes.RiskLevelHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := validator.AssessSecurityRisk(tt.outputPath)
			require.Equal(t, tt.expectedRisk, risk)
		})
	}
}

func TestConfigValidator_GenerateValidationReport(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name           string
		config         *runnertypes.Config
		expectedValid  bool
		expectedErrors int
	}{
		{
			name:           "nil config",
			config:         nil,
			expectedValid:  false,
			expectedErrors: 1,
		},
		{
			name: "valid config",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					MaxOutputSize: DefaultMaxOutputSize,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test",
						Commands: []runnertypes.Command{
							{Name: "cmd1", Cmd: "echo", Output: "output.txt"},
						},
					},
				},
			},
			expectedValid:  true,
			expectedErrors: 0,
		},
		{
			name: "invalid config with multiple errors",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					MaxOutputSize: -1, // Invalid
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test",
						Commands: []runnertypes.Command{
							{Name: "cmd1", Cmd: "echo", Output: "../../../etc/passwd"}, // Invalid
						},
					},
				},
			},
			expectedValid:  false,
			expectedErrors: 2, // Global config error + command error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := validator.GenerateValidationReport(tt.config)

			require.Equal(t, tt.expectedValid, report.Valid)
			require.Len(t, report.Errors, tt.expectedErrors)

			// Test string representation
			reportStr := report.String()
			require.NotEmpty(t, reportStr)
			require.Contains(t, reportStr, "Validation Report")
		})
	}
}

func TestConfigValidator_getEffectiveMaxSize(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name         string
		config       *runnertypes.GlobalConfig
		expectedSize int64
	}{
		{
			name:         "nil config",
			config:       nil,
			expectedSize: DefaultMaxOutputSize,
		},
		{
			name: "zero max size",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: 0,
			},
			expectedSize: DefaultMaxOutputSize,
		},
		{
			name: "negative max size",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: -1,
			},
			expectedSize: DefaultMaxOutputSize,
		},
		{
			name: "valid max size",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: 5 * 1024 * 1024,
			},
			expectedSize: 5 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := validator.getEffectiveMaxSize(tt.config)
			require.Equal(t, tt.expectedSize, size)
		})
	}
}

func TestConfigValidator_IntegratedPatternDetection(t *testing.T) {
	// Test that the integrated patterns from security config work correctly
	validator := NewConfigValidator()

	tests := []struct {
		name         string
		path         string
		expectedRisk runnertypes.RiskLevel
		description  string
	}{
		{
			name:         "critical system directory from security config",
			path:         "/etc/passwd",
			expectedRisk: runnertypes.RiskLevelCritical,
			description:  "Should detect /etc/passwd from OutputCriticalPathPatterns",
		},
		{
			name:         "high risk directory from security config",
			path:         "/var/log/system.log",
			expectedRisk: runnertypes.RiskLevelHigh,
			description:  "Should detect /var/log/ from OutputHighRiskPathPatterns",
		},
		{
			name:         "suspicious file pattern",
			path:         "id_rsa_backup",
			expectedRisk: runnertypes.RiskLevelHigh,
			description:  "Should detect id_rsa pattern from GetSuspiciousFilePatterns",
		},
		{
			name:         "authorized_keys pattern",
			path:         "home/user/.ssh/authorized_keys",
			expectedRisk: runnertypes.RiskLevelHigh,
			description:  "Should detect authorized_keys pattern",
		},
		{
			name:         "docker config pattern",
			path:         ".docker/config.json",
			expectedRisk: runnertypes.RiskLevelHigh,
			description:  "Should detect docker config pattern",
		},
		{
			name:         "safe relative path",
			path:         "logs/application.log",
			expectedRisk: runnertypes.RiskLevelLow,
			description:  "Should assess safe relative paths as low risk",
		},
		{
			name:         "safe tmp path",
			path:         "/tmp/safe_output.txt",
			expectedRisk: runnertypes.RiskLevelMedium,
			description:  "Should assess /tmp paths as medium risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := validator.AssessSecurityRisk(tt.path)

			require.Equal(t, tt.expectedRisk, risk, tt.description)
		})
	}
}

func TestConfigValidator_CustomSecurityConfig(t *testing.T) {
	// Test that custom security config is used correctly
	customConfig := security.DefaultConfig()
	customConfig.OutputCriticalPathPatterns = append(customConfig.OutputCriticalPathPatterns, "/custom/critical/")

	validator := NewConfigValidatorWithSecurity(customConfig)

	// Test that the custom pattern is detected with critical risk level
	risk := validator.AssessSecurityRisk("/custom/critical/file.txt")
	require.Equal(t, runnertypes.RiskLevelCritical, risk, "Should detect custom critical path pattern")
}

func TestConfigValidator_FalsePositivePrevention(t *testing.T) {
	// Test that string contains matching doesn't create false positives
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	tests := []struct {
		name        string
		command     *runnertypes.Command
		expectError bool
		description string
	}{
		{
			name: "false_positive_etc_in_path",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/home/user/project-etc/file.txt",
				MaxRiskLevel: "high", // Allow high risk (absolute paths outside working dir)
			},
			expectError: false,
			description: "Should not flag paths containing 'etc' in directory names when max_risk_level allows it",
		},
		{
			name: "false_positive_root_in_path",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/home/user/project-root/file.txt",
				MaxRiskLevel: "high", // Allow high risk
			},
			expectError: false,
			description: "Should not flag paths containing 'root' in directory names when max_risk_level allows it",
		},
		{
			name: "false_positive_bin_in_path",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/home/user/my-bin/file.txt",
				MaxRiskLevel: "high", // Allow high risk
			},
			expectError: false,
			description: "Should not flag paths containing 'bin' in directory names when max_risk_level allows it",
		},
		{
			name: "true_positive_actual_etc",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/etc/passwd",
				MaxRiskLevel: "high", // Even high risk should not allow critical system directories
			},
			expectError: true,
			description: "Should correctly flag actual /etc paths even with high max_risk_level",
		},
		{
			name: "true_positive_actual_root",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/root/sensitive.txt",
				MaxRiskLevel: "high", // Even high risk should not allow critical system directories
			},
			expectError: true,
			description: "Should correctly flag actual /root paths even with high max_risk_level",
		},
		{
			name: "false_positive_etc_in_filename",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/home/user/etc-backup.txt",
				MaxRiskLevel: "high", // Allow high risk
			},
			expectError: false,
			description: "Should not flag files with 'etc' in filename when not in /etc directory and max_risk_level allows it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.command, globalConfig)

			if tt.expectError {
				require.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

func TestConfigValidator_RiskAssessmentFalsePositivePrevention(t *testing.T) {
	// Test that risk assessment doesn't create false positives
	validator := NewConfigValidator()

	tests := []struct {
		name         string
		path         string
		expectedRisk runnertypes.RiskLevel
		description  string
	}{
		{
			name:         "false_positive_etc_in_path",
			path:         "/home/user/project-etc/file.txt",
			expectedRisk: runnertypes.RiskLevelHigh, // Absolute path outside /tmp is high risk
			description:  "Should not elevate to critical due to 'etc' in path",
		},
		{
			name:         "true_positive_actual_etc",
			path:         "/etc/config.txt",
			expectedRisk: runnertypes.RiskLevelCritical,
			description:  "Should correctly assess actual /etc as critical",
		},
		{
			name:         "false_positive_root_in_path",
			path:         "/home/user/project-root/file.txt",
			expectedRisk: runnertypes.RiskLevelHigh, // Absolute path outside /tmp is high risk
			description:  "Should not elevate to critical due to 'root' in path",
		},
		{
			name:         "true_positive_actual_root",
			path:         "/root/file.txt",
			expectedRisk: runnertypes.RiskLevelCritical,
			description:  "Should correctly assess actual /root as critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk := validator.AssessSecurityRisk(tt.path)
			require.Equal(t, tt.expectedRisk, risk, tt.description)
		})
	}
}

func TestConfigValidator_MaxRiskLevel(t *testing.T) {
	validator := NewConfigValidator()
	globalConfig := &runnertypes.GlobalConfig{
		MaxOutputSize: DefaultMaxOutputSize,
	}

	tests := []struct {
		name        string
		command     *runnertypes.Command
		expectError bool
		description string
	}{
		{
			name: "high risk path with max_risk_level high (should pass)",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "malicious.sh", // High risk due to .sh extension
				MaxRiskLevel: "high",         // Allow high risk
			},
			expectError: false,
			description: "High risk .sh file should be allowed when max_risk_level is high",
		},
		{
			name: "high risk path with max_risk_level low (should fail)",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "malicious.sh", // High risk due to .sh extension
				MaxRiskLevel: "low",          // Only allow low risk
			},
			expectError: true,
			description: "High risk .sh file should be rejected when max_risk_level is low",
		},
		{
			name: "critical risk path with max_risk_level high (should fail)",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/etc/passwd", // Critical risk
				MaxRiskLevel: "high",        // Allow high but not critical
			},
			expectError: true,
			description: "Critical risk /etc/passwd should be rejected even when max_risk_level is high",
		},
		{
			name: "medium risk path with max_risk_level medium (should pass)",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "/tmp/output.txt", // Medium risk (tmp directory)
				MaxRiskLevel: "medium",          // Allow medium risk
			},
			expectError: false,
			description: "Medium risk /tmp path should be allowed when max_risk_level is medium",
		},
		{
			name: "low risk path always passes",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "output.txt", // Low risk (relative path)
				MaxRiskLevel: "low",        // Most restrictive
			},
			expectError: false,
			description: "Low risk relative path should always be allowed",
		},
		{
			name: "default max_risk_level behavior (empty string defaults to low)",
			command: &runnertypes.Command{
				Name:         "test",
				Cmd:          "echo",
				Output:       "malicious.sh", // High risk due to .sh extension
				MaxRiskLevel: "",             // Default (should be low)
			},
			expectError: true,
			description: "High risk .sh file should be rejected when max_risk_level is default (low)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCommand(tt.command, globalConfig)

			if tt.expectError {
				require.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}
