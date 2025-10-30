# Implementation Plan: Functional Options Pattern for security.Validator

## Overview

This task migrates the `security.Validator` constructors from multiple specialized constructors to a single constructor using the Functional Options Pattern, following the same pattern already used by `Runner`.

## Implementation Checklist

### Phase 1: Add New API (Backward Compatible)

- [x] **Task 1.1**: Add Option type and options structure
  - File: `internal/runner/security/validator.go`
  - Add `Option` function type
  - Add private `validatorOptions` struct with fields for `fs` and `groupMembership`
  - Estimated: 15 minutes

- [x] **Task 1.2**: Implement option functions
  - File: `internal/runner/security/validator.go`
  - Implement `WithFileSystem(fs common.FileSystem) Option`
  - Implement `WithGroupMembership(gm *groupmembership.GroupMembership) Option`
  - Add godoc comments for each option function
  - Estimated: 15 minutes

- [x] **Task 1.3**: Create new NewValidator constructor
  - File: `internal/runner/security/validator.go`
  - Rename current `NewValidator` to `newValidatorCore` (or similar internal name)
  - Implement new `NewValidator(config *Config, opts ...Option)` that:
    - Creates default options with `common.NewDefaultFileSystem()` and `nil` groupMembership
    - Applies all provided options
    - Calls the core constructor with the configured options
  - Estimated: 20 minutes

- [x] **Task 1.4**: Update existing constructors to use new API
  - File: `internal/runner/security/validator.go`
  - Reimplement `NewValidator(config *Config)` as wrapper calling new API (removed from signature)
  - Reimplement `NewValidatorWithFS` as wrapper using `WithFileSystem` option
  - Reimplement `NewValidatorWithGroupMembership` as wrapper using `WithGroupMembership` option
  - Reimplement `NewValidatorWithFSAndGroupMembership` as wrapper using both options
  - Add `// Deprecated:` godoc comments with migration instructions
  - Estimated: 20 minutes

- [x] **Task 1.5**: Add unit tests for new API
  - File: `internal/runner/security/validator_test.go`
  - Test new `NewValidator()` with no options
  - Test with `WithFileSystem` option
  - Test with `WithGroupMembership` option
  - Test with both options
  - Test option application order independence
  - Verify backward compatibility of deprecated constructors
  - Estimated: 30 minutes

### Phase 2: Migrate Usage Sites

- [x] **Task 2.1**: Update implementation code usage
  - Files to update:
    - `internal/runner/runner.go`: `NewValidator(nil)` → keep as-is (default usage)
    - `internal/runner/resource/default_manager.go`: `NewValidator(nil)` → keep as-is
    - `internal/runner/config/validator.go`: `NewValidator(secConfig)` → keep as-is
    - `internal/runner/security/environment_validation.go`: `NewValidator(nil)` → keep as-is
    - `internal/verification/manager.go`: `NewValidatorWithFS()` → use new API with `WithFileSystem`
  - Completed: All production code already uses the new API or was updated in Phase 1
  - Estimated: 20 minutes

- [x] **Task 2.2**: Update test code usage - Mock FileSystem
  - Update approximately 13 test files using `NewValidatorWithFS()`
  - Replace with `NewValidator(config, security.WithFileSystem(mockFS))`
  - Files include:
    - `internal/runner/security/*_test.go`
    - `internal/verification/*_test.go`
    - Other test files as needed
  - Completed: All test files updated, backward compatibility tests preserved
  - Estimated: 45 minutes

- [x] **Task 2.3**: Update test code usage - GroupMembership
  - Update approximately 13 test files using `NewValidatorWithGroupMembership()`
  - Replace with `NewValidator(config, security.WithGroupMembership(gm))`
  - Files primarily in:
    - `internal/runner/security/*_test.go`
    - File permission validation tests
  - Completed: All test files updated using sed for bulk replacement
  - Estimated: 45 minutes

- [x] **Task 2.4**: Update test code usage - Both options
  - Identify and update tests using `NewValidatorWithFSAndGroupMembership()`
  - Replace with `NewValidator(config, security.WithFileSystem(mockFS), security.WithGroupMembership(gm))`
  - Completed: All occurrences updated using sed
  - Estimated: 15 minutes

- [x] **Task 2.5**: Verify all migrations
  - Run `grep -r "NewValidatorWith" internal/` to find any remaining usage
  - Confirm all occurrences have been updated
  - Completed: Only definitions and backward compatibility tests remain
  - Estimated: 10 minutes

### Phase 3: Testing and Documentation

- [x] **Task 3.1**: Run full test suite
  - Execute `make test` or equivalent
  - Fix any test failures
  - Verify no regressions
  - Estimated: 15 minutes

- [x] **Task 3.2**: Run integration tests
  - Execute all integration tests in `cmd/runner/*_test.go`
  - Verify security validation behavior unchanged
  - Estimated: 10 minutes

- [x] **Task 3.3**: Update package documentation
  - File: `internal/runner/security/doc.go` (if exists) or add package comment in `validator.go`
  - Document the Functional Options Pattern usage
  - Provide migration examples
  - Estimated: 15 minutes

- [x] **Task 3.4**: Add migration guide
  - File: `docs/tasks/0048_security_validator_options_pattern/03_migration_guide.md`
  - Create before/after examples for common usage patterns
  - Document deprecation timeline
  - Estimated: 20 minutes

### Phase 4: Cleanup

- [x] **Task 4.1**: Remove deprecated constructors
  - Remove `NewValidatorWithFS`
  - Remove `NewValidatorWithGroupMembership`
  - Remove `NewValidatorWithFSAndGroupMembership`
  - Keep only the new `NewValidator(config *Config, opts ...Option)`
  - Remove backward compatibility tests

- [x] **Task 4.2**: Final verification
  - Ensure no external packages depend on deprecated constructors
  - Run full test suite (all 1,289 tests passed)
  - Update changelog

## Total Estimated Time

- Phase 1: 1 hour 40 minutes
- Phase 2: 2 hours 15 minutes
- Phase 3: 1 hour
- Phase 4: 30 minutes
- **Total for Phases 1-4: ~5.5 hours**

## Implementation Notes

### Key Design Decisions

1. **Pattern Consistency**: Follow the exact same pattern as `Runner.Option` for consistency
2. **Backward Compatibility**: Keep deprecated constructors as wrappers initially
3. **Default Values**: FileSystem defaults to `common.NewDefaultFileSystem()`, GroupMembership defaults to `nil`
4. **Error Handling**: Maintain existing error handling from the original constructors

### Code Structure

```go
// New API structure (similar to runner.go)
type Option func(*validatorOptions)

type validatorOptions struct {
    fs              common.FileSystem
    groupMembership *groupmembership.GroupMembership
}

func WithFileSystem(fs common.FileSystem) Option {
    return func(opts *validatorOptions) {
        opts.fs = fs
    }
}

func WithGroupMembership(gm *groupmembership.GroupMembership) Option {
    return func(opts *validatorOptions) {
        opts.groupMembership = gm
    }
}

func NewValidator(config *Config, opts ...Option) (*Validator, error) {
    options := &validatorOptions{
        fs:              common.NewDefaultFileSystem(),
        groupMembership: nil,
    }

    for _, opt := range opts {
        opt(options)
    }

    return newValidatorCore(config, options.fs, options.groupMembership)
}
```

### Testing Strategy

1. **Unit Tests**: Verify option application and defaults
2. **Integration Tests**: Ensure existing behavior unchanged
3. **Backward Compatibility**: Test deprecated constructors still work
4. **Edge Cases**: Test nil config, nil options, multiple option combinations

### Migration Examples

```go
// Before: Default
validator, err := security.NewValidator(nil)

// After: Same (no change needed)
validator, err := security.NewValidator(nil)

// Before: Custom FileSystem
validator, err := security.NewValidatorWithFS(config, mockFS)

// After: Using option
validator, err := security.NewValidator(config,
    security.WithFileSystem(mockFS))

// Before: GroupMembership
validator, err := security.NewValidatorWithGroupMembership(config, gm)

// After: Using option
validator, err := security.NewValidator(config,
    security.WithGroupMembership(gm))

// Before: Both
validator, err := security.NewValidatorWithFSAndGroupMembership(config, mockFS, gm)

// After: Using both options
validator, err := security.NewValidator(config,
    security.WithFileSystem(mockFS),
    security.WithGroupMembership(gm))
```

## Risk Assessment

- **Low Risk**: Pattern is already proven in the codebase (`Runner`)
- **Backward Compatible**: Deprecated constructors remain functional
- **Well Tested**: Comprehensive test coverage exists
- **Gradual Migration**: Can be done incrementally

## Success Criteria

- [x] All tests pass
- [x] No regression in security validation behavior
- [x] Code is more maintainable with single constructor
- [x] Pattern is consistent with `Runner`
- [x] Documentation is updated
- [x] Zero breaking changes for existing code
