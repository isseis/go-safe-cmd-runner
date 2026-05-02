package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeSlackAllowedHost tests normalizeSlackAllowedHost input validation and normalization.
func TestNormalizeSlackAllowedHost(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantErr  bool
	}{
		{
			name:     "empty string disables Slack",
			input:    "",
			wantHost: "",
			wantErr:  false,
		},
		{
			name:     "valid plain hostname",
			input:    "hooks.slack.com",
			wantHost: "hooks.slack.com",
			wantErr:  false,
		},
		{
			name:     "IPv6 bracket notation normalized",
			input:    "[::1]",
			wantHost: "::1",
			wantErr:  false,
		},
		{
			name:    "port number rejected",
			input:   "hooks.slack.com:443",
			wantErr: true,
		},
		{
			name:    "path component rejected",
			input:   "hooks.slack.com/path",
			wantErr: true,
		},
		{
			name:    "scheme-included value rejected",
			input:   "https://hooks.slack.com",
			wantErr: true,
		},
		{
			name:    "leading whitespace rejected",
			input:   " hooks.slack.com",
			wantErr: true,
		},
		{
			name:    "trailing whitespace rejected",
			input:   "hooks.slack.com ",
			wantErr: true,
		},
		{
			name:    "userinfo prefix rejected",
			input:   "user@hooks.slack.com",
			wantErr: true,
		},
		{
			name:    "userinfo used as host spoofing rejected",
			input:   "hooks.slack.com@evil.com",
			wantErr: true,
		},
		{
			name:    "query string after path separator rejected",
			input:   "hooks.slack.com/?q=1",
			wantErr: true,
		},
		{
			name:    "fragment after path separator rejected",
			input:   "hooks.slack.com/#frag",
			wantErr: true,
		},
		{
			name:    "label with leading hyphen rejected",
			input:   "-hooks.slack.com",
			wantErr: true,
		},
		{
			name:    "label with trailing hyphen rejected",
			input:   "hooks-.slack.com",
			wantErr: true,
		},
		{
			name:     "uppercase hostname normalized to lowercase",
			input:    "Hooks.Slack.Com",
			wantHost: "hooks.slack.com",
			wantErr:  false,
		},
		{
			name:     "IPv4 literal accepted",
			input:    "192.0.2.1",
			wantHost: "192.0.2.1",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSlackAllowedHost(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidSlackAllowedHost)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantHost, got)
			}
		})
	}
}

// TestBootstrapCommandEnvExpansionIntegration verifies the complete expansion pipeline
// from Global.EnvVars -> Group.EnvVars -> Command.EnvVars/Cmd/Args during bootstrap process.
// This test ensures that:
// 1. Global.EnvVars is expanded correctly
// 2. Group.EnvVars can reference Global.EnvVars
// 3. Command.EnvVars can reference Global.EnvVars and Group.EnvVars
// 4. Command.Cmd can reference Group.EnvVars
// 5. Command.Args can reference Command.EnvVars
func TestBootstrapCommandEnvExpansionIntegration(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := commontesting.SafeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Path to test config file (relative to internal/runner/config/testdata/)
	configPath := filepath.Join("..", "..", "..", "internal", "runner", "config", "testdata", "command_env_references_global_group.toml")
	configPath, err = filepath.Abs(configPath)
	require.NoError(t, err)

	// Record hash for the config file using filevalidator
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	require.NoError(t, err)
	_, _, err = validator.SaveRecord(configPath, false)
	require.NoError(t, err)

	// Load and prepare config (returns ConfigSpec)
	cfg, err := LoadAndPrepareConfig(verificationManager, configPath, "test-run-001")
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Expand global spec to runtime
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)
	require.NotNil(t, runtimeGlobal)

	// Verify Global.EnvVars expansion
	expectedGlobalEnv := map[string]string{
		"BASE_DIR": "/opt",
	}
	assert.Equal(t, expectedGlobalEnv, runtimeGlobal.ExpandedEnv, "Global.EnvVars should be expanded correctly")

	// Verify groups
	require.Len(t, cfg.Groups, 1, "Should have exactly one group")

	// Find app_group
	var appGroupSpec *runnertypes.GroupSpec
	for i := range cfg.Groups {
		if cfg.Groups[i].Name == "app_group" {
			appGroupSpec = &cfg.Groups[i]
			break
		}
	}
	require.NotNil(t, appGroupSpec, "app_group should exist")

	// Expand group spec to runtime
	runtimeGroup, err := config.ExpandGroup(appGroupSpec, runtimeGlobal)
	require.NoError(t, err)
	require.NotNil(t, runtimeGroup)

	// Verify Group.EnvVars expansion (references Global.EnvVars)
	expectedGroupEnv := map[string]string{
		"APP_DIR": "/opt/myapp",
	}
	assert.Equal(t, expectedGroupEnv, runtimeGroup.ExpandedEnv, "Group.EnvVars should reference Global.EnvVars correctly")

	// Verify commands
	require.Len(t, appGroupSpec.Commands, 1, "app_group should have exactly one command")
	cmdSpec := &appGroupSpec.Commands[0]
	require.Equal(t, "run_app", cmdSpec.Name)

	// Expand command spec to runtime
	runtimeCmd, err := config.ExpandCommand(cmdSpec, nil, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
	require.NoError(t, err)
	require.NotNil(t, runtimeCmd)

	// Verify Command.EnvVars expansion (uses internal variables)
	// Note: In new system, each level's ExpandedEnv contains only that level's env field values
	// The final process environment is built by BuildProcessEnvironment which merges all levels
	assert.Equal(t, "/opt/myapp/logs", runtimeCmd.ExpandedEnv["LOG_DIR"], "Command.EnvVars variable LOG_DIR should be expanded correctly")

	// BASE_DIR and APP_DIR are in Global/Group ExpandedEnv respectively, not merged into Command.ExpandedEnv
	// They will be merged at execution time by BuildProcessEnvironment
	assert.Equal(t, "/opt", runtimeGlobal.ExpandedEnv["BASE_DIR"], "Global.EnvVars variable BASE_DIR should be in Global.ExpandedEnv")
	assert.Equal(t, "/opt/myapp", runtimeGroup.ExpandedEnv["APP_DIR"], "Group.EnvVars variable APP_DIR should be in Group.ExpandedEnv")

	// Verify Command.Cmd expansion (references internal variables)
	expectedCmd := "/opt/myapp/bin/server"
	assert.Equal(t, expectedCmd, runtimeCmd.ExpandedCmd, "Command.Cmd should reference internal variables correctly")

	// Verify Command.Args expansion (references internal variables)
	expectedArgs := []string{"--log", "/opt/myapp/logs/app.log"}
	assert.Equal(t, expectedArgs, runtimeCmd.ExpandedArgs, "Command.Args should reference internal variables correctly")

	// Also verify that raw values are preserved for debugging/auditing
	assert.Equal(t, "%{app_dir}/bin/server", cmdSpec.Cmd, "Raw Cmd should be preserved")
	assert.Equal(t, []string{"--log", "%{log_dir}/app.log"}, cmdSpec.Args, "Raw Args should be preserved")
	assert.Equal(t, []string{"LOG_DIR=%{log_dir}"}, cmdSpec.EnvVars, "Raw Env should be preserved")
}

// TestLoadAndPrepareConfig_MissingConfigFile verifies error handling for missing config files
func TestLoadAndPrepareConfig_MissingConfigFile(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := commontesting.SafeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Try to load non-existent config file
	nonExistentPath := filepath.Join(tempDir, "nonexistent.toml")
	cfg, err := LoadAndPrepareConfig(verificationManager, nonExistentPath, "test-run-002")

	// Should return error
	assert.Error(t, err, "Should return error for non-existent config file")
	assert.Nil(t, cfg, "Config should be nil on error")
}

// TestLoadAndPrepareConfig_EmptyConfigPath verifies error handling for empty config path
func TestLoadAndPrepareConfig_EmptyConfigPath(t *testing.T) {
	// Setup: Create temporary directory for hash storage
	tempDir := commontesting.SafeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755)
	require.NoError(t, err)

	// Create verification manager
	verificationManager, err := verification.NewManagerForTest(hashDir)
	require.NoError(t, err)

	// Try to load with empty config path
	cfg, err := LoadAndPrepareConfig(verificationManager, "", "test-run-003")

	// Should return error
	assert.Error(t, err, "Should return error for empty config path")
	assert.Nil(t, cfg, "Config should be nil on error")
	assert.Contains(t, err.Error(), "Config file path is required", "Error message should indicate required path")
}
