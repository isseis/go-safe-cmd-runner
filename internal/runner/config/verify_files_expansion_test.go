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
			Vars:        map[string]any{"base_dir": "/opt/my app", "file_name": "test-file_v1.0.sh"},
			VerifyFiles: []string{"%{base_dir}/%{file_name}"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/my app/test-file_v1.0.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle paths with spaces and special characters")
	})

	t.Run("GlobalVerifyFiles_WithDashes", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"base_dir": "/opt/app", "sub_dir": "sub-dir_v2.0"},
			VerifyFiles: []string{"%{base_dir}/%{sub_dir}/script.sh"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/app/sub-dir_v2.0/script.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle paths with dashes and underscores")
	})

	t.Run("GroupVerifyFiles_WithSpecialChars", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"root": "/opt/my-app"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name:        "test_group",
			Vars:        map[string]any{"sub_dir": "test dir v1.0"},
			VerifyFiles: []string{"%{root}/%{sub_dir}/verify.sh"},
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
			Vars:        map[string]any{"root": "/opt", "app_name": "myapp", "app_dir": "%{root}/%{app_name}"},
			VerifyFiles: []string{"%{app_dir}/verify.sh"},
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
				"root":          "/opt",
				"app_name":      "myapp",
				"version":       "v1.0",
				"app_dir":       "%{root}/%{app_name}",
				"versioned_dir": "%{app_dir}/%{version}",
			},
			VerifyFiles: []string{"%{versioned_dir}/scripts/verify.sh"},
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		require.Len(t, runtime.ExpandedVerifyFiles, 1)
		assert.Equal(t, "/opt/myapp/v1.0/scripts/verify.sh", runtime.ExpandedVerifyFiles[0],
			"verify_files should handle deeply nested variable references")
	})

	t.Run("GroupVerifyFiles_DeeplyNestedReferences", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"root": "/opt", "app_name": "myapp", "app_dir": "%{root}/%{app_name}"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name:        "test_group",
			Vars:        map[string]any{"subdir": "scripts", "full_path": "%{app_dir}/%{subdir}"},
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
			Vars:        map[string]any{"existing_var": "/opt"},
			VerifyFiles: []string{"%{undefined_var}/script.sh"},
		}

		_, err := config.ExpandGlobal(spec)
		require.Error(t, err, "Should fail when verify_files references undefined variable")
		assert.Contains(t, err.Error(), "undefined_var", "Error should mention the undefined variable name")
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
			Vars: map[string]any{"valid_dir": "/opt"},
			VerifyFiles: []string{
				"%{valid_dir}/good.sh",
				"%{invalid_var}/bad.sh",
			},
		}

		_, err := config.ExpandGlobal(spec)
		require.Error(t, err, "Should fail on first invalid verify_files entry")
		assert.Contains(t, err.Error(), "invalid_var", "Error should mention the first invalid variable")
	})

	t.Run("GroupVerifyFiles_UndefinedVariable", func(t *testing.T) {
		globalSpec := &runnertypes.GlobalSpec{
			Vars: map[string]any{"global_var": "/opt"},
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
			Vars:        map[string]any{"var1": "/opt"},
			VerifyFiles: nil,
		}

		runtime, err := config.ExpandGlobal(spec)
		require.NoError(t, err)
		assert.Empty(t, runtime.ExpandedVerifyFiles, "Empty VerifyFiles should result in empty ExpandedVerifyFiles")
	})

	t.Run("EmptyVerifyFilesList", func(t *testing.T) {
		spec := &runnertypes.GlobalSpec{
			Vars:        map[string]any{"var1": "/opt"},
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
			Vars: map[string]any{"dir1": "/opt/app1", "dir2": "/opt/app2"},
			VerifyFiles: []string{
				"%{dir1}/verify1.sh",
				"%{dir2}/verify2.sh",
				"%{dir1}/verify3.sh",
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
			Vars: map[string]any{"root": "/opt"},
		}
		globalRuntime, err := config.ExpandGlobal(globalSpec)
		require.NoError(t, err)

		groupSpec := &runnertypes.GroupSpec{
			Name: "test_group",
			Vars: map[string]any{"app": "myapp"},
			VerifyFiles: []string{
				"%{root}/%{app}/check1.sh",
				"%{root}/%{app}/check2.sh",
			},
		}

		groupRuntime, err := config.ExpandGroup(groupSpec, globalRuntime)
		require.NoError(t, err)
		require.Len(t, groupRuntime.ExpandedVerifyFiles, 2)
		assert.Equal(t, "/opt/myapp/check1.sh", groupRuntime.ExpandedVerifyFiles[0])
		assert.Equal(t, "/opt/myapp/check2.sh", groupRuntime.ExpandedVerifyFiles[1])
	})
}
