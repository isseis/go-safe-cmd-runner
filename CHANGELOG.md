# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **`__runner_workdir` reserved variable**: New automatic variable that references the runtime working directory for command execution
- **`--keep-temp-dirs` flag**: New command-line flag to preserve temporary directories after execution for debugging purposes
- **Automatic temporary directory generation**: Groups without specified `workdir` now automatically generate temporary directories
- **Dry-run mode support for temporary directories**: Dry-run mode now uses virtual paths for temporary directories

### Changed - Breaking Changes

- **Working directory specification redesign**: Simplified working directory configuration with automatic temporary directory support
  - **Removed `Global.WorkDir` field**: Global-level working directory configuration is no longer supported
  - **Removed `Group.TempDir` field**: Replaced with automatic temporary directory generation when `workdir` is not specified
  - **Renamed `Command.Dir` to `Command.WorkDir`**: Command-level directory specification now uses `workdir` field
  - **Default behavior change**: Groups without `workdir` now automatically generate temporary directories instead of using current directory
  - **Automatic cleanup**: Temporary directories are automatically deleted after group execution (unless `--keep-temp-dirs` is specified)

### Migration Guide

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

### verify_files Variable Expansion

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

- Configuration loading now automatically expands environment variables in `verify_files` fields
- Verification manager now uses expanded file paths for all verification operations

### Technical Details

- New fields: `GlobalConfig.ExpandedVerifyFiles` and `CommandGroup.ExpandedVerifyFiles`
- New functions: `ExpandGlobalVerifyFiles()` and `ExpandGroupVerifyFiles()` in config package
- New error types: `VerifyFilesExpansionError` with sentinel errors for better error handling
- Exported `ResolveAllowlistConfiguration()` method in environment package for reusability
- Integration with existing `Filter` and `VariableExpander` infrastructure from task 0026

## [Previous Releases]

(Previous release notes will be added here when available)
