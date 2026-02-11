//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateRiskLevelDebug verifies that the risk_level from a template is
// correctly applied to a command.
func TestTemplateRiskLevelDebug(t *testing.T) {
	toml := `version = "1.0"

[command_templates.s3_upload]
cmd = "/usr/bin/echo"
args = ["s3", "cp", "${src}", "${dst}"]
risk_level = "medium"

[[groups]]
name = "test"

[[groups.commands]]
name = "upload_file"
template = "s3_upload"

[groups.commands.params]
src = "/tmp/test.txt"
dst = "s3://my-bucket/test.txt"
`

	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(toml))
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

	t.Logf("Command Cmd: %s", runtimeCmd.Cmd())
	t.Logf("Command Args: %v", runtimeCmd.Args())
	t.Logf("Spec Cmd: %s", runtimeCmd.Spec.Cmd)
	if runtimeCmd.Spec.RiskLevel != nil {
		t.Logf("Spec RiskLevel: %s", *runtimeCmd.Spec.RiskLevel)
	} else {
		t.Logf("Spec RiskLevel: nil")
	}

	// Verify that the cmd from the template is correctly expanded.
	assert.Equal(t, "/usr/bin/echo", runtimeCmd.Spec.Cmd)

	// Verify that the args from the template are correctly expanded.
	assert.Equal(t, []string{"s3", "cp", "/tmp/test.txt", "s3://my-bucket/test.txt"}, runtimeCmd.Spec.Args)

	// Verify that the risk_level from the template is correctly applied.
	require.NotNil(t, runtimeCmd.Spec.RiskLevel)
	assert.Equal(t, "medium", *runtimeCmd.Spec.RiskLevel)

	riskLevel, err := runtimeCmd.GetRiskLevel()
	require.NoError(t, err)
	t.Logf("Parsed RiskLevel: %v", riskLevel)
}

// TestTemplateRiskLevelOverride verifies that when risk_level is explicitly
// specified in a command, it overrides the value from the template.
func TestTemplateRiskLevelOverride(t *testing.T) {
	toml := `version = "1.0"

[command_templates.s3_upload]
cmd = "/usr/bin/echo"
args = ["s3", "cp", "${src}", "${dst}"]
risk_level = "medium"

[[groups]]
name = "test"

[[groups.commands]]
name = "upload_file"
template = "s3_upload"
risk_level = "high"

[groups.commands.params]
src = "/tmp/test.txt"
dst = "s3://my-bucket/test.txt"
`

	loader := NewLoaderForTest()
	cfg, err := loader.LoadConfigForTest([]byte(toml))
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

	// Verify that the explicitly specified risk_level in the command overrides the template value.
	require.NotNil(t, runtimeCmd.Spec.RiskLevel)
	assert.Equal(t, "high", *runtimeCmd.Spec.RiskLevel)

	riskLevel, err := runtimeCmd.GetRiskLevel()
	require.NoError(t, err)
	t.Logf("Parsed RiskLevel: %v", riskLevel)
}
