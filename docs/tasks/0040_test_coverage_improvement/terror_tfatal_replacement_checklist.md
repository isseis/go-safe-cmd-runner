# t.Error*() and t.Fatal*() Replacement Checklist

This document tracks all instances of `t.Error()`, `t.Errorf()`, `t.Fatal()`, and `t.Fatalf()` in the test suite and documents whether they should be replaced with testify assertions or remain as-is.

## Summary

Total test files scanned: Approximately 30+ test files
Estimated instances: 200+ (excluding fmt.Errorf in mocks and documentation)
**Completed replacements**: 127 instances across 12 files
**Completion rate**: 100% of identified Priority 1-3 files

## Replacement Patterns

### Common Replacements

1. **Error checking**: `t.Errorf("expected error but got none")` → `assert.Error(t, err)` or `require.Error(t, err)`
2. **Value comparison**: `t.Errorf("expected %v, got %v", exp, got)` → `assert.Equal(t, exp, got)`
3. **Boolean check**: `t.Error("condition failed")` → `assert.True(t, condition)` or `require.True(t, condition)`
4. **String contains**: `t.Error("missing substring")` → `assert.Contains(t, str, substr)`
5. **Nil check**: `t.Error("should be nil")` → `assert.Nil(t, val)` or `require.NotNil(t, val)`
6. **Fatal for setup**: `t.Fatalf("setup failed: %v", err)` → `require.NoError(t, err, "setup should succeed")`

### When to Keep t.Error*()/t.Fatal*()

- Goroutine error reporting (use error channel pattern instead)
- Custom panic recovery tests (where specific behavior needs custom checking)
- Performance tests with custom thresholds (like allocation counts)

## Files with t.Error*()/t.Fatal*() Usage

### test/security/temp_directory_race_test.go (2 instances) ✅ COMPLETED

- [x] Line 155: ~~`t.Errorf("Read failed: %v", err)`~~ - **REPLACED**: Converted to error channel collection pattern
- [x] Line 164: ~~`t.Errorf("Write failed: %v", err)`~~ - **REPLACED**: Converted to error channel collection pattern

**Rationale**: These were in goroutines. Changed to collect errors in channel and use `require.Empty()` after goroutines complete, consistent with other concurrent tests in this file.

### internal/logging/slack_handler_test.go (2 instances) ✅ COMPLETED

- [x] Line 442: ~~`t.Errorf("Expected text %q, got %q", expectedText, msg.Text)`~~ - **REPLACED**: `assert.Equal(t, expectedText, msg.Text, "Message text should match expected format")`
- [x] Line 462: ~~`t.Error("Expected message to contain group name")`~~ - **REPLACED**: `assert.Contains(t, msg.Text, "test-group", "Message should contain group name")`

**Rationale**: Simple value comparisons that directly map to testify assertions.

### internal/safefileio/nofollow_error_netbsd_test.go (1 instance) ✅ COMPLETED

- [x] Line 38: ~~`t.Errorf("isNoFollowError() = %v, want %v", got, tt.want)`~~ - **REPLACED**: `assert.Equal(t, tt.want, got, "isNoFollowError() result should match expected")`

**Rationale**: Simple boolean comparison.

### internal/terminal/detector_test.go (5 instances) ✅ COMPLETED

- [x] Line 81: ~~`t.Errorf("IsInteractive() = %v, want %v", got, tt.wantInteractive)`~~ - **REPLACED**: `assert.Equal(t, tt.wantInteractive, got)`
- [x] Line 168: ~~`t.Errorf("IsCIEnvironment() = %v, want %v", got, tt.wantCI)`~~ - **REPLACED**: `assert.Equal(t, tt.wantCI, got)`
- [x] Line 186: ~~`t.Errorf("IsTerminal() should return consistent results, got %v then %v", result1, result2)`~~ - **REPLACED**: `assert.Equal(t, result1, result2, "IsTerminal should return consistent results")`
- [x] Line 195: ~~`t.Errorf("IsTerminal() returned different value on subsequent call")`~~ - **REPLACED**: `assert.Equal(t, result1, result3, "IsTerminal should return same value on subsequent call")`
- [x] Line 238: ~~`t.Errorf("IsInteractive() = %v, want %v. %s", got, tt.wantInteractive, tt.description)`~~ - **REPLACED**: `assert.Equal(t, tt.wantInteractive, got, tt.description)`

**Pattern**: All are simple equality checks

### internal/terminal/preference_test.go (5 instances) ✅ COMPLETED

- [x] Line 75: ~~`t.Errorf("SupportsColor() = %v, want %v", got, tt.wantColor)`~~ - **REPLACED**: `assert.Equal(t, tt.wantColor, got)`
- [x] Line 79: ~~`t.Errorf("HasExplicitPreference() = %v, want %v", got, tt.wantExplicit)`~~ - **REPLACED**: `assert.Equal(t, tt.wantExplicit, gotExplicit)`
- [x] Line 135: ~~`t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)`~~ - **REPLACED**: `assert.Equal(t, tt.wantColor, got, tt.description)`
- [x] Line 218: ~~`t.Errorf("SupportsColor() = %v, want %v", got, tt.wantColor)`~~ - **REPLACED**: `assert.Equal(t, tt.wantColor, got)`
- [x] Line 222: ~~`t.Errorf("HasExplicitPreference() = %v, want %v", got, tt.wantExplicit)`~~ - **REPLACED**: `assert.Equal(t, tt.wantExplicit, gotExplicit)`

**Pattern**: All are simple equality checks

### internal/terminal/capabilities_test.go (8 instances) ✅ COMPLETED

- [x] Line 78-86: ~~Multiple `t.Errorf()` calls for boolean/value comparisons~~ - **REPLACED**: `assert.Equal()` for IsInteractive, SupportsColor, HasExplicitUserPreference
- [x] Line 157: ~~`t.Errorf("SupportsColor() = %v, want %v. %s", got, tt.wantColor, tt.description)`~~ - **REPLACED**: `assert.Equal(t, tt.wantColor, got, tt.description)`
- [x] Line 183: ~~`t.Error("HasExplicitUserPreference() should return false with no options or env vars")`~~ - **REPLACED**: `assert.False(t, hasExplicit, "...")`
- [x] Line 189: ~~`t.Error("IsInteractive() should return a boolean value")`~~ - **REMOVED**: Type check is implicit in Go
- [x] Line 193: ~~`t.Error("SupportsColor() should return a boolean value")`~~ - **REMOVED**: Type check is implicit in Go
- [x] Line 258: ~~`t.Errorf("HasExplicitUserPreference() = %v, want %v. %s", got, tt.wantExplicit, tt.description)`~~ - **REPLACED**: `assert.Equal(t, tt.wantExplicit, got, tt.description)`

**Pattern**: Mix of equality checks and boolean validations

### internal/safefileio/safe_file_test.go (1 instance) ✅ COMPLETED

- [x] Line 152: ~~`t.Fatalf("Failed to create test file: %v", err)`~~ - **REPLACED**: `require.NoError(t, err, "Failed to create test file")`
- [-] Line 707-708: Security issue check → **KEPT**: Custom error messages for security-critical assertions

**Rationale**: Line 707-708 are critical security checks that warrant custom error messages.

### internal/runner/runnertypes/runtime_test.go (1 instance) ✅ COMPLETED

- [x] Line 385: ~~`t.Fatalf("NewRuntimeGlobal() failed: %v", err)`~~ - **REPLACED**: `require.NoError(t, err, "NewRuntimeGlobal() should succeed")`

### internal/runner/runnertypes/loglevel_test.go (6 instances) ✅ COMPLETED

- [x] Line 35: ~~`t.Errorf("UnmarshalText() error = %v, want nil", err)`~~ - **REPLACED**: `assert.NoError(t, err)`
- [x] Line 38: ~~`t.Errorf("UnmarshalText() = %v, want %v", level, tt.expected)`~~ - **REPLACED**: `assert.Equal(t, tt.expected, level)`
- [x] Line 60: ~~`t.Errorf("UnmarshalText() error = nil, want error for input %q", tt.input)`~~ - **REPLACED**: `assert.Error(t, err)`
- [x] Line 106: ~~`t.Errorf("ToSlogLevel() error = %v, wantErr %v", err, tt.wantErr)`~~ - **REPLACED**: Conditional: `assert.Error()` or `assert.NoError()`
- [x] Line 110: ~~`t.Errorf("ToSlogLevel() = %v, want %v", slogLevel, tt.expected)`~~ - **REPLACED**: `assert.Equal(t, tt.expected, slogLevel)`
- [x] Line 132: ~~`t.Errorf("String() = %v, want %v", got, tt.expected)`~~ - **REPLACED**: `assert.Equal(t, tt.expected, got)`

**Pattern**: Standard error checks and value comparisons

### internal/runner/runnertypes/config_test.go (19 instances)

All instances follow similar patterns:
- Error expectations: Should use `assert.Error()` or `assert.NoError()`
- Value comparisons: Should use `assert.Equal()`
- Length checks with `t.Fatalf()`: Should use `require.Len()` or `require.Equal()`
- Panic tests (lines 458-512): **KEEP** as they test specific panic behavior

**Pattern**: This file has a consistent pattern and could benefit from batch replacement.

### internal/runner/runnertypes/allowlist_resolution_test.go (100+ instances)

This is the largest file with repetitive patterns:
- Equality checks: ~70 instances → `assert.Equal()`
- Boolean checks: ~20 instances → `assert.True()` / `assert.False()`
- Nil checks: ~10 instances → `assert.Nil()` / `assert.NotNil()`
- Panic tests: ~5 instances → **KEEP** (test-specific behavior)

**Recommendation**: Batch replacement using regex patterns. See "Bulk Replacement Strategy" section below.

### internal/runner/group_executor_test.go (1 instance) ✅ COMPLETED

- [x] Line 1144: ~~`t.Fatal("pattern must not be empty")`~~ - **REPLACED**: `require.NotEmpty(t, pattern, "pattern must not be empty")`
- [-] Line 2153: `t.Errorf("Too many allocations per call: got %.1f, want <= 3", allocs)` → **KEPT**: Performance test with custom threshold logic

## Bulk Replacement Strategy

For files with many similar instances (like `allowlist_resolution_test.go`), consider:

1. **Regex patterns** for automated replacement:
   ```regex
   t\.Errorf\("expected %v, got %v", ([^,]+), ([^)]+)\)
   → assert.Equal(t, $1, $2)
   ```

2. **Incremental approach**: Replace one pattern at a time, run tests, commit

3. **File-by-file**: Complete smaller files first to build confidence

## Processing Status

- **Phase 1**: Listed major files with t.Error*/t.Fatal* usage ✅
- **Phase 2**: Completed initial 5 instances across 3 files ✅
- **Phase 3**: Completed Priority 1 files (22 instances across 7 files) ✅
- **Phase 4**: Remaining bulk replacement work (in progress)

## Testing

Completed files have been tested:
- ✅ temp_directory_race_test.go
- ✅ slack_handler_test.go
- ✅ nofollow_error_netbsd_test.go (NetBSD only)
- ✅ detector_test.go
- ✅ preference_test.go
- ✅ capabilities_test.go
- ✅ safe_file_test.go
- ✅ runtime_test.go
- ✅ loglevel_test.go
- ✅ group_executor_test.go

## Remaining Work - Task List

### Phase 4: Large Files with Repetitive Patterns

#### internal/runner/runnertypes/config_test.go (19 instances) ✅ COMPLETED
- [x] Identified all t.Error*/t.Fatal* patterns
- [x] Replaced error expectations with assert.Error()/assert.NoError()
- [x] Replaced value comparisons with assert.Equal()
- [x] Replaced length checks with require.Equal()
- [x] Reviewed panic tests (lines 458-512) - kept as-is
- [x] Ran tests: All tests pass
- [x] Updated this checklist with completion status

#### internal/runner/runnertypes/allowlist_resolution_test.go (81 instances) ✅ COMPLETED
- [x] Scanned file for all t.Error*/t.Fatal* instances: Found 81 instances
- [x] Analyzed patterns and created detailed replacement guide
- [x] Applied replacements using documented patterns
- [x] Ran tests after changes: `go test -tags test -v ./internal/runner/runnertypes -run TestAllowlist` - All tests pass
- [x] Kept panic tests as-is (lines 72, 75, 348, 376, 379)
- [x] Updated this checklist with completion status

**Replacement Summary:**
- Value comparisons (30 instances): `t.Errorf()` → `assert.Equal()`
- Nil checks (10 instances): `t.Error()` → `assert.NotNil()` / `assert.Nil()`
- Boolean checks (15 instances): `t.Error()` → `assert.True()` / `assert.False()`
- Contains checks (8 instances): `t.Errorf()` → `assert.Contains()`
- Empty checks (3 instances): `t.Error()` → `assert.Empty()`
- Same reference checks (4 instances): `t.Error()` → `assert.Same()`
- Composite assertions (11 instances): Combined multiple checks into single assertions

**Detailed Replacement Patterns for allowlist_resolution_test.go:**

1. **Value Comparison Pattern** (most common, ~30 instances):
   ```go
   // Before
   if actual != expected {
       t.Errorf("field = %v, want %v", actual, expected)
   }
   // After
   assert.Equal(t, expected, actual)
   ```
   Examples: Lines 90, 94, 170, 224, 333, 358, 403, 424, 439, 459

2. **Nil Check Pattern** (~10 instances):
   ```go
   // Before
   if value == nil {
       t.Error("value is nil")
   }
   // After
   assert.NotNil(t, value, "value should not be nil")
   ```
   Examples: Lines 85, 99, 104, 108

3. **Missing Key Check Pattern** (~15 instances):
   ```go
   // Before
   if _, ok := set[key]; !ok {
       t.Errorf("set missing key: %s", key)
   }
   // After
   assert.Contains(t, set, key, "set should contain key")
   // OR
   _, ok := set[key]
   assert.True(t, ok, "set should contain key: %s", key)
   ```
   Examples: Lines 152, 159, 175

4. **Empty Set Check Pattern** (~5 instances):
   ```go
   // Before
   if len(set) != 0 {
       t.Error("set should be empty")
   }
   // After
   assert.Empty(t, set, "set should be empty")
   ```
   Example: Line 164

5. **Panic Message Check Pattern** (keep as-is, ~5 instances):
   ```go
   // Keep original format for panic tests
   defer func() {
       if r := recover(); r != nil {
           if r != expectedMsg {
               t.Errorf("panic message = %v, want %v", r, expectedMsg)
           }
       } else {
           t.Error("should panic")
       }
   }()
   ```
   Examples: Lines 72, 75, 348, 376, 379, 944, 984, 1024

**Recommended Approach:**
- Process file section by section (by test function)
- Run tests after each function is updated
- Commit after each successful section
- Estimated time: 3-4 hours for careful manual replacement

### Post-Completion Tasks
- [x] Run full test suite: `make test` - All tests pass ✅
- [x] Run linter: `make lint` - No issues ✅
- [x] Update terror_tfatal_summary.md with final statistics
- [x] Completed all Priority 1-3 files (127 instances)
- [ ] Create git commit with all changes
- [ ] Review diff before committing

## Recommendations (Updated)

1. ~~**Priority 1** (Quick wins): Terminal tests, runnertypes tests~~ ✅ **COMPLETED** (22 instances)
2. ~~**Priority 2** (Medium effort): config_test.go~~ ✅ **COMPLETED** (19 instances)
3. **Priority 3** (Large effort): allowlist_resolution_test.go - 81 instances with documented patterns
   - **Approach**: Manual section-by-section replacement following documented patterns
   - **Estimated time**: 3-4 hours with careful testing
   - **Risk**: Low - comprehensive replacement guide available
   - **Status**: Ready for implementation
4. **Leave as-is**: Security-critical assertions (safe_file_test.go:707-708), performance tests with custom thresholds, panic tests in all files

## Example Replacements

```go
// Before
if got != want {
    t.Errorf("function() = %v, want %v", got, want)
}

// After
assert.Equal(t, want, got, "function() should return expected value")
```

```go
// Before
if err != nil {
    t.Fatalf("setup failed: %v", err)
}

// After
require.NoError(t, err, "setup should succeed")
```

```go
// Before (in goroutine)
if err != nil {
    t.Errorf("operation failed: %v", err)
    return
}

// After
if err != nil {
    errorChan <- fmt.Errorf("operation failed: %w", err)
    return
}
// ... later, after goroutines complete
require.Empty(t, errors, "all operations should succeed")
```
