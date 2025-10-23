# 詳細仕様書：タイムアウト設定仕様の明確化

## 1. 概要

本ドキュメントでは、go-safe-cmd-runnerにおけるタイムアウト設定の仕様変更に関する詳細な技術仕様を定義する。主要な変更は、TOML設定における「未設定」と「明示的なゼロ」の区別を可能にすることである。

## 2. データ型仕様

### 2.1. 基本型定義

#### 2.1.1. Nullable Timeout Type

```go
// TimeoutValue represents a timeout setting that can be unset, zero, or positive
type TimeoutValue *int

const (
    // DefaultTimeout is used when no timeout is explicitly set
    DefaultTimeout = 60 // seconds

    // MaxTimeout defines the maximum allowed timeout value (24 hours)
    // Using int32 max to ensure cross-platform compatibility
    MaxTimeout = 86400 // 24 hours in seconds
)
```

#### 2.1.2. 型の意味論

| Go表現 | TOML表現 | 意味 | 実行時動作 |
|--------|----------|------|-----------|
| `nil` | 設定なし | デフォルトタイムアウトを使用 | 60秒でタイムアウト |
| `*int(0)` | `timeout = 0` | 無制限実行 | タイムアウトしない |
| `*int(N)` (N>0) | `timeout = N` | N秒タイムアウト | N秒でタイムアウト |

### 2.2. 構造体定義変更

#### 2.2.1. GlobalSpec 構造体

```go
// GlobalSpec represents global configuration loaded from TOML
type GlobalSpec struct {
    // Timeout specifies the default timeout for all commands
    // nil: use DefaultTimeout (60 seconds)
    // *0: no timeout (unlimited execution)
    // *N (N>0): timeout after N seconds
    Timeout *int `toml:"timeout"`

    // 他のフィールドは変更なし
    LogLevel            string   `toml:"log_level"`
    VerifyStandardPaths *bool    `toml:"verify_standard_paths"`
    OutputSizeLimit     int64    `toml:"output_size_limit"`
    VerifyFiles         []string `toml:"verify_files"`
    EnvAllowlist        []string `toml:"env_allowed"`
    EnvVars             []string `toml:"env_vars"`
    EnvImport           []string `toml:"env_import"`
    Vars                []string `toml:"vars"`
}
```

#### 2.2.2. CommandSpec 構造体

```go
// CommandSpec represents a single command configuration
type CommandSpec struct {
    Name        string   `toml:"name"`
    Description string   `toml:"description"`
    Cmd         string   `toml:"cmd"`
    Args        []string `toml:"args"`

    // Timeout specifies command-specific timeout
    // nil: inherit from parent (group or global)
    // *0: no timeout (unlimited execution)
    // *N (N>0): timeout after N seconds
    Timeout *int `toml:"timeout"`

    // 他のフィールドは変更なし
    WorkDir     string   `toml:"workdir"`
    OutputFile  string   `toml:"output_file"`
    Background  bool     `toml:"background"`
    RunAsUser   string   `toml:"run_as_user"`
    RunAsGroup  string   `toml:"run_as_group"`
    RiskLevel   string   `toml:"risk_level"`
    EnvVars     []string `toml:"env_vars"`
    EnvImport   []string `toml:"env_import"`
    Vars        []string `toml:"vars"`
}
```

## 3. タイムアウト解決アルゴリズム

### 3.1. 階層的継承ロジック

#### 3.1.1. 解決アルゴリズム

```go
// ResolveTimeout resolves the effective timeout value from the hierarchy.
// It returns the resolved timeout in seconds.
// A value of 0 means unlimited execution.
func ResolveTimeout(cmdTimeout, groupTimeout, globalTimeout *int) int {
    // Determine which timeout pointer to use based on hierarchy
    var effectiveTimeout *int
    if cmdTimeout != nil {
        effectiveTimeout = cmdTimeout
    } else if groupTimeout != nil {
        effectiveTimeout = groupTimeout
    } else if globalTimeout != nil {
        effectiveTimeout = globalTimeout
    }

    // If no timeout is set at any level, use the default
    if effectiveTimeout == nil {
        return DefaultTimeout
    }

    // If a timeout is set, return its value (0 for unlimited)
    return *effectiveTimeout
}
```

#### 3.1.2. 解決順序の詳細

1. **コマンドレベル**: `[[groups.commands]]`の`timeout`
   - 存在する場合: その値を使用（0も含む）
   - 存在しない場合: 次のレベルへ

2. **グループレベル**: `[[groups]]`の`timeout`（将来実装予定）
   - 存在する場合: その値を使用（0も含む）
   - 存在しない場合: 次のレベルへ

3. **グローバルレベル**: `[global]`の`timeout`
   - 存在する場合: その値を使用（0も含む）
   - 存在しない場合: 次のレベルへ

4. **デフォルト**: システム定義の`DefaultTimeout`（60秒）

### 3.2. RuntimeGlobal の変更

#### 3.2.1. Timeout メソッドの更新

```go
// Timeout returns the effective global timeout value
// Returns DefaultTimeout if not specified in config (Spec.Timeout == nil)
// Returns 0 for unlimited execution if explicitly set to 0
func (r *RuntimeGlobal) Timeout() int {
    if r == nil || r.Spec == nil {
        panic("RuntimeGlobal.Timeout: nil receiver or Spec")
    }

    if r.Spec.Timeout == nil {
        return DefaultTimeout
    }

    return *r.Spec.Timeout
}
```

#### 3.2.2. RuntimeCommand の変更

```go
// RuntimeCommand represents runtime command configuration
type RuntimeCommand struct {
    Spec *CommandSpec

    // EffectiveTimeout is the resolved timeout value for this command
    // 0 means unlimited execution
    // >0 means timeout after N seconds
    EffectiveTimeout int

    // 他のフィールド
    ExpandedCmd         string
    ExpandedArgs        []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string
    ExpandedWorkDir     string
    ExpandedOutputFile  string
}

// NewRuntimeCommand creates a new RuntimeCommand with resolved timeout
func NewRuntimeCommand(spec *CommandSpec, globalTimeout *int) (*RuntimeCommand, error) {
    if spec == nil {
        return nil, ErrNilSpec
    }

    effectiveTimeout := ResolveTimeout(spec.Timeout, nil, globalTimeout)

    return &RuntimeCommand{
        Spec:             spec,
        EffectiveTimeout: effectiveTimeout,
        ExpandedEnv:      make(map[string]string),
        ExpandedVars:     make(map[string]string),
    }, nil
}
```

## 4. TOML パース仕様

### 4.1. TOML読み込み処理

#### 4.1.1. 型変換ロジック

```go
// parseTimeoutValue converts TOML value to *int
func parseTimeoutValue(value interface{}) (*int, error) {
    if value == nil {
        return nil, nil // Unset
    }

    switch v := value.(type) {
    case int64:
        if v < 0 {
            return nil, fmt.Errorf("timeout must be non-negative, got %d", v)
        }
        if v > MaxTimeout {
            return nil, fmt.Errorf("timeout value too large: %d. Maximum value is %d", v, MaxTimeout)
        }
        intVal := int(v)
        return &intVal, nil
    case int:
        if v < 0 {
            return nil, fmt.Errorf("timeout must be non-negative, got %d", v)
        }
        if v > MaxTimeout {
            return nil, fmt.Errorf("timeout value too large: %d. Maximum value is %d", v, MaxTimeout)
        }
        return &v, nil
    default:
        return nil, fmt.Errorf("timeout must be an integer, got %T", v)
    }
}
```

#### 4.1.2. バリデーション

```go
// ValidateTimeout validates timeout configuration
func ValidateTimeout(timeout *int, context string) error {
    if timeout == nil {
        return nil // Unset is valid
    }

    if *timeout < 0 {
        return fmt.Errorf("%s: timeout must be non-negative, got %d", context, *timeout)
    }

    if *timeout > MaxTimeout {
        return fmt.Errorf("%s: timeout value too large: %d. Maximum value is %d", context, *timeout, MaxTimeout)
    }

    return nil
}
```

### 4.2. エラーメッセージ仕様

#### 4.2.1. 設定エラー

| エラーケース | エラーメッセージ |
|-------------|-----------------|
| 負の値 | `Invalid timeout value: -1. Timeout must be a non-negative integer (0 for no timeout, positive values for timeout in seconds).` |
| 不正な型 | `Invalid timeout type: string. Timeout must be an integer.` |
| 上限超過 | `Timeout value too large: 90000. Maximum value is 86400 (24 hours).` |

#### 4.2.2. 実行時警告

| 状況 | ログレベル | メッセージ |
|-----|-----------|-----------|
| 無制限実行開始 | WARN | `Command '%s' configured with unlimited timeout (timeout=0). Monitor for resource usage.` |
| デフォルト使用 | DEBUG | `Using default timeout (%d seconds) for command '%s'` |
| 明示的設定 | DEBUG | `Command '%s' timeout set to %d seconds` |

## 5. 実行時制御仕様

### 5.1. タイムアウト制御

#### 5.1.1. タイムアウト実装

```go
// ExecuteWithTimeout executes a command with the specified timeout.
func ExecuteWithTimeout(cmd *exec.Cmd, timeout int) error {
    var ctx context.Context
    var cancel context.CancelFunc

    if timeout <= 0 {
        // For unlimited execution, use a non-cancellable context
        log.Warn("Executing command with unlimited timeout")
        ctx = context.Background()
        cancel = func() {} // No-op cancel function
    } else {
        // For limited execution, create a context with timeout
        ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
    }
    defer cancel()

    // Assign the context to the command
    cmd.Context = ctx

    return cmd.Run()
}
```

#### 5.1.2. 無制限実行の監視

```go
// MonitorUnlimitedExecution monitors commands running without timeout
// Returns a cancel function that should be called when the command finishes
func MonitorUnlimitedExecution(cmd *exec.Cmd, cmdName string) context.CancelFunc {
    ctx, cancel := context.WithCancel(context.Background())

    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()

        for {
            select {
            case <-ticker.C:
                if cmd.Process != nil {
                    log.Warnf("Command '%s' (PID: %d) running for more than 5 minutes with unlimited timeout",
                             cmdName, cmd.Process.Pid)
                }
            case <-ctx.Done():
                return
            }
        }
    }()

    return cancel
}
```

## 6. テスト仕様

### 6.1. 単体テスト仕様

#### 6.1.1. 型変換テスト

```go
func TestTimeoutParsing(t *testing.T) {
    tests := []struct {
        name     string
        toml     string
        expected *int
        wantErr  bool
    }{
        {
            name:     "unset timeout",
            toml:     ``,
            expected: nil,
            wantErr:  false,
        },
        {
            name:     "zero timeout",
            toml:     `timeout = 0`,
            expected: intPtr(0),
            wantErr:  false,
        },
        {
            name:     "positive timeout",
            toml:     `timeout = 300`,
            expected: intPtr(300),
            wantErr:  false,
        },
        {
            name:     "negative timeout",
            toml:     `timeout = -1`,
            expected: nil,
            wantErr:  true,
        },
        {
            name:     "timeout exceeds maximum",
            toml:     `timeout = 90000`,
            expected: nil,
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

#### 6.1.2. 階層継承テスト

```go
func TestTimeoutResolution(t *testing.T) {
    tests := []struct {
        name        string
        cmdTimeout  *int
        globalTimeout *int
        expected    int
    }{
        {
            name:        "command overrides global",
            cmdTimeout:  intPtr(120),
            globalTimeout: intPtr(60),
            expected:    120,
        },
        {
            name:        "command zero overrides global",
            cmdTimeout:  intPtr(0),
            globalTimeout: intPtr(60),
            expected:    0,
        },
        {
            name:        "inherit global",
            cmdTimeout:  nil,
            globalTimeout: intPtr(60),
            expected:    60,
        },
        {
            name:        "use default",
            cmdTimeout:  nil,
            globalTimeout: nil,
            expected:    DefaultTimeout,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ResolveTimeout(tt.cmdTimeout, tt.globalTimeout)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### 6.2. 統合テスト仕様

#### 6.2.1. E2Eタイムアウトテスト

```go
func TestE2ETimeoutBehavior(t *testing.T) {
    tests := []struct {
        name        string
        config      string
        command     string
        expectTimeout bool
        expectDuration time.Duration
    }{
        {
            name: "default timeout",
            config: `
                version = "1.0"
                [[groups]]
                name = "test"
                [[groups.commands]]
                name = "sleep"
                cmd = "/bin/sleep"
                args = ["90"]
            `,
            command: "sleep",
            expectTimeout: true,
            expectDuration: 60 * time.Second,
        },
        {
            name: "unlimited timeout",
            config: `
                version = "1.0"
                [global]
                timeout = 0
                [[groups]]
                name = "test"
                [[groups.commands]]
                name = "sleep"
                cmd = "/bin/sleep"
                args = ["30"]
            `,
            command: "sleep",
            expectTimeout: false,
            expectDuration: 30 * time.Second,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // E2Eテスト実装
        })
    }
}
```

## 7. 性能仕様

### 7.1. 性能要件

#### 7.1.1. メモリ使用量

- **追加メモリ**: ポインタ型導入による1構造体あたり8バイト増加
- **影響度**: 通常の設定ファイル（<100コマンド）では無視可能レベル

#### 7.1.2. 実行時性能

- **タイムアウト解決**: O(1)の計算量
- **追加オーバーヘッド**: 1マイクロ秒未満
- **TOML読み込み**: 既存性能から5%以内の増加

### 7.2. 最適化

#### 7.2.1. インライン化

```go
// Timeout resolution should be inlined for performance
//go:inline
func (r *RuntimeGlobal) Timeout() int {
    if r.Spec.Timeout == nil {
        return DefaultTimeout
    }
    return *r.Spec.Timeout
}
```

#### 7.2.2. プリコンピューテーション

```go
// Precompute timeout values during runtime creation
func NewRuntimeCommand(spec *CommandSpec, globalTimeout *int) *RuntimeCommand {
    effectiveTimeout := ResolveTimeout(
        spec.Timeout,
        globalTimeout,
    )

    return &RuntimeCommand{
        Spec:             spec,
        EffectiveTimeout: effectiveTimeout, // Pre-computed
    }
}
```

## 8. セキュリティ仕様

### 8.1. リソース保護

#### 8.1.1. 無制限実行の監視

```go
// UnlimitedExecutionMonitor tracks commands running without timeout
type UnlimitedExecutionMonitor struct {
    processes map[int]*ProcessInfo
    mutex     sync.RWMutex
}

type ProcessInfo struct {
    CommandName string
    StartTime   time.Time
    PID         int
}

func (m *UnlimitedExecutionMonitor) Register(pid int, name string) {
    m.mutex.Lock()
    defer m.mutex.Unlock()

    m.processes[pid] = &ProcessInfo{
        CommandName: name,
        StartTime:   time.Now(),
        PID:         pid,
    }
}

func (m *UnlimitedExecutionMonitor) CheckLongRunning() []ProcessInfo {
    m.mutex.RLock()
    defer m.mutex.RUnlock()

    threshold := 10 * time.Minute
    var longRunning []ProcessInfo

    for _, info := range m.processes {
        if time.Since(info.StartTime) > threshold {
            longRunning = append(longRunning, *info)
        }
    }

    return longRunning
}
```

#### 8.1.2. セキュリティログ

```go
// SecurityLogger logs security-relevant timeout events
type SecurityLogger struct {
    logger *log.Logger
}

func (s *SecurityLogger) LogUnlimitedExecution(cmdName string, user string) {
    s.logger.Warnf("SECURITY: Command '%s' started with unlimited timeout by user '%s'",
                   cmdName, user)
}

func (s *SecurityLogger) LogLongRunningProcess(cmdName string, duration time.Duration, pid int) {
    s.logger.Warnf("SECURITY: Long-running process detected - Command: '%s', Duration: %v, PID: %d",
                   cmdName, duration, pid)
}
```

## 9. ドキュメント更新仕様

### 9.1. 更新対象ファイル

#### 9.1.1. ユーザーガイド更新

```markdown
## 4.1 timeout - Timeout Setting

### Overview
Specifies the maximum wait time for command execution in seconds.

### Syntax
```toml
[global]
timeout = seconds  # or omit for default
```

### Parameter Details
| Item | Description |
|------|-------------|
| **Type** | Integer (int) or unset |
| **Required/Optional** | Optional |
| **Default Value** | 60 seconds when unset |
| **Valid Values** | 0 (unlimited) or positive integer |
| **Special Value** | `timeout = 0` means unlimited execution |

### ⚠️ Breaking Change
In previous versions, `timeout = 0` was treated as default timeout (60 seconds).
Starting from v2.0.0, `timeout = 0` means unlimited execution.
```

#### 9.1.2. 移行ガイド

```markdown
## Migration Guide: Timeout Setting Changes

### What Changed
- `timeout = 0` now means unlimited execution (previously: default 60s)
- Unset timeout still defaults to 60 seconds
- New capability: truly unlimited command execution

### Required Actions
1. **Review your configurations**: Search for `timeout = 0` in your TOML files
2. **Update if needed**:
   - Keep `timeout = 0` if you want unlimited execution
   - Change to `timeout = 60` or remove line if you want 60s timeout
3. **Test thoroughly**: Verify timeout behavior matches expectations

### Examples
```toml
# Before v2.0.0
timeout = 0     # Was: 60 seconds timeout

# v2.0.0 and later
timeout = 0     # Now: unlimited execution
timeout = 60    # Explicit 60 seconds
# timeout unset  # Default 60 seconds
```
```

## 10. エラーメッセージ仕様

### 10.1. 設定エラーメッセージ

```go
const (
    ErrNegativeTimeout = "Invalid timeout value: %d. Timeout must be a non-negative integer (0 for no timeout, positive values for timeout in seconds)."
    ErrInvalidTimeoutType = "Invalid timeout type: %T. Timeout must be an integer."
    ErrTimeoutOverflow = "Timeout value too large: %d. Maximum value is %d (24 hours)."
)
```

### 10.2. 実行時ログメッセージ

```go
const (
    LogUnlimitedExecution = "Command '%s' configured with unlimited timeout (timeout=0). Monitor for resource usage."
    LogDefaultTimeout = "Using default timeout (%d seconds) for command '%s'"
    LogExplicitTimeout = "Command '%s' timeout set to %d seconds"
    LogTimeoutExceeded = "Command '%s' exceeded timeout of %d seconds"
    LogLongRunning = "Command '%s' has been running for %v with unlimited timeout (PID: %d)"
)
```

## 11. 実装チェックリスト

### 11.1. コード変更

- [ ] `GlobalSpec.Timeout` を `*int` に変更
- [ ] `CommandSpec.Timeout` を `*int` に変更
- [ ] `RuntimeGlobal.Timeout()` メソッド更新
- [ ] `ResolveTimeout` 関数実装
- [ ] TOML パーサーでの負の値バリデーション
- [ ] 実行エンジンでの無制限タイムアウト対応

### 11.2. テスト実装

- [ ] 型変換の単体テスト
- [ ] 階層継承の単体テスト
- [ ] バリデーションの単体テスト
- [ ] E2Eタイムアウト動作テスト
- [ ] 無制限実行のE2Eテスト
- [ ] 性能回帰テスト

### 11.3. ドキュメント更新

- [ ] `04_global_level.md` 更新
- [ ] `06_command_level.md` 更新
- [ ] 日本語版ドキュメント更新
- [ ] `CHANGELOG.md` 更新
- [ ] 移行ガイド作成
- [ ] サンプル設定ファイル更新

### 11.4. 検証項目

- [ ] 既存テストの100%パス
- [ ] 新機能テストの95%以上カバレッジ
- [ ] 性能要件（5%以内の劣化）達成
- [ ] セキュリティレビュー完了
- [ ] ドキュメントレビュー完了
