package variable

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariableRegistry_RegisterGlobal(t *testing.T) {
	t.Run("valid_global_variable", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		// Verify it can be resolved
		value, err := registry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)
	})

	t.Run("valid_global_variable_uppercase_snake", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AWS_PATH", "/usr/bin/aws")
		require.NoError(t, err)

		value, err := registry.Resolve("AWS_PATH")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)
	})

	t.Run("multiple_global_variables", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		err = registry.RegisterGlobal("DataDir", "/var/data")
		require.NoError(t, err)

		// Verify both can be resolved
		value1, err := registry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value1)

		value2, err := registry.Resolve("DataDir")
		require.NoError(t, err)
		assert.Equal(t, "/var/data", value2)
	})

	t.Run("reject_lowercase_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("awsPath", "/usr/bin/aws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be global")
	})

	t.Run("reject_underscore_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("_internal", "/var/internal")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be global")
	})

	t.Run("reject_reserved_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("__reserved", "value")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
	})

	t.Run("reject_invalid_characters", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("Aws-Path", "/usr/bin/aws")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}

func TestVariableRegistry_WithLocals(t *testing.T) {
	t.Run("valid_local_variables", func(t *testing.T) {
		registry := NewVariableRegistry()

		// Register global variable
		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		// Add local variables
		childRegistry, err := registry.WithLocals(map[string]string{
			"dataDir":  "/var/data",
			"tempFile": "/tmp/file",
		})
		require.NoError(t, err)

		// Verify global variable is accessible
		value, err := childRegistry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)

		// Verify local variables are accessible
		value, err = childRegistry.Resolve("dataDir")
		require.NoError(t, err)
		assert.Equal(t, "/var/data", value)

		value, err = childRegistry.Resolve("tempFile")
		require.NoError(t, err)
		assert.Equal(t, "/tmp/file", value)
	})

	t.Run("valid_local_with_underscore", func(t *testing.T) {
		registry := NewVariableRegistry()

		childRegistry, err := registry.WithLocals(map[string]string{
			"_internal": "/var/internal",
		})
		require.NoError(t, err)

		value, err := childRegistry.Resolve("_internal")
		require.NoError(t, err)
		assert.Equal(t, "/var/internal", value)
	})

	t.Run("reject_uppercase_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.WithLocals(map[string]string{
			"AwsPath": "/usr/bin/aws",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be local")
	})

	t.Run("reject_reserved_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.WithLocals(map[string]string{
			"__reserved": "value",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved")
	})

	t.Run("original_registry_unchanged", func(t *testing.T) {
		registry := NewVariableRegistry()

		// Register global variable
		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		// Add local variables to child
		childRegistry, err := registry.WithLocals(map[string]string{
			"dataDir": "/var/data",
		})
		require.NoError(t, err)

		// Verify parent registry doesn't have local variables
		_, err = registry.Resolve("dataDir")
		require.Error(t, err)
		assert.IsType(t, &ErrUndefinedLocalVariable{}, err)

		// Verify child registry has local variables
		value, err := childRegistry.Resolve("dataDir")
		require.NoError(t, err)
		assert.Equal(t, "/var/data", value)
	})

	t.Run("empty_locals_map", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		childRegistry, err := registry.WithLocals(map[string]string{})
		require.NoError(t, err)

		// Verify global is still accessible
		value, err := childRegistry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)
	})
}

func TestVariableRegistry_Resolve(t *testing.T) {
	t.Run("resolve_global_from_parent", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		value, err := registry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)
	})

	t.Run("resolve_global_from_child", func(t *testing.T) {
		registry := NewVariableRegistry()

		err := registry.RegisterGlobal("AwsPath", "/usr/bin/aws")
		require.NoError(t, err)

		childRegistry, err := registry.WithLocals(map[string]string{
			"dataDir": "/var/data",
		})
		require.NoError(t, err)

		value, err := childRegistry.Resolve("AwsPath")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin/aws", value)
	})

	t.Run("resolve_local_from_child", func(t *testing.T) {
		registry := NewVariableRegistry()

		childRegistry, err := registry.WithLocals(map[string]string{
			"dataDir": "/var/data",
		})
		require.NoError(t, err)

		value, err := childRegistry.Resolve("dataDir")
		require.NoError(t, err)
		assert.Equal(t, "/var/data", value)
	})

	t.Run("local_not_accessible_from_parent", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.WithLocals(map[string]string{
			"dataDir": "/var/data",
		})
		require.NoError(t, err)

		// Parent registry should not have access to local variables
		_, err = registry.Resolve("dataDir")
		require.Error(t, err)
		assert.IsType(t, &ErrUndefinedLocalVariable{}, err)
	})

	t.Run("undefined_global_variable", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.Resolve("UndefinedVar")
		require.Error(t, err)
		assert.IsType(t, &ErrUndefinedGlobalVariable{}, err)
		assert.Contains(t, err.Error(), "UndefinedVar")
	})

	t.Run("undefined_local_variable", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.Resolve("undefinedVar")
		require.Error(t, err)
		assert.IsType(t, &ErrUndefinedLocalVariable{}, err)
		assert.Contains(t, err.Error(), "undefinedVar")
	})

	t.Run("invalid_variable_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.Resolve("123invalid")
		require.Error(t, err)
		assert.IsType(t, &ErrInvalidVariableName{}, err)
	})

	t.Run("reserved_variable_name", func(t *testing.T) {
		registry := NewVariableRegistry()

		_, err := registry.Resolve("__reserved")
		require.Error(t, err)
		assert.IsType(t, &ErrReservedVariableName{}, err)
	})
}

func TestVariableRegistry_NamespaceIsolation(t *testing.T) {
	t.Run("same_name_different_scopes", func(t *testing.T) {
		// This test verifies that global "Path" and local "path" are different variables
		// due to the naming convention (uppercase vs lowercase)
		registry := NewVariableRegistry()

		// Register global variable "Path"
		err := registry.RegisterGlobal("Path", "/usr/bin")
		require.NoError(t, err)

		// Add local variable "path"
		childRegistry, err := registry.WithLocals(map[string]string{
			"path": "/tmp",
		})
		require.NoError(t, err)

		// Verify they are independent
		globalValue, err := childRegistry.Resolve("Path")
		require.NoError(t, err)
		assert.Equal(t, "/usr/bin", globalValue)

		localValue, err := childRegistry.Resolve("path")
		require.NoError(t, err)
		assert.Equal(t, "/tmp", localValue)

		// Verify they are different
		assert.NotEqual(t, globalValue, localValue)
	})
}

func TestVariableRegistry_GlobalVars(t *testing.T) {
	t.Run("sorted_order", func(t *testing.T) {
		registry := NewVariableRegistry()

		// Register variables in random order
		_ = registry.RegisterGlobal("ZebraPath", "/zebra")
		_ = registry.RegisterGlobal("AwsPath", "/aws")
		_ = registry.RegisterGlobal("DataDir", "/data")

		entries := registry.GlobalVars()

		// Verify sorted by name
		require.Len(t, entries, 3)
		assert.Equal(t, "AwsPath", entries[0].Name)
		assert.Equal(t, "/aws", entries[0].Value)
		assert.Equal(t, "DataDir", entries[1].Name)
		assert.Equal(t, "/data", entries[1].Value)
		assert.Equal(t, "ZebraPath", entries[2].Name)
		assert.Equal(t, "/zebra", entries[2].Value)
	})

	t.Run("empty_registry", func(t *testing.T) {
		registry := NewVariableRegistry()

		entries := registry.GlobalVars()
		assert.Empty(t, entries)
	})

	t.Run("includes_all_globals", func(t *testing.T) {
		registry := NewVariableRegistry()

		_ = registry.RegisterGlobal("AwsPath", "/aws")
		_ = registry.RegisterGlobal("DataDir", "/data")

		entries := registry.GlobalVars()
		require.Len(t, entries, 2)

		// Verify all variables are included
		names := []string{entries[0].Name, entries[1].Name}
		assert.Contains(t, names, "AwsPath")
		assert.Contains(t, names, "DataDir")
	})
}

func TestVariableRegistry_LocalVars(t *testing.T) {
	t.Run("sorted_order", func(t *testing.T) {
		registry := NewVariableRegistry()

		childRegistry, err := registry.WithLocals(map[string]string{
			"zebraPath": "/zebra",
			"awsPath":   "/aws",
			"dataDir":   "/data",
		})
		require.NoError(t, err)

		entries := childRegistry.LocalVars()

		// Verify sorted by name
		require.Len(t, entries, 3)
		assert.Equal(t, "awsPath", entries[0].Name)
		assert.Equal(t, "/aws", entries[0].Value)
		assert.Equal(t, "dataDir", entries[1].Name)
		assert.Equal(t, "/data", entries[1].Value)
		assert.Equal(t, "zebraPath", entries[2].Name)
		assert.Equal(t, "/zebra", entries[2].Value)
	})

	t.Run("empty_locals", func(t *testing.T) {
		registry := NewVariableRegistry()

		childRegistry, err := registry.WithLocals(map[string]string{})
		require.NoError(t, err)

		entries := childRegistry.LocalVars()
		assert.Empty(t, entries)
	})

	t.Run("parent_has_no_locals", func(t *testing.T) {
		registry := NewVariableRegistry()

		entries := registry.LocalVars()
		assert.Empty(t, entries)
	})

	t.Run("includes_all_locals", func(t *testing.T) {
		registry := NewVariableRegistry()

		childRegistry, err := registry.WithLocals(map[string]string{
			"awsPath": "/aws",
			"dataDir": "/data",
		})
		require.NoError(t, err)

		entries := childRegistry.LocalVars()
		require.Len(t, entries, 2)

		// Verify all variables are included
		names := []string{entries[0].Name, entries[1].Name}
		assert.Contains(t, names, "awsPath")
		assert.Contains(t, names, "dataDir")
	})
}
