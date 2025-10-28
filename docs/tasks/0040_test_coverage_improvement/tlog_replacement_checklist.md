# t.Log*() Replacement Checklist

This document tracks all instances of `t.Log()` and `t.Logf()` in the test suite and documents whether they should be replaced with assertions or remain as logging statements.

## Summary

Total instances found: 81 (77 in code, 4 in documentation)
- [x] Replaced: 18
- [-] Kept as informational logging: 59

## Files with t.Log*() Usage

### test/security/hash_bypass_test.go (10 instances)

- [x] Line 107: ~~`t.Logf("Verification passed as expected: %s", tt.description)`~~ - **REMOVED**: Redundant after `require.NoError()`
- [x] Line 110: ~~`t.Logf("Verification failed as expected: %s (error: %v)", tt.description, err)`~~ - **REMOVED**: Redundant after `require.Error()`
- [x] Line 174: ~~`t.Logf("Tampered hash manifest file: %s", hashFilePath)`~~ - **REMOVED**: Redundant debug info
- [x] Line 205: ~~`t.Logf("Deleted hash file: %s", hashFilePath)`~~ - **REMOVED**: Redundant debug info
- [x] Line 218: ~~`t.Logf("Initial verification passed")`~~ - **REMOVED**: Redundant after `require.NoError()`
- [x] Line 226: ~~`t.Logf("Verification correctly failed after tampering: %v", err)`~~ - **REMOVED**: Redundant after `require.Error()`
- [x] Line 256: ~~`t.Logf("Legitimate file verification passed")`~~ - **REMOVED**: Redundant after `require.NoError()`
- [x] Line 278: ~~`t.Logf("Symlink verification failed as expected: %v", err)`~~ - **REMOVED**: Redundant after `require.Error()`
- [x] Line 306: ~~`t.Logf("Initial verification passed")`~~ - **REMOVED**: Redundant after `require.NoError()`
- [x] Line 316: ~~`t.Logf("TOCTOU protection: modification detected (%v)", err)`~~ - **REMOVED**: Redundant after `require.Error()`

### test/security/temp_directory_race_test.go (5 instances)

- [x] Line 71: ~~`t.Logf("Successfully completed %d concurrent goroutines with %d operations each", ...)`~~ - **REPLACED**: Changed `t.Errorf()` loop to `require.Empty()` with collected errors
- [x] Line 126: ~~`t.Logf("Successfully cleaned up %d directories concurrently", numDirs)`~~ - **REPLACED**: Changed `t.Errorf()` loop to `require.Empty()` with collected errors
- [x] Line 177: ~~`t.Logf("Race detection test completed with %d goroutines", numGoroutines)`~~ - **REMOVED**: Redundant completion message
- [x] Line 193: ~~`t.Logf("Recovered from panic: %v", r)`~~ - **REPLACED**: Changed to `require.NotNil()` assertion
- [x] Line 209: ~~`t.Logf("Cleanup on panic test completed")`~~ - **REMOVED**: Redundant completion message

### test/security/output_security_test.go (16 instances)

- [-] Line 111: `t.Logf("Command failed as expected for %s: %v", tc.outputPath, err)` - **KEPT**: Documents implementation status where validation integration is incomplete
- [-] Line 113: `t.Logf("Command succeeded for %s - validation may happen at output time", tc.outputPath)` - **KEPT**: Important note about deferred validation
- [-] Line 119: `t.Logf("Command failed unexpectedly for %s: %v", tc.outputPath, err)` - **KEPT**: Useful diagnostic for unexpected failures
- [-] Line 167: `t.Logf("Command failed as expected (symlink protection): %v", err)` - **KEPT**: Documents symlink protection behavior
- [-] Line 169: `t.Logf("Command succeeded - symlink validation may happen at output time")` - **KEPT**: Important note about deferred validation
- [-] Line 244: `t.Logf("Command failed as expected for %s: %v", tc.outputPath, err)` - **KEPT**: Documents expected failure cases
- [-] Line 247: `t.Logf("Command completed for %s but may fail at write time", tc.outputPath)` - **KEPT**: Important note about deferred validation
- [-] Line 253: `t.Logf("Expected potential failure for %s: %v", tc.outputPath, err)` - **KEPT**: Documents potential failure scenarios
- [-] Line 295: `t.Logf("Command failed as expected (likely due to system limits): %v", err)` - **KEPT**: Documents system-dependent behavior
- [-] Line 342: `t.Logf("Command failed: %v", err)` - **KEPT**: Useful diagnostic information
- [-] Line 358: `t.Logf("Output file not created (expected in current implementation): %v", err)` - **KEPT**: Documents current implementation behavior
- [-] Line 409: `t.Logf("Found %d files in temp directory", len(files))` - **KEPT**: Useful diagnostic for cleanup verification
- [-] Line 470: `t.Logf("Command failed unexpectedly: %v", err)` - **KEPT**: Useful diagnostic for unexpected failures
- [-] Line 478: `t.Logf("Command failed as expected: %v", err)` - **KEPT**: Documents expected failure cases
- [-] Line 480: `t.Logf("Command succeeded but validation may happen later")` - **KEPT**: Important note about deferred validation
- [-] Line 543: `t.Logf("Output file not created (expected in current implementation): %v", err)` - **KEPT**: Documents current implementation behavior

### test/performance/output_capture_test.go (6 instances)

- [-] Line 89: `t.Logf("Initial memory: %d bytes", initialMem.Alloc)` - **KEPT**: Performance metric useful for debugging
- [-] Line 90: `t.Logf("Final memory: %d bytes", finalMem.Alloc)` - **KEPT**: Performance metric useful for debugging
- [-] Line 91: `t.Logf("Memory increase: %d bytes", memIncrease)` - **KEPT**: Performance metric useful for debugging
- [-] Line 213: `t.Logf("Concurrent execution of %d commands took: %v", numCommands, duration)` - **KEPT**: Performance metric useful for debugging
- [-] Line 273: `t.Logf("Long-running command took: %v", duration)` - **KEPT**: Performance metric useful for debugging
- [-] Line 416: `t.Logf("Memory increase after %d iterations: %d bytes", iterations, memIncrease)` - **KEPT**: Performance metric useful for debugging

### internal/runner/privilege/race_test.go (2 instances)

- [-] Line 223: `t.Logf("All %d WithPrivileges calls completed successfully", numGoroutines)` - **KEPT**: Useful summary for concurrent test completion
- [-] Line 251: `t.Log("Thread safety test completed successfully")` - **KEPT**: Useful summary for test completion

### internal/runner/privilege/manager_test.go (3 instances)

- [-] Line 242: `t.Logf("Warning: Could not get current user: %v", err)` - **KEPT**: Warning for environment-specific issues, not a test failure
- [-] Line 254: `t.Logf("Warning: Could not get current user: %v", err)` - **KEPT**: Warning for environment-specific issues
- [-] Line 260: `t.Logf("Warning: Could not get primary group: %v", err)` - **KEPT**: Warning for environment-specific issues

### internal/runner/output_capture_integration_test.go (2 instances)

- [x] Line 111: ~~`t.Logf("Test completed: %s", tt.description)`~~ - **REPLACED**: Changed to `require.NoError()` for proper assertion
- [x] Line 230: ~~`t.Logf("Test completed successfully: %s", tt.description)`~~ - **REMOVED**: Redundant after `require.NoError()`

### internal/runner/cli/validation_test.go (1 instance)

- [-] Line 98: `t.Logf("ValidateConfigCommand() error = %v (acceptable)", err)` - **KEPT**: Documents acceptable alternative error types

### internal/runner/bootstrap/verification_test.go (4 instances)

- [-] Line 59: `t.Logf("InitializeVerificationManager() failed (expected in environments without default hash directory): %v", err)` - **KEPT**: Documents environment-dependent behavior
- [-] Line 61: `t.Logf("InitializeVerificationManager() succeeded (default hash directory exists)")` - **KEPT**: Documents environment-dependent behavior
- [-] Line 109: `t.Logf("InitializeVerificationManager() returned error (expected for non-existent default dir): %v", err)` - **KEPT**: Documents expected error conditions
- [-] Line 139: `t.Logf("InitializeVerificationManager() returned error (may be expected): %v", err)` - **KEPT**: Documents potentially expected error conditions

### internal/runner/output/file_test.go (4 instances)

- [-] Line 446: `t.Logf("Attacker's file descriptor became invalid after move: %v", readErr)` - **KEPT**: Documents security test behavior (TOCTOU protection)
- [-] Line 448: `t.Logf("Attacker's file descriptor returned no content")` - **KEPT**: Documents security test behavior
- [-] Line 454: `t.Logf("Attacker sees different content (safe): %q", attackerReadContent)` - **KEPT**: Documents security test behavior

### internal/runner/output/capture_test.go (1 instance)

- [-] Line 371: `t.Logf("Successful writes: %d, Current size: %d, Expected if all full: %d", ...)` - **KEPT**: Useful diagnostic for buffer behavior debugging

### internal/runner/config/loader_defaults_test.go (2 instances)

- [-] Line 108: `t.Logf("Global.VerifyStandardPaths: %v", cfg.Global.VerifyStandardPaths)` - **KEPT**: Useful diagnostic for configuration debugging
- [-] Line 110: `t.Logf("Commands[0].RiskLevel: %v", cfg.Groups[0].Commands[0].RiskLevel)` - **KEPT**: Useful diagnostic for configuration debugging

### internal/terminal/detector_test.go (1 instance)

- [-] Line 191: `t.Logf("IsTerminal() returned %v in test environment", result1)` - **KEPT**: Documents environment-dependent terminal detection behavior

### internal/runner/runner_test.go (2 instances)

- [x] Line 1809: ~~`t.Logf("Test %s: %s", tt.name, tt.description)`~~ - **REMOVED**: Redundant information already in test name
- [x] Line 1919: ~~`t.Logf("Test completed: %s", tt.description)`~~ - **REPLACED**: Changed to `require.NoError()` for proper assertion

### internal/safefileio/safe_file_test.go (3 instances)

- [-] Line 699: `t.Logf("Attacker's file descriptor became invalid after atomic move: %v", readErr)` - **KEPT**: Documents security test behavior (TOCTOU protection)
- [-] Line 702: `t.Logf("Attacker's file descriptor returned no content")` - **KEPT**: Documents security test behavior
- [-] Line 711: `t.Logf("Attacker's file descriptor sees different content (safe): %q", attackerReadContent)` - **KEPT**: Documents security test behavior

### internal/logging/safeopen_test.go (1 instance)

- [-] Line 220: `t.Logf("ValidateLogDir() error = %v", err)` - **KEPT**: Useful diagnostic for validation behavior

### internal/groupmembership/membership_common_test.go (1 instance)

- [-] Line 61: `t.Logf("Group %d has %d explicit members: %v", currentGID, len(members), members)` - **KEPT**: Useful diagnostic for environment-specific group membership

### internal/groupmembership/manager_test.go (7 instances)

- [-] Line 243: `t.Logf("Root user (UID 0) can safely write: %v", canWrite)` - **KEPT**: Documents platform-specific root user behavior
- [-] Line 264: `t.Log("File owner is allowed (is exclusive group member)")` - **KEPT**: Documents permission check result
- [-] Line 266: `t.Log("File owner is denied (not exclusive group member)")` - **KEPT**: Documents permission check result
- [-] Line 324: `t.Logf("Can read group writable file: %v", canRead)` - **KEPT**: Useful diagnostic for permission behavior
- [-] Line 352: `t.Logf("Write result: %v (err: %v), Read result: %v (err: %v)", writeResult, writeErr, readResult, readErr)` - **KEPT**: Useful diagnostic for complex permission scenarios
- [-] Line 384: `t.Logf("Non-member write result: %v, error: %v", canWrite, err)` - **KEPT**: Useful diagnostic for permission behavior
- [-] Line 445: `t.Logf("Permission %o: can write=%v, err=%v", tt.perm, canWrite, err)` - **KEPT**: Useful diagnostic for permission testing

### internal/filevalidator/validator_test.go (1 instance)

- [-] Line 276: `t.Logf("Warning: failed to restore original hash file: %v", err)` - **KEPT**: Warning for cleanup failure, not a test failure

### cmd/runner/integration_security_test.go (5 instances)

- [-] Line 56: `t.Log("Malicious config properly contains dangerous command - would require dry-run or security controls for safe execution")` - **KEPT**: Important security note about test setup
- [-] Line 423: `t.Logf("Security analysis completed: %s - Risk: %s, Target: %s", ...)` - **KEPT**: Useful diagnostic for security analysis
- [-] Line 432: `t.Logf("Analysis %d: Type=%s, Target=%s, SecurityRisk=%s", ...)` - **KEPT**: Useful diagnostic for security analysis
- [-] Line 439: `t.Logf("Dry-run protection verified: %s", tc.description)` - **KEPT**: Useful confirmation of security feature operation
- [-] Line 523: `t.Log("Successfully prevented access to unverified data - hash verification properly failed")` - **KEPT**: Useful confirmation of security feature operation

### Documentation files (4 instances - not actionable)

- docs/tasks/0025_command_output/03_detailed_specification.md: Lines 1568, 1570
- docs/tasks/0026_variable_expansion/maintenance_guide.md: Lines 333, 334
- docs/tasks/0007_verify_hash_all/03_specification.md: Line 1129

## Decision Criteria

### Removed (18 instances)
Instances were removed when:
- The log immediately follows a `require.*()` or `assert.*()` call with the same information
- The log provides no additional diagnostic value
- The log is a redundant "test completed" message

### Kept (59 instances)
Instances were kept when:
- They document environment-dependent or platform-specific behavior
- They provide performance metrics useful for debugging
- They document incomplete implementation status or deferred validation
- They provide diagnostic information about security test behavior (TOCTOU, permissions)
- They are warnings about non-critical issues that don't constitute test failures
- They document acceptable alternative outcomes in flexible validation scenarios
- They provide useful debugging information for complex concurrent or permission scenarios

## Notes

- Documentation file instances (4) are not included in the actionable count
- All code modifications maintain existing test assertions while eliminating redundancy
- Security and performance tests retain more logging due to their diagnostic value
