//go:build test

package config_test

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/variable"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndToEndExpansion_GlobalVariablesInTemplates(t *testing.T) {
	// Simulate a complete config with global vars and templates
	spec := &runnertypes.GlobalSpec{
		Vars: map[string]any{
			"AwsPath":   "/usr/bin/aws",
			"AwsRegion": "us-west-2",
		},
	}

	// Expand global
	globalRuntime, err := config.ExpandGlobal(spec)
	require.NoError(t, err)

	// Verify global vars are expanded
	assert.Equal(t, "/usr/bin/aws", globalRuntime.ExpandedVars["AwsPath"])
	assert.Equal(t, "us-west-2", globalRuntime.ExpandedVars["AwsRegion"])

	// Create template
	templates := map[string]runnertypes.CommandTemplate{
		"s3_sync": {
			Cmd:  "%{AwsPath}",
			Args: []string{"--region", "%{AwsRegion}", "s3", "sync", "${src}", "${dst}"},
		},
	}

	// Validate template
	err = config.ValidateAllTemplates(templates, globalRuntime.ExpandedVars)
	require.NoError(t, err)

	// Create command spec using the template
	cmdSpec := &runnertypes.CommandSpec{
		Name:     "sync_data",
		Template: "s3_sync",
		Params: map[string]any{
			"src": "/data",
			"dst": "s3://bucket",
		},
	}

	// Expand command
	groupSpec := &runnertypes.GroupSpec{
		Name: "test-group",
	}
	runtimeGroup, err := config.ExpandGroup(groupSpec, globalRuntime)
	require.NoError(t, err)

	runtimeCmd, err := config.ExpandCommand(
		cmdSpec,
		templates,
		runtimeGroup,
		globalRuntime,
		common.NewUnsetTimeout(),
		commontestutil.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)

	// Verify final expansion
	assert.Equal(t, "/usr/bin/aws", runtimeCmd.ExpandedCmd)
	assert.Equal(t, []string{"--region", "us-west-2", "s3", "sync", "/data", "s3://bucket"}, runtimeCmd.ExpandedArgs)
}

func TestEndToEndExpansion_LocalVariablesInParams(t *testing.T) {
	// Global vars
	globalSpec := &runnertypes.GlobalSpec{
		Vars: map[string]any{
			"AwsPath": "/usr/bin/aws",
		},
	}

	globalRuntime, err := config.ExpandGlobal(globalSpec)
	require.NoError(t, err)

	// Group with local variables
	groupSpec := &runnertypes.GroupSpec{
		Name: "backup",
		Vars: map[string]any{
			"data_dir":      "/data/prod",
			"backup_bucket": "s3://prod-backup",
		},
	}

	runtimeGroup, err := config.ExpandGroup(groupSpec, globalRuntime)
	require.NoError(t, err)

	// Verify local vars are expanded
	assert.Equal(t, "/data/prod", runtimeGroup.ExpandedVars["data_dir"])
	assert.Equal(t, "s3://prod-backup", runtimeGroup.ExpandedVars["backup_bucket"])

	// Template
	templates := map[string]runnertypes.CommandTemplate{
		"s3_sync": {
			Cmd:  "%{AwsPath}",
			Args: []string{"s3", "sync", "${src}", "${dst}"},
		},
	}

	// Command using local variables in params
	cmdSpec := &runnertypes.CommandSpec{
		Name:     "sync_data",
		Template: "s3_sync",
		Params: map[string]any{
			"src": "%{data_dir}",      // Local variable reference
			"dst": "%{backup_bucket}", // Local variable reference
		},
	}

	runtimeCmd, err := config.ExpandCommand(
		cmdSpec,
		templates,
		runtimeGroup,
		globalRuntime,
		common.NewUnsetTimeout(),
		commontestutil.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)

	// Verify expansion
	assert.Equal(t, "/usr/bin/aws", runtimeCmd.ExpandedCmd)
	assert.Equal(t, []string{"s3", "sync", "/data/prod", "s3://prod-backup"}, runtimeCmd.ExpandedArgs)
}

func TestEndToEndExpansion_ScopeMismatchErrors(t *testing.T) {
	t.Run("lowercase in global.vars", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars: map[string]any{
				"aws_path": "/usr/bin/aws", // Should be AwsPath
			},
		}

		_, err := config.ExpandGlobal(spec)
		mismatch, ok := errors.AsType[*variable.ErrScopeMismatch](err)
		require.True(t, ok, "a lowercase name in [global.vars] must be rejected as a scope mismatch, got: %v", err)
		assert.Equal(t, variable.ScopeGlobal, mismatch.ExpectedScope)
		assert.Equal(t, "aws_path", mismatch.Name)
	})

	t.Run("uppercase in groups.vars", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name: "test",
			Vars: map[string]any{
				"DataDir": "/data", // Should be data_dir
			},
		}

		_, err = config.ExpandGroup(groupSpec, globalRuntime)
		mismatch, ok := errors.AsType[*variable.ErrScopeMismatch](err)
		require.True(t, ok, "an uppercase name in [[groups]].vars must be rejected as a scope mismatch, got: %v", err)
		assert.Equal(t, variable.ScopeLocal, mismatch.ExpectedScope)
		assert.Equal(t, "DataDir", mismatch.Name)
	})
}

func TestEndToEndExpansion_TemplateValidationErrors(t *testing.T) {
	t.Run("local variable in template cmd", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{
				"AwsPath": "/usr/bin/aws",
			},
		}

		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		templates := map[string]runnertypes.CommandTemplate{
			"bad_template": {
				Cmd:  "%{local_var}", // Local variable not allowed
				Args: []string{"test"},
			},
		}

		err = config.ValidateAllTemplates(templates, globalRuntime.ExpandedVars)
		local, ok := errors.AsType[*config.ErrLocalVariableInTemplate](err)
		require.True(t, ok, "a local variable in a template must be rejected as such, got: %v", err)
		assert.Equal(t, "local_var", local.VariableName)
	})

	t.Run("undefined global variable in template", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{
				"AwsPath": "/usr/bin/aws",
			},
		}

		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		templates := map[string]runnertypes.CommandTemplate{
			"bad_template": {
				Cmd:  "%{UndefinedVar}", // Not defined in global.vars
				Args: []string{"test"},
			},
		}

		err = config.ValidateAllTemplates(templates, globalRuntime.ExpandedVars)
		undefined, ok := errors.AsType[*config.ErrUndefinedGlobalVariableInTemplate](err)
		require.True(t, ok, "a template referencing an undefined global must be rejected as such, got: %v", err)
		assert.Equal(t, "UndefinedVar", undefined.VariableName)
	})

	t.Run("local variable in template args", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		templates := map[string]runnertypes.CommandTemplate{
			"bad_template": {
				Cmd:  "echo",
				Args: []string{"%{local_var}"}, // Local variable not allowed
			},
		}

		err = config.ValidateAllTemplates(templates, globalRuntime.ExpandedVars)
		local, ok := errors.AsType[*config.ErrLocalVariableInTemplate](err)
		require.True(t, ok, "a local variable in a template must be rejected as such, got: %v", err)
		assert.Equal(t, "local_var", local.VariableName)
	})

	t.Run("local variable in template workdir", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		templates := map[string]runnertypes.CommandTemplate{
			"bad_template": {
				Cmd:     "echo",
				WorkDir: new("%{local_dir}"), // Local variable not allowed
			},
		}

		err = config.ValidateAllTemplates(templates, globalRuntime.ExpandedVars)
		local, ok := errors.AsType[*config.ErrLocalVariableInTemplate](err)
		require.True(t, ok, "a local variable in a template workdir must be rejected as such, got: %v", err)
		assert.Equal(t, "local_dir", local.VariableName)
	})
}
