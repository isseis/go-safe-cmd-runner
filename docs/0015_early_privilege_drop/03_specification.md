# 詳細仕様書：早期特権放棄による安全なSUIDバイナリ実装

## 1. 機能概要

本仕様書は、早期特権放棄（Early Privilege Drop）による安全なSUIDバイナリ実装の詳細な技術仕様を定義する。main関数での即座的な`seteuid(getuid())`実行により、デフォルト状態を非特権とし、必要時のみ一時的に特権昇格を行う機能を提供する。

## 2. データ構造仕様

### 2.1 拡張PrivilegeManagerインターフェース

```go
// internal/runner/runnertypes/privilege.go

// PrivilegeManager defines methods for privilege management with early drop support
type PrivilegeManager interface {
    // 既存メソッド（互換性維持）
    IsPrivilegedExecutionSupported() bool
    ExecuteWithPrivileges(cmd *exec.Cmd) error
    GetOriginalUID() int

    // 早期特権放棄サポート（新規追加）
    WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error
}

// PrivilegeState represents the current privilege state
type PrivilegeState struct {
    RealUID      int       `json:"real_uid"`
    EffectiveUID int       `json:"effective_uid"`
    IsElevated   bool      `json:"is_elevated"`
    CanElevate   bool      `json:"can_elevate"`
    LastChanged  time.Time `json:"last_changed"`
    OperationID  string    `json:"operation_id,omitempty"`
}

// ElevationContext contains context information for privilege operations
type ElevationContext struct {
    Operation   Operation `json:"operation"`
    Command     string    `json:"command,omitempty"`
    FilePath    string    `json:"file_path,omitempty"`
    StartTime   time.Time `json:"start_time"`
    OriginalUID int       `json:"original_uid"`
    TargetUID   int       `json:"target_uid"`
    Reason      string    `json:"reason"`
    OperationID string    `json:"operation_id"`
}

// Operation types for privilege elevation context
type Operation string

const (
    OperationCommandExecution   Operation = "command_execution"
    OperationDirectoryCreation  Operation = "directory_creation"
    OperationFileAccess        Operation = "file_access"
    OperationSystemCall        Operation = "system_call"
    OperationInitialization    Operation = "initialization"
    OperationCleanup           Operation = "cleanup"
)
```

### 2.2 早期特権放棄管理構造

```go
// internal/runner/privilege/early_drop.go

// EarlyDropManager implements early privilege drop functionality
type EarlyDropManager struct {
    originalUID      int
    originalEUID     int
    currentState     PrivilegeState
    logger          *slog.Logger
    securityLogger  SecurityLogger
    metrics         *Metrics
    isEarlyDropped  bool
    mutex           sync.RWMutex
}

// EarlyDropConfig configures early drop behavior
type EarlyDropConfig struct {
    ForceEarlyDrop        bool          `json:"force_early_drop"`
    MaxElevationDuration  time.Duration `json:"max_elevation_duration"`
    AuditLogEnabled       bool          `json:"audit_log_enabled"`
    EmergencyShutdownDelay time.Duration `json:"emergency_shutdown_delay"`
    SecurityLogLevel      string        `json:"security_log_level"`
}

// Metrics for privilege operations
type Metrics struct {
    TotalElevations       int64         `json:"total_elevations"`
    TotalDrops           int64         `json:"total_drops"`
    FailedElevations     int64         `json:"failed_elevations"`
    FailedDrops          int64         `json:"failed_drops"`
    AverageElevationTime time.Duration `json:"avg_elevation_time_ns"`
    MaxElevationTime     time.Duration `json:"max_elevation_time_ns"`
    TotalElevationTime   time.Duration `json:"total_elevation_time_ns"`
    EmergencyShutdowns   int64         `json:"emergency_shutdowns"`
    LastOperation        time.Time     `json:"last_operation"`
}
```

### 2.3 エラー定義

```go
// internal/runner/privilege/errors.go

// 権限管理関連のエラー定義
var (
    // 致命的エラー（プロセス終了が必要）
    ErrPrivilegeRestorationFailed = errors.New("privilege restoration failed")
    ErrPrivilegeStateCorrupted    = errors.New("privilege state corrupted")
    ErrCriticalSecurityFailure   = errors.New("critical security failure")

    // 回復可能エラー
    ErrPrivilegeElevationFailed   = errors.New("privilege elevation failed")
    ErrPrivilegeDropFailed        = errors.New("privilege drop failed")
    ErrNotPrivilegedBinary        = errors.New("not a privileged binary")
    ErrAlreadyElevated           = errors.New("privileges already elevated")
    ErrNotElevated               = errors.New("privileges not currently elevated")

    // 設定エラー
    ErrInvalidConfiguration      = errors.New("invalid privilege configuration")
    ErrUnsupportedPlatform      = errors.New("privilege management not supported on this platform")
)

// PrivilegeError provides detailed error information
type PrivilegeError struct {
    Operation   Operation
    Reason      string
    OriginalUID int
    TargetUID   int
    SystemError error
    Timestamp   time.Time
}

func (e *PrivilegeError) Error() string {
    return fmt.Sprintf("privilege %s failed: %s (UID %d -> %d): %v",
        e.Operation, e.Reason, e.OriginalUID, e.TargetUID, e.SystemError)
}
```

## 3. 実装仕様

### 3.1 main関数での早期特権放棄

```go
// cmd/runner/main.go

func main() {
    // Phase 1: 致命的初期化（root権限必要）
    if err := performCriticalInitialization(); err != nil {
        log.Fatal("Critical initialization failed:", err)
    }

    // Phase 2: 早期特権放棄
    if err := performEarlyPrivilegeDrop(); err != nil {
        log.Fatal("Early privilege drop failed:", err)
    }

    // Phase 3: 通常アプリケーション実行（実UID権限）
    if err := runApplication(); err != nil {
        log.Fatal("Application execution failed:", err)
    }
}

// performCriticalInitialization performs initialization that requires root privileges
func performCriticalInitialization() error {
    // ログシステムの初期化
    if err := initializeLogging(); err != nil {
        return fmt.Errorf("logging initialization failed: %w", err)
    }

    // 重要なシステム設定の読み込み（root権限必要）
    if err := loadCriticalSystemConfig(); err != nil {
        return fmt.Errorf("critical system config load failed: %w", err)
    }

    // セキュリティ監査システムの初期化
    if err := initializeSecurityAudit(); err != nil {
        return fmt.Errorf("security audit initialization failed: %w", err)
    }

    return nil
}

// performEarlyPrivilegeDrop implements the early privilege drop pattern
func performEarlyPrivilegeDrop() error {
    realUID := syscall.Getuid()
    effectiveUID := syscall.Geteuid()

    // SUIDバイナリでない場合はスキップ
    if realUID == effectiveUID {
        log.Printf("Not a SUID binary (UID=%d, EUID=%d), skipping privilege drop", realUID, effectiveUID)
        return nil
    }

    // 早期特権放棄を実行
    log.Printf("Performing early privilege drop: EUID %d -> %d", effectiveUID, realUID)

    if err := syscall.Seteuid(realUID); err != nil {
        return fmt.Errorf("failed to drop privileges to real UID %d: %w", realUID, err)
    }

    // 特権放棄の検証
    newEUID := syscall.Geteuid()
    if newEUID != realUID {
        return fmt.Errorf("privilege drop verification failed: expected EUID %d, got %d", realUID, newEUID)
    }

    log.Printf("Early privilege drop successful: now running as UID %d", realUID)

    // セキュリティログに記録
    auditLog := PrivilegeAuditLog{
        Timestamp:   time.Now(),
        Operation:   OperationInitialization,
        Action:      "early_privilege_drop",
        ProcessID:   os.Getpid(),
        OriginalUID: realUID,
        OriginalEUID: effectiveUID,
        TargetUID:   realUID,
        Success:     true,
    }
    logSecurityEvent(auditLog)

    return nil
}
```

### 3.2 EarlyDropManagerの実装

```go
// internal/runner/privilege/early_drop.go

// NewEarlyDropManager creates a new privilege manager with early drop support
func NewEarlyDropManager(config EarlyDropConfig, logger *slog.Logger) (*EarlyDropManager, error) {
    manager := &EarlyDropManager{
        originalUID:    syscall.Getuid(),
        originalEUID:   syscall.Geteuid(),
        logger:         logger,
        metrics:        &Metrics{},
        isEarlyDropped: false,
    }

    // 初期状態の設定
    manager.currentState = PrivilegeState{
        RealUID:      manager.originalUID,
        EffectiveUID: manager.originalEUID,
        IsElevated:   manager.originalEUID == 0 && manager.originalUID != 0,
        CanElevate:   manager.canElevatePrivileges(),
        LastChanged:  time.Now(),
    }

    // セキュリティロガーの初期化
    if config.AuditLogEnabled {
        secLogger, err := NewSecurityLogger(config.SecurityLogLevel)
        if err != nil {
            return nil, fmt.Errorf("failed to initialize security logger: %w", err)
        }
        manager.securityLogger = secLogger
    }

    return manager, nil
}

// DropToRealUID implements early privilege drop
func (m *EarlyDropManager) DropToRealUID() error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    // 既に実UIDで動作中の場合はスキップ
    if m.currentState.EffectiveUID == m.originalUID {
        m.logger.Debug("Already running as real UID", "uid", m.originalUID)
        return nil
    }

    startTime := time.Now()

    // 実UIDに切り替え
    if err := syscall.Seteuid(m.originalUID); err != nil {
        m.metrics.FailedDrops++
        return &PrivilegeError{
            Operation:   OperationInitialization,
            Reason:      "early privilege drop failed",
            OriginalUID: m.originalUID,
            TargetUID:   m.originalUID,
            SystemError: err,
            Timestamp:   time.Now(),
        }
    }

    // 状態更新
    m.currentState.EffectiveUID = m.originalUID
    m.currentState.IsElevated = false
    m.currentState.LastChanged = time.Now()
    m.isEarlyDropped = true
    m.metrics.TotalDrops++

    duration := time.Since(startTime)
    m.logger.Info("Early privilege drop completed",
        "original_euid", m.originalEUID,
        "target_uid", m.originalUID,
        "duration_ns", duration.Nanoseconds())

    // セキュリティログ
    if m.securityLogger != nil {
        m.securityLogger.LogPrivilegeEvent(PrivilegeAuditLog{
            Timestamp:    startTime,
            Operation:    OperationInitialization,
            Action:       "early_drop",
            ProcessID:    os.Getpid(),
            OriginalUID:  m.originalUID,
            OriginalEUID: m.originalEUID,
            TargetUID:    m.originalUID,
            Duration:     duration.Nanoseconds(),
            Success:      true,
        })
    }

    return nil
}
```

### 3.3 WithPrivileges メソッドの詳細実装

```go
// WithPrivileges executes a function with temporary privilege elevation
func (m *EarlyDropManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error {
    // 操作IDの生成
    operationID := generateOperationID()
    elevationCtx.OperationID = operationID

    m.mutex.Lock()
    defer m.mutex.Unlock()

    startTime := time.Now()
    elevationCtx.StartTime = startTime

    // 特権昇格可能性の確認
    if !m.currentState.CanElevate {
        return fmt.Errorf("%w: cannot elevate privileges in current state", ErrNotPrivilegedBinary)
    }

    // 既に昇格済みかチェック
    if m.currentState.IsElevated {
        return fmt.Errorf("%w: already elevated for operation %s", ErrAlreadyElevated, m.currentState.OperationID)
    }

    m.logger.Info("Elevating privileges",
        "operation", elevationCtx.Operation,
        "command", elevationCtx.CommandName,
        "operation_id", operationID,
        "reason", elevationCtx.Reason)

    // 特権昇格実行
    if err := m.elevatePrivilegesInternal(elevationCtx); err != nil {
        m.metrics.FailedElevations++
        return fmt.Errorf("privilege elevation failed: %w", err)
    }

    // 確実な権限復帰のためのdefer設定
    var executionError error
    defer func() {
        // panic recovery
        if panicValue := recover(); panicValue != nil {
            m.logger.Error("Panic occurred during privileged operation",
                "panic", panicValue,
                "operation_id", operationID,
                "operation", elevationCtx.Operation)
            executionError = fmt.Errorf("panic during privileged operation: %v", panicValue)
        }

        // 権限復帰（最重要処理）
        if err := m.restorePrivilegesInternal(elevationCtx); err != nil {
            m.emergencyShutdown(err, fmt.Sprintf("privilege restoration after %s (operation_id: %s)", elevationCtx.Operation, operationID))
        }

        // パフォーマンスメトリクス更新
        duration := time.Since(startTime)
        m.updateMetrics(duration, executionError == nil)

        m.logger.Info("Privilege operation completed",
            "operation", elevationCtx.Operation,
            "operation_id", operationID,
            "duration_ms", duration.Milliseconds(),
            "success", executionError == nil)

        // panic再発生（必要な場合）
        if panicValue := recover(); panicValue != nil {
            panic(panicValue)
        }
    }()

    // 特権処理の実行
    executionError = fn()
    return executionError
}

// elevatePrivilegesInternal performs the actual privilege elevation
func (m *EarlyDropManager) elevatePrivilegesInternal(ctx ElevationContext) error {
    if err := syscall.Seteuid(0); err != nil {
        return &PrivilegeError{
            Operation:   ctx.Operation,
            Reason:      "seteuid(0) failed",
            OriginalUID: m.originalUID,
            TargetUID:   0,
            SystemError: err,
            Timestamp:   time.Now(),
        }
    }

    // 状態更新
    m.currentState.EffectiveUID = 0
    m.currentState.IsElevated = true
    m.currentState.LastChanged = time.Now()
    m.currentState.OperationID = ctx.OperationID
    m.metrics.TotalElevations++

    // セキュリティログ
    if m.securityLogger != nil {
        m.securityLogger.LogPrivilegeEvent(PrivilegeAuditLog{
            Timestamp:    ctx.StartTime,
            Operation:    ctx.Operation,
            Action:       "elevate",
            CommandName:  ctx.CommandName,
            ProcessID:    os.Getpid(),
            OriginalUID:  m.originalUID,
            TargetUID:    0,
            OperationID:  ctx.OperationID,
            Success:      true,
        })
    }

    return nil
}

// restorePrivilegesInternal performs privilege restoration
func (m *EarlyDropManager) restorePrivilegesInternal(ctx ElevationContext) error {
    if err := syscall.Seteuid(m.originalUID); err != nil {
        return &PrivilegeError{
            Operation:   ctx.Operation,
            Reason:      "privilege restoration failed",
            OriginalUID: m.originalUID,
            TargetUID:   m.originalUID,
            SystemError: err,
            Timestamp:   time.Now(),
        }
    }

    // 権限復帰の検証
    currentEUID := syscall.Geteuid()
    if currentEUID != m.originalUID {
        return fmt.Errorf("privilege restoration verification failed: expected EUID %d, got %d", m.originalUID, currentEUID)
    }

    // 状態更新
    m.currentState.EffectiveUID = m.originalUID
    m.currentState.IsElevated = false
    m.currentState.LastChanged = time.Now()
    m.currentState.OperationID = ""

    // セキュリティログ
    if m.securityLogger != nil {
        m.securityLogger.LogPrivilegeEvent(PrivilegeAuditLog{
            Timestamp:    time.Now(),
            Operation:    ctx.Operation,
            Action:       "restore",
            CommandName:  ctx.CommandName,
            ProcessID:    os.Getpid(),
            OriginalUID:  m.originalUID,
            TargetUID:    m.originalUID,
            OperationID:  ctx.OperationID,
            Duration:     time.Since(ctx.StartTime).Nanoseconds(),
            Success:      true,
        })
    }

    return nil
}
```

### 3.4 緊急シャットダウンメカニズム

```go
// emergencyShutdown handles critical privilege restoration failures
func (m *EarlyDropManager) emergencyShutdown(restoreErr error, context string) {
    m.metrics.EmergencyShutdowns++

    criticalMsg := fmt.Sprintf("CRITICAL SECURITY FAILURE: %s", context)

    // 詳細な状態情報を収集
    currentUID := syscall.Getuid()
    currentEUID := syscall.Geteuid()
    pid := os.Getpid()
    timestamp := time.Now().UTC()

    // 複数の出力先への緊急ログ記録
    errorDetails := map[string]interface{}{
        "error":           restoreErr.Error(),
        "context":         context,
        "original_uid":    m.originalUID,
        "original_euid":   m.originalEUID,
        "current_uid":     currentUID,
        "current_euid":    currentEUID,
        "process_id":      pid,
        "timestamp":       timestamp,
        "metrics":         m.metrics,
        "current_state":   m.currentState,
    }

    // 1. 構造化ログシステム
    m.logger.Error(criticalMsg, "emergency_shutdown", errorDetails)

    // 2. システムログ（syslog）
    if err := logToSyslog(syslog.LOG_CRIT, fmt.Sprintf("%s: %v (PID: %d, UID: %d->%d, EUID: %d->%d)",
        criticalMsg, restoreErr, pid, m.originalUID, currentUID, m.originalEUID, currentEUID)); err != nil {
        // syslogの失敗は無視（既に致命的状況）
        fmt.Fprintf(os.Stderr, "WARNING: Failed to write to syslog during emergency shutdown: %v\n", err)
    }

    // 3. セキュリティログ
    if m.securityLogger != nil {
        m.securityLogger.LogCriticalEvent(CriticalSecurityEvent{
            Type:        "privilege_restoration_failure",
            Message:     criticalMsg,
            Error:       restoreErr.Error(),
            Context:     context,
            ProcessInfo: errorDetails,
            Timestamp:   timestamp,
        })
    }

    // 4. 標準エラー出力（最後の手段）
    fmt.Fprintf(os.Stderr, "FATAL SECURITY ERROR: %s\n", criticalMsg)
    fmt.Fprintf(os.Stderr, "Error: %v\n", restoreErr)
    fmt.Fprintf(os.Stderr, "Process ID: %d\n", pid)
    fmt.Fprintf(os.Stderr, "UID: %d -> %d\n", m.originalUID, currentUID)
    fmt.Fprintf(os.Stderr, "EUID: %d -> %d\n", m.originalEUID, currentEUID)
    fmt.Fprintf(os.Stderr, "Timestamp: %s\n", timestamp.Format(time.RFC3339))

    // 5. コアダンプの有効化（デバッグ情報保存）
    enableCoreDump()

    // 6. 即座にプロセス終了（defer処理をスキップ）
    os.Exit(1)
}

// enableCoreDump enables core dump generation for debugging
func enableCoreDump() {
    // Set core dump size limit to unlimited
    var rlimit syscall.Rlimit
    if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &rlimit); err == nil {
        rlimit.Cur = rlimit.Max
        syscall.Setrlimit(syscall.RLIMIT_CORE, &rlimit)
    }
}
```

### 3.5 ResourceManager統合仕様

```go
// internal/runner/resource/manager.go

// CreateTempDir creates a temporary directory with natural ownership (no chown needed)
func (m *Manager) CreateTempDir(commandName string) (*Resource, error) {
    // Generate unique resource ID
    resourceID := generateResourceID()

    // Sanitize command name for directory path
    safeName := sanitizeName(commandName)
    tempDirPath := filepath.Join(m.baseDir, "cmd-runner", safeName, resourceID)

    // Create directory with real UID ownership (no privilege escalation needed)
    // Since we're running as real UID after early privilege drop,
    // the directory will naturally be owned by the correct user
    tempDirPath, err := m.fs.CreateTempDir(m.baseDir, resourceID)
    if err != nil {
        return nil, fmt.Errorf("failed to create temporary directory: %w", err)
    }

    // Remove the adjustDirectoryOwnership call entirely
    // The directory is already created with correct ownership

    // Create the resource
    resource := &Resource{
        ID:          resourceID,
        Type:        TypeTempDir,
        Path:        tempDirPath,
        CommandName: commandName,
        CreatedAt:   time.Now(),
    }

    // Register the resource
    m.mu.Lock()
    m.resources[resourceID] = resource
    m.mu.Unlock()

    m.logger.Info("Temporary directory created",
        "resource_id", resourceID,
        "path", tempDirPath,
        "command", commandName,
        "uid", syscall.Geteuid()) // Should be real UID

    return resource, nil
}

// DEPRECATED: adjustDirectoryOwnership is no longer needed
// This method is kept for backward compatibility but does nothing
func (m *Manager) adjustDirectoryOwnership(dirPath string) error {
    m.logger.Debug("adjustDirectoryOwnership called but no longer needed due to early privilege drop",
        "path", dirPath)
    return nil
}
```

## 4. 統合仕様

### 4.1 Command Executor統合

```go
// internal/runner/executor/privileged_executor.go

// ExecuteCommand executes a command with appropriate privilege management
func (e *Executor) ExecuteCommand(ctx context.Context, cmd *Command) error {
    if cmd.Privileged {
        return e.executePrivilegedCommand(ctx, cmd)
    }
    return e.executeNormalCommand(ctx, cmd)
}

// executePrivilegedCommand executes a command with temporary privilege elevation
func (e *Executor) executePrivilegedCommand(ctx context.Context, cmd *Command) error {
    // Check privilege manager availability
    if e.privilegeManager == nil {
        return fmt.Errorf("privileged command '%s' requires privilege manager but none available", cmd.Name)
    }

    if !e.privilegeManager.CanElevatePrivileges() {
        return fmt.Errorf("privileged command '%s' cannot be executed: privilege escalation not available", cmd.Name)
    }

    // Create elevation context
    elevationCtx := ElevationContext{
        Operation:   OperationCommandExecution,
        CommandName: cmd.Name,
        StartTime:   time.Now(),
        OriginalUID: e.privilegeManager.GetOriginalUID(),
        TargetUID:   0,
        Reason:      fmt.Sprintf("Execute privileged command: %s %v", cmd.Name, cmd.Args),
    }

    // Execute with temporary privilege elevation
    return e.privilegeManager.WithPrivileges(ctx, elevationCtx, func() error {
        return e.executeCommandInternal(ctx, cmd)
    })
}

// executeNormalCommand executes a command with current (real UID) privileges
func (e *Executor) executeNormalCommand(ctx context.Context, cmd *Command) error {
    // Verify we're running as real UID
    if e.privilegeManager != nil && e.privilegeManager.IsCurrentlyElevated() {
        return fmt.Errorf("normal command '%s' attempted while privileges are elevated", cmd.Name)
    }

    return e.executeCommandInternal(ctx, cmd)
}
```

### 4.2 設定ファイル統合

```toml
# Configuration for early privilege drop
[global]
# ... existing configuration

# Early privilege drop settings
[privilege_management]
early_drop_enabled = true
max_elevation_duration = "30s"
audit_log_enabled = true
security_log_level = "info"
emergency_shutdown_delay = "100ms"

# Commands with privilege requirements
[[groups]]
name = "privileged_operations"
[[groups.commands]]
name = "system_config_update"
cmd = "/usr/bin/systemctl"
args = ["reload", "nginx"]
privileged = true  # Requires temporary privilege elevation

[[groups.commands]]
name = "log_rotation"
cmd = "/usr/sbin/logrotate"
args = ["/etc/logrotate.conf"]
privileged = true  # Requires temporary privilege elevation

[[groups.commands]]
name = "user_file_operation"
cmd = "touch"
args = ["user_file.txt"]
privileged = false  # Runs with real UID
```

## 5. テスト仕様

### 5.1 単体テスト仕様

```go
// internal/runner/privilege/early_drop_test.go

// TestEarlyPrivilegeDrop tests the early privilege drop functionality
func TestEarlyPrivilegeDrop(t *testing.T) {
    if !isSUIDBinary() {
        t.Skip("SUID binary required for this test")
    }

    // Create manager
    config := EarlyDropConfig{
        ForceEarlyDrop:       true,
        MaxElevationDuration: 30 * time.Second,
        AuditLogEnabled:      true,
    }

    manager, err := NewEarlyDropManager(config, slog.Default())
    require.NoError(t, err)

    // Record initial state
    initialUID := syscall.Getuid()
    initialEUID := syscall.Geteuid()

    // Perform early drop
    err = manager.DropToRealUID()
    require.NoError(t, err)

    // Verify privilege drop
    assert.Equal(t, initialUID, syscall.Geteuid())
    assert.False(t, manager.IsCurrentlyElevated())
    assert.True(t, manager.isEarlyDropped)
}

// TestPrivilegeElevationAndRestoration tests temporary privilege elevation
func TestPrivilegeElevationAndRestoration(t *testing.T) {
    manager := setupTestManager(t)

    // Ensure we start in dropped state
    err := manager.DropToRealUID()
    require.NoError(t, err)

    originalUID := syscall.Getuid()
    assert.Equal(t, originalUID, syscall.Geteuid())

    // Test privilege elevation and restoration
    ctx := context.Background()
    elevationCtx := ElevationContext{
        Operation:   OperationCommandExecution,
        CommandName: "test",
        StartTime:   time.Now(),
        OriginalUID: originalUID,
        TargetUID:   0,
        Reason:      "test privilege elevation",
    }

    var elevatedUID int
    err = manager.WithPrivileges(ctx, elevationCtx, func() error {
        elevatedUID = syscall.Geteuid()
        return nil
    })

    require.NoError(t, err)
    assert.Equal(t, 0, elevatedUID)              // Was elevated during execution
    assert.Equal(t, originalUID, syscall.Geteuid()) // Restored after execution
    assert.False(t, manager.IsCurrentlyElevated())   // Not elevated anymore
}

// TestEmergencyShutdown tests the emergency shutdown mechanism
func TestEmergencyShutdown(t *testing.T) {
    // This test requires subprocess execution to test os.Exit(1)
    if os.Getenv("TEST_EMERGENCY_SHUTDOWN") == "1" {
        // This subprocess will perform emergency shutdown
        manager := setupTestManager(t)

        // Force a privilege restoration failure scenario
        manager.emergencyShutdown(errors.New("simulated restore failure"), "test scenario")
        return
    }

    // Run the subprocess
    cmd := exec.Command(os.Args[0], "-test.run=TestEmergencyShutdown")
    cmd.Env = append(os.Environ(), "TEST_EMERGENCY_SHUTDOWN=1")

    output, err := cmd.CombinedOutput()

    // Verify the subprocess exited with code 1
    if exitError, ok := err.(*exec.ExitError); ok {
        assert.Equal(t, 1, exitError.ExitCode())
    } else {
        t.Fatalf("Expected subprocess to exit with code 1, got: %v", err)
    }

    // Verify emergency shutdown messages in output
    outputStr := string(output)
    assert.Contains(t, outputStr, "CRITICAL SECURITY FAILURE")
    assert.Contains(t, outputStr, "simulated restore failure")
}
```

### 5.2 統合テスト仕様

```go
// integration_test.go

// TestEndToEndPrivilegeManagement tests the complete privilege management flow
func TestEndToEndPrivilegeManagement(t *testing.T) {
    if !isSUIDBinary() {
        t.Skip("SUID binary required for integration test")
    }

    // Create test configuration
    config := createTestConfig(t)

    // Initialize runner with privilege management
    runner, err := NewRunner(config)
    require.NoError(t, err)

    // Verify early privilege drop occurred
    assert.Equal(t, syscall.Getuid(), syscall.Geteuid())

    // Execute mixed privileged and normal commands
    groups := []*Group{
        createNormalCommandGroup(),
        createPrivilegedCommandGroup(),
        createMixedCommandGroup(),
    }

    for _, group := range groups {
        err := runner.ExecuteGroup(context.Background(), group)
        assert.NoError(t, err)

        // Verify we end in non-privileged state
        assert.Equal(t, syscall.Getuid(), syscall.Geteuid())
        assert.False(t, runner.privilegeManager.IsCurrentlyElevated())
    }

    // Verify metrics
    metrics := runner.privilegeManager.GetMetrics()
    assert.Greater(t, metrics.TotalElevations, int64(0))
    assert.Equal(t, metrics.TotalElevations, metrics.TotalDrops)
    assert.Equal(t, int64(0), metrics.EmergencyShutdowns)
}
```

## 6. セキュリティ仕様

### 6.1 権限状態検証

```go
// validatePrivilegeState performs comprehensive privilege state validation
func (m *EarlyDropManager) validatePrivilegeState() error {
    currentUID := syscall.Getuid()
    currentEUID := syscall.Geteuid()

    // Basic UID consistency check
    if m.originalUID != currentUID {
        return fmt.Errorf("real UID changed unexpectedly: expected %d, got %d", m.originalUID, currentUID)
    }

    // State consistency check
    expectedEUID := m.originalUID
    if m.currentState.IsElevated {
        expectedEUID = 0
    }

    if currentEUID != expectedEUID {
        return fmt.Errorf("effective UID inconsistent with state: expected %d, got %d (elevated: %t)",
            expectedEUID, currentEUID, m.currentState.IsElevated)
    }

    // Time-based validation
    if m.currentState.IsElevated {
        elevationDuration := time.Since(m.currentState.LastChanged)
        if elevationDuration > m.maxElevationDuration {
            return fmt.Errorf("privilege elevation exceeded maximum duration: %v > %v",
                elevationDuration, m.maxElevationDuration)
        }
    }

    return nil
}
```

### 6.2 監査ログ仕様

```go
// PrivilegeAuditLog represents an auditable privilege operation
type PrivilegeAuditLog struct {
    Timestamp    time.Time `json:"timestamp"`
    Operation    Operation `json:"operation"`
    Action       string    `json:"action"`        // "elevate", "restore", "early_drop", "emergency_shutdown"
    Command      string    `json:"command,omitempty"`
    ProcessID    int       `json:"process_id"`
    OriginalUID  int       `json:"original_uid"`
    OriginalEUID int       `json:"original_euid,omitempty"`
    TargetUID    int       `json:"target_uid"`
    OperationID  string    `json:"operation_id,omitempty"`
    Duration     int64     `json:"duration_ns,omitempty"`
    Success      bool      `json:"success"`
    Error        string    `json:"error,omitempty"`
    Context      string    `json:"context,omitempty"`
}

// CriticalSecurityEvent represents a critical security event
type CriticalSecurityEvent struct {
    Type        string                 `json:"type"`
    Message     string                 `json:"message"`
    Error       string                 `json:"error"`
    Context     string                 `json:"context"`
    ProcessInfo map[string]interface{} `json:"process_info"`
    Timestamp   time.Time              `json:"timestamp"`
    Severity    string                 `json:"severity"`
}
```

## 7. パフォーマンス仕様

### 7.1 性能要件

| 操作 | 最大許容時間 | 測定方法 |
|------|-------------|----------|
| 早期特権放棄 | 1ms | main関数内での測定 |
| 特権昇格 | 0.1ms | seteuid(0)の実行時間 |
| 特権復帰 | 0.1ms | seteuid(originalUID)の実行時間 |
| 状態検証 | 0.01ms | validatePrivilegeState()の実行時間 |

### 7.2 メトリクス収集仕様

```go
// updateMetrics updates performance metrics after privilege operations
func (m *EarlyDropManager) updateMetrics(duration time.Duration, success bool) {
    m.metrics.LastOperation = time.Now()

    if success {
        // Update average elevation time
        if m.metrics.TotalElevations > 0 {
            totalTime := m.metrics.TotalElevationTime + duration
            m.metrics.AverageElevationTime = totalTime / time.Duration(m.metrics.TotalElevations)
        } else {
            m.metrics.AverageElevationTime = duration
        }

        m.metrics.TotalElevationTime += duration

        // Update maximum elevation time
        if duration > m.metrics.MaxElevationTime {
            m.metrics.MaxElevationTime = duration
        }
    }

    // Log performance metrics periodically
    if m.metrics.TotalElevations%100 == 0 {
        m.logger.Info("Privilege operation metrics",
            "total_elevations", m.metrics.TotalElevations,
            "avg_time_ns", m.metrics.AverageElevationTime.Nanoseconds(),
            "max_time_ns", m.metrics.MaxElevationTime.Nanoseconds(),
            "failed_elevations", m.metrics.FailedElevations,
            "emergency_shutdowns", m.metrics.EmergencyShutdowns)
    }
}
```

## 8. 実装チェックリスト

### 8.1 コア実装

- [ ] `EarlyDropManager` 構造体実装
- [ ] `DropToRealUID()` メソッド実装
- [ ] `WithPrivileges()` メソッド実装
- [ ] `emergencyShutdown()` メソッド実装
- [ ] `validatePrivilegeState()` メソッド実装

### 8.2 統合実装

- [ ] `main()` 関数での早期特権放棄統合
- [ ] `ResourceManager.CreateTempDir()` 簡素化
- [ ] `adjustDirectoryOwnership()` 削除
- [ ] Command Executor での特権管理統合
- [ ] 設定ファイル対応追加

### 8.3 将来的な拡張（YAGNI原則により現在は未実装）

- [ ] `ElevatePrivileges()` メソッド（テスト・デバッグ用途向け）
- [ ] `DropPrivileges()` メソッド（緊急時復旧用途向け）
- [ ] `IsCurrentlyElevated()` メソッド（状態確認用途向け）
- [ ] `CanElevatePrivileges()` メソッド（事前チェック用途向け）
- [ ] `GetCurrentState()` メソッド（詳細状態取得用途向け）
- [ ] 直接的特権制御の監査ログ強化
- [ ] 長時間特権保持のための最適化

### 8.3 テスト実装

- [ ] SUID環境での単体テスト
- [ ] 権限昇格/復帰の統合テスト
- [ ] 緊急シャットダウンのテスト
- [ ] End-to-endテスト
- [ ] パフォーマンステスト

### 8.4 セキュリティ実装

- [ ] 詳細な監査ログ実装
- [ ] セキュリティイベント記録
- [ ] 複数出力先でのログ記録

## 9. 将来的な拡張仕様（YAGNI原則により現在は未実装）

### 9.1 直接的特権制御メソッド

現在は `WithPrivileges()` による安全な特権管理のみを実装しているが、将来的に必要になった場合の拡張仕様：

#### 9.1.1 ElevatePrivileges() メソッド

```go
// ElevatePrivileges elevates privileges to root (future extension)
func (m *EarlyDropManager) ElevatePrivileges() error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    // 状態チェック
    if m.currentState.IsElevated {
        return fmt.Errorf("%w: already elevated", ErrAlreadyElevated)
    }

    if !m.currentState.CanElevate {
        return fmt.Errorf("%w: cannot elevate in current state", ErrNotPrivilegedBinary)
    }

    // 権限昇格実行
    if err := syscall.Seteuid(0); err != nil {
        return &PrivilegeError{
            Operation:   OperationDirectElevation,
            Reason:      "direct privilege elevation failed",
            OriginalUID: m.originalUID,
            TargetUID:   0,
            SystemError: err,
            Timestamp:   time.Now(),
        }
    }

    // 状態更新
    m.currentState.EffectiveUID = 0
    m.currentState.IsElevated = true
    m.currentState.LastChanged = time.Now()

    m.logger.Warn("Direct privilege elevation performed",
        "original_uid", m.originalUID,
        "target_uid", 0,
        "operation", OperationDirectElevation)

    return nil
}
```

#### 9.1.2 DropPrivileges() メソッド

```go
// DropPrivileges drops privileges to original UID (future extension)
func (m *EarlyDropManager) DropPrivileges() error {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    // 状態チェック
    if !m.currentState.IsElevated {
        return fmt.Errorf("%w: not currently elevated", ErrNotElevated)
    }

    // 権限放棄実行
    if err := syscall.Seteuid(m.originalUID); err != nil {
        // 権限放棄失敗は致命的
        m.emergencyShutdown(err, "direct privilege drop failed")
    }

    // 状態更新
    m.currentState.EffectiveUID = m.originalUID
    m.currentState.IsElevated = false
    m.currentState.LastChanged = time.Now()

    m.logger.Info("Direct privilege drop completed",
        "target_uid", m.originalUID,
        "operation", OperationDirectDrop)

    return nil
}
```

### 9.2 状態確認メソッド

#### 9.2.1 IsCurrentlyElevated() メソッド

```go
// IsCurrentlyElevated checks if privileges are currently elevated (future extension)
func (m *EarlyDropManager) IsCurrentlyElevated() bool {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    return m.currentState.IsElevated
}
```

#### 9.2.2 CanElevatePrivileges() メソッド

```go
// CanElevatePrivileges checks if privilege elevation is possible (future extension)
func (m *EarlyDropManager) CanElevatePrivileges() bool {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    return m.currentState.CanElevate
}
```

#### 9.2.3 GetCurrentState() メソッド

```go
// GetCurrentState returns detailed privilege state information (future extension)
func (m *EarlyDropManager) GetCurrentState() PrivilegeState {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    // Return a copy to prevent external modification
    return PrivilegeState{
        RealUID:      m.currentState.RealUID,
        EffectiveUID: m.currentState.EffectiveUID,
        IsElevated:   m.currentState.IsElevated,
        CanElevate:   m.currentState.CanElevate,
        LastChanged:  m.currentState.LastChanged,
        OperationID:  m.currentState.OperationID,
    }
}
```

### 9.3 拡張時の設計考慮事項

#### 9.3.1 セキュリティ要件

- **監査ログの強化**: 直接制御はすべて `WARN` レベル以上でログ記録
- **権限状態検証**: 操作前後で必ず権限状態を検証
- **失敗時の安全性**: 権限放棄失敗時は `emergencyShutdown()` を実行
- **スレッドセーフ**: 状態確認メソッドは読み取り専用ロックを使用

#### 9.3.2 使用制限

- **テスト環境のみ**: 本番環境では `WithPrivileges()` の使用を推奨
- **ドキュメント化**: 使用理由と安全性確保の手順を明記
- **コードレビュー**: 直接制御を使用するコードは必須レビュー
- **状態確認の限定**: デバッグ・バリデーション用途のみに制限

#### 9.3.3 実装優先度

1. **高**: `WithPrivileges()` による安全な特権管理
2. **中**: 早期特権放棄の実装
3. **低**: 状態確認メソッド（デバッグ・テスト用途）
4. **最低**: 直接制御メソッド（必要時に実装）

### 9.4 代替案検討

直接制御メソッドの代わりに検討可能な代替案：

#### 9.3.1 高レベルヘルパー関数

```go
// WithTemporaryElevation provides scoped elevation for multiple operations
func (m *EarlyDropManager) WithTemporaryElevation(ctx context.Context, operations []func() error) error {
    elevationCtx := ElevationContext{
        Operation: OperationMultiplePrivileged,
        Reason:    "batch privileged operations",
    }

    return m.WithPrivileges(ctx, elevationCtx, func() error {
        for i, op := range operations {
            if err := op(); err != nil {
                return fmt.Errorf("operation %d failed: %w", i, err)
            }
        }
        return nil
    })
}
```

#### 9.3.2 テスト専用インターフェース

```go
#### 9.4.2 テスト専用インターフェース

```go
// TestPrivilegeManager extends PrivilegeManager for testing
type TestPrivilegeManager interface {
    PrivilegeManager

    // Test-only methods
    ForceElevate() error    // テスト専用の直接昇格
    ForceDrop() error       // テスト専用の直接放棄
    GetTestState() TestPrivilegeState  // テスト専用状態取得
}
```

#### 9.4.3 ログベース状態確認

```go
// 状態確認メソッドの代わりにログ解析で状態を把握
func (m *EarlyDropManager) WithPrivileges(ctx context.Context, elevationCtx ElevationContext, fn func() error) error {
    // 詳細な開始ログ
    m.logger.Info("Starting privileged operation",
        "operation", elevationCtx.Operation,
        "current_state", "about_to_elevate",
        "original_uid", m.originalUID)

    // ... 実装

    // 詳細な終了ログ
    m.logger.Info("Completed privileged operation",
        "operation", elevationCtx.Operation,
        "final_state", "restored_to_original_uid",
        "duration_ms", duration.Milliseconds())

    return err
}
```
```
- [ ] 権限状態の継続的検証
- [ ] メトリクス収集と監視

この詳細仕様書に基づき、早期特権放棄による安全なSUIDバイナリ実装を段階的に進めることができる。
