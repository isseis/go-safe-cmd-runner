//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateExecutionSettingsDefaultHandling tests that execution settings
// (timeout, output_size_limit, risk_level) correctly handle default values
// when using templates.
//
// This test ensures that:
// 1. For pointer fields (timeout, output_size_limit): nil means inherit, explicit values (including 0) override
// 2. For string fields with defaults (risk_level): explicit values override, otherwise use template or default
func TestTemplateExecutionSettingsDefaultHandling(t *testing.T) {
	tests := []struct {
		name                     string
		toml                     string
		expectedTimeout          *int32
		expectedOutputSizeLimit  *int64
		expectedRiskLevel        *string
		expectedEffectiveTimeout int32
	}{
		{
			name: "template with all execution settings, command uses defaults (inherits from template)",
			toml: `
version = "1.0"

[command_templates.full_settings]
cmd = "echo"
args = ["${msg}"]
timeout = 30
output_size_limit = 2048
risk_level = "medium"

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "full_settings"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          commontesting.Int32Ptr(30),
			expectedOutputSizeLimit:  commontesting.Int64Ptr(2048),
			expectedRiskLevel:        runnertypes.StringPtr("medium"),
			expectedEffectiveTimeout: 30,
		},
		{
			name: "template with timeout=0 (unlimited), command inherits",
			toml: `
version = "1.0"

[command_templates.unlimited]
cmd = "echo"
args = ["${msg}"]
timeout = 0

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "unlimited"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          commontesting.Int32Ptr(0),
			expectedOutputSizeLimit:  nil,
			expectedRiskLevel:        nil, // Neither template nor command set it, so nil (default from GetRiskLevel())
			expectedEffectiveTimeout: 0,
		},
		{
			name: "command sets timeout=0 overriding template's timeout=30",
			toml: `
version = "1.0"

[command_templates.with_timeout]
cmd = "echo"
args = ["${msg}"]
timeout = 30

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "with_timeout"
timeout = 0
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          commontesting.Int32Ptr(0),
			expectedOutputSizeLimit:  nil,
			expectedRiskLevel:        nil, // Neither template nor command set it, so nil (default from GetRiskLevel())
			expectedEffectiveTimeout: 0,
		},
		{
			name: "command explicitly sets risk_level=low overriding template's medium",
			toml: `
version = "1.0"

[command_templates.medium_risk]
cmd = "echo"
args = ["${msg}"]
risk_level = "medium"

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "medium_risk"
risk_level = "low"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:         nil,
			expectedOutputSizeLimit: nil,
			// With pointer type, we can now distinguish between nil (not set) and explicit value
			// When user explicitly sets risk_level = "low" in TOML, it overrides template's value
			expectedRiskLevel:        runnertypes.StringPtr("low"), // Command's explicit value overrides template
			expectedEffectiveTimeout: -1,                           // unset
		},
		{
			name: "template has no execution settings, command sets risk_level explicitly",
			toml: `
version = "1.0"

[command_templates.minimal]
cmd = "echo"
args = ["${msg}"]

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "minimal"
risk_level = "high"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          nil,
			expectedOutputSizeLimit:  nil,
			expectedRiskLevel:        runnertypes.StringPtr("high"),
			expectedEffectiveTimeout: -1, // unset
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoaderForTest()
			cfg, err := loader.LoadConfigForTest([]byte(tt.toml))
			require.NoError(t, err)

			runtimeGlobal, err := ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
			require.NoError(t, err)

			runtimeCmd, err := ExpandCommand(
				&cfg.Groups[0].Commands[0],
				cfg.CommandTemplates,
				runtimeGroup,
				runtimeGlobal,
				common.NewUnsetTimeout(),
				commontesting.NewUnsetOutputSizeLimit(),
			)
			require.NoError(t, err)

			// Check timeout
			if tt.expectedTimeout != nil {
				require.NotNil(t, runtimeCmd.Spec.Timeout, "expected timeout to be set")
				assert.Equal(t, *tt.expectedTimeout, *runtimeCmd.Spec.Timeout, "timeout mismatch")
			} else {
				assert.Nil(t, runtimeCmd.Spec.Timeout, "expected timeout to be nil")
			}

			// Check output_size_limit
			if tt.expectedOutputSizeLimit != nil {
				require.NotNil(t, runtimeCmd.Spec.OutputSizeLimit, "expected output_size_limit to be set")
				assert.Equal(t, *tt.expectedOutputSizeLimit, *runtimeCmd.Spec.OutputSizeLimit, "output_size_limit mismatch")
			} else {
				assert.Nil(t, runtimeCmd.Spec.OutputSizeLimit, "expected output_size_limit to be nil")
			}

			// Check risk_level
			if tt.expectedRiskLevel != nil {
				require.NotNil(t, runtimeCmd.Spec.RiskLevel, "expected risk_level to be set")
				assert.Equal(t, *tt.expectedRiskLevel, *runtimeCmd.Spec.RiskLevel, "risk_level mismatch")
			} else {
				assert.Nil(t, runtimeCmd.Spec.RiskLevel, "expected risk_level to be nil")
			}

			// Check effective timeout
			if tt.expectedEffectiveTimeout >= 0 {
				assert.Equal(t, tt.expectedEffectiveTimeout, runtimeCmd.EffectiveTimeout, "effective timeout mismatch")
			}
		})
	}
}
