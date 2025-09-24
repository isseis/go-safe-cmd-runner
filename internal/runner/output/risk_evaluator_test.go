package output

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/stretchr/testify/require"
)

func TestRiskEvaluator_EvaluateOutputRisk(t *testing.T) {
	evaluator := NewRiskEvaluator(nil)

	tests := []struct {
		name             string
		outputPath       string
		workDir          string
		expectedRisk     runnertypes.RiskLevel
		expectedCategory string
		description      string
	}{
		{
			name:             "empty path",
			outputPath:       "",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelHigh,
			expectedCategory: "empty_path",
			description:      "Empty output path should be high risk",
		},
		{
			name:             "path traversal",
			outputPath:       "../../../etc/passwd",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelHigh,
			expectedCategory: "path_traversal",
			description:      "Path traversal should be high risk",
		},
		{
			name:             "critical system directory",
			outputPath:       "/etc/passwd",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelCritical,
			expectedCategory: "critical_system_directory",
			description:      "Critical system directory should be critical risk",
		},
		{
			name:             "high risk system directory",
			outputPath:       "/var/log/system.log",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelCritical,
			expectedCategory: "sensitive_system_directory",
			description:      "High risk system directory should be treated as critical",
		},
		{
			name:             "tmp directory",
			outputPath:       "/tmp/output.txt",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelMedium,
			expectedCategory: "temporary_directory",
			description:      "Tmp directory should be medium risk",
		},
		{
			name:             "other absolute path",
			outputPath:       "/home/user/output.txt",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelHigh,
			expectedCategory: "absolute_path",
			description:      "Other absolute paths should be high risk",
		},
		{
			name:             "suspicious file pattern",
			outputPath:       "id_rsa_backup",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelHigh,
			expectedCategory: "suspicious_file_pattern",
			description:      "Suspicious file patterns should be high risk",
		},
		{
			name:             "suspicious extension",
			outputPath:       "malicious.sh",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelHigh,
			expectedCategory: "suspicious_extension",
			description:      "Suspicious extensions should be high risk",
		},
		{
			name:             "safe relative path",
			outputPath:       "output.txt",
			workDir:          "/tmp",
			expectedRisk:     runnertypes.RiskLevelLow,
			expectedCategory: "safe_relative_path",
			description:      "Safe relative paths should be low risk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluation := evaluator.EvaluateOutputRisk(tt.outputPath, tt.workDir)

			require.Equal(t, tt.expectedRisk, evaluation.Level, tt.description)
			require.Equal(t, tt.expectedCategory, evaluation.Category, "Category should match expected")
			require.NotEmpty(t, evaluation.Reason, "Should provide a reason")
		})
	}
}

func TestRiskEvaluator_EvaluateWithMaxRiskLevel(t *testing.T) {
	evaluator := NewRiskEvaluator(nil)

	tests := []struct {
		name           string
		outputPath     string
		workDir        string
		maxAllowedRisk runnertypes.RiskLevel
		expectBlocking bool
		description    string
	}{
		{
			name:           "high risk path with high max allowed",
			outputPath:     "malicious.sh",
			workDir:        "/tmp",
			maxAllowedRisk: runnertypes.RiskLevelHigh,
			expectBlocking: false,
			description:    "High risk should not be blocking when max allowed is high",
		},
		{
			name:           "high risk path with low max allowed",
			outputPath:     "malicious.sh",
			workDir:        "/tmp",
			maxAllowedRisk: runnertypes.RiskLevelLow,
			expectBlocking: true,
			description:    "High risk should be blocking when max allowed is low",
		},
		{
			name:           "critical risk path with high max allowed",
			outputPath:     "/etc/passwd",
			workDir:        "/tmp",
			maxAllowedRisk: runnertypes.RiskLevelHigh,
			expectBlocking: true,
			description:    "Critical risk should be blocking even when max allowed is high",
		},
		{
			name:           "low risk path with low max allowed",
			outputPath:     "output.txt",
			workDir:        "/tmp",
			maxAllowedRisk: runnertypes.RiskLevelLow,
			expectBlocking: false,
			description:    "Low risk should not be blocking when max allowed is low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluation := evaluator.EvaluateWithMaxRiskLevel(tt.outputPath, tt.workDir, tt.maxAllowedRisk)

			require.Equal(t, tt.expectBlocking, evaluation.IsBlocking, tt.description)
		})
	}
}

func TestRiskEvaluator_CreateValidationError(t *testing.T) {
	evaluator := NewRiskEvaluator(nil)

	tests := []struct {
		name        string
		evaluation  *RiskEvaluation
		maxAllowed  runnertypes.RiskLevel
		expectError bool
		description string
	}{
		{
			name: "non-blocking evaluation",
			evaluation: &RiskEvaluation{
				Level:      runnertypes.RiskLevelLow,
				IsBlocking: false,
				Category:   "safe_relative_path",
			},
			maxAllowed:  runnertypes.RiskLevelLow,
			expectError: false,
			description: "Non-blocking evaluation should not create error",
		},
		{
			name: "path traversal error",
			evaluation: &RiskEvaluation{
				Level:      runnertypes.RiskLevelHigh,
				IsBlocking: true,
				Category:   "path_traversal",
				Pattern:    "..",
			},
			maxAllowed:  runnertypes.RiskLevelLow,
			expectError: true,
			description: "Path traversal should create specific error",
		},
		{
			name: "critical system directory error",
			evaluation: &RiskEvaluation{
				Level:      runnertypes.RiskLevelCritical,
				IsBlocking: true,
				Category:   "critical_system_directory",
				Pattern:    "/etc/",
			},
			maxAllowed:  runnertypes.RiskLevelHigh,
			expectError: true,
			description: "Critical system directory should create specific error",
		},
		{
			name: "suspicious extension error",
			evaluation: &RiskEvaluation{
				Level:      runnertypes.RiskLevelHigh,
				IsBlocking: true,
				Category:   "suspicious_extension",
				Pattern:    ".sh",
			},
			maxAllowed:  runnertypes.RiskLevelLow,
			expectError: true,
			description: "Suspicious extension should create specific error",
		},
		{
			name: "generic risk level error",
			evaluation: &RiskEvaluation{
				Level:      runnertypes.RiskLevelHigh,
				IsBlocking: true,
				Category:   "absolute_path",
				Reason:     "Absolute path outside working directory",
			},
			maxAllowed:  runnertypes.RiskLevelLow,
			expectError: true,
			description: "Generic risk should create generic error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.CreateValidationError(tt.evaluation, tt.maxAllowed)

			if tt.expectError {
				require.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}

func TestRiskEvaluator_WithCustomSecurityConfig(t *testing.T) {
	// Test with custom security configuration
	customConfig := security.DefaultConfig()
	customConfig.OutputCriticalPathPatterns = append(customConfig.OutputCriticalPathPatterns, "/custom/critical/")
	customConfig.SuspiciousExtensions = append(customConfig.SuspiciousExtensions, ".custom")

	evaluator := NewRiskEvaluator(customConfig)

	t.Run("custom critical pattern", func(t *testing.T) {
		evaluation := evaluator.EvaluateOutputRisk("/custom/critical/file.txt", "/tmp")

		require.Equal(t, runnertypes.RiskLevelCritical, evaluation.Level)
		require.Equal(t, "critical_system_directory", evaluation.Category)
		require.Equal(t, "/custom/critical/", evaluation.Pattern)
	})

	t.Run("custom suspicious extension", func(t *testing.T) {
		evaluation := evaluator.EvaluateOutputRisk("file.custom", "/tmp")

		require.Equal(t, runnertypes.RiskLevelHigh, evaluation.Level)
		require.Equal(t, "suspicious_extension", evaluation.Category)
		require.Equal(t, ".custom", evaluation.Pattern)
	})
}
