# 実装計画書: Privileged コマンド実行機能

## 1. 実装概要

本実装計画書は、go-safe-cmd-runnerにおけるprivilegedコマンド実行機能を段階的に実装するための詳細な計画を定義する。最小権限の原則に基づき、安全で効率的な権限昇格機能を提供する。

## 2. 実装フェーズ

### 2.1 Phase 1: 基盤インフラストラクチャ
**期間**: 1週間
**目的**: 権限管理の基盤となるコンポーネントの実装

#### 2.1.1 Phase 1の実装項目

```
Phase 1 実装項目:
├─ Privilege Manager Interface      [priority: P0]
├─ Platform-specific Implementation [priority: P0]
├─ Error Types and Handling        [priority: P0]
├─ Logging Infrastructure          [priority: P0]
└─ Basic Unit Tests               [priority: P1]
```

### 2.2 Phase 2: Executor統合
**期間**: 1週間
**目的**: Command Executorとの統合と基本的な実行機能

#### 2.2.1 Phase 2の実装項目

```
Phase 2 実装項目:
├─ DefaultExecutor拡張             [priority: P0]
├─ Privileged Command Detection    [priority: P0]
├─ Command Path Resolution         [priority: P0]
├─ Basic Privileged Execution      [priority: P0]
└─ Integration Tests               [priority: P1]
```

### 2.3 Phase 3: セキュリティ強化
**期間**: 1週間
**目的**: セキュリティ機能の実装と監査ログ

#### 2.3.1 Phase 3の実装項目

```
Phase 3 実装項目:
├─ File Validator統合              [priority: P0]
├─ Structured Audit Logging        [priority: P0]
├─ Security Fail-Safe Mechanisms   [priority: P0]
├─ Performance Optimization        [priority: P1]
└─ Comprehensive Testing           [priority: P1]
```

### 2.4 Phase 4: 運用機能
**期間**: 0.5週間
**目的**: 運用に必要な機能の追加

#### 2.4.1 Phase 4の実装項目

```
Phase 4 実装項目:
├─ Health Check機能                [priority: P1]
├─ Metrics Collection              [priority: P1]
├─ Configuration Validation        [priority: P1]
└─ Documentation                   [priority: P1]
```

## 3. Phase 1: 基盤インフラストラクチャ

### 3.1 ディレクトリ構造

```
internal/runner/
├─ privilege/
│  ├─ manager.go        # Interface定義
│  ├─ types.go          # 型定義
│  ├─ errors.go         # エラー定義
│  ├─ linux.go          # Linux/Unix実装
│  ├─ windows.go        # Windows実装
│  └─ manager_test.go   # Unit tests
└─ ...
```

### 3.2 実装ステップ

#### 3.2.1 Step 1: Interface定義とTypes

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
```

#### 3.2.2 Step 2: Manager Interface

```go
// internal/runner/privilege/manager.go
package privilege

import (
    "context"
    "time"
)

// Manager interface for privilege management
type Manager interface {
    // WithPrivileges executes a function with elevated privileges
    // This is the ONLY public method to ensure safe privilege management
    WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error

    // IsPrivilegedExecutionSupported checks if privileged execution is available
    IsPrivilegedExecutionSupported() bool

    // GetCurrentUID returns the current effective user ID
    GetCurrentUID() int

    // GetOriginalUID returns the original user ID
    GetOriginalUID() int

    // HealthCheck verifies privilege escalation works
    HealthCheck(ctx context.Context) error
}

// Factory function for creating platform-appropriate managers
func NewManager(logger *slog.Logger) Manager {
    return newPlatformManager(logger)
}
```

#### 3.2.3 Step 3: Error Types

```go
// internal/runner/privilege/errors.go
package privilege

import (
    "fmt"
    "syscall"
    "time"
)

// Standard errors
var (
    ErrPrivilegedExecutionNotAvailable = fmt.Errorf("privileged execution not available (setuid not configured)")
    ErrPrivilegeElevationFailed        = fmt.Errorf("failed to elevate privileges")
    ErrPrivilegeRestorationFailed      = fmt.Errorf("failed to restore privileges")
    ErrPlatformNotSupported           = fmt.Errorf("privileged execution not supported on this platform")
    ErrInvalidUID                     = fmt.Errorf("invalid user ID")
)

// PrivilegeError contains detailed information about failures
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
```

#### 3.2.4 Step 4: Linux実装

```go
// internal/runner/privilege/linux.go
//go:build !windows

package privilege

import (
    "context"
    "fmt"
    "log/slog"
    "os"
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

func newPlatformManager(logger *slog.Logger) Manager {
    originalUID := syscall.Getuid()
    effectiveUID := syscall.Geteuid()

    return &LinuxPrivilegeManager{
        logger:      logger,
        originalUID: originalUID,
        originalGID: syscall.Getgid(),
        isSetuid:    effectiveUID == 0 && originalUID != 0,
    }
}

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

    if !m.IsPrivilegedExecutionSupported() {
        return fmt.Errorf("%w: binary not configured with setuid", ErrPrivilegedExecutionNotAvailable)
    }

    elevationCtx.StartTime = time.Now()
    elevationCtx.OriginalUID = m.originalUID
    elevationCtx.TargetUID = 0

    if err := syscall.Seteuid(0); err != nil {
        return &PrivilegeError{
            Operation:   elevationCtx.Operation,
            CommandName: elevationCtx.CommandName,
            OriginalUID: elevationCtx.OriginalUID,
            TargetUID:   elevationCtx.TargetUID,
            SyscallErr:  err,
            Timestamp:   time.Now(),
        }
    }

    m.logger.Info("Privileges elevated",
        "operation", elevationCtx.Operation,
        "command", elevationCtx.CommandName,
        "original_uid", elevationCtx.OriginalUID)

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

// ... remaining methods
```

#### 3.2.5 Step 5: Windows実装

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

func newPlatformManager(logger *slog.Logger) Manager {
    return &WindowsPrivilegeManager{
        logger: logger,
    }
}

func (m *WindowsPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error {
    m.logger.Error("Privileged execution requested on unsupported platform",
        "operation", elevationCtx.Operation,
        "command", elevationCtx.CommandName)
    return ErrPlatformNotSupported
}

// ... remaining methods returning appropriate errors/no-ops
```

### 3.3 Phase 1テストケース

#### 3.3.1 Unit Test実装

```go
// internal/runner/privilege/manager_test.go
package privilege

import (
    "context"
    "log/slog"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestLinuxPrivilegeManager_WithPrivileges(t *testing.T) {
    tests := []struct {
        name        string
        isSetuid    bool
        expectError bool
        errorType   error
    }{
        {
            name:        "successful elevation when setuid configured",
            isSetuid:    true,
            expectError: false,
        },
        {
            name:        "fails when setuid not configured",
            isSetuid:    false,
            expectError: true,
            errorType:   ErrPrivilegedExecutionNotAvailable,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            manager := &LinuxPrivilegeManager{
                logger:      slog.Default(),
                originalUID: 1000,
                isSetuid:    tt.isSetuid,
            }

            ctx := context.Background()
            elevationCtx := ElevationContext{
                Operation:   OperationHealthCheck,
                CommandName: "test",
            }

            err := manager.WithPrivileges(ctx, elevationCtx, func() error {
                // Test function that verifies privilege escalation
                return nil
            })

            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 3.4 Phase 1チェックリスト

- [ ] `internal/runner/privilege/`ディレクトリ作成
- [ ] types.goファイル作成（Operation, ElevationContext定義）
- [ ] manager.goファイル作成（Managerインターフェース定義）
- [ ] errors.goファイル作成（エラー型定義）
- [ ] linux.goファイル作成（LinuxPrivilegeManager実装）
- [ ] windows.goファイル作成（WindowsPrivilegeManager実装）
- [ ] manager_test.goファイル作成（基本テスト）
- [ ] build tagの動作確認（Linux/Windows）
- [ ] 全テストの通過確認

## 4. Phase 2: Executor統合

### 4.1 実装ステップ

#### 4.1.1 Step 1: DefaultExecutorの拡張

```go
// internal/runner/executor/executor.go (拡張)
import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

type DefaultExecutor struct {
    FS       FileSystem
    Out      OutputWriter
    PrivMgr  privilege.Manager // 新規追加
}

// Constructor with privilege manager
func NewDefaultExecutorWithPrivileges(privMgr privilege.Manager) CommandExecutor {
    return &DefaultExecutor{
        FS:      &osFileSystem{},
        Out:     &consoleOutputWriter{},
        PrivMgr: privMgr,
    }
}

// Option pattern for backward compatibility
type ExecutorOption func(*DefaultExecutor)

func WithPrivilegeManager(privMgr privilege.Manager) ExecutorOption {
    return func(e *DefaultExecutor) {
        e.PrivMgr = privMgr
    }
}

func NewDefaultExecutor(opts ...ExecutorOption) CommandExecutor {
    e := &DefaultExecutor{
        FS:  &osFileSystem{},
        Out: &consoleOutputWriter{},
    }

    for _, opt := range opts {
        opt(e)
    }

    return e
}
```

#### 4.1.2 Step 2: Execute メソッドの拡張

```go
func (e *DefaultExecutor) Execute(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    if cmd.Privileged {
        return e.executePrivileged(ctx, cmd, envVars)
    }
    return e.executeNormal(ctx, cmd, envVars)
}

func (e *DefaultExecutor) executePrivileged(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    if e.PrivMgr == nil {
        return nil, fmt.Errorf("privileged execution requested but no privilege manager available")
    }

    if !e.PrivMgr.IsPrivilegedExecutionSupported() {
        return nil, fmt.Errorf("privileged execution not supported on this system")
    }

    // Validation first
    if err := e.Validate(cmd); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }

    // Path resolution with privileges
    var resolvedPath string
    pathCtx := privilege.ElevationContext{
        Operation:   privilege.OperationFileAccess,
        CommandName: cmd.Name,
        FilePath:    cmd.Cmd,
    }

    err := e.PrivMgr.WithPrivileges(ctx, pathCtx, func() error {
        path, lookErr := exec.LookPath(cmd.Cmd)
        if lookErr != nil {
            return fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
        }
        resolvedPath = path
        return nil
    })

    if err != nil {
        return nil, fmt.Errorf("failed to resolve command path: %w", err)
    }

    // Command execution with privileges
    execCtx := privilege.ElevationContext{
        Operation:   privilege.OperationCommandExecution,
        CommandName: cmd.Name,
        FilePath:    resolvedPath,
    }

    var result *Result
    err = e.PrivMgr.WithPrivileges(ctx, execCtx, func() error {
        var execErr error
        result, execErr = e.executeCommandWithPath(ctx, resolvedPath, cmd, envVars)
        return execErr
    })

    return result, err
}

func (e *DefaultExecutor) executeNormal(ctx context.Context, cmd runnertypes.Command, envVars map[string]string) (*Result, error) {
    // 既存の実装を使用 - 変更なし
    if err := e.Validate(cmd); err != nil {
        return nil, fmt.Errorf("command validation failed: %w", err)
    }

    path, lookErr := exec.LookPath(cmd.Cmd)
    if lookErr != nil {
        return nil, fmt.Errorf("failed to find command %q: %w", cmd.Cmd, lookErr)
    }

    return e.executeCommandWithPath(ctx, path, cmd, envVars)
}
```

#### 4.1.3 Step 3: Runner統合

```go
// internal/runner/runner.go (拡張)
import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

type Runner struct {
    config             *runnertypes.Config
    executor           executor.CommandExecutor
    verificationManager *verification.Manager
    validator          *security.Validator
    privilegeManager   privilege.Manager // 新規追加
}

type RunnerOption func(*Runner)

func WithPrivilegeManager(privMgr privilege.Manager) RunnerOption {
    return func(r *Runner) {
        r.privilegeManager = privMgr
    }
}

func NewRunner(config *runnertypes.Config, opts ...RunnerOption) (*Runner, error) {
    r := &Runner{
        config: config,
        // Default privilege manager
        privilegeManager: privilege.NewManager(slog.Default()),
    }

    // Apply options
    for _, opt := range opts {
        opt(r)
    }

    // Create executor with privilege manager
    r.executor = executor.NewDefaultExecutor(
        executor.WithPrivilegeManager(r.privilegeManager),
    )

    // ... rest of initialization

    return r, nil
}
```

### 4.2 Phase 2テストケース

#### 4.2.1 Executor Integration Tests

```go
// internal/runner/executor/executor_test.go (追加テスト)
func TestDefaultExecutor_PrivilegedExecution(t *testing.T) {
    tests := []struct {
        name               string
        cmd                runnertypes.Command
        privilegeSupported bool
        expectError        bool
        expectElevations   []string
    }{
        {
            name: "privileged command executes with elevation",
            cmd: runnertypes.Command{
                Name:       "test_privileged",
                Cmd:        "echo",
                Args:       []string{"test"},
                Privileged: true,
            },
            privilegeSupported: true,
            expectError:        false,
            expectElevations:   []string{"file_access", "command_execution"},
        },
        {
            name: "normal command bypasses privilege manager",
            cmd: runnertypes.Command{
                Name:       "test_normal",
                Cmd:        "echo",
                Args:       []string{"test"},
                Privileged: false,
            },
            privilegeSupported: false,
            expectError:        false,
            expectElevations:   []string{}, // No elevations expected
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockPrivMgr := &MockPrivilegeManager{
                supported: tt.privilegeSupported,
            }

            executor := executor.NewDefaultExecutor(
                executor.WithPrivilegeManager(mockPrivMgr),
            )

            ctx := context.Background()
            envVars := map[string]string{"PATH": "/usr/bin"}

            result, err := executor.Execute(ctx, tt.cmd, envVars)

            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.NotNil(t, result)
            }

            assert.Equal(t, tt.expectElevations, mockPrivMgr.elevationCalls)
        })
    }
}

type MockPrivilegeManager struct {
    supported      bool
    elevationCalls []string
    shouldFail     bool
}

func (m *MockPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx privilege.ElevationContext, fn func() error) error {
    m.elevationCalls = append(m.elevationCalls, string(elevationCtx.Operation))
    if m.shouldFail {
        return errors.New("mock elevation failure")
    }
    return fn()
}

func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
    return m.supported
}

// ... other required methods
```

### 4.3 Phase 2チェックリスト

- [ ] DefaultExecutorにPrivMgrフィールド追加
- [ ] NewDefaultExecutorWithPrivileges()実装
- [ ] ExecutorOptionパターン実装
- [ ] Execute()メソッドの分岐処理実装
- [ ] executePrivileged()メソッド実装
- [ ] executeNormal()メソッド実装（既存ロジック）
- [ ] RunnerクラスにPrivilegeManager統合
- [ ] WithPrivilegeManager()オプション実装
- [ ] MockPrivilegeManager実装（テスト用）
- [ ] Integration Tests実装
- [ ] 全テストの通過確認

## 5. Phase 3: セキュリティ強化

### 5.1 実装ステップ

#### 5.1.1 Step 1: File Validator統合

```go
// internal/filevalidator/validator.go (拡張)
import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
)

type Validator struct {
    hashAlgo HashAlgorithm
    privMgr  privilege.Manager // 新規追加
    logger   *slog.Logger
}

func NewValidatorWithPrivileges(hashAlgo HashAlgorithm, privMgr privilege.Manager, logger *slog.Logger) *Validator {
    return &Validator{
        hashAlgo: hashAlgo,
        privMgr:  privMgr,
        logger:   logger,
    }
}

func (v *Validator) ValidateWithPrivileges(ctx context.Context, filePath string, expectedHash string, needsPrivileges bool) error {
    if needsPrivileges && v.privMgr != nil && v.privMgr.IsPrivilegedExecutionSupported() {
        elevationCtx := privilege.ElevationContext{
            Operation:   privilege.OperationFileHashCalculation,
            CommandName: "file_validation",
            FilePath:    filePath,
        }

        return v.privMgr.WithPrivileges(ctx, elevationCtx, func() error {
            return v.validateFileHash(filePath, expectedHash)
        })
    }

    return v.validateFileHash(filePath, expectedHash)
}
```

#### 5.1.2 Step 2: 構造化監査ログ

```go
// internal/runner/audit/logger.go
package audit

import (
    "context"
    "log/slog"
    "os"
    "strings"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

type Logger struct {
    logger *slog.Logger
}

func NewAuditLogger(logger *slog.Logger) *Logger {
    return &Logger{logger: logger}
}

func (a *Logger) LogPrivilegedExecution(
    ctx context.Context,
    cmd runnertypes.Command,
    result *executor.Result,
    duration time.Duration,
    privilegeMetrics PrivilegeMetrics,
) {
    baseAttrs := []slog.Attr{
        slog.String("audit_type", "privileged_execution"),
        slog.Int64("timestamp", time.Now().Unix()),
        slog.String("command_name", cmd.Name),
        slog.String("command_path", cmd.Cmd),
        slog.String("command_args", strings.Join(cmd.Args, " ")),
        slog.Int("exit_code", result.ExitCode),
        slog.Int64("execution_duration_ms", duration.Milliseconds()),
        slog.Int("user_id", os.Getuid()),
        slog.Int("process_id", os.Getpid()),
        slog.Int("elevation_count", privilegeMetrics.ElevationCount),
        slog.Int64("total_privilege_duration_ms", privilegeMetrics.TotalDuration.Milliseconds()),
    }

    if result.ExitCode == 0 {
        a.logger.Info("Privileged command executed successfully", baseAttrs...)
    } else {
        errorAttrs := append(baseAttrs,
            slog.String("stdout", result.Stdout),
            slog.String("stderr", result.Stderr))
        a.logger.Error("Privileged command failed", errorAttrs...)
    }
}

type PrivilegeMetrics struct {
    ElevationCount int
    TotalDuration  time.Duration
}
```

#### 5.1.3 Step 3: Fail-Safe機構

```go
// internal/runner/privilege/linux.go (強化版)
func (m *LinuxPrivilegeManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) (err error) {
    // 権限昇格
    if err := m.escalatePrivileges(ctx, elevationCtx); err != nil {
        return fmt.Errorf("privilege escalation failed: %w", err)
    }

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

#### 5.1.4 Step 4: Performance Optimization

```go
// internal/runner/privilege/metrics.go
package privilege

import (
    "sync"
    "time"
)

type Metrics struct {
    mu                      sync.RWMutex
    ElevationAttempts      int64
    ElevationSuccesses     int64
    ElevationFailures      int64
    TotalElevationTime     time.Duration
    AverageElevationTime   time.Duration
    MaxElevationTime       time.Duration
    LastElevationTime      time.Time
    LastError             string
}

func (m *Metrics) RecordElevationSuccess(duration time.Duration) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.ElevationAttempts++
    m.ElevationSuccesses++
    m.TotalElevationTime += duration
    m.AverageElevationTime = m.TotalElevationTime / time.Duration(m.ElevationSuccesses)

    if duration > m.MaxElevationTime {
        m.MaxElevationTime = duration
    }

    m.LastElevationTime = time.Now()
}

func (m *Metrics) RecordElevationFailure(err error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.ElevationAttempts++
    m.ElevationFailures++
    m.LastError = err.Error()
}

func (m *Metrics) GetSnapshot() Metrics {
    m.mu.RLock()
    defer m.mu.RUnlock()
    return *m
}
```

### 5.2 Phase 3テストケース

#### 5.2.1 Security Tests

```go
// internal/runner/privilege/security_test.go
func TestPrivilegeManager_SecurityFeatures(t *testing.T) {
    t.Run("panic recovery restores privileges", func(t *testing.T) {
        manager := createTestManager(t)

        defer func() {
            if r := recover(); r != nil {
                // Verify privileges were restored after panic
                assert.Equal(t, 1000, syscall.Geteuid())
            }
        }()

        ctx := context.Background()
        elevationCtx := privilege.ElevationContext{
            Operation:   privilege.OperationHealthCheck,
            CommandName: "panic_test",
        }

        err := manager.WithPrivileges(ctx, elevationCtx, func() error {
            panic("test panic")
        })

        // Should not reach here due to panic
        t.Fatal("Expected panic but function returned normally")
    })

    t.Run("context cancellation prevents elevation", func(t *testing.T) {
        manager := createTestManager(t)

        ctx, cancel := context.WithCancel(context.Background())
        cancel() // Cancel immediately

        elevationCtx := privilege.ElevationContext{
            Operation:   privilege.OperationHealthCheck,
            CommandName: "cancelled_test",
        }

        err := manager.WithPrivileges(ctx, elevationCtx, func() error {
            return nil
        })

        assert.Error(t, err)
        assert.Equal(t, context.Canceled, err)
    })
}
```

### 5.3 Phase 3チェックリスト

- [ ] File Validator統合実装
- [ ] ValidateWithPrivileges()メソッド実装
- [ ] audit.Loggerパッケージ実装
- [ ] 構造化監査ログ実装
- [ ] Panic recovery機構実装
- [ ] Context cancellation対応
- [ ] Metrics収集機能実装
- [ ] Performance optimization実装
- [ ] Security tests実装
- [ ] Comprehensive integration tests実装
- [ ] 全テストの通過確認

## 6. Phase 4: 運用機能

### 6.1 実装ステップ

#### 6.1.1 Step 1: Health Check機能

```go
// internal/runner/privilege/health.go
package privilege

import (
    "context"
    "fmt"
    "time"
)

type HealthStatus struct {
    IsSupported        bool          `json:"is_supported"`
    LastCheck          time.Time     `json:"last_check"`
    CheckDuration      time.Duration `json:"check_duration"`
    Error              string        `json:"error,omitempty"`
    SetuidConfigured   bool          `json:"setuid_configured"`
    OriginalUID        int           `json:"original_uid"`
    CanElevate         bool          `json:"can_elevate"`
}

func (m *LinuxPrivilegeManager) GetHealthStatus(ctx context.Context) HealthStatus {
    status := HealthStatus{
        IsSupported:      m.IsPrivilegedExecutionSupported(),
        SetuidConfigured: m.isSetuid,
        OriginalUID:      m.originalUID,
        LastCheck:        time.Now(),
    }

    if !status.IsSupported {
        status.Error = "Privileged execution not supported"
        return status
    }

    // Perform actual health check
    start := time.Now()
    err := m.HealthCheck(ctx)
    status.CheckDuration = time.Since(start)

    if err != nil {
        status.Error = err.Error()
        status.CanElevate = false
    } else {
        status.CanElevate = true
    }

    return status
}
```

#### 6.1.2 Step 2: Configuration Validation

```go
// internal/runner/config/validation.go (拡張)
func ValidatePrivilegedCommands(cfg *runnertypes.Config) []ValidationWarning {
    var warnings []ValidationWarning

    for _, group := range cfg.Groups {
        for _, cmd := range group.Commands {
            if cmd.Privileged {
                // Check for potentially dangerous commands
                if isDangerousCommand(cmd.Cmd) {
                    warnings = append(warnings, ValidationWarning{
                        Type:     "security",
                        Severity: "high",
                        Group:    group.Name,
                        Command:  cmd.Name,
                        Message:  fmt.Sprintf("Privileged command uses potentially dangerous path: %s", cmd.Cmd),
                    })
                }

                // Check for shell commands
                if isShellCommand(cmd.Cmd) {
                    warnings = append(warnings, ValidationWarning{
                        Type:     "security",
                        Severity: "medium",
                        Group:    group.Name,
                        Command:  cmd.Name,
                        Message:  "Privileged shell commands require extra caution",
                    })
                }
            }
        }
    }

    return warnings
}

func isDangerousCommand(cmdPath string) bool {
    dangerous := []string{
        "/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash",
        "/bin/su", "/usr/bin/su", "/usr/bin/sudo",
        "/sbin/init", "/usr/sbin/init",
        "/bin/rm", "/usr/bin/rm", // without args checking
    }

    for _, d := range dangerous {
        if cmdPath == d {
            return true
        }
    }
    return false
}
```

#### 6.1.3 Step 3: Metrics Collection

```go
// internal/runner/privilege/prometheus.go (optional)
package privilege

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    privilegeElevationAttempts = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "privilege_elevation_attempts_total",
            Help: "Total number of privilege elevation attempts",
        },
        []string{"operation", "success"},
    )

    privilegeElevationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "privilege_elevation_duration_seconds",
            Help: "Time spent in elevated privileges",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )

    privilegedCommandsExecuted = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "privileged_commands_executed_total",
            Help: "Total number of privileged commands executed",
        },
        []string{"command", "exit_code"},
    )
)

func (m *LinuxPrivilegeManager) recordMetrics(operation string, duration time.Duration, success bool) {
    successLabel := "false"
    if success {
        successLabel = "true"
    }

    privilegeElevationAttempts.WithLabelValues(string(operation), successLabel).Inc()
    privilegeElevationDuration.WithLabelValues(string(operation)).Observe(duration.Seconds())
}
```

### 6.2 Phase 4チェックリスト

- [ ] HealthStatus構造体実装
- [ ] GetHealthStatus()メソッド実装
- [ ] Configuration validation実装
- [ ] Dangerous command detection実装
- [ ] Prometheus metrics実装（オプション）
- [ ] CLI health check command実装
- [ ] Documentation更新
- [ ] 運用ガイド作成

## 7. 統合テスト計画

### 7.1 End-to-End テストシナリオ

#### 7.1.1 基本シナリオ

```go
// integration_test.go
func TestPrivilegedExecution_E2E(t *testing.T) {
    if !canRunPrivilegedTests() {
        t.Skip("Privileged tests require setuid binary or root")
    }

    scenarios := []struct {
        name     string
        config   string
        expected []ExpectedResult
    }{
        {
            name: "mixed privileged and normal commands",
            config: `
                [[groups]]
                name = "test_group"

                [[groups.commands]]
                name = "normal_cmd"
                cmd = "echo"
                args = ["normal"]
                privileged = false

                [[groups.commands]]
                name = "privileged_cmd"
                cmd = "id"
                args = ["-u"]
                privileged = true
            `,
            expected: []ExpectedResult{
                {Command: "normal_cmd", ExitCode: 0, NoPrivilegeElevation: true},
                {Command: "privileged_cmd", ExitCode: 0, PrivilegeElevationCount: 2},
            },
        },
    }

    for _, scenario := range scenarios {
        t.Run(scenario.name, func(t *testing.T) {
            runE2ETest(t, scenario.config, scenario.expected)
        })
    }
}

type ExpectedResult struct {
    Command                string
    ExitCode               int
    NoPrivilegeElevation   bool
    PrivilegeElevationCount int
}

func runE2ETest(t *testing.T, configToml string, expected []ExpectedResult) {
    // Setup test configuration
    tempDir := t.TempDir()
    configFile := filepath.Join(tempDir, "config.toml")
    err := os.WriteFile(configFile, []byte(configToml), 0644)
    require.NoError(t, err)

    // Create runner with real privilege manager
    cfg, err := config.LoadConfig(configFile)
    require.NoError(t, err)

    // Track privilege manager calls
    mockLogger := NewMockLogger()
    privMgr := privilege.NewManager(mockLogger.Logger)

    runner, err := NewRunner(cfg, WithPrivilegeManager(privMgr))
    require.NoError(t, err)

    // Execute all groups
    ctx := context.Background()
    for _, group := range cfg.Groups {
        err = runner.ExecuteGroup(ctx, group)
        assert.NoError(t, err)
    }

    // Verify results
    for _, expect := range expected {
        if expect.NoPrivilegeElevation {
            assert.False(t, mockLogger.HasPrivilegeElevationLogs(expect.Command))
        } else {
            assert.Equal(t, expect.PrivilegeElevationCount,
                mockLogger.GetPrivilegeElevationCount(expect.Command))
        }
    }
}
```

### 7.2 セキュリティテストシナリオ

```go
func TestSecurity_Scenarios(t *testing.T) {
    testCases := []struct {
        name           string
        setupFunc      func(t *testing.T) *Runner
        testFunc       func(t *testing.T, runner *Runner)
        expectPanic    bool
    }{
        {
            name: "privilege restoration after panic",
            setupFunc: func(t *testing.T) *Runner {
                return createRunnerWithTestPrivilegeManager(t)
            },
            testFunc: func(t *testing.T, runner *Runner) {
                // Test that privileges are restored even after panic
                defer func() {
                    r := recover()
                    assert.NotNil(t, r)
                    // Verify we're back to original UID
                    assert.Equal(t, 1000, syscall.Geteuid())
                }()

                // This should panic but still restore privileges
                runner.executeWithPanic()
            },
            expectPanic: true,
        },
    }
}
```

## 8. デプロイメント計画

### 8.1 バイナリ配布準備

#### 8.1.1 Makefile拡張

```makefile
# Makefile (拡張)

# Build targets with privilege support
.PHONY: build-privileged
build-privileged:
	@echo "Building with privilege support..."
	@CGO_ENABLED=0 GOOS=linux go build -o build/runner-privileged \
		-ldflags="-X main.version=$(VERSION) -X main.privilegeSupport=enabled" \
		./cmd/runner

# Install with setuid (requires root)
.PHONY: install-setuid
install-setuid: build-privileged
	@echo "Installing with setuid permissions (requires root)..."
	@sudo cp build/runner-privileged /usr/local/bin/go-safe-cmd-runner
	@sudo chown root:root /usr/local/bin/go-safe-cmd-runner
	@sudo chmod 4755 /usr/local/bin/go-safe-cmd-runner
	@echo "Setuid installation complete"

# Verify setuid installation
.PHONY: verify-setuid
verify-setuid:
	@echo "Verifying setuid installation..."
	@ls -la /usr/local/bin/go-safe-cmd-runner | grep "rws"
	@/usr/local/bin/go-safe-cmd-runner --health-check
```

#### 8.1.2 インストールスクリプト

```bash
#!/bin/bash
# install.sh - Privileged installation script

set -e

VERSION=${1:-"latest"}
INSTALL_DIR=${2:-"/usr/local/bin"}
BINARY_NAME="go-safe-cmd-runner"

echo "Installing go-safe-cmd-runner with privileged execution support..."

# Check if running as root
if [[ $EUID -eq 0 ]]; then
    echo "Installing as root - setuid will be configured"
    SETUID_INSTALL=true
else
    echo "Installing as user - privileged execution will not be available"
    SETUID_INSTALL=false
fi

# Download or copy binary
if [[ "$VERSION" == "latest" ]]; then
    echo "Building from source..."
    make build-privileged
    cp build/runner-privileged "$INSTALL_DIR/$BINARY_NAME"
else
    echo "Downloading version $VERSION..."
    # curl download logic here
fi

# Set permissions
if [[ "$SETUID_INSTALL" == "true" ]]; then
    chown root:root "$INSTALL_DIR/$BINARY_NAME"
    chmod 4755 "$INSTALL_DIR/$BINARY_NAME"
    echo "Setuid permissions configured"
else
    chmod 755 "$INSTALL_DIR/$BINARY_NAME"
    echo "Standard permissions configured"
fi

# Verify installation
echo "Verifying installation..."
"$INSTALL_DIR/$BINARY_NAME" --version
"$INSTALL_DIR/$BINARY_NAME" --health-check

echo "Installation complete!"
```

### 8.2 Documentation

#### 8.2.1 運用ガイド

```markdown
# Privileged Execution Setup Guide

## Prerequisites

1. Linux/Unix system
2. Root access for setuid configuration
3. go-safe-cmd-runner built with privilege support

## Installation Steps

### Step 1: Build Binary
```bash
make build-privileged
```

### Step 2: Install with Setuid (requires root)
```bash
sudo make install-setuid
```

### Step 3: Verify Installation
```bash
go-safe-cmd-runner --health-check
```

## Configuration

### Enable Privileged Commands
```toml
[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/mysqldump"
args = ["-u", "root", "-p${DB_PASSWORD}", "production"]
privileged = true  # Enables root execution
```

## Security Considerations

1. **Minimal Usage**: Only use privileged=true when absolutely necessary
2. **Command Validation**: All privileged commands are validated before execution
3. **Audit Logging**: All privileged executions are logged for audit purposes
4. **File Verification**: Command binaries are verified if hash manifests are configured

## Troubleshooting

### Common Issues

1. **"privileged execution not supported"**
   - Ensure binary is installed with setuid bit
   - Check: `ls -la $(which go-safe-cmd-runner)` should show `rws`

2. **"failed to elevate privileges"**
   - Verify setuid bit is set correctly
   - Check system permissions
   - Review audit logs

### Health Check Command
```bash
go-safe-cmd-runner --health-check --verbose
```
```

## 9. 実装完了基準

### 9.1 機能完了基準

- [ ] **Phase 1 Complete**: 基盤インフラ実装完了
  - [ ] Privilege Manager interface完成
  - [ ] Linux/Windows platform実装完成
  - [ ] Basic unit tests通過 (90%+ coverage)

- [ ] **Phase 2 Complete**: Executor統合完了
  - [ ] DefaultExecutor拡張完成
  - [ ] Runner integration完成
  - [ ] Integration tests通過

- [ ] **Phase 3 Complete**: セキュリティ強化完了
  - [ ] File Validator統合完成
  - [ ] Audit logging実装完成
  - [ ] Fail-safe mechanisms実装完成
  - [ ] Security tests通過

- [ ] **Phase 4 Complete**: 運用機能完了
  - [ ] Health check機能完成
  - [ ] Configuration validation完成
  - [ ] Documentation完成

### 9.2 品質基準

- [ ] **Test Coverage**: 90%以上のテストカバレッジ
- [ ] **Security Review**: セキュリティレビュー完了
- [ ] **Performance**: Privilege elevation < 10ms per operation
- [ ] **Documentation**: 運用ガイド・APIドキュメント完成
- [ ] **Integration**: 全既存テストが引き続き通過

### 9.3 リリース基準

- [ ] **Backward Compatibility**: privileged=falseコマンドの動作変更なし
- [ ] **Error Handling**: 全エラーケースの適切な処理
- [ ] **Logging**: 構造化ログによる適切な監査証跡
- [ ] **Platform Support**: Linux/Windows両対応
- [ ] **Security Validation**: セキュリティ脆弱性スキャン通過

## 10. リスク軽減策

### 10.1 技術リスク

| リスク | 影響度 | 対策 |
|--------|---------|------|
| setuidの権限昇格失敗 | 高 | 詳細なエラーログ・フォールバック機構 |
| 権限復帰失敗 | 致命的 | panic()による強制終了・defer保証 |
| プラットフォーム互換性 | 中 | Build tag分離・十分なテスト |
| Performance regression | 低 | Benchmark test・プロファイリング |

### 10.2 セキュリティリスク

| リスク | 影響度 | 対策 |
|--------|---------|------|
| 特権乱用 | 高 | 最小権限原則・監査ログ・設定検証 |
| Code injection | 高 | 引数検証・Shellコマンド制限 |
| 権限昇格時間延長 | 中 | タイムアウト設定・メトリクス監視 |

この実装計画に従って段階的に開発を進めることで、安全で効率的なprivilegedコマンド実行機能を実装できます。
