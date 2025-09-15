# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2024-09-15

### ⚠️ BREAKING CHANGES

This release introduces critical security enhancements that include breaking changes. Please review the migration guide carefully before upgrading.

#### Removed Features (Security)
- **`--hash-directory` command line flag**: Completely removed from the runner binary to prevent custom hash directory specification in production environments
- **Custom hash directory API**: Internal APIs no longer accept custom hash directories in production builds
- **Bootstrap InitializeVerificationManager**: Legacy API has been removed and replaced with secure alternatives
- **hashdir.GetWithValidation**: Legacy hash directory validation function removed

#### API Changes
- **New Production API**: `verification.NewManager()` - Secure API for production use with fixed hash directory
- **New Testing API**: `verification.NewManagerForTest()` - Test-only API with build tag restrictions (`//go:build test`)
- **Separated Build Targets**: Production and test builds now use different APIs and validation logic

### Added
- **Enhanced Security Validation**: Multi-layered security validation system
  - AST-based static analysis using `golangci-lint forbidigo`
  - Binary security validation with test artifact detection
  - Build-time security checks with `additional-security-checks.py`
- **API Separation**: Clear separation between production and testing APIs
- **Caller Validation**: Runtime verification that test APIs are only called from test files
- **Security Logging**: Enhanced logging for security events and API usage
- **Centralized Verification**: Unified verification management with fallback privilege handling

### Security Enhancements
- **Fixed Hash Directory**: Production builds enforce use of default hash directory only (`/usr/local/etc/go-safe-cmd-runner/hashes`)
- **Test Environment Isolation**: Test functionality completely isolated from production builds using build tags
- **Static Analysis Integration**: Automated detection of security violations during build process
- **Binary Validation**: Production binaries are automatically validated to ensure no test artifacts are included

### Changed
- **Verification API**: Complete overhaul of verification management system
- **Error Handling**: New security-focused error types:
  - `HashDirectorySecurityError`: For hash directory security violations
  - `ProductionAPIViolationError`: For inappropriate API usage
- **Build Process**: Enhanced build pipeline with automatic security validation
- **Testing Framework**: Improved testing utilities with proper API separation

### Migration Guide

#### For Production Usage
**Before:**
```bash
# Old command with custom hash directory
./runner -config config.toml -hash-directory /custom/path
```

**After:**
```bash
# New secure command (uses fixed default directory)
./runner -config config.toml
```

**Before:**
```go
// Old internal API
hashDir, err := hashdir.GetWithValidation(customDir, defaultDir)
verificationManager, err := bootstrap.InitializeVerificationManager(hashDir, runID)
```

**After:**
```go
// New secure production API
verificationManager, err := verification.NewManager()
```

#### For Testing Usage
**Before:**
```go
// Old testing approach
verificationManager, err := bootstrap.InitializeVerificationManager("/tmp/test", "test-run")
```

**After:**
```go
//go:build test

// New testing API with build tag
verificationManager, err := verification.NewManagerForTest("/tmp/test")
```

#### Configuration Changes
- Remove any `hash_directory` specifications from TOML configuration files
- Update any scripts or automation that use the `--hash-directory` flag
- Test code must use the new test API with appropriate build tags

### Technical Details

#### Security Architecture
- **Multi-layered Validation**:
  1. Build-time static analysis
  2. Binary security validation
  3. Runtime caller verification
  4. API usage monitoring
- **Zero Trust Model**: No external hash directory specification allowed in production
- **Audit Trail**: Complete logging of API usage and security events

#### Build System Changes
- **Production Builds**: `make build` - Creates secure production binaries
- **Test Builds**: `make build-test` - Creates test binaries with additional APIs
- **Security Validation**: `make security-check` - Validates production binary security
- **Enhanced CI/CD**: Automated security checks in build pipeline

### Compatibility
- **Go Version**: Requires Go 1.23+ (no change)
- **Runtime**: No changes to command execution or configuration syntax (except removed flags)
- **Dependencies**: No new external dependencies added

### Performance Impact
- **Build Time**: Slightly increased due to additional security validation
- **Runtime**: No significant performance impact
- **Binary Size**: Minimal increase in production binaries

### Documentation
- **New API Documentation**: [docs/verification_api.md](docs/verification_api.md)
- **Migration Guide**: Detailed in README.md and API documentation
- **Security Documentation**: Enhanced security model documentation

## [1.x.x] - Previous Versions

[Previous changelog entries would be listed here for older versions]
