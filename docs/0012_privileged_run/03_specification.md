# 詳細仕様書: Privileged コマンド実行機能

## 1. 機能概要

本仕様書は、go-safe-cmd-runnerにおけるprivilegedコマンド実行機能の詳細実装仕様を定義する。`Command.Privileged = true`が設定されたコマンドのみを管理者権限で実行し、setuid/seteuidを使用した安全な権限切り替え機能を提供する。

## 2. データ構造仕様

### 2.1 既存構造体の確認

既存の`runnertypes.Command`構造体は以下の通りで、`Privileged`フィールドが既に定義されている：

```go
// internal/runner/runnertypes/config.go
type Command struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`
    Env         []string `toml:"env"`
    Dir         string   `toml:"dir"`
    Privileged  bool     `toml:"privileged"`  // 本機能で活用
    Timeout     int      `toml:"timeout"`
}
```

### 2.2 新規データ構造

#### 2.2.1 Privilege Manager関連

```go
// internal/runner/privilege/types.go
package privilege

import (
    "context"
    "time"
)

// Operation represents different types of privileged operations
type Operation string

const (
    OperationFileHashCalculation Operation = "file_hash_calculation"
    OperationCommandExecution    Operation = "command_execution"
    OperationFileAccess          Operation = "file_access"
    OperationHealthCheck         Operation = "health_check"
)

// ElevationContext contains context information for privilege elevation
type ElevationContext struct {
    Operation   Operation
    CommandName string
    FilePath    string
    StartTime   time.Time
    OriginalUID int
    TargetUID   int
}

// Manager interface for privilege management
type Manager interface {
    // WithPrivileges executes a function with elevated privileges
    // This is the ONLY public method to ensure safe privilege management
    WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error

    // IsPrivilegedExecutionSupported checks if privileged execution is available
    IsPrivilegedExecutionSupported() bool

    // GetCurrentUID returns the current effective user ID
    GetCurrentUID() int

    // GetOriginalUID returns the original user ID (before any elevation)
    GetOriginalUID() int

    // HealthCheck verifies that privilege escalation works correctly
    HealthCheck(ctx context.Context) error
}

// Metrics for privilege operations
type Metrics struct {
    ElevationAttempts    int64
    ElevationSuccesses   int64
    ElevationFailures    int64
    TotalElevationTime   time.Duration
    LastElevationTime    time.Time
    LastError           string
}
```

#### 2.2.2 エラー定義

```go
// internal/runner/privilege/errors.go
package privilege

import (
    "fmt"
    "syscall"
)

// Error types for privilege operations
var (
    ErrPrivilegedExecutionNotAvailable = fmt.Errorf("privileged execution not available (setuid not configured)")
    ErrPrivilegeElevationFailed        = fmt.Errorf("failed to elevate privileges")
    ErrPrivilegeRestorationFailed      = fmt.Errorf("failed to restore privileges")
    ErrPlatformNotSupported           = fmt.Errorf("privileged execution not supported on this platform")
    ErrInvalidUID                     = fmt.Errorf("invalid user ID")
)

// PrivilegeError contains detailed information about privilege operation failures
type PrivilegeError struct {
    Operation   Operation
    CommandName string
    OriginalUID int
    TargetUID   int
    SyscallErr  error
    Timestamp   time.Time
}

func (e *PrivilegeError) Error() string {
    return fmt.Sprintf("privilege operation '%s' failed for command '%s' (uid %d->%d): %v",
        e.Operation, e.CommandName, e.OriginalUID, e.TargetUID, e.SyscallErr)
}

func (e *PrivilegeError) Unwrap() error {
    return e.SyscallErr
}

// IsPermissionDenied checks if the error is due to permission denial
func IsPermissionDenied(err error) bool {
    if pe, ok := err.(*PrivilegeError); ok {
        return pe.SyscallErr == syscall.EPERM
    }
    return err == syscall.EPERM
}
```

## 3. API仕様

### 3.1 Privilege Manager実装

#### 3.1.1 Linux/Unix実装

```go
// internal/runner/privilege/linux.go
//go:build !windows

package privilege

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "path/filepath"
    "syscall"
    "sync"
    "time"
)

type LinuxPrivilegeManager struct {
    logger      *slog.Logger
    originalUID int
    originalGID int
    isSetuid    bool
    metrics     Metrics
    mu          sync.RWMutex
}

// NewManager creates a new privilege manager for Linux/Unix systems
func NewManager(logger *slog.Logger) Manager {
    originalUID := syscall.Getuid()
    effectiveUID := syscall.Geteuid()

    return &LinuxPrivilegeManager{
        logger:      logger,
        originalUID: originalUID,
        originalGID: syscall.Getgid(),
        isSetuid:    effectiveUID == 0 && originalUID != 0,
    }
}

// Note: ElevatePrivileges is intentionally removed from the public interface
// All privilege management is handled through WithPrivileges for security

func (m *LinuxPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) (err error) {
    // 権限昇格
    if err := m.escalatePrivileges(ctx, elevationCtx); err != nil {
        return fmt.Errorf("privilege escalation failed: %w", err)
    }

    // 単一のdefer文で権限復帰とpanic処理を統合
    defer func() {
        var panicValue interface{}
        var context string

        // panic検出
        if r := recover(); r != nil {
            panicValue = r
            context = fmt.Sprintf("after panic: %v", r)
            m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
                "panic", r,
                "original_uid", m.originalUID)
        } else {
            context = "normal execution"
        }

        // 権限復帰実行（常に実行される）
        if err := m.restorePrivileges(); err != nil {
            // 権限復帰失敗は致命的セキュリティリスク - 即座に終了
            m.emergencyShutdown(err, context)
        }

        // panic再発生（必要な場合のみ）
        if panicValue != nil {
            panic(panicValue)
        }
    }()

    return fn()
}

// escalatePrivileges performs the actual privilege escalation (private method)
func (m *LinuxPrivilegeManager) escalatePrivileges(ctx context.Context, elevationCtx ElevationContext) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Pre-elevation checks
    if !m.IsPrivilegedExecutionSupported() {
        return fmt.Errorf("%w: binary not configured with setuid", ErrPrivilegedExecutionNotAvailable)
    }

    // Verify we're not already elevated for a different operation
    currentUID := syscall.Geteuid()
    if currentUID == 0 {
        m.logger.Warn("Already running with elevated privileges",
            "current_uid", currentUID,
            "operation", elevationCtx.Operation)
    }

    // Perform privilege elevation
    elevationCtx.StartTime = time.Now()
    elevationCtx.OriginalUID = m.originalUID
    elevationCtx.TargetUID = 0

    if err := syscall.Seteuid(0); err != nil {
        m.recordElevationFailure(elevationCtx, err)
        return &PrivilegeError{
            Operation:   elevationCtx.Operation,
            CommandName: elevationCtx.CommandName,
            OriginalUID: elevationCtx.OriginalUID,
            TargetUID:   elevationCtx.TargetUID,
            SyscallErr:  err,
            Timestamp:   time.Now(),
        }
    }

    // Log successful elevation
    m.logger.Info("Privileges elevated",
        "operation", elevationCtx.Operation,
        "command", elevationCtx.CommandName,
        "original_uid", elevationCtx.OriginalUID,
        "target_uid", elevationCtx.TargetUID)

    return nil
}

// restorePrivileges restores original privileges (private method)
func (m *LinuxPrivilegeManager) restorePrivileges() error {
    if err := syscall.Seteuid(m.originalUID); err != nil {
        return err
    }

    m.logger.Info("Privileges restored",
        "restored_uid", m.originalUID)

    return nil
}

func (m *LinuxPrivilegeManager) IsPrivilegedExecutionSupported() bool {
    return m.isSetuid
}

func (m *LinuxPrivilegeManager) GetCurrentUID() int {
    return syscall.Geteuid()
}

func (m *LinuxPrivilegeManager) GetOriginalUID() int {
    return m.originalUID
}

func (m *LinuxPrivilegeManager) HealthCheck(ctx context.Context) error {
    if !m.IsPrivilegedExecutionSupported() {
        return ErrPrivilegedExecutionNotAvailable
    }

    // Test privilege elevation and restoration
    testCtx := ElevationContext{
        Operation:   OperationHealthCheck,
        CommandName: "health_check",
    }

    return m.WithPrivileges(ctx, testCtx, func() error {
        // Verify we're actually running as root
        if syscall.Geteuid() != 0 {
            return fmt.Errorf("privilege elevation failed: still running as uid %d", syscall.Geteuid())
        }
        return nil
    })
}


// Metrics recording functions
func (m *LinuxPrivilegeManager) recordElevationSuccess(ctx ElevationContext, duration time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.metrics.ElevationAttempts++
    m.metrics.ElevationSuccesses++
    m.metrics.TotalElevationTime += duration
    m.metrics.LastElevationTime = time.Now()
}

func (m *LinuxPrivilegeManager) recordElevationFailure(ctx ElevationContext, err error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.metrics.ElevationAttempts++
    m.metrics.ElevationFailures++
    m.metrics.LastError = err.Error()
}

func (m *LinuxPrivilegeManager) GetMetrics() Metrics {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return m.metrics
}
```

#### 3.1.2 Windows実装

```go
// internal/runner/privilege/windows.go
//go:build windows

package privilege

import (
    "context"
    "log/slog"
)

type WindowsPrivilegeManager struct {
    logger *slog.Logger
}

func NewManager(logger *slog.Logger) Manager {
    return &WindowsPrivilegeManager{
        logger: logger,
    }
}


func (m *WindowsPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error {
    return ErrPlatformNotSupported
}

func (m *WindowsPrivilegeManager) IsPrivilegedExecutionSupported() bool {
    return false
}

func (m *WindowsPrivilegeManager) GetCurrentUID() int {
    return -1 // Windows doesn't use UIDs
}

func (m *WindowsPrivilegeManager) GetOriginalUID() int {
    return -1 // Windows doesn't use UIDs
}

func (m *WindowsPrivilegeManager) HealthCheck(ctx context.Context) error {
    return ErrPlatformNotSupported
}
```

### 3.2 Enhanced Command Executor

#### 3.2.1 拡張されたDefaultExecutor

```go
// internal/runner/executor/executor.go (拡張部分)

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

// DefaultExecutor with privilege support
type DefaultExecutor struct {
    FS       FileSystem
    Out      OutputWriter
    PrivMgr  privilege.Manager // 新規追加
}

// NewDefaultExecutorWithPrivileges creates an executor with privilege management
func NewDefaultExecutorWithPrivileges(privMgr privilege.Manager) CommandExecutor {
    return &DefaultExecutor{
        FS:      &osFileSystem{},
        Out:     &consoleOutputWriter{},
        PrivMgr: privMgr,
    }
}

// Execute implements the CommandExecutor interface with privilege support
func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    if cmd.Privileged {
        return e.executePrivileged(ctx, cmd, envVars)
    }
    return e.executeNormal(ctx, cmd, envVars)
}

// executePrivileged handles privileged command execution
func (e *DefaultExecutor) executePrivileged(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // Pre-execution validation
    if e.PrivMgr == nil {
        return nil, fmt.Errorf("privileged execution requested but no privilege manager available")
    }

    if !e.PrivMgr.IsPrivilegedExecutionSupported() {
        return nil, fmt.Errorf("privileged execution not supported on this system")
    }

    // Validate the command before any privilege elevation
    if err := e.Validate(cmd); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }

    // Resolve command path with elevated privileges (needed for file access)
    var resolvedPath string
    pathResolutionCtx := privilege.ElevationContext{
        Operation:   privilege.OperationFileAccess,
        CommandName: cmd.Name,
        FilePath:    cmd.Cmd,
    }

    err := e.PrivMgr.WithPrivileges(ctx, pathResolutionCtx, func() error {
        path, lookErr := exec.LookPath(cmd.Cmd)
        if lookErr != nil {
            return fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
        }
        resolvedPath = path
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("failed to resolve command path with privileges: %w", err)
    }

    // Execute command with elevated privileges
    executionCtx := privilege.ElevationContext{
        Operation:   privilege.OperationCommandExecution,
        CommandName: cmd.Name,
        FilePath:    resolvedPath,
    }

    var result *Result
    err = e.PrivMgr.WithPrivileges(ctx, executionCtx, func() error {
        var execErr error
        result, execErr = e.executeCommandWithPath(ctx, resolvedPath, cmd, envVars)
        return execErr
    })

    if err != nil {
        return nil, fmt.Errorf("privileged command execution failed: %w", err)
    }

    return result, nil
}

// executeNormal handles normal (non-privileged) command execution
func (e *DefaultExecutor) executeNormal(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // This is the existing implementation - no changes required
    if err := e.Validate(cmd); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }

    path, lookErr := exec.LookPath(cmd.Cmd)
    if lookErr != nil {
        return nil, fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
    }

    return e.executeCommandWithPath(ctx, path, cmd, envVars)
}

// executeCommandWithPath executes a command with the given resolved path
func (e *DefaultExecutor) executeCommandWithPath(ctx context.Context, path string, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // Create the command with the resolved path
    // #nosec G204 - The command and arguments are validated before execution
    execCmd := exec.CommandContext(ctx, path, cmd.Args...)

    // Set up working directory
    if cmd.Dir != "" {
        execCmd.Dir = cmd.Dir
    }

    // Set up environment variables
    execCmd.Env = make([]string, 0, len(envVars))
    for k, v := range envVars {
        execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    // Set up output capture
    var stdout, stderr []byte
    var cmdErr error

    if e.Out != nil {
        // Create buffered wrappers that both capture output and write to OutputWriter
        stdoutWrapper := &outputWrapper{writer: e.Out, stream: StdoutStream}
        stderrWrapper := &outputWrapper{writer: e.Out, stream: StderrStream}

        execCmd.Stdout = stdoutWrapper
        execCmd.Stderr = stderrWrapper

        // Run the command
        cmdErr = execCmd.Run()

        // Get the captured output
        stdout = stdoutWrapper.GetBuffer()
        stderr = stderrWrapper.GetBuffer()
    } else {
        // Otherwise, capture output in memory
        stdout, cmdErr = execCmd.Output()
        if exitErr, ok := cmdErr.(*exec.ExitError); ok {
            stderr = exitErr.Stderr
        }
    }

    // Prepare the result
    result := &Result{
        Stdout: string(stdout),
        Stderr: string(stderr),
    }
    if execCmd.ProcessState != nil {
        result.ExitCode = execCmd.ProcessState.ExitCode()
    } else {
        result.ExitCode = ExitCodeUnknown
    }

    if cmdErr != nil {
        return result, fmt.Errorf("command execution failed: %w", cmdErr)
    }

    return result, nil
}
```

### 3.3 File Validator拡張

#### 3.3.1 Privileged Hash Calculation

```go
// internal/filevalidator/validator.go (拡張部分)

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

// Validator with privilege support
type Validator struct {
    hashAlgo HashAlgorithm
    privMgr  privilege.Manager
    logger   *slog.Logger
}

// NewValidatorWithPrivileges creates a new validator with privilege support
func NewValidatorWithPrivileges(hashAlgo HashAlgorithm, privMgr privilege.Manager, logger *slog.Logger) *Validator {
    return &Validator{
        hashAlgo: hashAlgo,
        privMgr:  privMgr,
        logger:   logger,
    }
}

// ValidateWithPrivileges validates file hash with temporary privilege elevation if needed
func (v *Validator) ValidateWithPrivileges(ctx context.Context, filePath string, expectedHash string, needsPrivileges bool) error {
    if needsPrivileges && v.privMgr != nil {
        elevationCtx := privilege.ElevationContext{
            Operation:   privilege.OperationFileHashCalculation,
            CommandName: "hash_validation",
            FilePath:    filePath,
        }

        return v.privMgr.WithPrivileges(ctx, elevationCtx, func() error {
            return v.validateFileHash(filePath, expectedHash)
        })
    }

    return v.validateFileHash(filePath, expectedHash)
}

// validateFileHash performs the actual hash validation
func (v *Validator) validateFileHash(filePath string, expectedHash string) error {
    actualHash, err := v.hashAlgo.HashFile(filePath)
    if err != nil {
        return fmt.Errorf("failed to calculate hash for %s: %w", filePath, err)
    }

    if actualHash != expectedHash {
        return fmt.Errorf("hash mismatch for %s: expected %s, got %s",
            filePath, expectedHash, actualHash)
    }

    return nil
}
```

## 4. 実行フロー仕様

### 4.1 通常コマンドの実行フロー

```
1. Command Processing
   ├─ Load command from config
   ├─ Check Privileged flag = false
   └─ Continue to normal execution

2. Normal Execution Flow
   ├─ Validate command structure
   ├─ Resolve command path (user privileges)
   ├─ Setup environment variables
   ├─ Execute command (user privileges)
   └─ Return result
```

### 4.2 Privilegedコマンドの実行フロー

```
1. Command Processing
   ├─ Load command from config
   ├─ Check Privileged flag = true
   └─ Route to privileged execution

2. Pre-execution Validation
   ├─ Check privilege manager availability
   ├─ Verify setuid binary configuration
   ├─ Validate command structure
   └─ Continue if all checks pass

3. Path Resolution with Privileges
   ├─ ElevationContext(OperationFileAccess)
   ├─ seteuid(0) [Root privileges]
   ├─ exec.LookPath(cmd.Cmd)
   ├─ seteuid(originalUID) [Restore privileges]
   └─ Store resolved path

4. File Validation with Privileges (if verification enabled)
   ├─ ElevationContext(OperationFileHashCalculation)
   ├─ seteuid(0) [Root privileges]
   ├─ Calculate file hash
   ├─ Compare with expected hash
   ├─ seteuid(originalUID) [Restore privileges]
   └─ Validate result

5. Command Execution with Privileges
   ├─ ElevationContext(OperationCommandExecution)
   ├─ seteuid(0) [Root privileges]
   ├─ exec.CommandContext(ctx, resolvedPath, args...)
   ├─ Setup environment and execute
   ├─ seteuid(originalUID) [Restore privileges]
   └─ Return result

6. Result Processing
   ├─ Log execution metrics
   ├─ Record privilege usage
   └─ Return to caller
```

### 4.3 エラーハンドリングフロー

```
Error Scenarios:
├─ Privilege Manager Not Available
│  ├─ Check: e.PrivMgr == nil
│  └─ Return: "no privilege manager available"
│
├─ Platform Not Supported
│  ├─ Check: IsPrivilegedExecutionSupported() == false
│  └─ Return: "privileged execution not supported"
│
├─ Setuid Not Configured
│  ├─ Check: isSetuid == false
│  └─ Return: "setuid not configured"
│
├─ Privilege Elevation Failed
│  ├─ Check: syscall.Seteuid(0) error
│  ├─ Log: detailed error with errno
│  └─ Return: PrivilegeError
│
├─ Privilege Restoration Failed
│  ├─ Check: syscall.Seteuid(originalUID) error
│  ├─ Log: critical error
│  └─ Action: panic() [security critical]
│
└─ Command Execution Failed
   ├─ Ensure: privileges restored via defer
   ├─ Log: command failure details
   └─ Return: wrapped execution error
```

## 5. 設定仕様

### 5.1 設定ファイル形式

```toml
# sample/config.toml での使用例

[[groups]]
name = "system-maintenance"
description = "System maintenance commands requiring root privileges"

  [[groups.commands]]
  name = "mysql_backup"
  description = "Backup MySQL database as root user"
  cmd = "/usr/bin/mysqldump"
  args = ["-u", "root", "-p${MYSQL_ROOT_PASSWORD}", "--single-transaction", "production"]
  privileged = true  # Root権限で実行
  timeout = 1800     # 30分のタイムアウト

  [[groups.commands]]
  name = "system_backup"
  description = "Backup system configuration files"
  cmd = "/usr/bin/rsync"
  args = ["-av", "/etc/", "/backup/system/"]
  privileged = true  # Root権限で実行

  [[groups.commands]]
  name = "log_rotation"
  description = "Manual log rotation"
  cmd = "/usr/sbin/logrotate"
  args = ["-f", "/etc/logrotate.conf"]
  privileged = true  # Root権限で実行

[[groups]]
name = "monitoring"
description = "Monitoring commands that can run as regular user"

  [[groups.commands]]
  name = "check_disk"
  description = "Check disk usage"
  cmd = "df"
  args = ["-h"]
  privileged = false  # 通常ユーザー権限で実行（デフォルト）
```

### 5.2 設定検証仕様

```go
// 設定ファイル読み込み時の検証ロジック
func (l *ConfigLoader) ValidatePrivilegedCommands(cfg *Config) []ValidationError {
    var errors []ValidationError

    for _, group := range cfg.Groups {
        for _, cmd := range group.Commands {
            if cmd.Privileged {
                // Privilegedコマンドの追加検証
                if cmd.Cmd == "" {
                    errors = append(errors, ValidationError{
                        Type:    "privileged_validation",
                        Group:   group.Name,
                        Command: cmd.Name,
                        Message: "privileged command must specify 'cmd' field",
                    })
                }

                // 危険なコマンドパスの警告
                if isDangerousPath(cmd.Cmd) {
                    errors = append(errors, ValidationError{
                        Type:     "security_warning",
                        Group:    group.Name,
                        Command:  cmd.Name,
                        Message:  fmt.Sprintf("privileged command uses potentially dangerous path: %s", cmd.Cmd),
                        Severity: "warning",
                    })
                }
            }
        }
    }

    return errors
}

func isDangerousPath(cmdPath string) bool {
    dangerousPaths := []string{
        "/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash",
        "/bin/su", "/usr/bin/su",
        "/usr/bin/sudo",
        "/sbin/init", "/usr/sbin/init",
    }

    for _, dangerous := range dangerousPaths {
        if cmdPath == dangerous {
            return true
        }
    }
    return false
}
```

## 6. ログ出力仕様

### 6.1 構造化ログ出力

#### 6.1.1 権限昇格開始ログ

```go
slog.Info("Privilege elevation started",
    "operation", "command_execution",
    "command", cmd.Name,
    "file_path", resolvedPath,
    "original_uid", manager.GetOriginalUID(),
    "target_uid", 0,
    "timestamp", time.Now().Unix())
```

#### 6.1.2 権限昇格完了ログ

```go
slog.Info("Privilege elevation completed",
    "operation", "command_execution",
    "command", cmd.Name,
    "elevation_duration_ms", duration.Milliseconds(),
    "success", true,
    "restored_uid", manager.GetOriginalUID())
```

#### 6.1.3 権限昇格失敗ログ

```go
slog.Error("Privilege elevation failed",
    "operation", "file_hash_calculation",
    "command", cmd.Name,
    "file_path", filePath,
    "error", err.Error(),
    "original_uid", manager.GetOriginalUID(),
    "attempted_uid", 0)
```

### 6.2 セキュリティ監査ログ

```go
// 成功時の監査ログ
slog.Info("Privileged command executed successfully",
    "audit", "privileged_execution",
    "command", cmd.Name,
    "cmd_path", resolvedPath,
    "args", strings.Join(cmd.Args, " "),
    "exit_code", result.ExitCode,
    "execution_duration_ms", executionDuration.Milliseconds(),
    "privilege_elevation_count", 2, // path resolution + execution
    "total_privilege_duration_ms", totalPrivilegeDuration.Milliseconds(),
    "user", getUserName(manager.GetOriginalUID()),
    "timestamp", time.Now().Unix())

// 失敗時の監査ログ
slog.Error("Privileged command execution failed",
    "audit", "privileged_execution_failure",
    "command", cmd.Name,
    "cmd_path", resolvedPath,
    "error", err.Error(),
    "failure_stage", "command_execution", // or "privilege_elevation", "path_resolution"
    "user", getUserName(manager.GetOriginalUID()),
    "timestamp", time.Now().Unix())
```

## 7. テスト仕様

### 7.1 単体テスト

#### 7.1.1 Privilege Manager Tests

```go
// internal/runner/privilege/linux_test.go
func TestLinuxPrivilegeManager_WithPrivileges(t *testing.T) {
    tests := []struct {
        name           string
        isSetuid       bool
        mockSeteuidErr error
        expectError    bool
        errorType      error
    }{
        {
            name:        "successful elevation",
            isSetuid:    true,
            expectError: false,
        },
        {
            name:        "setuid not configured",
            isSetuid:    false,
            expectError: true,
            errorType:   ErrPrivilegedExecutionNotAvailable,
        },
        {
            name:           "seteuid fails",
            isSetuid:       true,
            mockSeteuidErr: syscall.EPERM,
            expectError:    true,
            errorType:      &PrivilegeError{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Mock syscall functions for testing
            originalSeteuid := syscallSeteuid
            defer func() { syscallSeteuid = originalSeteuid }()

            if tt.mockSeteuidErr != nil {
                syscallSeteuid = func(euid int) error {
                    return tt.mockSeteuidErr
                }
            }

            manager := &LinuxPrivilegeManager{
                logger:      slog.Default(),
                originalUID: 1000,
                isSetuid:    tt.isSetuid,
            }

            ctx := context.Background()
            elevationCtx := ElevationContext{
                Operation:   OperationCommandExecution,
                CommandName: "test_command",
            }

            err := manager.WithPrivileges(ctx, elevationCtx, func() error {
                // Test function for privilege operation
                return nil
            })

            if tt.expectError {
                assert.Error(t, err)
                if tt.errorType != nil {
                    assert.IsType(t, tt.errorType, err)
                }
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### 7.1.2 Enhanced Executor Tests

```go
// internal/runner/executor/executor_test.go (追加テスト)
func TestDefaultExecutor_ExecutePrivileged(t *testing.T) {
    tests := []struct {
        name            string
        cmd             runnertypes.Command
        privilegeSupported bool
        expectError     bool
        errorMessage    string
    }{
        {
            name: "privileged command success",
            cmd: runnertypes.Command{
                Name:       "test_cmd",
                Cmd:        "echo",
                Args:       []string{"test"},
                Privileged: true,
            },
            privilegeSupported: true,
            expectError:        false,
        },
        {
            name: "privileged command without privilege support",
            cmd: runnertypes.Command{
                Name:       "test_cmd",
                Cmd:        "echo",
                Args:       []string{"test"},
                Privileged: true,
            },
            privilegeSupported: false,
            expectError:        true,
            errorMessage:       "privileged execution not supported",
        },
        {
            name: "normal command execution unaffected",
            cmd: runnertypes.Command{
                Name:       "test_cmd",
                Cmd:        "echo",
                Args:       []string{"test"},
                Privileged: false,
            },
            privilegeSupported: false,  // Should not matter
            expectError:        false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockPrivMgr := &MockPrivilegeManager{
                supported: tt.privilegeSupported,
            }

            executor := &DefaultExecutor{
                FS:      &mockFileSystem{},
                Out:     nil,
                PrivMgr: mockPrivMgr,
            }

            ctx := context.Background()
            envVars := map[string]string{"PATH": "/usr/bin"}

            result, err := executor.Execute(ctx, tt.cmd, envVars)

            if tt.expectError {
                assert.Error(t, err)
                if tt.errorMessage != "" {
                    assert.Contains(t, err.Error(), tt.errorMessage)
                }
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
                assert.Equal(t, 0, result.ExitCode)
            }
        })
    }
}

// Mock Privilege Manager for testing
type MockPrivilegeManager struct {
    supported      bool
    elevationCalls []string
    shouldFail     bool
}

func (m *MockPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx privilege.ElevationContext, fn func() error) error {
    m.elevationCalls = append(m.elevationCalls, string(elevationCtx.Operation))
    if m.shouldFail {
        return errors.New("mock privilege elevation failure")
    }
    return fn()
}

func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
    return m.supported
}

// ... other mock methods
```

### 7.2 インテグレーションテスト

#### 7.2.1 End-to-End Privileged Execution Test

```go
// internal/runner/runner_integration_test.go
func TestRunner_PrivilegedExecution_Integration(t *testing.T) {
    // This test requires a setuid binary or root execution
    if !canRunPrivilegedTests() {
        t.Skip("Privileged integration tests require setuid binary or root execution")
    }

    tempDir := t.TempDir()
    configFile := filepath.Join(tempDir, "config.toml")

    // Create test configuration
    config := `
[[groups]]
name = "privileged_test"

  [[groups.commands]]
  name = "create_root_file"
  cmd = "touch"
  args = ["/tmp/privileged_test_file"]
  privileged = true

  [[groups.commands]]
  name = "remove_root_file"
  cmd = "rm"
  args = ["/tmp/privileged_test_file"]
  privileged = true
`

    err := os.WriteFile(configFile, []byte(config), 0644)
    require.NoError(t, err)

    // Load configuration
    cfg, err := config.LoadConfig(configFile)
    require.NoError(t, err)

    // Create runner with real privilege manager
    runner, err := NewRunner(cfg)
    require.NoError(t, err)

    // Execute privileged commands
    ctx := context.Background()
    for _, group := range cfg.Groups {
        err = runner.ExecuteGroup(ctx, group)
        assert.NoError(t, err)
    }

    // Verify that commands were executed with proper privileges
    // Check logs for privilege elevation messages
    // This would require log capture mechanism
}

func canRunPrivilegedTests() bool {
    // Check if running as root or with setuid binary
    if os.Geteuid() == 0 {
        return true
    }

    executable, err := os.Executable()
    if err != nil {
        return false
    }

    info, err := os.Stat(executable)
    if err != nil {
        return false
    }

    return info.Mode()&os.ModeSetuid != 0
}
```

## 8. セキュリティ仕様

### 8.1 権限昇格の制約

#### 8.1.1 権限昇格時間の最小化

```go
// 権限昇格期間のモニタリング
type ElevationTracker struct {
    maxElevationDuration time.Duration
    elevations          map[string]time.Time
    mu                  sync.RWMutex
}

func (t *ElevationTracker) TrackElevation(operation string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.elevations[operation] = time.Now()
}

func (t *ElevationTracker) TrackRestoration(operation string) time.Duration {
    t.mu.Lock()
    defer t.mu.Unlock()

    start, exists := t.elevations[operation]
    if !exists {
        return 0
    }

    duration := time.Since(start)
    delete(t.elevations, operation)

    if duration > t.maxElevationDuration {
        // Log warning about long privilege elevation
        slog.Warn("Long privilege elevation detected",
            "operation", operation,
            "duration_ms", duration.Milliseconds(),
            "max_expected_ms", t.maxElevationDuration.Milliseconds())
    }

    return duration
}
```

#### 8.1.2 Fail-Safe機構

```go
func (m *LinuxPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) (err error) {
    // 権限昇格
    if err := m.escalatePrivileges(ctx, elevationCtx); err != nil {
        return fmt.Errorf("privilege escalation failed: %w", err)
    }

    // 単一のdefer文で権限復帰とpanic処理を統合
    defer func() {
        var panicValue interface{}
        var context string

        // panic検出
        if r := recover(); r != nil {
            panicValue = r
            context = fmt.Sprintf("after panic: %v", r)
            m.logger.Error("Panic occurred during privileged operation, attempting privilege restoration",
                "panic", r,
                "original_uid", m.originalUID)
        } else {
            context = "normal execution"
        }

        // 権限復帰実行（常に実行される）
        if err := m.restorePrivileges(); err != nil {
            // 権限復帰失敗は致命的セキュリティリスク - 即座に終了
            m.emergencyShutdown(err, context)
        }

        // panic再発生（必要な場合のみ）
        if panicValue != nil {
            panic(panicValue)
        }
    }()

    return fn()
}

// emergencyShutdown handles critical privilege restoration failures
func (m *LinuxPrivilegeManager) emergencyShutdown(restoreErr error, context string) {
    // 詳細なエラー情報を記録（複数の出力先に確実に記録）
    criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: Privilege restoration failed during %s", context)

    // 構造化ログに記録
    m.logger.Error(criticalMsg,
        "error", restoreErr,
        "original_uid", m.originalUID,
        "current_uid", os.Getuid(),
        "current_euid", os.Geteuid(),
        "timestamp", time.Now().UTC(),
        "process_id", os.Getpid(),
    )

    // システムログにも記録（rsyslog等による外部転送対応）
    syslog.Err(fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d)",
        criticalMsg, restoreErr, os.Getpid(), m.originalUID, os.Geteuid()))

    // 標準エラー出力にも記録（最後の手段）
    fmt.Fprintf(os.Stderr, "FATAL: %s: %v\n", criticalMsg, restoreErr)

    // 即座にプロセス終了（defer処理をスキップ）
    os.Exit(1)
}
```

### 8.2 監査証跡

#### 8.2.1 詳細監査ログ

```go
type AuditLogger struct {
    logger *slog.Logger
}

func (a *AuditLogger) LogPrivilegedExecution(ctx context.Context, cmd runnertypes.Command, result *executor.Result, duration time.Duration) {
    entry := map[string]interface{}{
        "audit_type":           "privileged_execution",
        "timestamp":           time.Now().Unix(),
        "command_name":        cmd.Name,
        "command_path":        cmd.Cmd,
        "command_args":        strings.Join(cmd.Args, " "),
        "exit_code":           result.ExitCode,
        "execution_duration":  duration.Milliseconds(),
        "user_id":            os.Getuid(),
        "user_name":          getUserName(os.Getuid()),
        "process_id":         os.Getpid(),
        "parent_process_id":  os.Getppid(),
    }

    if result.ExitCode == 0 {
        a.logger.Info("Privileged command executed successfully",
            convertMapToLogAttrs(entry)...)
    } else {
        entry["stdout"] = result.Stdout
        entry["stderr"] = result.Stderr
        a.logger.Error("Privileged command failed",
            convertMapToLogAttrs(entry)...)
    }
}
```

## 9. パフォーマンス仕様

### 9.1 性能要件

- 権限昇格のオーバーヘッド: < 10ms per operation
- 通常コマンドへの影響: 0% (privileged=false時の性能劣化なし)
- メモリ使用量の増加: < 1MB per runner instance

### 9.2 パフォーマンス測定

```go
// Benchmark test for privilege operations
func BenchmarkPrivilegeElevation(b *testing.B) {
    manager := NewLinuxPrivilegeManager(slog.Default())
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        elevationCtx := privilege.ElevationContext{
            Operation:   privilege.OperationHealthCheck,
            CommandName: "benchmark_test",
        }

        err := manager.WithPrivileges(ctx, elevationCtx, func() error {
            // Minimal operation to measure just privilege overhead
            return nil
        })

        if err != nil {
            b.Fatal(err)
        }
    }
}
```

この詳細仕様書に基づいて、安全で効率的なprivilegedコマンド実行機能が実装できます。
