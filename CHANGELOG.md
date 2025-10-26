# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Final Environment Variable Display in Dry-Run Mode

When using `--dry-run-detail=full`, the final environment variables for each command are now displayed with their origin information.

**Features:**
- Display final environment variables before each command execution in dry-run mode
- Show the origin of each variable (System, Global, Group, Command)
- Long values are truncated to 60 characters for readability
- Sensitive information (passwords, tokens, secrets) is masked by default as `[REDACTED]`

**New Flag:**
- `--show-sensitive`: Explicitly show sensitive environment variable values in plain text (use with caution)
  - Default: sensitive values are masked
  - Security warning: do not use in production or CI/CD environments

**Example Output:**
```
===== Final Process Environment =====

Environment variables (5):
  PATH=/usr/local/bin:/usr/bin:/bin
    (from Global)
  HOME=/home/testuser
    (from System (filtered by allowlist))
  APP_DIR=/opt/myapp
    (from Group[build])
  DB_PASSWORD=[REDACTED]
    (from Global)
  LOG_FILE=/opt/myapp/logs/app.log
    (from Command[run_tests])
```

**Performance:**
- The overhead for displaying the final environment in dry-run mode is negligible (less than 10% in tests), ensuring minimal impact on performance.

### Breaking Changes

#### Timeout Behavior Change

**BREAKING**: `timeout = 0` now means unlimited execution (previously defaulted to 60 seconds)

- **Before**: `timeout = 0` was treated as invalid (not accepted)
- **After**: `timeout = 0` explicitly means unlimited execution time (no timeout)

**Migration Required**: Review all `timeout = 0` settings in existing configuration files.

#### TOML Field Renaming

All TOML configuration field names have been updated to improve clarity and consistency.

**Migration Required**: Existing configuration files must be manually updated.

##### Field Name Mapping

| Level | Old Field Name | New Field Name | Default Value Change |
|-------|----------------|----------------|---------------------|
| Global | `skip_standard_paths` | `verify_standard_paths` | `false` (verify) → `true` (verify) |
| Global | `env` | `env_vars` | - |
| Global | `env_allowlist` | `env_allowed` | - |
| Global | `from_env` | `env_import` | - |
| Global | `max_output_size` | `output_size_limit` | - |
| Group | `env` | `env_vars` | - |
| Group | `env_allowlist` | `env_allowed` | - |
| Group | `from_env` | `env_import` | - |
| Command | `env` | `env_vars` | - |
| Command | `from_env` | `env_import` | - |
| Command | `max_risk_level` | `risk_level` | - |
| Command | `output` | `output_file` | - |

##### Key Changes

1. **Positive Naming**: `skip_standard_paths` → `verify_standard_paths`
   - Old: `skip_standard_paths = false` (default: verify standard paths)
   - New: `verify_standard_paths = true` (default: verify standard paths)
   - **Default behavior unchanged (verification continues), but field name is now clearer**

2. **Environment Variable Prefix Unification**: All environment-related fields now use `env_` prefix
   - `env` → `env_vars`
   - `env_allowlist` → `env_allowed`
   - `from_env` → `env_import`

3. **Natural Word Order**: `max_output_size` → `output_size_limit`

4. **Clarity**: `output` → `output_file`, `max_risk_level` → `risk_level`

#### Working Directory Specification Redesign

**Working directory specification redesign**: Simplified working directory configuration with automatic temporary directory support
- **Removed `Global.WorkDir` field**: Global-level working directory configuration is no longer supported
- **Removed `Group.TempDir` field**: Replaced with automatic temporary directory generation when `workdir` is not specified
- **Renamed `Command.Dir` to `Command.WorkDir`**: Command-level directory specification now uses `workdir` field
- **Default behavior change**: Groups without `workdir` now automatically generate temporary directories instead of using current directory
- **Automatic cleanup**: Temporary directories are automatically deleted after group execution (unless `--keep-temp-dirs` is specified)

### Added

- Support for unlimited command execution with `timeout = 0`
- Enhanced timeout hierarchy resolution (command → global → system default)
- Security monitoring for unlimited execution commands
- Long-running process detection and logging
- Comprehensive timeout examples in `sample/timeout_examples.toml`
- Migration guide for timeout changes
- **`__runner_workdir` reserved variable**: New automatic variable that references the runtime working directory for command execution
- **`--keep-temp-dirs` flag**: New command-line flag to preserve temporary directories after execution for debugging purposes
- **Automatic temporary directory generation**: Groups without specified `workdir` now automatically generate temporary directories
- **Dry-run mode support for temporary directories**: Dry-run mode now uses virtual paths for temporary directories
- **verify_files Variable Expansion**: Environment variable expansion support for `verify_files` fields in both global and group configurations
  - Global-level `verify_files` can now use environment variables (e.g., `${HOME}/config.toml`)
  - Group-level `verify_files` can now use environment variables with allowlist inheritance
  - Support for multiple variables in a single path (e.g., `${BASE}/${ENV}/config.toml`)
  - Comprehensive error handling with detailed error messages
  - Security controls through `env_allowlist` validation
  - Circular reference detection for environment variables
  - Sample configuration: `sample/verify_files_expansion.toml`
  - Documentation: Added section 7.11 to variable expansion user guide

### Changed

- Timeout configuration now uses nullable integers for better control
- Improved timeout resolution logic with clear inheritance hierarchy
- Enhanced error messages for timeout configuration errors
- Updated documentation with breaking change notices and examples
- Configuration loading now automatically expands environment variables in `verify_files` fields
- Verification manager now uses expanded file paths for all verification operations

### Security

- Added security logging for unlimited timeout executions
- Implemented monitoring for long-running processes
- Enhanced resource usage tracking for unlimited execution commands

### Technical Details

- New fields: `GlobalConfig.ExpandedVerifyFiles` and `CommandGroup.ExpandedVerifyFiles`
- New functions: `ExpandGlobalVerifyFiles()` and `ExpandGroupVerifyFiles()` in config package
- New error types: `VerifyFilesExpansionError` with sentinel errors for better error handling
- Exported `ResolveAllowlistConfiguration()` method in environment package for reusability
- Integration with existing `Filter` and `VariableExpander` infrastructure from task 0026

### Migration Guide

#### Timeout Configuration

For detailed migration instructions, see the timeout configuration documentation.

#### TOML Field Renaming

See [Migration Guide](docs/migration/toml_field_renaming.en.md) for detailed instructions.

#### Working Directory Configuration

Existing TOML configuration files must be updated as follows:

1. **Remove `[global]` section `workdir`**:
   ```toml
   # Before (will cause error)
   [global]
   workdir = "/tmp"

   # After
   [global]
   # workdir field removed
   ```

2. **Remove `[[groups]]` section `temp_dir`**:
   ```toml
   # Before (will cause error)
   [[groups]]
   name = "backup"
   temp_dir = true

   # After (automatic temporary directory)
   [[groups]]
   name = "backup"
   # temp_dir field removed - automatic temporary directory will be created
   ```

3. **Change `[[groups.commands]]` `dir` to `workdir`**:
   ```toml
   # Before (will cause error)
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   dir = "/var/backups"

   # After
   [[groups.commands]]
   name = "backup"
   cmd = "pg_dump"
   workdir = "/var/backups"
   ```

4. **Use `%{__runner_workdir}` variable** for dynamic path references:
   ```toml
   [[groups]]
   name = "backup"
   # No workdir specified - automatic temporary directory

   [[groups.commands]]
   name = "dump"
   cmd = "pg_dump"
   args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
   ```

## [Previous Releases]

(Previous release notes will be added here when available)
