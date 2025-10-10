# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

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
