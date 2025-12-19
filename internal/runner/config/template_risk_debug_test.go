//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateRiskLevelDebug は、テンプレートのrisk_levelが正しく適用されることを確認するデバッグテスト
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

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
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

	// テンプレートのcmdが正しく展開されていることを確認
	assert.Equal(t, "/usr/bin/echo", runtimeCmd.Spec.Cmd)

	// テンプレートのargsが正しく展開されていることを確認
	assert.Equal(t, []string{"s3", "cp", "/tmp/test.txt", "s3://my-bucket/test.txt"}, runtimeCmd.Spec.Args)

	// テンプレートのrisk_levelが正しく適用されていることを確認
	require.NotNil(t, runtimeCmd.Spec.RiskLevel)
	assert.Equal(t, "medium", *runtimeCmd.Spec.RiskLevel)

	riskLevel, err := runtimeCmd.GetRiskLevel()
	require.NoError(t, err)
	t.Logf("Parsed RiskLevel: %v", riskLevel)
}

// TestTemplateRiskLevelOverride は、コマンドでrisk_levelを明示的に指定した場合に
// テンプレートの値を上書きすることを確認するテスト
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

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
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

	// コマンドで明示的に指定したrisk_levelがテンプレートの値を上書きすることを確認
	require.NotNil(t, runtimeCmd.Spec.RiskLevel)
	assert.Equal(t, "high", *runtimeCmd.Spec.RiskLevel)

	riskLevel, err := runtimeCmd.GetRiskLevel()
	require.NoError(t, err)
	t.Logf("Parsed RiskLevel: %v", riskLevel)
}
