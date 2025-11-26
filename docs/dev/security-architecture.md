# Go Safe Command Runner - Security Architecture Technical Document

## Overview

This document provides a comprehensive technical analysis of the security measures implemented in the Go Safe Command Runner project. It is intended for software engineers and security professionals who need to understand the design principles, implementation details, and security guarantees of the system.

## Executive Summary

Go Safe Command Runner implements multiple layers of security controls to enable secure delegation of privileged operations and automated batch processing. The security model is built on defense-in-depth principles, combining file integrity verification, environment variable isolation, privilege management, and secure file operations.

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
// Location: internal/filevalidator/validator.go:169-197
func (v *Validator) Verify(filePath string) error {
    // 1. Validate and resolve file path
    targetPath, err := validatePath(filePath)

    // 2. Calculate current file hash
    actualHash, err := v.calculateHash(targetPath.String())

    // 3. Read stored hash from manifest
    _, expectedHash, err := v.readAndParseHashFile(targetPath)

    // 4. Compare hashes
    if expectedHash != actualHash {
        return ErrMismatch
    }
    return nil
}
```


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

### 2. Environment Variable Isolation

#### Purpose
Implements strict allowlist-based filtering of environment variables to prevent information leakage and command injection attacks via environment manipulation.

#### Implementation Details

**Allowlist Architecture**:
```go
// Location: internal/runner/environment/filter.go:31-50
type Filter struct {
    config          *runnertypes.Config
    globalAllowlist map[string]bool // O(1) lookup performance
}
```

**3-Level Inheritance Model**:

1. **Global Allowlist**: Base environment variables available to all groups
2. **Group Override**: Groups can define their own allowlist, completely overriding global settings
3. **Inheritance Control**: Groups without explicit allowlist inherit global settings

**Inheritance Modes**:
- `InheritanceModeInherit`: Use global allowlist
- `InheritanceModeExplicit`: Use only group-specific allowlist
- `InheritanceModeReject`: Allow no environment variables (empty allowlist)

**Variable Validation**:
```go
// Location: internal/runner/config/validator.go
func (v *Validator) validateVariableValue(value string) error {
    // Use centralized security validation
    if err := security.IsVariableValueSafe(value); err != nil {
        // Wrap security error with validation error type for consistency
        return fmt.Errorf("%w: %s", ErrDangerousPattern, err.Error())
    }
    return nil
}
```

**Dangerous Pattern Detection**:
- Command separators: `;`, `|`, `&&`, `||`
- Command substitution: `$(...)`, backticks
- File operations: `>`, `<`, `rm `, `dd if=`, `dd of=`
- Code execution: `exec `, `system `, `eval `

#### Security Guarantees
- Zero-trust environment variable model (allowlist only)
- Prevents environment-based command injection
- Group-level isolation of sensitive variables
- Validation of variable names and values against dangerous patterns

### 3. Secure File Operations

#### Purpose
Provides symlink-safe file I/O operations to prevent symlink attacks, TOCTOU (Time-of-Check-Time-of-Use) race conditions, and path traversal attacks.

#### Implementation Details

**Modern Linux Security (openat2)**:
```go
// Location: internal/safefileio/safe_file.go:99-122
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
    // Atomically prevent symlink following with RESOLVE_NO_SYMLINKS flag
    pathBytes, err := syscall.BytePtrFromString(pathname)
    fd, _, errno := syscall.Syscall6(SysOpenat2, ...)
    return int(fd), nil
}
```

**Fallback Security (Legacy Systems)**:
```go
// Location: internal/safefileio/safe_file.go:409-433
func ensureParentDirsNoSymlinks(absPath string) error {
    // Step-by-step path validation from root to target
    for _, component := range components {
        fi, err := os.Lstat(currentPath) // Does not follow symlinks
        if fi.Mode()&os.ModeSymlink != 0 {
            return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
        }
    }
    return nil
}
```

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

### 4. Privilege Management

#### Purpose
Enables controlled privilege escalation for specific operations while maintaining the principle of least privilege and providing comprehensive audit trails.

#### Implementation Details

**Unix Privilege Architecture**:
```go
// Location: internal/runner/privilege/unix.go:18-25
type UnixPrivilegeManager struct {
    logger             *slog.Logger
    originalUID        int
    privilegeSupported bool
    metrics            Metrics
    mu                 sync.Mutex  // Prevents race conditions
}
```

**Privilege Escalation Process**:
```go
// Location: internal/runner/privilege/unix.go:36-87
func (m *UnixPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) (err error) {
    m.mu.Lock()  // Global lock for thread safety
    defer m.mu.Unlock()

    // 1. Escalate privileges
    if err := m.escalatePrivileges(elevationCtx); err != nil {
        return err
    }

    // 2. Execute operation with defer-based cleanup
    defer func() {
        if err := m.restorePrivileges(); err != nil {
            m.emergencyShutdown(err, shutdownContext) // Exit on failure
        }
    }()

    return fn()
}
```

**Execution Modes**:

1. **Native Root Execution**: Running as root user (UID 0)
   - No privilege escalation required
   - Direct execution with full privileges

2. **setuid Binary Execution**: Binary with setuid bit set and root ownership
   - Uses `syscall.Seteuid(0)` for privilege escalation
   - Automatic privilege restoration after operation

**Security Validation**:
```go
// Location: internal/runner/privilege/unix.go:232-294
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
- Immediate process termination on privilege restoration failure
- Multi-channel logging (structured logging, syslog, stderr)
- Security event recording with full context
- Prevents continued execution in compromised state

#### Security Guarantees
- Thread-safe privilege operations with global mutex
- Automatic privilege restoration with panic protection
- Comprehensive audit logging of all privilege operations
- Emergency shutdown on security failures
- Supports both native root and setuid binary execution models

### 5. Command Path Verification

#### Purpose
Validates command paths against a configurable allowlist and prevents execution of dangerous binaries, ensuring only authorized commands can be executed. Stops environment variable inheritance and uses a secure fixed PATH.

#### Implementation Details

**Secure PATH Environment Enforcement**:
```go
// Location: internal/verification/manager.go
const securePathEnv = "/sbin:/usr/sbin:/bin:/usr/bin"

// Does not inherit environment variable PATH, uses secure fixed PATH
pathResolver := NewPathResolver(securePathEnv, securityValidator, false)
```

**Path Resolution**:
```go
// Location: internal/verification/path_resolver.go
type PathResolver struct {
    pathEnv            string    // Uses secure fixed PATH
    securityValidator  *security.Validator
    skipStandardPaths  bool
}
```

**Command Verification Process**:
1. Resolve command to full path using PATH environment variable
2. Validate against allowlist patterns (regex-based)
3. Check for dangerous privileged commands
4. Verify file integrity if hash is available

**Default Allowed Patterns**:
```go
// Location: internal/runner/security/types.go:147-154
AllowedCommands: []string{
    "^/bin/.*",
    "^/usr/bin/.*",
    "^/usr/sbin/.*",
    "^/usr/local/bin/.*",
},
```

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

### 6. Risk-Based Command Control

#### Purpose
Implements intelligent security controls based on command risk assessment, automatically blocking high-risk operations while allowing safe commands to execute normally.

#### Implementation Details

**Risk Assessment Engine**:
```go
// Location: internal/runner/risk/evaluator.go
type StandardEvaluator struct{}

func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.Command) (runnertypes.RiskLevel, error) {
    // Check privilege escalation commands (critical risk - should be blocked)
    isPrivEsc, err := security.IsPrivilegeEscalationCommand(cmd.Cmd)
    if err != nil {
        return runnertypes.RiskLevelUnknown, err
    }
    if isPrivEsc {
        return runnertypes.RiskLevelCritical, nil
    }
    // ... Additional risk assessment logic
}
```

**Command Risk Analysis**:
- Low risk: Standard system utilities (ls, cat, grep)
- Medium risk: File modification commands (cp, mv, chmod), package management (apt, yum)
- High risk: System administration commands (mount, systemctl), destructive operations (rm -rf)
- Critical risk: Privilege escalation commands (sudo, su) - automatically blocked

**Risk Level Configuration**:
```go
// Location: internal/runner/runnertypes/config.go
type Command struct {
    MaxRiskLevel string `toml:"max_risk_level"` // Maximum allowed risk level
}
```

#### Security Guarantees
- Automatic blocking of privilege escalation attempts
- Configurable risk threshold per command
- Comprehensive command pattern matching
- Risk-based audit logging

### 7. Resource Management Security

#### Purpose
Provides secure resource management that maintains security boundaries in both normal execution and dry-run modes.

#### Implementation Details

**Unified Resource Interface**:
```go
// Location: internal/runner/resource/manager.go
type ResourceManager interface {
    ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error)
    WithPrivileges(ctx context.Context, fn func() error) error
    SendNotification(message string, details map[string]any) error
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

### 8. Secure Logging and Sensitive Data Protection

#### Purpose
Prevents sensitive information such as passwords, API keys, and tokens from being exposed in log files, providing a safe audit trail without compromising sensitive data. Enhanced with dedicated redaction services to achieve comprehensive protection through a defense-in-depth approach.

#### Implementation Details

##### Current Implementation

**Centralized Data Redaction Foundation**:
```go
// Location: internal/redaction/redactor.go
type Config struct {
    LogPlaceholder   string
    TextPlaceholder  string
    Patterns         *SensitivePatterns
    KeyValuePatterns []string
}

func (c *Config) RedactText(text string) string {
    // Apply all configured redaction patterns
}

func (c *Config) RedactLogAttribute(attr slog.Attr) slog.Attr {
    // Redact sensitive information in log attributes
}
```

**RedactingHandler (Log-Level Redaction)**:
```go
// Location: internal/redaction/redactor.go:200-259
type RedactingHandler struct {
    handler slog.Handler
    config  *Config
}

// Location: internal/runner/bootstrap/logger.go:138
redactedHandler := redaction.NewRedactingHandler(multiHandler, nil)
logger := slog.New(redactedHandler)
```
- Automatically redacts sensitive information at log output time
- Wraps all log handlers (file, syslog, Slack)
- Recursive processing of structured logs including `slog.KindGroup`
- Supports both key=value format and authentication header patterns

**Current Slack Notification Implementation**:
```go
// Location: internal/logging/slack_handler.go:64-73
type SlackHandler struct {
    webhookURL    string
    runID         string
    httpClient    *http.Client
    level         slog.Level
    attrs         []slog.Attr
    groups        []string
    backoffConfig BackoffConfig
}
```
- Wrapped by RedactingHandler, so basic redaction is applied
- Length limits on command output (stdout: 1000 characters, stderr: 500 characters)

##### Planned Enhancements (task 0055)

**Enhanced Sensitive Information Protection with Multiple Layers of Defense**:

The system adopts a dual defense layer approach where one layer protects even if the other fails:

1. **Layer 1: Redaction at CommandResult Creation** (Planned)
   - Location: `internal/runner/group_executor.go` (to be modified)
   - Pre-redact when storing command output into `CommandResult`
   - Uses `security.Validator.SanitizeOutputForLogging()`
   - Current implementation: Stores raw output without redaction

2. **Layer 2: Redaction in RedactingHandler** (Implemented)
   - Location: `internal/redaction/redactor.go:200-259`
   - Handles `slog.KindAny` type and `LogValuer` interface at log output time
   - Catches sensitive information missed by Layer 1
   - Prevents infinite recursion with recursion depth limit

**Enhanced Sensitive Information Protection for Slack Notifications** (Planned):
```go
// Planned implementation: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor  // To be added
}
```
- Add explicit redaction processing before sending Slack notifications
- Complete redaction of sensitive information within `[]common.CommandResult` slices
- Additional protection layer in external communication

**Log Security Configuration**:
```go
// Location: internal/runner/security/types.go:92-107
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
// Location: internal/runner/security/logging_security.go:49-52
func (v *Validator) redactSensitivePatterns(text string) string {
    sensitivePatterns := []struct {
        pattern     string
        replacement string
    }{
        // API keys, tokens, passwords (common patterns)
        {"password=", "password=[REDACTED]"},
        {"token=", "token=[REDACTED]"},
        {"key=", "key=[REDACTED]"},
        {"secret=", "secret=[REDACTED]"},
        {"api_key=", "api_key=[REDACTED]"},

        // Environment variable assignments that may contain sensitive information
        {"_PASSWORD=", "_PASSWORD=[REDACTED]"},
        {"_TOKEN=", "_TOKEN=[REDACTED]"},
        {"_KEY=", "_KEY=[REDACTED]"},
        {"_SECRET=", "_SECRET=[REDACTED]"},

        // Common authentication patterns
        {"Bearer ", "Bearer [REDACTED]"},
        {"Basic ", "Basic [REDACTED]"},
    }
    // Pattern matching and replacement logic
}
```

**Error Message Sanitization**:
```go
// Location: internal/runner/security/logging_security.go:4-26
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

    // Truncate if too long
    if len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
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

##### Currently Provided Guarantees
- Automatic redaction in all log output via RedactingHandler
- Detection and redaction of common sensitive patterns (passwords, tokens, API keys)
- Configurable log detail levels for different security environments
- Protection from credential exposure via error messages and command output
- Length-based truncation to prevent log file bloat and potential DoS
- Detection and sanitization of environment variable patterns
- Supports both key=value format and authentication header patterns (Bearer, Basic)

##### Guarantees to be Added After task 0055 Implementation
- Dual defense with pre-redaction at CommandResult creation time
- Additional protection layer in external communication with explicit redaction processing in Slack notifications
- Catch by Layer 2 (RedactingHandler) even if redaction is missed in Layer 1

### 9. Terminal Capability Detection (`internal/terminal/`)

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

### 10. Color Management (`internal/color/`)

#### Purpose
Provides safe colored output based on terminal color support capability and proper management of color control sequences.

#### Implementation Details

**Color Management Interface**:
```go
// Location: internal/color/color.go
type ColorManager interface {
    Enable() bool
    Colorize(text string, color ColorCode) string
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

### 11. Common Utilities (`internal/common/`, `internal/cmdcommon/`)

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

### 12. User and Group Execution Security

#### Purpose
Provides secure user and group switching functionality while maintaining strict security boundaries and comprehensive audit trails.

#### Implementation Details

**User and Group Configuration**:
```go
// Location: internal/runner/runnertypes/config.go
type Command struct {
    RunAsUser    string `toml:"run_as_user"`    // User to run the command as
    RunAsGroup   string `toml:"run_as_group"`   // Group to run the command as
    MaxRiskLevel string `toml:"max_risk_level"` // Maximum allowed risk level
}
```

**Group Membership Verification**:
```go
// Location: internal/groupmembership/membership.go
type GroupMembershipChecker interface {
    IsUserInGroup(username, groupname string) (bool, error)
    GetGroupMembers(groupname string) ([]string, error)
}
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

### 13. Multi-Channel Notification Security

#### Purpose
Provides secure notification functionality for critical security events while protecting sensitive information in external communication.

#### Implementation Details

**Slack Integration**:
```go
// Location: internal/logging/slack_handler.go
type SlackHandler struct {
    webhookURL string
    redactor   *redaction.Redactor
}
```

**Secure Notification Handling**:
- Automatic sensitive data redaction before sending
- Configurable notification channels
- Rate limiting and error handling
- Secure webhook URL management

#### Security Guarantees
- Sensitive data protection in external notifications
- Secure communication channel management
- Rate limiting to prevent abuse
- Comprehensive error handling

### 14. Command Execution Environment Isolation

#### Purpose
Prevents child processes from reading unexpected input and explicitly controls the execution environment to improve security and stability.

#### Implementation Details

**Standard Input Disabling**:
```go
// Location: internal/runner/executor/executor.go:210-224
// Set up stdin to null device for security and stability:
// 1. Security: Prevents child processes from reading unexpected input from stdin
// 2. Stability: Prevents errors in commands that try to allocate a pseudo-TTY when stdin is nil
//    (e.g., docker-compose exec can fail with "exit status 255" if stdin is not configured)
// 3. Best practice: Batch processing tools should explicitly control stdin rather than inheriting it
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

### 15. Resource Protection with Output Size Limits

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
// Location: internal/common/output_size_limit_type.go:20-21
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

### 16. Configuration Security

#### Purpose
Ensures that configuration files and overall system configuration are not tampered with and follows security best practices.

#### Implementation Details

**File Permission Validation**:
```go
// Location: internal/runner/security/file_validation.go:44-75
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
```go
// Location: cmd/runner/main.go (after modification)
func getHashDir() string {
    // Always use default directory only in production environment
    // --hash-directory flag completely removed (security vulnerability countermeasure)
    return cmdcommon.DefaultHashDirectory
}
```

**Configuration File Pre-Verification**:
```go
// Location: cmd/runner/main.go (after modification)
// Execute hash verification before reading configuration file
if err := verificationManager.VerifyConfigFile(configPath); err != nil {
    // Completely eliminate system operation with unverified data
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeConfigValidation,
        Message:   fmt.Sprintf("Configuration file verification failed: %s", err),
        Component: "config",
        RunID:     runID,
    }
}
```

**Early Path Validation**:
```go
// Location: cmd/runner/main.go:188-199
hashDir := getHashDir()
if !filepath.IsAbs(hashDir) {
    return &logging.PreExecutionError{
        Type:      logging.ErrorTypeFileAccess,
        Message:   fmt.Sprintf("Hash directory must be absolute path, got relative path: %s", hashDir),
        Component: "file",
        RunID:     runID,
    }
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
2. **Pre-Verification**: Hash verification of configuration files before use
3. **Path Security**: Comprehensive path validation and symlink protection, secure fixed PATH use
4. **File Integrity**: Hash-based verification of all critical files (configuration, executables)
5. **Privilege Control**: Principle of least privilege with controlled escalation
6. **Environment Isolation**: Strict allowlist-based environment filtering, PATH inheritance elimination
7. **Command Validation**: Risk-based command execution control with allowlist verification
8. **Data Protection**: Automatic sensitive information redaction in all log output via RedactingHandler (task 0055 will add pre-redaction at CommandResult creation time to strengthen defense-in-depth with dual protection)
9. **User and Group Security**: Secure user and group switching with membership verification
10. **Hash Directory Security**: Complete prevention of custom hash directory attacks
11. **Execution Environment Isolation**: Prevention of unexpected input via stdin disabling
12. **Resource Protection**: Prevention of memory and disk exhaustion attacks with output size limits

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
- Graceful degradation when security features are unavailable

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
- Privilege escalation via LD_PRELOAD, etc.

**Countermeasures**:
- Strict allowlist-based filtering
- Dangerous pattern detection
- Group-level environment isolation
- Variable name and value validation

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
- Redaction at log output time via RedactingHandler (currently implemented)
- Pre-compiled patterns for sensitive data
- Minimal performance impact on normal operations
- Configurable redaction policy
- Addition of pre-redaction at CommandResult creation time for defense-in-depth (to be implemented in task 0055)

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

#### Vulnerability Overview

A theoretical TOCTOU race condition exists between command path validation (`ValidateCommandAllowed`) and actual command execution. An attacker with filesystem write permissions could potentially replace a symlink target between these operations.

**Vulnerability Location**:
```go
// Location: internal/runner/security/validator.go:255-295
func (v *Validator) ValidateCommandAllowed(cmdPath string, ...) error {
    // 1. Resolve symlinks and validate (Check)
    resolvedCmd, err := filepath.EvalSymlinks(cmdPath)
    // Pattern matching validation...
}

// Location: internal/runner/group_executor.go:396-412
// TOCTOU window exists between validation and actual execution
if err := ge.validator.ValidateCommandAllowed(...); err != nil {
    return nil, fmt.Errorf("command not allowed: %w", err)
}
// ... (attacker can modify symlink here)
token, resourceResult, err := ge.resourceManager.ExecuteCommand(...) // Use
```

#### Attack Requirements

To exploit this vulnerability, an attacker must:
1. Have filesystem write permissions
2. Precisely time the attack between validation and execution
3. Be able to place and modify symlinks

#### Mitigation Measures

The following defense-in-depth mechanisms significantly reduce the feasibility and impact of this attack:

**1. File Integrity Verification**:
- All executables are verified against SHA-256 hashes before execution
- The hash verification system provides detection and prevention of tampered binaries
- Location: `internal/filevalidator/`, `internal/verification/`

**2. Security Model Boundaries**:
- The system's security model defines attackers with filesystem write permissions as outside the trust boundary
- In properly configured systems, write permissions to executable directories should be restricted

**3. Deployment Recommendations**:
For high-security environments, the following additional measures are recommended:
- Mount executable directories as read-only filesystems
- Use the `nosymfollow` mount option (where available)
- Enforce strict filesystem permissions
- Implement regular file integrity monitoring

#### Technical Background

**Difficulty of Complete Mitigation**:
Go's standard `os/exec` package does not support the `fexecve()` system call that would completely prevent TOCTOU attacks. A complete solution would require:
1. Low-level system call implementation using CGO
2. File descriptor-based execution flow
3. Platform-specific code (Linux `fexecve()`, Windows alternatives)

Such an implementation is impractical due to:
- Significant architectural changes required
- Increased platform compatibility complexity
- Reduced maintainability
- Existing defense-in-depth provides sufficient protection

#### Impact Assessment

**Risk Level**: Low to Medium
- **Likelihood**: Low (strict requirements, precise timing needed)
- **Impact**: Medium (limited by file integrity verification)
- **Detectability**: High (audit logs, file integrity monitoring)

#### References

- [Safe programming. How to avoid TOCTOU vulnerability](https://stackoverflow.com/questions/41069166/)
- [CERT C Coding Standard: POS35-C](https://wiki.sei.cmu.edu/confluence/display/c/POS35-C.+Avoid+race+conditions+while+checking+for+the+existence+of+a+symbolic+link)
- [Wikipedia: Symlink race](https://en.wikipedia.org/wiki/Symlink_race)
- [Star Lab Software: Linux Symbolic Links Security](https://www.starlab.io/blog/linux-symbolic-links-convenient-useful-and-a-whole-lot-of-trouble)

## Conclusion

Go Safe Command Runner provides a comprehensive security framework for secure command execution with privilege delegation. The multi-layered approach combines modern security primitives (openat2) with proven security principles (defense-in-depth, zero-trust, fail-safe design) to create a robust system suitable for production use in security-conscious environments.

The implementation demonstrates security engineering best practices including comprehensive input validation, risk-based command control, secure privilege management, automatic sensitive data protection, and extensive audit capabilities. The system is designed to fail safely and provide complete visibility into security-related operations.

Key security innovation features include:
- Intelligent risk assessment for command execution
- Unified resource management with consistent security boundaries
- Automatic sensitive data redaction in all log output via RedactingHandler (task 0055 will add pre-redaction at CommandResult creation time to achieve dual protection with defense-in-depth)
- Secure user and group execution functionality
- Comprehensive multi-channel notifications with security-aware messaging
- Explicit control of execution environment via stdin disabling
- Prevention of resource exhaustion attacks with output size limits

The system provides enterprise-grade security controls while maintaining operational flexibility and transparency. Recent improvements have strengthened execution environment isolation and resource protection, achieving more comprehensive security countermeasures. Implementation of task 0055 will further strengthen sensitive data protection.
