# Go Safe Command Runner - Security Architecture Technical Document

## Overview

This document provides a comprehensive technical analysis of the security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

Go Safe Command Runner implements multiple layers of security controls to enable secure delegation of privileged operations and automated batch processing. The security model is built on defense-in-depth principles, combining file integrity verification, ELF binary static analysis, environment variable isolation, privilege management, and secure file operations.

## Key Security Features

### 1. File Integrity Verification

#### Purpose
Verify that executables and critical files have not been tampered with before execution, preventing the execution of compromised binaries. The system now provides centralized verification management via the `internal/verification/` package.

#### Implementation Details

**Hash Algorithm**: SHA-256 cryptographic hash
- Location: `internal/filevalidator/hash_algo.go`
- Uses Go's standard `crypto/sha256` library
- Provides 256-bit hash values for strong collision resistance

**Hash Storage System**:
- Hash files are stored as JSON manifests in a dedicated directory
- File paths are encoded using Base64 URL-safe encoding to handle special characters
- Manifest format includes file path, hash value, algorithm, and timestamp
- Collision detection prevents different file paths from mapping to the same hash manifest file when path hashes collide

**Verification Process**:
```go
// Location: internal/filevalidator/validator.go, the Verify() method
func (v *Validator) Verify(filePath string) error {
    // 1. Validate and resolve file path
    targetPath, err := validatePath(filePath)

    // 2. Calculate current file hash
    actualHash, err := v.calculateHash(targetPath.String())

    // 3. verifyHash() compares against the hash in the analysis record
    return v.verifyHash(targetPath, actualHash)
}
```

`verifyHash()` reads the analysis record (hash manifest) from `internal/fileanalysis` via `v.store.Load()`, checks that the recorded file path matches (hash collision detection), and compares an expected value in `algorithm:hash` format against the recorded `ContentHash`, returning `ErrMismatch` on mismatch.

**Read-Safety Check at `record` Time**:

`calculateHash()` (`internal/filevalidator/validator.go`) opens the file via `v.fileSystem.SafeOpenFile()`. This call internally succeeds only if it passes `internal/groupmembership`'s read-safety check (for group-writable files, this confirms that the permission-check base UID (because `record` declares `SudoUIDAware` as its base UID policy, if the real UID is 0 and `SUDO_UID` is set to a numeric UID in range 0..MaxUint32, and that UID exists in the user database, that value is adopted; if its existence cannot be confirmed or `SUDO_UID` is not a valid number, the read-safety check fails. The real UID is adopted only when the real UID is non-zero or `SUDO_UID` is unset) belongs to the file's group) — it fails if the check does not pass.

This read-safety check is necessary because the match check that `verify`/`runner` later perform only looks at whether "the hash at `record` time matches the current hash." Suppose `record` read the file without going through this check. If an untrusted member of the group the file belongs to had already rewritten the content by that point, `record` would hash that tampered content and record it as the "correct baseline." From then on, `verify`/`runner` would keep finding that the tampered content matches the recorded hash, so this tampering could never be detected. In other words, this check is a defense that is effective only at the exact moment `record` fixes the trust baseline (`ContentHash`); it cannot be substituted by re-verification at execution time (the same reasoning applies to the analysis-result cache in §2).

**Centralized Verification Management**:
- Location: `internal/verification/manager.go`
- Unified interface for all file verification operations
- Automatic privilege escalation fallback for permission-restricted files
- Standard system path skipping capability

**Privileged File Access**:
- Falls back to privilege escalation if normal verification fails due to permissions
- Uses secure privilege management (see Privilege Management section)
- Location: `internal/filevalidator/privileged_file.go`

#### Security Guarantees
- Detects unauthorized modifications to executables and configuration files
- Prevents execution of tampered binaries
- Cryptographically strong hash algorithm (SHA-256)
- Atomic file operations to prevent race conditions

### 2. ELF Binary Static Analysis and Interpreter Tracking

#### Purpose
At `record` command execution time, performs static analysis of ELF and Mach-O binaries to record dangerous system call patterns, network capability usage, dynamic library dependencies, and script interpreters. The runner uses the stored data to verify the integrity of dynamic libraries, eliminating the need for runtime ELF re-analysis.

The responsibility split here is as follows. `record` stores static-analysis results as normalized features that are easy for the runner to consume. For network symbol analysis, for example, it uses `networkSymbols` to narrow the set of symbol names persisted to the record, but this is normalization of stored analysis results rather than the final execution or `risk_level` decision. The `runner` reads stored `detected_symbols`, `dynamic_load_symbols`, and `known_network_lib_deps`, re-derives categories when needed, and performs runtime risk evaluation.

#### Implementation Details

**Analysis flow in the record command** (`cmd/record/main.go`):

```go
// Each analysis component is injected via a filevalidator.ValidatorConfig struct literal
vCfg := filevalidator.ValidatorConfig{
    BinaryAnalyzer:    security.NewBinaryAnalyzer(runtime.GOOS),      // network symbol detection (socket, connect, bind, etc.)
    SyscallAnalyzer:   libccache.NewSyscallAdapter(syscallAnalyzer),  // syscall pattern analysis (x86_64 / arm64 support)
    LibcCache:         libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer),        // Linux: libc syscall wrapper symbol cache
    LibSystemCache:    libccache.NewMachoLibSystemAdapter(machoCacheMgr, fs),       // macOS: libSystem syscall symbol cache
    MachoSyscallTable: libccache.MacOSSyscallTable{},
    DebugInfo:         debugInfo,
}
// Recursive analysis of dynamic library dependencies (set only when applicable)
vCfg.ELFDynLibAnalyzer = d.elfDynlibAnalyzerFactory()
vCfg.MachODynLibAnalyzer = d.machoDynlibAnalyzerFactory()

validator, _ := d.validatorFactory(cfg.hashDir, vCfg)
```

**Analysis content**:
- **syscall analysis** (internal/security/elfanalyzer/): Supports both x86_64 and arm64 architectures. Enumerates SYSCALL instructions (0F 05) / SVC #0 and identifies syscall numbers via backward scanning. Detects mprotect/pkey_mprotect + PROT_EXEC combinations (equivalent to JIT code execution) as dangerous patterns. Detects exec-related syscalls (Linux: execve/execveat, macOS: execve/__mac_execve) and maps them to high-risk classification at runtime. Also analyzes Go wrapper calls (syscall.Syscall, etc.) in Pass 2 (requires Go 1.18+ for pclntab parsing). Branch convergence analysis tracks register copy chains to identify syscall numbers across conditional branches. Cache misses (e.g., ErrNoSyscallAnalysis) trigger a fallback to live analysis, though schema version mismatches (SchemaVersionMismatchError) require re-recording.
- **Network capability detection** (internal/security/binaryanalyzer/, internal/security/elfanalyzer/): Normalizes symbol names such as socket, connect, and bind through networkSymbols and produces detected_symbols / dynamic_load_symbols for later runner-side policy evaluation. For ELF binaries without undefined symbols (SHN_UNDEF), the analysis returns NoNetworkSymbols instead of StaticBinary.
- **Dynamic library dependency analysis** (`internal/dynlib/elfdynlib/`, `internal/dynlib/machodylib/`): Recursively analyzes ELF DT_NEEDED / Mach-O LC_LOAD_DYLIB to record the paths and hashes of all dependency libraries.
- **libc syscall cache** (`internal/libccache/`): On Linux, caches libc syscall wrapper symbols; on macOS, caches libSystem syscall symbols, enabling analysis of indirect syscall calls on both platforms.
- **shebang tracking** (`internal/shebang/`): Parses and records interpreter paths from `#!/bin/sh` (direct form) / `#!/usr/bin/env python3` (env form), etc.

**Analysis result persistence** (`internal/fileanalysis/`):

```
fileanalysis.Record (SchemaVersion = fileanalysis.CurrentSchemaVersion, currently 23)
  ├── ContentHash           // SHA-256 hash of the file
  ├── DynLibDeps            // List of dependency library paths and hashes ([]LibEntry)
  ├── SyscallAnalysis       // syscall analysis result
  │     ├── DetectedSyscalls   // list of detected syscalls (number, name, occurrences, determination method)
  │     ├── AnalysisWarnings   // analysis warnings (e.g., mprotect+PROT_EXEC detected)
  │     ├── ArgEvalResults     // syscall argument evaluation results (PROT_EXEC flag determination for mprotect)
  │     └── DeterminationStats // diagnostic statistics for syscall number determination methods
  ├── SymbolAnalysis        // network symbol analysis result
  ├── ShebangChain          // interpreter information for the shebang chain ([]ShebangChainEntry, for scripts)
  └── AnalysisWarnings      // non-fatal warnings from dynlib analysis
```

**Runner-side verification** (`internal/verification/manager.go`, the `verifyDynLibDeps()` method):

`verifyDynLibDeps()` operates roughly as follows.
1. It reads the recorded record via `LoadRecord()`, individually handling errors such as `ErrRecordNotFound` and `SchemaVersionMismatchError` (indicating the record predates dynlib support).
2. If `DynLibDeps` is recorded, after hash verification it also calls `verifyDynLibDepsResolution()` to check for replacement via search-path shadowing (verified hashes are cached).
3. It checks for dynamic library dependencies in Mach-O binaries as well, via `hasMachODynamicLibraryDeps()`, not just ELF.
4. For dynamically linked binaries with unrecorded `DynLibDeps`, it returns `dynlib.ErrDynLibDepsRequired` and requires re-recording.

For binaries with recorded DynLibDeps, verification is optimized by matching against the recorded hash list rather than re-analyzing the ELF at runtime.

Likewise, for network capability handling, the runner does not re-analyze the ELF at runtime. It reads the `detected_symbols` and `known_network_lib_deps` recorded by `record`, re-derives categories from symbol names, and maps that information to runtime policy decisions.

#### Security Guarantees
- Detection of dynamic library tampering (hash comparison of dependency libraries)
- Requires re-recording before execution if dependencies of dynamically linked binaries are not recorded
- Pre-detection and warning of dangerous syscall patterns (mprotect+PROT_EXEC)
- Pre-detection of exec-related syscalls and automatic high-risk classification
- Identification and visualization of binaries with network capabilities
- Detection of script interpreter tampering (shebang tracking)
- Support for analysis of indirect syscall calls via libc (libccache)

### 3. Environment Variable Isolation

#### Purpose
Implements strict allowlist-based filtering of environment variables to prevent information leakage and command injection attacks via environment manipulation.

#### Implementation Details

**Actual Location of Allowlist Checks**:
- `config.ProcessEnvImport` (`internal/runner/config/expansion.go`): performs allowlist matching for `env_import` declarations at config expansion time
- `executor.BuildProcessEnvironment` (`internal/runner/base/executor/environment.go`): filters by allowlist when building the child process environment
- The `internal/runner/base/environment` package provides system environment enumeration (`system_env.go`) and denylist checking (`denylist.go`); it does not handle allowlists

**3-Level Inheritance Model**:

1. **Global Allowlist**: Base environment variables available to all groups
2. **Group Override**: Groups can define their own allowlist, completely overriding global settings
3. **Inheritance Control**: Groups without explicit allowlist inherit global settings

**Inheritance Modes**:
- `InheritanceModeInherit`: Use global allowlist
- `InheritanceModeExplicit`: Use only group-specific allowlist
- `InheritanceModeReject`: Allow no environment variables (empty allowlist)

**Environment Variable Value Safety Validation**:
```go
// Location: internal/runner/base/security/environment_validation.go
// Shell metacharacters such as ; | $( > < are intentionally allowed because
// commands are executed directly (not via a shell), so these characters carry no injection risk.
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
    if strings.ContainsRune(value, '\x00') {
        return fmt.Errorf("%w: environment variable %s contains null byte",
            ErrUnsafeEnvironmentVar, key)
    }
    if strings.ContainsAny(value, "\n\r") {
        return fmt.Errorf("%w: environment variable %s contains newline or carriage return character",
            ErrUnsafeEnvironmentVar, key)
    }
    return nil
}
```

**Validated Variable Value Constraints**:
Because commands are executed directly without a shell, shell metacharacters (`;`, `|`, `$(...)`, `>`, `<`, etc.) carry no injection risk and are not validated. Only the following characters are rejected:
- Null byte: `\x00` (can be used to inject headers or corrupt structured output)
- Newline characters: `\n`, `\r` (same)

#### Security Guarantees
- Zero-trust environment variable model (allowlist only)
- Prevents environment-based command injection
- Group-level isolation of sensitive variables
- Validation of variable names (POSIX compliance and reserved prefix check)
- Validation of variable values for null bytes and newline characters (shell metacharacters are not validated because commands are executed directly, not via a shell)

### 4. Secure File Operations

#### Purpose
Provides symlink-safe file I/O operations to prevent symlink attacks, TOCTOU (Time-of-Check-Time-of-Use) race conditions, and path traversal attacks.

#### Implementation Details

**Modern Linux Security (openat2)**:
```go
// Location: internal/safefileio/safe_file_linux.go (Linux-only build file), the openat2() function
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // Atomically prevent symlink following with RESOLVE_NO_SYMLINKS flag
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**Fallback Security (Legacy Systems)**:
```go
// Location: internal/safefileio/safe_file.go, the ensureParentDirsNoSymlinks() function
func ensureParentDirsNoSymlinks(absPath string) error {
    // Step-by-step path validation from root to target
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // Does not follow symlinks
        if fi.Mode()&os.ModeSymlink != 0 {
            // Known OS-managed symlinks (e.g. /etc/mtab) are allowed as an exception;
            // resolve them via EvalSymlinks and continue validation
            if !common.IsAllowedOSManagedSymlink(currentPath) {
                return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
            }
            resolved, err := filepath.EvalSymlinks(currentPath)
            // Continue validation using resolved from here on
        }
    }
    return nil
}
```

Symlinks are rejected in principle, but known OS-managed symlinks present since installation (determined via `common.IsAllowedOSManagedSymlink()`) are allowed as an exception, with their resolved target treated as the validation subject.

**File Size Protection**:
- Maximum file size limit: 128 MB
- Prevents memory exhaustion attacks
- Controls write size with custom size-limiting writer

**Path Validation**:
- Requires absolute paths
- Path length limit (configurable, default 4096 characters)
- Validates regular file type
- Does not allow device files, pipes, or special files

#### Security Guarantees
- Atomic symlink-safe operations on modern Linux (openat2)
- Comprehensive path traversal protection
- Eliminates TOCTOU race conditions
- Protection against memory exhaustion attacks
- Secure file type validation

### 5. Privilege Management

#### Purpose
Enables controlled privilege escalation for specific operations while maintaining the principle of least privilege. It also provides comprehensive audit trails and two-layer defensive verification after restoration.

#### Implementation Details

**Unix Privilege Architecture**:
```go
// Location: internal/runner/base/privilege/unix.go
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    mu                 sync.Mutex  // Prevents race conditions
    osExit             func(code int)                      // Injectable os.Exit for testing
    identityVerifier   func() error                         // Verifies EUID==UID / EGID==GID (injectable for testing)
    readSavedIDs       func() (suid, sgid int, err error)   // Reads saved-set-uid/gid (injectable for testing)
}
```

**Privilege Escalation Process**:
`WithPrivileges` is divided into three stages: pre-execution preparation, escalation, and cleanup.

```go
// Location: internal/runner/base/privilege/unix.go
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()  // Global lock for thread safety
    defer m.mu.Unlock()

    // 1. Record saved-set-uid/gid and decide whether escalation is needed, based on the operation type
    execCtx, err := m.prepareExecution(elevationCtx)
    if err != nil {
        return err
    }

    // 2. Call syscall.Seteuid(0) only for operations that require escalation
    if err := m.performElevation(execCtx); err != nil {
        return err
    }

    // 3. Run restoration and verification together via defer
    defer m.handleCleanup(execCtx)
    return fn()
}
```

Whether escalation is needed is determined by `elevationCtx.Operation`: only `OperationUserGroupExecution` and `OperationFileValidation` escalate. For any other operation type, `prepareExecution` returns an `ErrUnsupportedOperationType` error, and `WithPrivileges` returns that error directly to the caller without calling `fn()`.

**Execution Modes**:

1. **Native Root Execution**: Running as root user (UID 0)
   - No privilege escalation required
   - Direct execution with full privileges

2. **setuid Binary Execution**: Binary with setuid bit set and root ownership
   - Uses `syscall.Seteuid(0)` for privilege escalation
   - Automatic privilege restoration after operation

**Defensive Verification After Restoration**:
`handleCleanup` handles panic recovery, while the actual privilege restoration and two-stage invariant check are performed by `restorePrivilegesAndVerify`, which it calls internally. If either check fails, `emergencyShutdown` is called.

```go
// Location: internal/runner/base/privilege/unix.go, the restorePrivilegesAndVerify() function
// 1. Verify EUID==UID / EGID==GID (an independent check that detects bugs in the restoration logic itself)
if err := m.identityVerifier(); err != nil {
    m.emergencyShutdown(err, fmt.Sprintf("identity_verification_failure_%s", shutdownContext))
}

// 2. Verify that saved-set-uid/gid have not changed across the restore
//    (structurally skipped on unsupported platforms via the originalSUID < 0 guard)
if execCtx.originalSUID >= 0 {
    suid, sgid, err := m.getReadSavedIDs()()
    if err != nil {
        m.emergencyShutdown(fmt.Errorf("failed to read saved-set IDs after restore: %w", err),
            fmt.Sprintf("saved_set_read_failure_%s", shutdownContext))
    }
    if suid != execCtx.originalSUID || sgid != execCtx.originalSGID {
        err := fmt.Errorf("saved-set-uid/gid changed after restore: "+
            "original suid=%d, sgid=%d; post-restore suid=%d, sgid=%d: %w",
            execCtx.originalSUID, execCtx.originalSGID, suid, sgid, ErrIdentityLeak)
        m.emergencyShutdown(err, fmt.Sprintf("saved_set_identity_verification_failure_%s", shutdownContext))
    }
}
```

The saved-set-uid/gid check is a stronger invariant than the EUID match check: it can detect cases where the EUID was correctly restored but the saved-set was left corrupted at the previous effective UID by a partial `seteuid` call.

**Security Validation**:
```go
// Location: internal/runner/base/privilege/unix.go
func isRootOwnedSetuidBinary(logger *slog.Logger) bool {
    // Verify setuid bit is set
    hasSetuidBit := fileInfo.Mode()&os.ModeSetuid != 0

    // Verify root ownership (essential for setuid to work)
    isOwnedByRoot := stat.Uid == 0

    // Verify non-root real UID (true setuid scenario)
    isValidSetuid := hasSetuidBit && isOwnedByRoot && originalUID != 0

    return isValidSetuid
}
```

**Emergency Shutdown Protocol**:
- Immediate process termination on privilege restoration failure, or on failure of either the EUID/EGID match check or the saved-set-uid/gid invariant check
- Recorded to both structured logging and stderr
- Security event recording with full context
- Prevents continued execution in compromised state

#### Security Guarantees
- Thread-safe privilege operations with global mutex
- Automatic privilege restoration with panic protection
- Two-layer defense via post-restoration EUID/EGID match verification and the saved-set-uid/gid invariant check
- Comprehensive audit logging of all privilege operations
- Emergency shutdown on security failures
- Supports both native root and setuid binary execution models
- Operations that do not require escalation, such as dry-run, never acquire privileges

### 6. Command Path Verification

#### Purpose
Validates command paths against a configurable allowlist and prevents execution of dangerous binaries, ensuring only authorized commands can be executed. Stops environment variable inheritance and uses a secure fixed PATH.

#### Implementation Details

**Secure PATH Environment Enforcement**:
```go
// Location: internal/common/secure_path.go
// common.SecurePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin:" + CoreutilsDir

// Does not inherit environment variable PATH, uses secure fixed PATH
pathResolver := NewPathResolver(common.SecurePathEnv)
```

**Path Resolution**:
```go
// Location: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv string          // Uses secure fixed PATH
    cache   map[string]string
    mu      sync.RWMutex
}
```

**Command Verification Process**:
1. Resolve command to full path using PATH environment variable
2. Validate against allowlist patterns (regex-based)
3. Check for dangerous privileged commands
4. Verify file integrity if hash is available

**Default Allowed Patterns**:
```go
// Location: internal/runner/base/security/types.go
// DefaultConfig() calls GenerateAllowedCommandsFromPath() to dynamically
// generate the default allowed patterns from common.SecurePathEnv (the secure fixed PATH)
allowedCommands, err := GenerateAllowedCommandsFromPath(common.SecurePathEnv)
```

The default allowed command patterns are not a fixed list; they are generated dynamically at runtime from the directory list in the secure fixed PATH (and the process panics if generation fails).

**Dangerous Command Detection**:
- Shell executables: `/bin/bash`, `/bin/sh`
- Privilege escalation tools: `sudo`, `su`, `doas`
- System administration: `rm`, `dd`, `mount`, `umount`
- Package management: `apt`, `yum`, `dnf`
- Service management: `systemctl`, `service`

#### Security Guarantees
- Allowlist-based command execution
- Prevents arbitrary command execution
- Detection of dangerous privileged operations
- Path resolution security validation
- Complete elimination of environment variable PATH inheritance
- Enforced use of secure fixed PATH (/sbin:/usr/sbin:/bin:/usr/bin)

### 7. Risk-Based Command Control

#### Purpose
Implements intelligent security controls based on command risk assessment, automatically blocking high-risk operations while allowing safe commands to execute normally.

#### Implementation Details

**Risk Assessment Engine**:
```go
// Location: internal/runner/base/risk/evaluator.go
// StandardEvaluator holds fields such as networkAnalyzer (network symbol
// analysis), openIdentity (a verified identity opener for fd-bound execution),
// zoning, and resolveRunAs (run-as-user resolution)
type StandardEvaluator struct {
    networkAnalyzer *security.NetworkAnalyzer
    openIdentity    identityOpener
    zoning          *zoningParams
    resolveRunAs    runAsResolver
}

// EvaluateRisk returns a VerifiedCommandPlan rather than a bare RiskLevel: the
// evaluated identity and the executed identity are bound together so the executor
// runs only the plan (the verified argv/env/identity), never the raw argv/env.
// The effective risk is the maximum across all applicable dimensions (profile,
// destructive, system-modification, dangerous-arg patterns, arbitrary-code
// runners, binary analysis); fail-closed gates short-circuit before the maximum
// is taken.
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
    // First, validate that the path is absolute (relative paths are denied immediately)
    if !filepath.IsAbs(cmdPath) {
        return blockingPlan(...), nil
    }
    // Then the identity gate: without a verified hash, or with binary analysis
    // disabled, the binary's identity cannot be confirmed, so deny (Blocking)
    // regardless of the configured risk_level. This runs before every risk
    // dimension so no path can confirm a Low/High-allowable risk for an
    // unverified binary.
    if blocked, ok := e.identityGate(cmd); ok {
        return blockingPlan(blocked), nil
    }
    // Indirect-execution resolution (wrappers, inline shells, loaders) can
    // itself deny or force Critical, then privilege escalation (sudo/su/doas)
    // -> Critical (always blocked). Finally coreutils classification, profile
    // factors, dangerous-arg patterns, arbitrary-code runners, and binary
    // analysis are folded into the dimension maximum.
}
```

**Indirect-execution resolver (wrapper inner commands)**: a wrapper (`env`/`timeout`/`nice`, etc.) has its inner command extracted and risk-assessed (extraction is retained). A privilege-escalation token is Critical, and forbidden forms (loader-control variables and interpreter startup code-injection variables, `env -C`, an uninterpretable `env -S`, find/xargs child-process execution, direct dynamic-loader invocation, remote-shell helpers, non-extractable wrappers, exceeding the depth limit, symlink-resolution failure) take priority as a **Blocking deny** (`IndirectReject`); any other extractable ordinary inner command is a flat High regardless of its content (no fine-grained computation). The runner does not re-implement wrapper semantics, and it does not fd-bind or automatically hash-verify the inner command (the inner is recorded in the execution plan for auditing, but that is not identity pinning; Task 0138). Only the shebang interpreter chain of a direct script execution keeps the fine-grained computation as before.

**Command Risk Analysis**:
- Low risk: Standard system utilities (ls, cat, grep)
- Medium risk: File modification commands (cp, mv, chmod), other system modifications (mount, crontab)
- High risk: Package management (apt, yum, dpkg), service/system administration (systemctl, service), destructive operations (rm -rf)
- Critical risk: Privilege escalation commands (sudo, su) - automatically blocked

**Risk Level Configuration**:
```go
// Location: internal/runner/base/runnertypes/spec.go
type CommandSpec struct {
    RiskLevel *string `toml:"risk_level"` // Risk level of the command (nil when unset)
}

// GetRiskLevel() returns RiskLevelLow as the default when RiskLevel is nil
func (c *CommandSpec) GetRiskLevel() (RiskLevel, error)
```

#### Security Guarantees
- Automatic blocking of privilege escalation attempts
- Configurable risk threshold per command
- Comprehensive command pattern matching
- Risk-based audit logging

### 8. Resource Management Security

#### Purpose
Provides secure resource management that maintains security boundaries in both normal execution and dry-run modes.

#### Implementation Details

**Unified Resource Interface**:
```go
// Location: internal/runner/resource/manager.go
// The interface is named Manager, not ResourceManager
type Manager interface {
    ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (CommandToken, *ExecutionResult, error)
    WithPrivileges(ctx context.Context, fn func() error) error
    SendNotification(message string, details map[string]any) error
    // Plus additional methods such as ValidateOutputPath / CreateTempDir /
    // CleanupTempDir / CleanupAllTempDirs / GetDryRunResults, for output-path
    // validation, temp-directory management, and dry-run result retrieval
}
```

**Execution Mode Security**:
- Normal mode: Full privilege management and command execution
- dry-run mode: Security analysis without actual execution
- Consistent security validation across both modes

#### Security Guarantees
- Mode-independent security validation
- Privilege boundary enforcement
- Secure notification handling
- Resource lifecycle management

### 9. Secure Logging and Sensitive Data Protection

#### Purpose
Prevents sensitive information such as passwords, API keys, and tokens from being exposed in log files, providing a safe audit trail without compromising sensitive data. Enhanced with dedicated redaction services to achieve comprehensive protection through a defense-in-depth approach.

#### Implementation Details

**Centralized Data Redaction Foundation**:
```go
// Location: internal/redaction/redactor.go
// Config is constructed exclusively via NewConfig, which validates and
// pre-compiles the patterns; its fields are unexported. Placeholder() exposes
// the substitution text.
type Config struct {
    placeholder      string
    patterns         *SensitivePatterns
    keyValuePatterns []KeyValuePattern // each element has a Literal to match in the input and a Kind that declares the rule (key-value / next token / header value)
    valueDetector    *ValueDetector    // value-based detection of AWS keys, GitHub tokens, PEM format, etc.
}

func (c *Config) RedactText(text string) string {
    // Apply all configured redaction patterns
}

func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    // Redact sensitive information in log attributes
}
```

**Two-Layer Defense Architecture**:

Sensitive data protection is implemented as a dual defense where if one layer has a gap, the other catches it.

**Layer 1: Redaction at CommandResult Creation** (`internal/runner/group_executor.go`):
```go
// Location: internal/runner/group_executor.go, the CommandResult construction path
// Redact sensitive information before storing command output into CommandResult
sanitizedStdout := ge.validator.SanitizeOutputForLogging(stdout)
sanitizedStderr := ge.validator.SanitizeOutputForLogging(stderr)
```
- `SanitizeOutputForLogging()` is implemented in `internal/runner/base/security/logging_security.go`
- Redacts sensitive information at the point of storing command output, preventing leakage to Slack notifications and other external services

**Layer 2: Redaction in RedactingHandler** (`internal/redaction/redactor.go`):
```go
// Location: internal/redaction/redactor.go, the RedactingHandler type
type RedactingHandler struct {
    handler       slog.Handler
    config        *Config
    failureLogger *slog.Logger // fallback logger to prevent recursion if redaction itself panics
}

// Location: internal/runner/bootstrap/logger.go
// NewRedactingHandler takes 3 arguments including failureLogger, and WithErrorCollector chains an error collector
redactedHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger).WithErrorCollector(collector)
logger := slog.New(redactedHandler)
```
- Automatically redacts sensitive information at log output time
- Wraps all log handlers (file, syslog, Slack)
- Recursive processing of structured logs including `slog.KindGroup`
- Supports both key=value format and authentication header patterns

**Slack Notification Implementation**:
```go
// Location: internal/logging/slack_handler.go, the SlackHandler type
type SlackHandler struct {
    runID     string
    level     slog.Level
    attrs     []slog.Attr
    groups    []string
    isDryRun  bool                  // suppresses Slack notifications during dry-run execution
    levelMode SlackHandlerLevelMode // controls whether notification is required based on log level
    // sender owns the delivery machinery (send queues, worker goroutine,
    // counters). Handlers derived via WithAttrs / WithGroup share it by
    // pointer, so one webhook configuration has one worker no matter how many
    // derived handlers exist. It is nil for a handler built as a struct
    // literal and in dry-run mode.
    sender *slackSender
}
```
- Wrapped by RedactingHandler, so Layer 2 redaction is applied
- Command output is already redacted before storage via Layer 1 (CommandResult creation time), so it is redacted before notification
- Length limits on command output (stdout: 1000 characters, stderr: 500 characters)
- Notifications are delivered asynchronously: `Handle` only enqueues a message and returns without waiting for the HTTP send. A per-webhook worker goroutine performs the send with retries. `WithAttrs` / `WithGroup` derived handlers share the sender by pointer, so one webhook configuration has one worker no matter how many derived handlers exist. On process exit, `FlushSlackNotifications` flushes remaining notifications within a configurable deadline; send failures and drops are recorded to the failure logger.

**Log Security Configuration**:
```go
// Location: internal/runner/base/security/types.go, the LoggingOptions type
type LoggingOptions struct {
    // IncludeErrorDetails controls whether to include full error messages in logs
    IncludeErrorDetails bool `json:"include_error_details"`

    // MaxErrorMessageLength limits the length of error messages in logs
    MaxErrorMessageLength int `json:"max_error_message_length"`

    // RedactSensitiveInfo enables automatic redaction of sensitive patterns
    RedactSensitiveInfo bool `json:"redact_sensitive_info"`

    // TruncateStdout controls whether to truncate stdout in error logs
    TruncateStdout bool `json:"truncate_stdout"`

    // MaxStdoutLength limits the length of stdout in error logs
    MaxStdoutLength int `json:"max_stdout_length"`
}
```

**Sensitive Pattern Detection and Redaction**:
```go
// Location: internal/runner/base/security/logging_security.go, the redactSensitivePatterns() method
// Pattern definitions and matching/replacement logic have been centralized into
// the internal/redaction package; this method now simply delegates
func (v *Validator) redactSensitivePatterns(text string) string {
    return v.redactionConfig.RedactText(text)
}
```

The actual pattern definitions for passwords, tokens, API keys, etc. (`password=`, `token=`, `key=`, `secret=`, `api_key=`, environment variable assignments such as `_PASSWORD=`, and authentication headers such as `Bearer `/`Basic `) are now centralized in `SensitivePatterns`/`Config.RedactText()` in `internal/redaction/redactor.go` (see the Centralized Data Redaction Foundation in "9. Secure Logging and Sensitive Data Protection").

**Error Message Sanitization**:
```go
// Location: internal/runner/base/security/logging_security.go, the SanitizeErrorForLogging() function
func (v *Validator) SanitizeErrorForLogging(err error) string {
    if err == nil {
        return ""
    }

    errMsg := err.Error()

    // Return generic message if error details should not be included
    if !v.config.LoggingOptions.IncludeErrorDetails {
        return "[error details redacted for security]"
    }

    // Redact sensitive information if enabled
    if v.config.LoggingOptions.RedactSensitiveInfo {
        errMsg = v.redactSensitivePatterns(errMsg)
    }

    // Truncate only if a length limit is configured and the message exceeds it
    if v.config.LoggingOptions.MaxErrorMessageLength > 0 && len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
        errMsg = errMsg[:v.config.LoggingOptions.MaxErrorMessageLength] + "...[truncated]"
    }

    return errMsg
}
```

**Output Sanitization**:
- Sanitizes command output to prevent credential leakage
- Configurable output length truncation
- Automatic pattern-based redaction of sensitive information
- Supports both key=value format and authentication header patterns

**Safe Log Functions**:
- `CreateSafeLogFields()`: Creates sanitized log field map
- `LogFieldsWithError()`: Combines base fields with sanitized error information
- Automatic detection and redaction of sensitive patterns in structured logs

#### Security Guarantees
- Dual defense via Layer 1 (CommandResult creation time) and Layer 2 (RedactingHandler)
- Even if redaction is missed in Layer 1, Layer 2 (RedactingHandler) catches it
- Detection and redaction of common sensitive patterns (passwords, tokens, API keys)
- Configurable log detail levels for different security environments
- Protection from credential exposure via error messages and command output
- Length-based truncation to prevent log file bloat and potential DoS
- Detection and sanitization of environment variable patterns
- Supports both key=value format and authentication header patterns (Bearer, Basic)

### 10. Terminal Capability Detection (`internal/terminal/`)

#### Purpose
Provides terminal capability detection functionality to detect color support and interactive execution environments, enabling selection of appropriate output formats.

#### Implementation Details

**Terminal Capability Detection Interface**:
```go
// Location: internal/terminal/capabilities.go
type Capabilities interface {
    IsInteractive() bool
    SupportsColor() bool
    HasExplicitUserPreference() bool
}
```

**Interactive Environment Detection**:
```go
// Location: internal/terminal/detector.go
type InteractiveDetector interface {
    IsInteractive() bool
    IsTerminal() bool // Checks for TTY environment or terminal-like environment
    IsCIEnvironment() bool
}
```

**Implementation Features**:
- **CI/CD Environment Detection**: Automatic detection of GitHub Actions, Travis CI, Jenkins, etc.
- **TTY Detection**: Checks stdout/stderr TTY connection status
- **Terminal Environment Heuristics**: Determines terminal-like environment via TERM environment variable
- **Color Support Detection**: Identifies color-capable terminals based on TERM value
- **User Configuration Priority**: Priority control of command-line arguments and environment variables

#### Security Characteristics
- **Conservative Default**: Disables color output for unknown terminals
- **Environment Variable Validation**: Proper parsing of CI environment variables
- **Configuration Priority Control**: Security-aware configuration inheritance

### 11. Color Management (`internal/ansicolor/`)

#### Purpose
Provides safe colored output based on terminal color support capability and proper management of color control sequences.

#### Implementation Details

**Color Function Type**:
```go
// Location: internal/ansicolor/color.go
// Color is a function type that wraps text with ANSI escape sequences
type Color func(text string) string

// NewColor creates a color function with the specified ANSI code
func NewColor(ansiCode string) Color {
    return func(text string) string {
        return ansiCode + text + resetCode
    }
}
```

**Color Support Detection**:
```go
// Location: internal/terminal/color.go
type ColorDetector interface {
    SupportsColor() bool
}
```

**Implementation Features**:
- **Known Terminal Pattern Matching**: Identifies color-capable terminals like xterm, screen, tmux, etc.
- **Conservative Fallback**: Disables color output for unknown terminals
- **TERM Environment Variable Parsing**: Color support determination based on terminal type
- **User Configuration Integration**: Priority control of terminal capability and user configuration

#### Security Characteristics
- **Conservative Approach**: Disables color output for unknown terminals to prevent escape sequence output
- **Verified Patterns**: Enables color only for known color-capable terminals
- **Safe Default**: Guarantees safe behavior when color support is unknown

### 12. Common Utilities (`internal/common/`, `internal/cmdcommon/`)

#### Purpose
Provides cross-package foundational functionality, guaranteeing testable, reproducible, and secure implementations.

#### Implementation Details

**Filesystem Abstraction**:
```go
// Location: internal/common/filesystem.go
type FileSystem interface {
    CreateTempDir(dir string, prefix string) (string, error)
    FileExists(path string) (bool, error)
    Lstat(path string) (fs.FileInfo, error)
    IsDir(path string) (bool, error)
    TempDir() string
    RemoveAll(path string) error
    Remove(path string) error
    CreateTemp(dir, pattern string) (*os.File, error)
    MkdirAll(path string, perm fs.FileMode) error
}
```

**Mock Implementation**:
- Provides mock filesystem for testing, enabling testing with security characteristics equivalent to production
- Supports testing of error conditions and boundary cases

#### Security Guarantees
- Consistent security behavior across implementations
- Comprehensive test coverage of security paths
- Type-safe interface contracts
- Mock implementations preserve security properties

### 13. User and Group Execution Security

#### Purpose
Provides secure user and group switching functionality while maintaining strict security boundaries and comprehensive audit trails.

#### Implementation Details

**User and Group Configuration**:
```go
// Location: internal/runner/base/runnertypes/spec.go
type CommandSpec struct {
    RunAsUser    string  `toml:"run_as_user"`    // User to run the command as
    RunAsGroup   string  `toml:"run_as_group"`   // Group to run the command as
    RiskLevel    *string `toml:"risk_level"`     // Risk level of the command (nil when unset)
}
```

**Group Membership Verification**:
```go
// Location: internal/groupmembership/manager.go
// A concrete struct, not an interface. Takes numeric uid/gid rather than username/groupname
type GroupMembership struct{ /* ... */ }

func New(opts ...Option) *GroupMembership

func (gm *GroupMembership) IsUserInGroup(uid, gid uint32) (bool, error)
func (gm *GroupMembership) GetGroupMembers(gid uint32) ([]string, error)
```

**Security Verification Flow**:
1. Validate user existence and permissions
2. Confirm group membership when group is specified
3. Check privilege escalation requirements
4. Apply risk-based restrictions
5. Execute command with appropriate privileges

#### Security Guarantees
- Comprehensive user and group validation
- Privilege escalation boundary enforcement
- Group membership confirmation
- Complete audit trail of user and group switching

### 14. Multi-Channel Notification Security

#### Purpose
Provides secure notification functionality for critical security events while protecting sensitive information in external communication.

#### Implementation Details

**Slack Integration**:
```go
// Location: internal/logging/slack_handler.go
type SlackHandler struct {
    runID     string
    level     slog.Level
    attrs     []slog.Attr
    groups    []string
    isDryRun  bool                  // suppresses Slack notifications during dry-run execution
    levelMode SlackHandlerLevelMode // controls whether notification is required based on log level
    sender    *slackSender          // shared delivery machinery; see internal/logging/slack_sender.go
}
```

**Secure Notification Handling**:
- Notifications are delivered asynchronously: `Handle` only enqueues a message and returns, and a per-webhook worker goroutine performs the HTTP send with retries. The worker is shared by all handlers derived via `WithAttrs` / `WithGroup`, keeping the goroutine count bounded.
- Wrapped by RedactingHandler, so sensitive data is automatically redacted (Layer 2)
- Command output is pre-redacted at CommandResult creation time, so it is already redacted before notification (Layer 1)
- Configurable notification channels
- Send failures and drops are written to the failure logger (a Slack-free handler chain), never back to Slack
- Secure webhook URL management

#### Security Guarantees
- Sensitive data protection in external notifications (dual-layer defense)
- Secure communication channel management
- Rate limiting to prevent abuse
- Comprehensive error handling

### 15. Command Execution Environment Isolation

#### Purpose
Prevents child processes from reading unexpected input and explicitly controls the execution environment to improve security and stability.

#### Implementation Details

**Standard Input Disabling**:
```go
// Location: internal/runner/base/executor/executor.go, the stdin setup logic
// Set up stdin to null device to prevent issues with commands that expect stdin
// This prevents "exit status 255" errors from docker-compose exec and similar commands
// that try to allocate a pseudo-TTY when stdin is nil (file descriptor -1)
devNull, err := os.Open(os.DevNull)
if err != nil {
    return nil, fmt.Errorf("failed to open null device for stdin: %w", err)
}
defer func() {
    if closeErr := devNull.Close(); closeErr != nil {
        e.Logger.Warn("Failed to close null device", "error", closeErr)
    }
}()
execCmd.Stdin = devNull
```

**Security Benefits**:
- Prevents child processes from reading unexpected input from stdin
- Prevents processing from being halted by interactive prompts
- Guarantees consistent behavior in batch processing environments
- Mitigates risk of malicious input injection attacks

**Stability Improvement**:
- Prevents errors in commands that try to allocate a pseudo-TTY when stdin is nil (such as docker-compose exec)
- Consistent behavior across platforms (using `os.DevNull`)

#### Security Guarantees
- Explicitly disables stdin input in all child processes
- Prevents processing halt or tampering via unexpected input
- Cross-platform support (Linux: `/dev/null`, Windows: `NUL`)

### 16. Resource Protection with Output Size Limits

#### Purpose
Limits command output size to prevent memory exhaustion attacks and disk space exhaustion, ensuring system stability and security.

#### Implementation Details

**Hierarchical Output Size Limits**:
```go
// Location: internal/common/output_size_limit.go
func ResolveOutputSizeLimit(commandLimit OutputSizeLimit, globalLimit OutputSizeLimit) OutputSizeLimit {
    // 1. Command-level output_size_limit (if configured)
    // 2. Global-level output_size_limit (if configured)
    // 3. Default output size limit (10MB)
}
```

**Default Configuration**:
```go
// Location: internal/common/output_size_limit_type.go, the DefaultOutputSizeLimit constant
// DefaultOutputSizeLimit is the default output size limit when not specified (10MB)
const DefaultOutputSizeLimit = 10 * 1024 * 1024
```

**Limit Enforcement**:
- Location: `internal/runner/output/capture.go`
- Limits output size with custom size-limiting writer
- Prevents limit violations with pre-write size checks
- Error detection and reporting when limit is exceeded
- Flexible limit configuration per command

**Configuration Hierarchy**:
1. **Command Level**: Can configure `output_size_limit` per individual command
2. **Global Level**: Default value applied to all commands
3. **Default**: 10MB (when not configured)
4. **Unlimited**: Can disable limit by setting value to 0 (requires caution)

#### Security Guarantees
- Protection from memory exhaustion attacks (DoS)
- Prevention of disk space exhaustion from excessive output
- Clear error messages when output size limit is exceeded
- Fine-grained control with flexible limit configuration per command

### 17. Configuration Security

#### Purpose
Ensures that configuration files and overall system configuration are not tampered with and follows security best practices.

#### Implementation Details

**File Permission Validation**:
```go
// Location: internal/runner/base/security/file_validation.go, the ValidateFilePermissions() function
// (in practice it also validates regular-file type and emits structured slog.Debug/Warn logging)
func (v *Validator) ValidateFilePermissions(filePath string) error {
    // Check for world-writable files
    disallowedBits := perm &^ requiredPerms
    if disallowedBits != 0 {
        return ErrInvalidFilePermissions
    }
    return nil
}
```

**Hash Directory Security Enhancement (Command-Line Argument Removal)**:
- The `--hash-directory` flag has been completely removed, and no wrapper function such as `getHashDir()` exists
- `cmd/runner/main.go` references `cmdcommon.DefaultHashDirectory` directly at each use site (always using only the default directory in production environments)

**Configuration File Pre-Verification**:

Before loading the configuration file, `main()` in `cmd/runner/main.go` calls `bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)`. Internally, this function calls `verificationManager.VerifyAndReadConfigFile(configPath)`, which performs hash verification and file reading atomically in a single read to prevent TOCTOU attacks.

```go
// Location: internal/runner/bootstrap/config.go, the LoadAndPrepareConfig() function
// Atomically verify and read the configuration file to prevent TOCTOU attacks
content, err := verificationManager.VerifyAndReadConfigFile(configPath)
if err != nil {
    return nil, &logging.PreExecutionError{
        Type:      logging.ErrorTypeFileAccess,
        Message:   err.Error(),
        Component: string(resource.ComponentVerification),
        RunID:     runID,
    }
}

// Load the configuration using the verified content
cfg, err := cfgLoader.LoadConfig(configPath, content)
```

Note that `verificationManager.VerifyGlobalFiles()` is called separately from `main()`, not for the configuration file itself, but for the global files (such as `verify_files`) that become known only after the configuration file has been loaded and expanded.

**Early Path Validation**:
```go
// Location: cmd/runner/main.go, the main() function
if !filepath.IsAbs(cmdcommon.DefaultHashDirectory) {
    logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig,
        fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", cmdcommon.DefaultHashDirectory),
        "file", runID)
    os.Exit(1)
}
```

**Directory Security Validation**:
- Complete path traversal from root to target
- Symlink detection in path components
- World-writable directory detection
- Group write restriction (requires root ownership)

**Configuration Verification Timing Improvement**:
- Execute hash verification before reading configuration file
- Completely eliminate system operation with unverified data
- Forced stderr output on verification failure (independent of log level settings)

**Hash Directory Configuration Security Enhancement**:
- Complete removal of `--hash-directory` command-line argument
- Always use default directory only in production environment
- Complete elimination of attack path via custom hash directory
- Maintain testability with test-environment-only API

**Configuration Integrity**:
- TOML format validation
- Required field validation
- Type safety enforcement
- Duplicate group name detection and environment variable inheritance analysis

#### Security Guarantees
- Prevention of configuration tampering
- Secure file and directory permissions
- Prevention of path traversal attacks
- Configuration format validation
- Tampering detection with configuration file pre-verification
- Complete elimination of hash directory attack path
- Strengthened early validation with absolute path requirement

## Security Architecture Patterns

### Defense-in-Depth

The system implements multiple security layers:

1. **Input Validation**: All inputs are validated at entry points (including absolute path requirement)
2. **ELF Binary Static Analysis**: Pre-detection of dangerous syscall patterns and network capabilities by the record command, tracking and hash verification of dynamic library dependencies
3. **Pre-Verification**: Hash verification of configuration files before use
4. **Path Security**: Comprehensive path validation and symlink protection, secure fixed PATH use
5. **File Integrity**: Hash-based verification of all critical files (configuration, executables, dependency libraries)
6. **Privilege Control**: Principle of least privilege with controlled escalation
7. **Environment Isolation**: Strict allowlist-based environment filtering, PATH inheritance elimination
8. **Command Validation**: Risk-based command execution control with allowlist verification
9. **Data Protection**: Dual-layer defense — automatic sensitive information redaction via Layer 1 (CommandResult creation time) and Layer 2 (RedactingHandler for all log output)
10. **User and Group Security**: Secure user and group switching with membership verification
11. **Hash Directory Security**: Complete prevention of custom hash directory attacks
12. **Execution Environment Isolation**: Prevention of unexpected input via stdin disabling
13. **Resource Protection**: Prevention of memory and disk exhaustion attacks with output size limits

### Zero-Trust Model

- No implicit trust in system environment
- All files are verified before use
- Environment variables are filtered by allowlist
- Commands are validated against known good patterns
- Privileges are granted only when needed and revoked immediately

### Fail-Safe Design

- Default deny for all operations
- Emergency shutdown on security failures
- Comprehensive error handling and logging
- Execution is denied when binary analysis / file verification is unavailable (fail-closed, not graceful degradation): a binary whose identity cannot be confirmed is never executed. Dry-run preview remains available.

### Audit and Monitoring

- Structured logging with security context
- Privilege operation metrics and tracking
- Security event recording
- Multi-channel reporting of critical errors

## Threat Model and Countermeasures

### Filesystem Attacks

**Threats**:
- Symlink attacks
- Path traversal
- TOCTOU race conditions
- File tampering
- System operation manipulation via malicious configuration files
- Verification bypass via custom hash directory

**Countermeasures**:
- openat2 with RESOLVE_NO_SYMLINKS
- Step-by-step path validation
- SHA-256 hash verification
- Atomic file operations
- Pre-hash verification of configuration files
- Fixed hash directory default (custom specification completely prohibited)

### Dangerous Binary Execution

**Threats**:
- Dynamic code execution using mprotect+PROT_EXEC (equivalent to JIT code injection)
- Process replacement/execution via exec-related syscalls (execve family)
- Unexpected external communication from binaries with network capabilities
- Behavior tampering via replacement of dynamic libraries (.so / dylib)
- Arbitrary code execution via script interpreter tampering

**Countermeasures**:
- Pre-detection of dangerous syscall patterns via ELF static analysis by the record command
- Pre-detection of exec-related syscalls and runtime escalation to high-risk policy
- Visualization of communication capabilities via network symbol analysis
- Hash recording of dynamic library dependencies and pre-execution verification
- Hash recording of shebang interpreters and pre-execution verification
- Requires re-recording before execution if DynLibDeps are not recorded for dynamically linked binaries

### Privilege Escalation

**Threats**:
- Unauthorized privilege acquisition
- Privilege persistence
- Race conditions in privilege handling

**Countermeasures**:
- Controlled privilege escalation
- Automatic privilege restoration
- Thread-safe operations
- Emergency shutdown on failure

### Environment Manipulation

**Threats**:
- Command injection via environment variables
- Information leakage via environment
- Privilege escalation via dynamic-loader control variables (`LD_PRELOAD`, `LD_LIBRARY_PATH`, `DYLD_INSERT_LIBRARIES`, `GLIBC_TUNABLES`, etc.) and interpreter startup code-injection variables (`BASH_ENV`, `PYTHONPATH`, `NODE_OPTIONS`, etc.)

**Countermeasures**:
- Strict allowlist-based filtering
- Group-level environment isolation
- Variable name validation (POSIX compliance)
- Variable value validation for null bytes and newline characters (shell metacharacter injection does not apply because commands are not executed via a shell)

### Command Injection

**Threats**:
- Arbitrary command execution
- Shell metacharacter exploitation
- PATH manipulation
- Privilege escalation via command manipulation
- Malicious binary execution via environment variable PATH
- Unexpected input injection via stdin

**Countermeasures**:
- Risk-based command validation with allowlist enforcement
- Full path resolution with security validation
- Shell metacharacter detection
- Command path validation
- Risk level enforcement and blocking
- User and group execution validation
- Complete elimination of environment variable PATH inheritance
- Enforced use of secure fixed PATH (/sbin:/usr/sbin:/bin:/usr/bin)
- Prevention of input injection attacks via stdin disabling

### Resource Exhaustion Attacks

**Threats**:
- DoS attacks via memory exhaustion
- Disk space exhaustion from excessive output
- Log file bloat
- System resource monopolization by long-running commands

**Countermeasures**:
- Output size limits (default 10MB, configurable)
- Prevention of long execution with timeout settings
- Log truncation settings (MaxStdoutLength, MaxErrorMessageLength)
- Hierarchical limit configuration (global, group, command level)
- Resource usage monitoring and alerting

## Performance Considerations

### Hash Calculation
- Efficient streaming hash calculation
- File size limit to prevent resource exhaustion

### Environment Processing
- O(1) allowlist lookup using map structure
- Pre-compiled regular expressions for pattern matching
- Minimal string operations

### Privilege Operations
- Global mutex prevents race conditions but serializes privilege operations
- Fast privilege escalation/restoration using system calls
- Metrics collection for performance monitoring

### Risk Assessment
- Pre-compiled regular expression patterns for efficient command analysis
- O(1) risk level lookup using pre-compiled patterns
- Minimal overhead for risk assessment
- Result caching for repeated command analysis

### Data Redaction
- Dual-layer defense via Layer 1 (CommandResult creation time) and Layer 2 (RedactingHandler)
- Pre-compiled patterns for sensitive data
- Minimal performance impact on normal operations
- Configurable redaction policy

### ELF Binary Analysis
- Analysis performed only at record command execution time (runner references stored data at runtime)
- For binaries with recorded DynLibDeps: no runtime ELF re-analysis needed (only matches against stored hash list)
- Avoidance of redundant analysis via libc syscall wrapper caching

### Resource Management
- Controls memory usage with output size limits
- Efficient limit implementation with custom size-limiting writer
- Prevents limit violations with pre-write size checks
- Flexible limit configuration per command
- Early detection and error reporting when limit is exceeded

## Deployment Security

### Binary Distribution
- Binary must have setuid bit set for privilege escalation
- Root ownership required for setuid functionality
- Verify binary integrity before deployment

### Configuration Management
- Hash directory must have secure permissions (755 or less)
- Configuration files should be write-protected
- Regular integrity verification of critical files

### Monitoring and Alerting
- Structured logging of security events
- Syslog integration for centralized logging
- Emergency shutdown events require immediate attention
- Slack integration for real-time security alerts
- Automatic sensitive data redaction in all monitoring channels

## Known Security Limitations

### TOCTOU (Time-of-Check to Time-of-Use) Race Condition

#### Resolution via fd-bound execution (fexecve-equivalent)

The TOCTOU race condition between command path validation (`ValidateCommandAllowed`) and actual command execution has been **structurally closed** for the primary execution path via fd-bound execution. The following describes the approach the current implementation takes.

Path resolution happens exactly once for the entire execution flow. `verifyGroupFiles` in `internal/runner/group_executor.go` resolves the command path to a symlink-resolved absolute path via `verificationManager.ResolvePath()`, and pins that resolved path into `cmd.ExpandedCmd`. From that point on, `executeCommandInGroup`, which runs immediately before execution, does not re-resolve the path. Re-resolving immediately before execution is deliberately avoided because it would reopen a TOCTOU re-resolution window between verification and execution.

`ValidateCommandAllowed` in `internal/runner/base/security/validator.go` assumes that the path passed to it has already been resolved by `PathResolver.ResolvePath()` at this point, and does not call `filepath.EvalSymlinks` (it performs only allowlist regex matching).

After that, `openVerifiedIdentity` in `internal/runner/base/risk/evaluator.go` opens the resolved path exactly once with `O_RDONLY|O_CLOEXEC`, recomputes the hash of that file descriptor's content, and compares it against the hash captured at verification time (a TOCTOU-safe identity check performed via the file descriptor). The resulting file descriptor is duplicated as the child process's fd 3 by `fdExecExtraFile` in `internal/runner/base/executor/fdexec_linux.go`, and `/proc/self/fd/3` is exec'd as the execution target. In other words, the kernel executes the exact inode that was verified, so replacing the symlink or file at the path-string level after verification has no effect on what actually gets executed.

#### Remaining Known Limitation (Shebang Interpreter)

The fd-bound execution described above covers the path of directly executed command binaries. For the **interpreter** referenced by a script's shebang line, however, `verifyInterpreterSymlinkTarget` checks the symlink's resolved target at verification time, but it is the kernel itself that re-resolves the interpreter path when the script is actually executed, and this window cannot be closed from application-level Go code (fd-binding the interpreter itself would require an `execveat`-equivalent mechanism, which is currently out of scope).

Exploiting this remaining gap requires an attacker to have filesystem write permissions that let them swap the interpreter's symlink with precise timing between verification and actual script execution. In an environment with properly restricted permissions, this precondition does not hold.

#### References

- [Safe programming. How to avoid TOCTOU vulnerability](https://stackoverflow.com/questions/41069166/)
- [CERT C Coding Standard: POS35-C](https://wiki.sei.cmu.edu/confluence/display/c/POS35-C.+Avoid+race_conditions+while+checking+for+the+existence+of+a+symbolic_link)
- [Wikipedia: Symlink race](https://en.wikipedia.org/wiki/Symlink_race)
- [Star Lab Software: Linux Symbolic Links Security](https://www.starlab.io/blog/linux-symbolic-links-convenient-useful-and-a-whole-lot-of-trouble)
- Related tasks: `docs/tasks/0090_toctou_fexecve/`, `docs/tasks/0155_toctou_verify_use_residual_gaps/`

## Conclusion

Go Safe Command Runner provides a comprehensive security framework for secure command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense-in-depth, zero-trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, ELF binary static analysis, risk-based command control, secure privilege management, automatic sensitive data protection, and extensive audit capabilities. The system is designed to fail safely and provide complete visibility into security-related operations.

Key security features include:
- ELF binary static analysis by the record command (detection of dangerous syscall patterns and network capabilities, hash recording of dynamic library dependencies)
- Intelligent risk assessment for command execution
- Unified resource management with consistent security boundaries
- Dual-layer defense — automatic sensitive data redaction via Layer 1 (CommandResult creation time) and Layer 2 (RedactingHandler for all log output)
- Secure user and group execution functionality
- Comprehensive multi-channel notifications with security-aware messaging
- Explicit control of execution environment via stdin disabling
- Prevention of resource exhaustion attacks with output size limits

The system provides enterprise-grade security controls while maintaining operational flexibility and transparency. ELF binary static analysis and dual-layer sensitive data redaction deliver comprehensive security countermeasures.
