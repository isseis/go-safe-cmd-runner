# TOML Field Renaming Migration Guide

## Overview

This guide explains the migration process for TOML configuration field name changes.

### Background of Breaking Changes

The TOML configuration field names have been improved for better clarity and consistency. This change improves the readability and maintainability of configuration files.

### Impact Scope

All existing TOML configuration files are affected. **Manual migration is required**.

## Field Name Changes List

### Global Level

| Old Field Name | New Field Name | Default Value Change | Notes |
|----------------|----------------|---------------------|-------|
| `skip_standard_paths` | `verify_standard_paths` | No behavior change | Changed from negative to positive form |
| `env` | `env_vars` | None | Environment variable prefix unification |
| `env_allowlist` | `env_allowed` | None | Shorter name |
| `from_env` | `env_import` | None | Environment variable prefix unification |
| `max_output_size` | `output_size_limit` | None | More natural word order |

### Group Level

| Old Field Name | New Field Name | Default Value Change | Notes |
|----------------|----------------|---------------------|-------|
| `env` | `env_vars` | None | Environment variable prefix unification |
| `env_allowlist` | `env_allowed` | None | Shorter name |
| `from_env` | `env_import` | None | Environment variable prefix unification |

### Command Level

| Old Field Name | New Field Name | Default Value Change | Notes |
|----------------|----------------|---------------------|-------|
| `env` | `env_vars` | None | Environment variable prefix unification |
| `from_env` | `env_import` | None | Environment variable prefix unification |
| `max_risk_level` | `risk_level` | None | More concise name |
| `output` | `output_file` | None | More explicit name |

## Important Changes

### 1. `skip_standard_paths` → `verify_standard_paths`

**Most important change**: Changed from negative to positive form

#### Before
```toml
[global]
skip_standard_paths = false  # Verify standard paths (default)
skip_standard_paths = true   # Skip standard path verification
```

#### After
```toml
[global]
verify_standard_paths = true   # Verify standard paths (default)
verify_standard_paths = false  # Skip standard path verification
```

#### Default Behavior
- **No behavior change**: Standard path verification is executed by default
- **Field name is now clearer**: What it does is intuitively understandable

### 2. Environment Variable Field Unification

All environment variable related fields are now unified with `env_` prefix:

```toml
# Before
env = ["VAR1", "VAR2"]
env_allowlist = ["ALLOWED_VAR"]
from_env = ["IMPORTED_VAR"]

# After
env_vars = ["VAR1", "VAR2"]
env_allowed = ["ALLOWED_VAR"]
env_import = ["IMPORTED_VAR"]
```

## Migration Steps

### Step 1: Create Backup

Create backups of existing configuration files before migration:

```bash
# For single file
cp config.toml config.toml.backup

# For multiple files
find . -name "*.toml" -exec cp {} {}.backup \;
```

### Step 2: Bulk Replacement with sed

Use the following sed commands for bulk replacement:

```bash
# Global level replacements
sed -i 's/skip_standard_paths = false/verify_standard_paths = true/g' *.toml
sed -i 's/skip_standard_paths = true/verify_standard_paths = false/g' *.toml
sed -i 's/^env = /env_vars = /g' *.toml
sed -i 's/^env_allowlist = /env_allowed = /g' *.toml
sed -i 's/^from_env = /env_import = /g' *.toml
sed -i 's/^max_output_size = /output_size_limit = /g' *.toml

# Group/Command level replacements
sed -i 's/max_risk_level = /risk_level = /g' *.toml
sed -i 's/^output = /output_file = /g' *.toml
```

### Step 3: Manual Verification

After bulk replacement with sed, manually verify the following:

1. **Value inversion for `skip_standard_paths`**:
   - `skip_standard_paths = false` → `verify_standard_paths = true`
   - `skip_standard_paths = true` → `verify_standard_paths = false`

2. **Field names in comments**: sed may not change field names in comments

3. **Indentation and whitespace**: Verify that TOML structure is properly preserved

### Step 4: Configuration File Validation

After migration, verify that configuration files work correctly:

```bash
# Verify operation with dry-run
./build/runner --config config.toml --dry-run

# Check for syntax errors
./build/runner --config config.toml --validate-only
```

## Migration Example

### Complete Migration Example

#### Before
```toml
[global]
skip_standard_paths = false
env = ["GLOBAL_VAR"]
env_allowlist = ["ALLOWED_*"]
from_env = ["PATH"]
max_output_size = 1024

[[groups]]
name = "example"
env = ["GROUP_VAR"]
env_allowlist = ["GROUP_*"]
from_env = ["HOME"]

  [[groups.commands]]
  name = "test"
  cmd = ["echo", "test"]
  env = ["CMD_VAR"]
  from_env = ["USER"]
  max_risk_level = "medium"
  output = "test.log"
```

#### After
```toml
[global]
verify_standard_paths = true
env_vars = ["GLOBAL_VAR"]
env_allowed = ["ALLOWED_*"]
env_import = ["PATH"]
output_size_limit = 1024

[[groups]]
name = "example"
env_vars = ["GROUP_VAR"]
env_allowed = ["GROUP_*"]
env_import = ["HOME"]

  [[groups.commands]]
  name = "test"
  cmd = ["echo", "test"]
  env_vars = ["CMD_VAR"]
  env_import = ["USER"]
  risk_level = "medium"
  output_file = "test.log"
```

## Frequently Asked Questions (FAQ)

### Q1: Does the default behavior change?

A1: No, the default behavior does not change. The default for `skip_standard_paths` was `false` (verification executed), and the default for `verify_standard_paths` is `true` (verification executed).

### Q2: What happens if I use the old field names?

A2: It will result in an error. The new version does not recognize old field names.

### Q3: Can I migrate gradually?

A3: No, this is a Breaking Change, so all fields must be migrated at once.

### Q4: Are there migration support tools?

A4: Currently, there are no dedicated migration tools. Please use the sed commands in this guide.

### Q5: What if I have a large number of configuration files?

A5: You can batch process multiple files as follows:

```bash
# All .toml files in a specific directory
find /path/to/configs -name "*.toml" -exec sed -i 's/old/new/g' {} \;

# Process recursively
find . -name "*.toml" -type f -exec sed -i 's/old/new/g' {} \;
```

## Troubleshooting

### Issue: Configuration file loading error

**Symptom**:
```
Error: unknown field 'env' in TOML
```

**Solution**:
Old field names remain. Re-execute the migration steps in this guide.

### Issue: Value inversion forgotten

**Symptom**:
Standard path verification behavior is opposite to expectation

**Solution**:
Verify the relationship between `skip_standard_paths` and `verify_standard_paths` values:
- `skip_standard_paths = false` → `verify_standard_paths = true`
- `skip_standard_paths = true` → `verify_standard_paths = false`

### Issue: Some files not migrated

**Solution**:
Search for files containing old field names:

```bash
# Search for old field names
grep -r "skip_standard_paths\|^env =\|env_allowlist\|from_env\|max_output_size\|max_risk_level\|^output =" *.toml
```

## Support

If you encounter issues during migration, please refer to the following resources:

- [GitHub Issues](https://github.com/isseis/go-safe-cmd-runner/issues)
- [User Guide](../user/)
- [Configuration Reference](../user/toml_config/)
