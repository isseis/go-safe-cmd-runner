package output

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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
			name: "zero max output size (should set default)",
			config: &runnertypes.GlobalConfig{
				MaxOutputSize: 0,
			},
			expectError: false,
			expectedMax: DefaultMaxOutputSize,
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
			expectError: true,
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
			name:        "safe /tmp path",
			path:        "/tmp/output.txt",
			expectError: false,
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
	workDir := "/tmp"

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
			risk := validator.AssessSecurityRisk(tt.outputPath, workDir)
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
