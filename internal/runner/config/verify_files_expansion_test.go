package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyFilesExpansion_SpecialCharacters tests verify_files expansion with special characters
func TestVerifyFilesExpansion_SpecialCharacters(t *testing.T) {
	t.Run("GlobalVerifyFiles_WithSpaces", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Base_dir": "/opt/my app", "File_name": "test-file_v1.0.sh"},
			VerifyFiles: []string{"%{Base_dir}/%{File_name}"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/my app/test-file_v1.0.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle paths with spaces and special characters")
	})

	t.Run("GlobalVerifyFiles_WithDashes", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Base_dir": "/opt/app", "Sub_dir": "sub-dir_v2.0"},
			VerifyFiles: []string{"%{Base_dir}/%{Sub_dir}/script.sh"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/app/sub-dir_v2.0/script.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle paths with dashes and underscores")
	})

	t.Run("GroupVerifyFiles_WithSpecialChars", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Root": "/opt/my-app"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name:        "test_group",
			Vars:        map[string]any{"sub_dir": "test dir v1.0"},
			VerifyFiles: []string{"%{Root}/%{sub_dir}/verify.sh"},
		}

		groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err)
		require.Len(t, groupRuntime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/my-app/test dir v1.0/verify.sh", groupRuntime.ExpandedVerifyFiles[0],
			"Group verify_files should handle paths with spaces and special characters")
	})
}

// TestVerifyFilesExpansion_NestedReferences tests verify_files with nested variable references
func TestVerifyFilesExpansion_NestedReferences(t *testing.T) {
	t.Run("GlobalVerifyFiles_NestedReferences", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Root": "/opt", "App_name": "myapp", "App_dir": "%{Root}/%{App_name}"},
			VerifyFiles: []string{"%{App_dir}/verify.sh"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/verify.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle nested variable references (root -> app_name -> app_dir)")
	})

	t.Run("GlobalVerifyFiles_DeeplyNestedReferences", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars: map[string]any{
				"Root":          "/opt",
				"App_name":      "myapp",
				"Version":       "v1.0",
				"App_dir":       "%{Root}/%{App_name}",
				"Versioned_dir": "%{App_dir}/%{Version}",
			},
			VerifyFiles: []string{"%{Versioned_dir}/scripts/verify.sh"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/v1.0/scripts/verify.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle deeply nested variable references")
	})

	t.Run("GroupVerifyFiles_DeeplyNestedReferences", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Root": "/opt", "App_name": "myapp", "App_dir": "%{Root}/%{App_name}"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name:        "test_group",
			Vars:        map[string]any{"subdir": "scripts", "full_path": "%{App_dir}/%{subdir}"},
			VerifyFiles: []string{"%{full_path}/check.sh"},
		}

		groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err)
		require.Len(t, groupRuntime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/scripts/check.sh", groupRuntime.ExpandedVerifyFiles[0],
			"Group verify_files should handle deeply nested references (root -> app_name -> app_dir -> subdir -> full_path)")
	})
}

// TestVerifyFilesExpansion_ErrorHandling tests error handling for invalid verify_files expansion
func TestVerifyFilesExpansion_ErrorHandling(t *testing.T) {
	t.Run("UndefinedVariable", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Existing_var": "/opt"},
			VerifyFiles: []string{"%{Undefined_var}/script.sh"},
		}

		_, err := config.ExpandGlobal(spec)
		require.Error(t, err, "Should fail when verify_files references undefined variable")
		assert.Contains(t, err.Error(), "Undefined_var", "Error should mention the undefined variable name")
		assert.Contains(t, err.Error(), "undefined variable", "Error should indicate it's an undefined variable error")
	})

	t.Run("EmptyVariableName", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			VerifyFiles: []string{"%{}/script.sh"},
		}

		_, err := config.ExpandGlobal(spec)
		require.Error(t, err, "Should fail when verify_files has empty variable name")
		assert.Contains(t, err.Error(), "variable name cannot be empty", "Error should mention empty variable name")
	})

	t.Run("MultipleVerifyFilesWithMixedErrors", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Valid_dir": "/opt"},
			VerifyFiles: []string{
				"%{Valid_dir}/good.sh",
				"%{Invalid_var}/bad.sh",
			},
		}

		_, err := config.ExpandGlobal(spec)
		require.Error(t, err, "Should fail on first invalid verify_files entry")
		assert.Contains(t, err.Error(), "Invalid_var", "Error should mention the first invalid variable")
	})

	t.Run("GroupVerifyFiles_UndefinedVariable", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Global_var": "/opt"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name:        "test_group",
			VerifyFiles: []string{"%{undefined_group_var}/script.sh"},
		}

		_, err = config.ExpandGroup(groupSpec, globalRuntime)
		require.Error(t, err, "Should fail when group verify_files references undefined variable")
		assert.Contains(t, err.Error(), "undefined_group_var")
	})
}

// TestVerifyFilesExpansion_EmptyAndNoFiles tests behavior with no verify_files entries
func TestVerifyFilesExpansion_EmptyAndNoFiles(t *testing.T) {
	t.Run("NoVerifyFiles", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Var1": "/opt"},
			VerifyFiles: nil,
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		assert.Empty(t, runtime.ExpandedVerifyFiles, "Empty VerifyFiles should result in empty ExpandedVerifyFiles")
	})

	t.Run("EmptyVerifyFilesList", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"Var1": "/opt"},
			VerifyFiles: []string{},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		assert.Empty(t, runtime.ExpandedVerifyFiles, "Empty VerifyFiles list should result in empty ExpandedVerifyFiles")
	})
}

// TestVerifyFilesExpansion_MultipleFiles tests expansion of multiple verify_files entries
func TestVerifyFilesExpansion_MultipleFiles(t *testing.T) {
	t.Run("GlobalMultipleVerifyFiles", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Dir1": "/opt/app1", "Dir2": "/opt/app2"},
			VerifyFiles: []string{
				"%{Dir1}/verify1.sh",
				"%{Dir2}/verify2.sh",
				"%{Dir1}/verify3.sh",
			},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 3)
		assert.Equal(t, "/opt/app1/verify1.sh", runtime.ExpandedVerifyFiles[0])
		assert.Equal(t, "/opt/app2/verify2.sh", runtime.ExpandedVerifyFiles[1])
		assert.Equal(t, "/opt/app1/verify3.sh", runtime.ExpandedVerifyFiles[2])
	})

	t.Run("GroupMultipleVerifyFiles", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"Root": "/opt"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name: "test_group",
			Vars: map[string]any{"app": "myapp"},
			VerifyFiles: []string{
				"%{Root}/%{app}/check1.sh",
				"%{Root}/%{app}/check2.sh",
			},
		}

		groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err)
		require.Len(t, groupRuntime.ExpandedVerifyFiles, 2)
		assert.Equal(t, "/opt/myapp/check1.sh", groupRuntime.ExpandedVerifyFiles[0])
		assert.Equal(t, "/opt/myapp/check2.sh", groupRuntime.ExpandedVerifyFiles[1])
	})
}
