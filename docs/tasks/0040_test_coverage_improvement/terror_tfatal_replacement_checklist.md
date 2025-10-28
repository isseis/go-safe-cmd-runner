# t.Error*() and t.Fatal*() Replacement Checklist

This document tracks all instances of `t.Error()`, `t.Errorf()`, `t.Fatal()`, and `t.Fatalf()` in the test suite and documents whether they should be replaced with testify assertions or remain as-is.

## Summary

Total test files scanned: Approximately 30+ test files
Estimated instances: 200+ (excluding fmt.Errorf in mocks and documentation)
**Completed replacements**: 5 instances
**Remaining to review**: 195+ instances

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
- **Phase 2**: Completed 5 instances across 3 files ✅
- **Phase 3**: Remaining files documented with replacement patterns ✅
- **Phase 4**: Bulk replacement (Optional - can be done incrementally)

## Testing

Completed files have been tested:
- ✅ temp_directory_race_test.go
- ✅ slack_handler_test.go
- ✅ nofollow_error_netbsd_test.go (NetBSD only)

## Recommendations

1. **Priority 1** (Quick wins): Terminal tests (detector, preference, capabilities) - simple patterns, ~18 instances
2. **Priority 2** (Medium effort): runnertypes tests (loglevel, runtime) - straightforward replacements, ~7 instances
3. **Priority 3** (Large effort): config_test.go and allowlist_resolution_test.go - repetitive patterns, 100+ instances total
4. **Leave as-is**: Security-critical assertions (safe_file_test.go:707-708), performance tests with custom thresholds

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
