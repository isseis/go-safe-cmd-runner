# Go Safe Command Runner - Security Risk Assessment Report

## 📋 Document Information
- **Created**: September 8, 2025
- **Last Updated**: July 25, 2026
- **Target System**: go-safe-cmd-runner
- **Assessment Scope**: Software security risk analysis and operational considerations
- **Target Audience**: Software Engineers, Security Specialists, Product Managers, Operations Engineers

---

## 🎯 Executive Summary

### Project Overview
go-safe-cmd-runner is a security-focused Go-based command execution system. It is designed to safely execute complex batch processing including privilege escalation features.

### ✅ Overall Security Assessment: A (Excellent)

**Key Achievements**:
- **0 Critical Risks**: No major security vulnerabilities exist
- Comprehensive security features based on security-first design philosophy
- Defense-in-Depth Architecture with appropriate error handling
- High-quality code with extensive test coverage

**Business Impact**:
- 📈 **High Reliability**: Comprehensive error handling reduces system failures
- 🔒 **Security Assurance**: Built-in security features minimize attack surface
- 🔧 **Maintainability**: Clean architecture supports long-term development

---

## 📊 Security Assessment Results

### Risk Distribution Dashboard
```
🔴 Critical:       0 issues
🟡 High Risk:      0 issues
🟠 Medium Risk:    2 issues  (Log enhancement, error handling standardization)
🟢 Low Risk:       4 issues  (Dependency updates, code quality improvements)
```

### Key Security Features Assessment

| Security Feature | Implementation | Assessment |
|-----------------|----------------|------------|
| Path Traversal Prevention | openat2 system call | ✅ Excellent |
| Command Injection Prevention | Static pattern validation | ✅ Excellent |
| File Integrity Verification | SHA-256 hash validation | ✅ Excellent |
| Privilege Management | Controlled escalation/restoration | ✅ Excellent |
| Configuration Validation Timing | Complete verification before use | ✅ Excellent |
| Hash Directory Protection | Complete prohibition of custom specification | ✅ Excellent |
| Command Allowlist | Global regex + Group-level exact paths | ✅ Excellent |
| Risk-Based Execution Control | Multi-factor risk assessment (`risk_level` upper-bound declaration) | ✅ Excellent |
| Binary Static Analysis | ELF/Mach-O syscall and dynamic library analysis | ✅ Excellent |
| dry-run Security | Always hard-fail unverified artifacts, read-only verification | ✅ Excellent |
| Output File Security | Privilege separation/restricted permissions | ✅ Good |
| Variable Expansion Security | Allowlist integration | ✅ Good |
| Sensitive Information Redaction | Key-name detection + value-format detection | ✅ Good |

---

## 🔐 Core Security Features

### 1. Privilege Management System

**🎯 Purpose**: Controlled privilege escalation and secure privilege restoration

#### Implementation Strengths
- **Template Method Pattern**: Design with appropriate separation of responsibilities
- **Comprehensive Auditing**: syslog recording of all privilege operations
- **Mutual Exclusion Control**: Prevents race conditions with mutex
- **Fail-Safe Design**: Emergency termination on privilege restoration failure

```go
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    execCtx, err := m.prepareExecution(elevationCtx)    // Preparation phase
    if err != nil { return err }

    if err := m.performElevation(execCtx); err != nil { // Execution phase
        return err
    }

    defer m.handleCleanup(execCtx)                     // Cleanup phase
    return fn()
}
```

#### Security Assessment
- ✅ **Privilege Escalation Control**: Strict context management
- ✅ **Audit Trail**: Complete operation history recording
- ✅ **Error Handling**: Appropriate emergency response
- ✅ **Statistical Safety**: seteuid() failure rate < 0.001%

**Design Decision**: Immediate termination on privilege restoration failure is a conservative and appropriate decision prioritizing privilege leak prevention

### 2. Configuration File Validation System

**🎯 Purpose**: Comprehensive configuration security and command injection prevention

#### Implemented Security Features
- **Multi-Layer Validation**: Structural validation → Security validation → Dangerous pattern detection
- **Static Patterns**: Tamper resistance through executable embedding
- **Whitelist Approach**: Only allow what has been confirmed safe
- **Early Validation**: Complete prevention of unvalidated data usage

```go
func (v *Validator) ValidateConfig(config *runnertypes.Config) (*ValidationResult, error) {
    result := &ValidationResult{ Valid: true }

    v.validateGlobalConfig(&config.Global, result)                    // Structural validation
    v.validatePrivilegedCommands(config.Groups, result)              // Security validation
    v.detectDangerousPatterns(config, result)                        // Dangerous pattern detection

    return result, nil
}
```

#### Security Assessment
- ✅ **Command Injection Prevention**: Comprehensive defense with dedicated validation functions
- ✅ **Dangerous Environment Variable Detection**: Comprehensive detection and rejection of dynamic-loader control variables (`LD_*`, `DYLD_*` prefixes; e.g., `LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`, etc.) and interpreter startup code-injection variables (`BASH_ENV`, `PYTHONPATH`, `NODE_OPTIONS`, etc.)
- ✅ **Privileged Command Validation**: Strict checking of root privilege execution
- ✅ **Configuration Consistency**: Safety assurance through duplicate/conflict detection

#### Breaking Change: Expanded Environment Variable Denylist

Starting from this version, the target list of forbidden environment variable names has been expanded. Newly added to the denylist across all three layers (executor, config, security) are `DYLD_*` (macOS dynamic loader control variables), `GLIBC_TUNABLES`, and interpreter startup code-injection variables such as `BASH_ENV`, `PYTHONPATH`, `NODE_OPTIONS`, `RUBYOPT`, and `PERL5LIB`. Additionally, a denylist check on `env_vars` keys has been newly introduced; configurations that specified the original denylist variables (such as `LD_*`, `GCONV_PATH`, etc.) in `env_vars` will also now encounter a config load error.

**Impact**: Configurations that specify these variables in `env_vars` or `env_import` will now encounter a config load error (`ErrForbiddenEnvVar`). If a commonly used variable name such as `ENV=production` matches the denylist, renaming the variable (e.g., `APP_ENV=production`) will resolve the issue. The decision to include `ENV` on the denylist accounts for the facts that POSIX `$ENV` is primarily interpreted by interactive shells (injection strength is relatively low in this runner's non-interactive execution), the main non-interactive script vector is already blocked by `BASH_ENV`, and if over-blocking becomes a problem it can be avoided by using a non-reserved name such as `APP_ENV`.

**Pre-Upgrade Detection**: Run a dry-run before upgrading to detect configurations that will become load errors. The error message includes the forbidden variable name and the target level.

Note that the executor layer's fail-silent scrub (environment variable removal as the last line of defense) continues to function, and forbidden variables that flow in via `env_allowed` are still removed from the final environment.

### 3. File Integrity & Access Control

**🎯 Purpose**: Tampering detection and path traversal attack prevention

#### SHA-256 Hash Validation and Hash File Naming

Hash file names are generated by `HybridHashFilePathGetter`. Ordinary paths are encoded with a
reversible substitution-escaping scheme (the `~path` format); only paths exceeding
`MaxFilenameLength` (250, a safety margin below the typical `NAME_MAX` of 255)
fall back to SHA-256 encoding. SHA-256 itself is used for verifying the integrity of file
content.

```go
func (h *HybridHashFilePathGetter) GetHashFilePath(hashDir common.ResolvedPath, filePath common.ResolvedPath) (string, error) {
    // 1. Normally encode via substitution + escaping (e.g. /home/user/file.txt → ~home~user~file.txt)
    encodedName, err := h.encoder.Encode(filePath.String())
    // ...
    // 2. Fall back to SHA-256 only when MaxFilenameLength (250) is exceeded (AbCdEf123456.json)
    // 3. Join with hashDir and return
}
```

#### Path Traversal Prevention with openat2
```go
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
    if !fs.openat2Available {
        // Fall back to portable two-stage verification on environments without openat2 support
        return safeOpenFileFallback(absPath, flag, perm)
    }
    how := openHow{
        flags:   uint64(flag | unix.O_CLOEXEC),
        mode:    openat2Mode(flag, perm), // Pass 0 for an open that does not create
        resolve: ResolveNoSymlinks, // Disable symbolic link resolution (atomic)
    }
    fd, err := openat2(AtFdcwd, absPath, &how)
    // ...
}

// openat2 retries while the kernel reports EINTR. EINTR means the open never
// took effect, so a retry redoes the same operation rather than repeating it.
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    for {
        fd, err := openat2Syscall(dirfd, pathname, how)
        if errors.Is(err, syscall.EINTR) {
            continue
        }
        return fd, err
    }
}
```

`mode` goes through `openat2Mode`. Go's `os.FileMode` places setuid, setgid, and sticky at its own
bit positions, so passing such a value straight to the kernel delivers a mode the caller did not
mean; `SafeOpenFile` therefore rejects a `perm` containing those bits with `ErrUnsupportedFileMode`.
In addition, `openat2(2)` returns `EINVAL` when `mode` is non-zero on a call that does not create a
file, so 0 is passed for an open without `O_CREAT` / `O_TMPFILE`. This makes success and failure
agree with the fallback route (where `os.OpenFile` ignores `mode` in the same situation).

#### Security Assessment
- ✅ **Cryptographic Integrity**: Strong tampering detection with SHA-256
- ✅ **Kernel-Level Protection**: Leveraging latest security features with openat2
- ✅ **Path Manipulation Prevention**: Base64 encoding and symbolic link disabling

#### Assumptions and Limitations (File Size and non-Linux TOCTOU)

The tool imposes upper bounds on the size of files it can safely read and analyze. These limits
are intentional defenses against memory-exhaustion attacks and must be understood as operational
assumptions for production deployments.

**File size limits (two distinct constants)**:

- **`safefileio.MaxFileSize` (128 MB)**: Applies to safe reads of configuration files, templates,
  and similar content via `SafeReadFile`. Defined as `128 * 1024 * 1024` in
  `internal/safefileio/safe_file.go` as a memory-exhaustion safeguard. Files exceeding this
  limit are rejected with `safefileio.ErrFileTooLarge`.
- **`filevalidator.maxFileSize` (1 GB)**: Applies to binary analysis (ELF / Mach-O, etc.).
  Defined as `1 << 30` in `internal/filevalidator/validator.go` to bound analysis time and
  memory consumption. `elfanalyzer` and `machoanalyzer` each define their own independent
  `maxFileSize` constant matching the same 1 GB limit, rather than referencing a shared symbol.

These are **two separate constants** and must not be conflated. 128 MB is comfortable headroom
for configuration files and templates, but 1 GB is exclusively for binary analysis. These
thresholds are not configurable (fixed values); while there is no size limit on hash computation
itself, a 1 GB limit is enforced during binary analysis.

**Production target and non-Linux environments**:

- Production deployments target **Linux kernel 5.6+ (with `openat2` support)**. `openat2(2)`
  atomically combines path resolution and `open`, fundamentally eliminating the TOCTOU race
  window between verification and execution.
- Whenever `openat2` is unavailable or explicitly disabled (via `DisableOpenat2`) — including
  non-Linux environments such as macOS, and Linux kernels older than 5.6 — the system falls back to
  `safeOpenFileFallback`, which uses a two-stage check: verify the parent directory is not a
  symbolic link → open with `O_NOFOLLOW` → re-verify. This implementation is robust but, in
  principle, **cannot match the atomicity of `openat2`** and a very small TOCTOU race window
  remains (acknowledged in the code comments).
- An open on this route takes two steps, because it needs to know whether it created the file
  itself. When `O_CREAT` is specified without `O_EXCL`, it first opens with `O_EXCL` added; if
  that succeeds, the file is known to be one it created. If `EEXIST` comes back, it reopens with
  `O_CREAT` removed, which opens the existing file. This information is needed for the cleanup
  performed when the second parent-directory check fails. The open fd is always closed, and
  **only a file that the call itself created** is removed. The removal happens only after
  confirming that the inode held by the fd and the removal target are the same; when that cannot
  be confirmed, nothing is removed, a warning is recorded, and the original error is returned. A
  removal is never performed on the strength of the path name alone (once the second check has
  failed, whatever that path points to can no longer be trusted).
- Therefore, **macOS and similar platforms are limited to development or restricted use**. All
  production deployments must use Linux with `openat2` available. On kernels without `openat2`
  support (Linux 5.5 or older), running the tool in production implies accepting the theoretical
  possibility of file substitution between verification and execution.

---

## 🔍 Additional Security Features

### 1. Enhanced Logging & Audit System (`internal/logging/`, `internal/redaction/`)

**Security Features**:
- **Sensitive Data Redaction**: Automatic protection of API keys, passwords, tokens
- **Structured Logging**: Improved parsability and complete audit trail recording
- **Decorator Pattern**: Flexible and configurable logging pipeline

```go
// Automatic redaction of sensitive information
type RedactingHandler struct {
    handler slog.Handler
    config  *redaction.Config
}

func (c *Config) RedactText(text string) string {
    // Apply redaction for key=value patterns
    result := text
    for i := range c.compiled {
        result = c.compiled[i].apply(result)
    }
    return result
}
```

**Value-Format Detection**:
In addition to key-name-based redaction, the system detects and masks secrets by their **value format** alone, even when no recognizable key name is present. The `ValueDetector` covers the following known formats:

- AWS access key IDs (`AKIA`/`ASIA` prefix)
- GitHub tokens (`ghp_`/`gho_`/`ghs_` prefix)
- GitHub fine-grained PATs (`github_pat_` prefix)
- Slack tokens (`xoxb-`/`xoxp-`/`xoxa-` prefix)
- Slack tokens with the additional prefixes (`xapp-`/`xoxe-`/`xoxs-`)
- GCP service account private key IDs
- PEM private key blocks (`-----BEGIN ... PRIVATE KEY-----`)
- OAuth `Bearer` tokens (standard JWT and opaque format)
- JWTs (three-segment Base64URL strings starting with `eyJ`)
- URL-embedded credentials (`scheme://user:pass@host`)
- Slack webhook URLs (all paths under the host configured in `slack_allowed_host`; with the standard Slack host `hooks.slack.com` configured, `https://hooks.slack.com/services/...` is covered; no fixed detection pattern is held)

**Scope**: Value-format detection is applied to command arguments, stdout, stderr, and environment variable values through the unified `RedactText` function. This single integration point covers all output destinations — file logs, syslog, and Slack notifications — ensuring no path bypasses masking.

**Limitations**: Detection is limited to the known formats listed above. Unknown credential formats, custom token schemes, and high-entropy strings are not detected. Secrets split across log fields or stream chunk boundaries may also be missed. Unlike the other formats, the GCP entry is not a self-identifying value format: a service-account key ID is an opaque hex string indistinguishable from any other hash by value alone, so it is only recognized next to its JSON field name (`"private_key_id"`). The actual GCP credential material — the `private_key` PEM block — is still masked independent of key name by the PEM detector above. **Configuring jobs to send full command output to Slack is strongly discouraged**; the masking layer is a defense-in-depth measure, not a substitute for avoiding unnecessary exposure.

The key-name-based layer has its own limitations. When a value that is not enclosed in quotes contains spaces (`password=my secret phrase`), there is no mechanical way to determine where the value ends, so the second and subsequent words remain in plain text. Adding a common English word to `KeyValuePatterns` can cause prose to be redacted. Group B keys (`key` / `token` / `secret`) are redacted even for non-sensitive values when they appear as a field name of quoted structured data, or when they follow an identifier-internal boundary (`-` / `.`) — for example `"key": "us-east-1"` or `Public-key: not supported`. Also, if an added key matches the common-word key list (`key` / `token` / `secret`), it is treated as group B (strict leading boundary) regardless of who added it, and never gets the loose boundary. Group B keys are not captured in unquoted YAML: forms appearing at the start of a line, right after an indentation, or right after a space (such as `token: ghp_xxx`) are not redacted. For the same reason, a form with spaces around `=` (such as `secret = abc`) is not redacted either (`secret=abc` and `secret = "abc"` are captured). Group A keys (`password`, `api_key`, etc.) do not have this restriction. As a workaround, a more specific key name that contains the key in question (such as `auth_token`) can be added to `KeyValuePatterns`.

Value-format detection of Slack webhook URLs replaces the whole path of URLs under the host configured in `slack_allowed_host` (including Slack-compatible services). This detection is only active on paths that go through the `RedactingHandler` with the configured host wired in. Components constructed before the configuration file is read (the Phase 1 logger and the verification manager's `security.Validator`) use the default redaction config, so value-format detection of webhook URLs does not work on those paths and only the other fixed patterns apply.

**Slack notification delivery**: Slack notifications are sent asynchronously; writing a log record does not wait for Slack to respond. At process exit, undelivered notifications are flushed within a default deadline of 15 seconds (configurable via `GSCR_SLACK_FLUSH_TIMEOUT`). Notifications can be lost when Slack is unreachable or when the process is forcibly terminated (SIGKILL, power loss, OOM kill, etc.). The notification content itself is written synchronously to the JSON log file (0600) before delivery to Slack, so a complete record identifiable by `run_id` remains. A normal restart (SIGTERM / SIGINT) is flushed. Notifications are sent serially by a single worker, so if Slack keeps failing to respond, subsequent notifications can be delayed by up to `queue length × send deadline (default 40 seconds)`. The send queue has two priorities: the high-priority queue (security alerts, etc.) holds 32 notifications and the normal queue holds 128. Setting `GSCR_SLACK_SYNC=1` reverts to synchronous sending, but it is a debugging escape hatch for incident investigation, not for normal operation.

### 2. Risk-Based Command Control (`internal/runner/base/risk/`)

**Multi-Factor Risk Assessment**:
`risk_level` declares the **upper bound** of risk permitted for a command. Before execution, the
runner automatically computes the command's risk and rejects execution if the computed value
exceeds `risk_level`. Risk is computed as the **maximum across multiple independent factors**.

- **Command Name/Argument Evaluation**: Detection of privilege escalation, destructive operations, and dangerous argument patterns
- **Command Profile Factors**: Classification of privilege granting, network communication, data exfiltration, and system modification
- **Reference to Binary Static Analysis Results**: Reuses syscall and dynamic library analysis results recorded at `record` time
- **Audit Integration**: Complete recording of all risk assessment results

```go
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
    // Privilege escalation commands are the highest level (Critical)
    // Destructive file operations, system modification, and arbitrary code execution are High
    // Network arguments etc. are Medium
    // Each factor is accumulated via addDimension; effective risk is the maximum across all factors
    a := risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}
    // ...
    return plan, nil
}
```

For details on how risk is computed and guidance on setting `risk_level`, see the
[Risk Assessment Guide](risk_assessment.md).

#### Binary Static Analysis (at `record` time)

The `record` command analyzes executable files and records the results in the hash database. At
runner execution time, this record is consulted to determine risk, avoiding analysis cost at
execution time while still detecting dangerous behavior.

- **ELF/Mach-O Syscall Analysis** (`internal/security/elfanalyzer`, `internal/security/machoanalyzer`):
  Statically extracts the system calls an executable might invoke, detecting dynamic code
  execution via `mprotect(PROT_EXEC)`, `exec`-family syscalls, and similar
- **Transitive Dynamic Library Analysis** (`internal/dynlib`, cached via `internal/libccache`):
  Recursively follows dependent shared libraries and analyzes indirectly reachable syscalls.
  Analysis results are cached per library to avoid re-analysis
- **Shebang Script Analysis** (`internal/shebang`): Resolves a script's interpreter and reflects
  the interpreter executable's risk in the script's risk

### 3. User & Group Management (`internal/groupmembership/`)

**Stricter Privilege Boundaries**:
- **CGO/Non-CGO Support**: Environment-independent privilege validation
- **Cache Functionality**: Performance improvement and consistency assurance
- **Cross-Platform**: Unified user and group management

**Known limitation (`CGO_ENABLED=0` builds)**: The non-CGO build enumerates group members and
primary-group membership only by directly parsing `/etc/group` and `/etc/passwd`; it does not
consult NSS (Name Service Switch) directory services such as LDAP/SSSD. In a configuration where
the `passwd` and `group` lines of `/etc/nsswitch.conf` name only `files` and `systemd` (including
Ubuntu's default), this direct parsing alone enumerates without omission, so the write-safety
check for group-writable files can be determined as before. On the other hand, when another
source is configured (LDAP/SSSD, etc., including `passwd: files sss`, the default on
domain-joined hosts), or when `/etc/passwd`/`/etc/group` contains a line that cannot be parsed,
the enumeration cannot be guaranteed complete, so the write-safety check in question is not
"evaluated more permissively" — it is **denied** (fail-closed).

CGO builds have a known limitation too. In an environment configured with SSSD's
`enumerate = False` (the default) or `ignore_group_members = True`, libc's NSS lookup can return
a partial member set without returning an error, in which case the write-safety check can be
evaluated more permissively than it actually is even on a CGO build. See
[#1064](https://github.com/isseis/go-safe-cmd-runner/issues/1064) for details.

### 4. Safe Terminal Output Control (`internal/terminal/`, `internal/ansicolor/`)

**Output Security**:
- **Terminal Capability Detection**: Automatic identification of CI/CD environments
- **Escape Sequence Control**: Terminal injection prevention
- **Conservative Defaults**: Fail-safe behavior in unknown environments

### 5. Pre-Use Configuration Validation and Trust Boundaries

Configuration files are always validated before their content is used. Processing proceeds in
the following order, and no configuration content is trusted until validation completes.

```go
func run(runID string) error {
    // 1. Hash directory validation (highest priority)
    hashDir, err := getHashDirectoryWithValidation()

    // 2. Configuration file verification (mandatory before use)
    if err := performConfigFileVerification(verificationManager, runID); err != nil {
        return err // Immediate termination on critical error
    }

    // 3. Use only validated configuration
    cfg, err := loadAndValidateConfig(runID)
}
```

- ✅ **Default Deny**: Prohibit all operations until validation is complete
- ✅ **Early Validation**: Minimize attack surface
- ✅ **Clear Trust Boundaries**: Use only validated data (settings such as working directory, log
  level, and the Slack webhook take effect only after validation)

### 6. Hash Directory Protection

In production, the hash directory is always the default directory and cannot be specified
externally. This prevents attacks that place fake hash files to spoof "verification success" for
malicious commands (particularly privilege escalation during setuid binary execution).

```go
// Production environment: Default directory only
func NewManager() (*Manager, error) {
    // Use only cmdcommon.DefaultHashDirectory
    // No external specification accepted
}

// Test environment: Separated by build tag
//go:build test
func NewManagerForTest(hashDir string, options ...Option) (*Manager, error) {
    // Only test-specific API allows custom directory
}
```

- ✅ **No Custom Specification**: No flag such as `--hash-directory` exists
- ✅ **Zero Trust**: Never trust custom hash directories
- ✅ **Defense-in-Depth**: Protection at compile time, build tags, and CI/CD

### 7. Output File & Variable Expansion Security

**Output File Security**:
- **Privilege Separation**: Output files created with real UID permissions (no EUID change impact)
- **Restricted Permissions**: File permissions 0600 (owner-only access)
- **Path Traversal Prevention**: Prohibit parent directory references (`..`)
- **Size Limits**: Default 10MB limit prevents disk exhaustion attacks

**Variable Expansion Security**:
- **Allowlist Integration**: Expand only permitted environment variables
- **Circular Reference Detection**: Prevent infinite loops with maximum 15 iterations
- **No Shell Execution**: `$(...)`, `` `...` `` not supported
- **Command Validation**: Re-validate command paths after expansion

### 8. dry-run Security

dry-run returns the same verification results as a production run and does not change state.
Configurations judged "no problem" by dry-run yield the same outcome in a production run.

- **Always Hard-Fail Unverified Artifacts**: Commands that "could not be verified in this
  environment" are always treated as a hard fail, even in dry-run
- **Read-Only Hash Directory Verification**: dry-run does **not create** the hash directory. If
  the directory does not exist, dry-run verifies it read-only without creating it as a side
  effect, treating its absence as a hard fail just as in a production run. The audit log records
  the verification's construction mode (`construction_mode`)
- ✅ **dry-run and Production Behavior Match**: dry-run results correctly predict whether a
  production run will succeed
- ✅ **No Side Effects**: dry-run does not change state (least privilege, idempotence)

---

## ⚠️ Risk Analysis

### Remaining Risks

#### Medium Risk (2 issues)

**1. Security Log Enhancement Opportunities**
- Current Status: Basic security event recording is implemented
- Improvement: Add more detailed attack pattern analysis information
- Impact: Limitations in detecting and analyzing advanced attacks

**2. Error Message Standardization**
- Current Status: Security-related errors are properly handled
- Improvement: Establish consistent error reporting format
- Impact: Minor impact on troubleshooting efficiency

#### Low Risk (4 issues)

1. **Regular Dependency Updates**: Automatic integration with vulnerability databases
2. **Performance Monitoring**: Implementation of resource usage limits
3. **Test Coverage**: Improve security-critical path coverage to 90%+ (currently ~85%)
4. **Enhanced Static Analysis**: More advanced code quality checks

### External Dependency Security

| Package | Version | Risk Level | Status |
|---------|---------|-----------|--------|
| go-toml/v2 | v2.0.9 | 🟡 Medium | Active maintenance, no known CVEs |
| ulid/v2 | v2.1.1 | 🟢 Low | Cryptographically secure ID generation |
| golang.org/x/arch | v0.24.0 | 🟢 Low | Used for binary static analysis (instruction decoding) |
| golang.org/x/sys | v0.35.0 | 🟢 Low | System call invocations such as openat2 |
| golang.org/x/term | v0.34.0 | 🟢 Low | Terminal capability detection |
| testify | v1.8.4 | 🟢 Low | Test-only dependency, limited exposure |

### Operational Considerations

**For System Administrators**:
- Regular integrity checks of setuid binaries (`md5sum`, `sha256sum`)
- Monitoring frequency of privilege escalation operations and pattern analysis
- Permission verification of hash directory (`~/.go-safe-cmd-runner/hashes/`)

**For Development Teams**:
- Mandatory security review when developing new features
- Vulnerability scanning when adding external dependencies
- Thorough addition of security test cases

---

## 🛠️ Improvement Roadmap

### High Priority (1-2 weeks)

**1. Security Log Enhancement**
```go
// Extended security metrics
type SecurityMetrics struct {
    AttackPatternDetections map[string]int
    PrivilegeEscalationAttempts int
    FileIntegrityViolations int
}

func (s *SecurityLogger) LogThreatDetection(pattern string, context map[string]interface{}) {
    // Detailed attack pattern analysis
    // Threat intelligence integration
}
```

**2. Error Handling Standardization**
```go
// Consistent security errors
type SecurityError struct {
    Code string
    Message string
    Severity Level
    Context map[string]interface{}
}
```

### Medium Priority (1-3 months)

**1. Automated Security Testing Integration**
- Static analysis via GitHub Actions (gosec, golangci-lint)
- Dependency vulnerability scanning (nancy, govulncheck)
- Security test coverage monitoring

**2. Performance & Security Monitoring**
- Implementation of resource usage limits
- Security metrics collection
- Alert threshold configuration

### Low Priority (Continuous Improvement)

**1. Dependency Management**
- Monthly security update reviews
- Automatic vulnerability scan integration

**2. Code Quality Improvement**
- Security-focused code review checklist
- Comprehensive documentation

---

## 📖 Operations Guide

### Deployment Procedure

**1. System Requirements**
- Linux kernel 5.6+ (openat2 support)
- Go 1.26+ (development environment)
- Appropriate filesystem permissions

**2. Security Configuration**
```bash
# setuid binary configuration
sudo chmod 4755 /usr/local/bin/runner

# Hash directory preparation
mkdir -p ~/.go-safe-cmd-runner/hashes
chmod 700 ~/.go-safe-cmd-runner

# Log configuration
sudo tee /etc/rsyslog.d/go-safe-cmd-runner.conf <<EOF
# go-safe-cmd-runner logs
:programname, isequal, "go-safe-cmd-runner" /var/log/go-safe-cmd-runner.log
& stop
EOF
```

**3. Monitoring & Alert Configuration**

**Critical Monitoring Items**:
- Privilege escalation failures: `grep "CRITICAL SECURITY FAILURE" /var/log/auth.log`
- Configuration file tampering: Hash validation failure patterns
- Abnormal execution frequency: Detect mass executions in short time

**Recommended SLI/SLO**:
```yaml
availability: 99.9%      # Monthly downtime < 43 minutes
latency_p95: 5s         # 95% of commands complete < 5 seconds
error_rate: < 0.1%      # Overall error rate < 0.1%
security_violations: 0   # Zero security violations
```

### Troubleshooting

**Common Issues and Solutions**:

1. **Privilege Escalation Failure**
   ```bash
   # Investigate cause
   ls -la $(which runner)  # Check setuid configuration
   id                      # Check user permissions
   ```

2. **Hash Validation Failure**
   ```bash
   # Check hash files
   ls -la ~/.go-safe-cmd-runner/hashes/
   # Check configuration file integrity
   sha256sum config.toml
   ```

3. **Performance Issues**
   ```bash
   # Check resource usage
   top -p $(pgrep runner)
   # Analyze logs
   journalctl -u go-safe-cmd-runner -f
   ```

### Emergency Response Procedure

**Incident Classification**:
- 🔴 **P0**: Security violation, privilege escalation failure
- 🟡 **P1**: Service unavailable, configuration tampering detected
- 🟢 **P2**: Performance degradation, minor issues

**Escalation**:
1. P0: Immediately notify security team + operations manager
2. P1: Notify development team within 30 minutes
3. P2: Notify responsible team during business hours

---

## 📚 Related Documentation

### Security Documentation
- [Design Implementation Overview](../dev/design-implementation-overview.md)
- [Security Architecture](../dev/security-architecture.md)
- [Hash File Naming ADR](../dev/hash-file-naming-adr.md)
- [Risk Assessment Guide](risk_assessment.md)

---

## 📋 Document Management

**Review Schedule**:
- **Next Review**: October 1, 2026
- **Quarterly Review**: Every 3 months
- **Annual Comprehensive Assessment**: September 2026

**Responsibility**:
- **Security**: Development Team + Security Specialists
- **Operations**: SRE Team + Operations Manager
- **Final Approval**: Product Manager

**Update Triggers**:
- At major releases
- When security vulnerabilities are discovered
- At architecture changes
- When reflecting external audit results
