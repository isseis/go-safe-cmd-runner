# 詳細仕様書：タイムアウト設定仕様の明確化

## 1. 概要

本ドキュメントでは、go-safe-cmd-runnerにおけるタイムアウト設定の仕様変更に関する詳細な技術仕様を定義する。主要な変更は、TOML設定における「未設定」と「明示的なゼロ」の区別を可能にすることである。

## 2. データ型仕様

### 2.1. 基本型定義

#### 2.1.1. Timeout Type

```go
// Package common provides shared data types and constants used throughout the command runner.
package common

const (
    // DefaultTimeout is used when no timeout is explicitly set
    DefaultTimeout = 60 // seconds

    // MaxTimeout defines the maximum allowed timeout value (24 hours)
    // The value is well within int32 range, ensuring cross-platform compatibility.
    MaxTimeout = 86400 // 24 hours in seconds
)

// Timeout represents a timeout configuration value.
// It distinguishes between three states:
// - Unset (use default or inherit from parent)
// - Zero (unlimited execution, no timeout)
// - Positive value (timeout in seconds)
//
// This type provides type safety and explicit semantics compared to using *int directly.
type Timeout struct {
    value *int
}

// NewUnsetTimeout creates an unset Timeout (will use default or inherit from parent).
func NewUnsetTimeout() Timeout {
    return Timeout{value: nil}
}

// NewUnlimitedTimeout creates a Timeout with unlimited execution (no timeout).
func NewUnlimitedTimeout() Timeout {
    zero := 0
    return Timeout{value: &zero}
}

// NewTimeout creates a Timeout with the specified duration in seconds.
// Returns error if seconds is negative or exceeds MaxTimeout.
func NewTimeout(seconds int) (Timeout, error) {
    if seconds < 0 {
        return Timeout{}, ErrInvalidTimeout{
            Value:   seconds,
            Context: "timeout cannot be negative",
        }
    }
    if seconds > MaxTimeout {
        return Timeout{}, ErrInvalidTimeout{
            Value:   seconds,
            Context: "timeout exceeds maximum allowed value",
        }
    }
    return Timeout{value: &seconds}, nil
}

// IsSet returns true if the timeout has been explicitly set (non-nil).
func (t Timeout) IsSet() bool {
    return t.value != nil
}

// IsUnlimited returns true if the timeout is explicitly set to unlimited (0).
// Returns false if the timeout is unset (nil).
func (t Timeout) IsUnlimited() bool {
    return t.value != nil && *t.value == 0
}

// Value returns the timeout value in seconds.
// Panics if the timeout is not set (IsSet() == false).
// Callers must check IsSet() before calling Value().
// For unlimited timeout, returns 0 (use IsUnlimited to distinguish from set non-zero values).
func (t Timeout) Value() int {
	if t.value == nil {
		panic("Value() called on unset Timeout - check IsSet() before calling Value()")
	}
	return *t.value
}
```

**注記**: `Timeout`型は内部に`*int`を持つ構造体として実装されており、型安全性と明示的なセマンティクスを提供します。

#### 2.1.2. 型の意味論

| Go表現 | TOML表現 | 意味 | 実行時動作 |
|--------|----------|------|-----------|
| `NewUnsetTimeout()` | 設定なし | デフォルトタイムアウトを使用 | 60秒でタイムアウト |
| `NewUnlimitedTimeout()` | `timeout = 0` | 無制限実行 | タイムアウトしない |
| `NewTimeout(N)` (N>0) | `timeout = N` | N秒タイムアウト | N秒でタイムアウト |

### 2.2. 構造体定義変更

#### 2.2.1. GlobalSpec 構造体

```go
// GlobalSpec represents global configuration loaded from TOML
type GlobalSpec struct {
    // Timeout specifies the default timeout for all commands
    // Use NewUnsetTimeout(): use DefaultTimeout (60 seconds)
    // Use NewUnlimitedTimeout(): no timeout (unlimited execution)
    // Use NewTimeout(N): timeout after N seconds
    Timeout common.Timeout `toml:"timeout"`

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
    // Unset: inherit from parent (group or global)
    // Unlimited: no timeout (unlimited execution)
    // N seconds: timeout after N seconds
    Timeout common.Timeout `toml:"timeout"`

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
// ResolveEffectiveTimeout determines the effective timeout value using the priority chain:
// 1. Command-level timeout (if set)
// 2. Global-level timeout (if set)
// 3. DefaultTimeout constant (60 seconds)
//
// This function encapsulates the timeout resolution logic used throughout the command runner,
// ensuring consistent behavior in both production code and tests.
//
// Parameters:
//   - commandTimeout: The command-specific timeout (may be unset)
//   - globalTimeout: The global timeout (may be unset)
//
// Returns:
//   - The resolved timeout value in seconds
func ResolveEffectiveTimeout(commandTimeout, globalTimeout Timeout) int {
    if commandTimeout.IsSet() {
        return commandTimeout.Value()
    }
    if globalTimeout.IsSet() {
        return globalTimeout.Value()
    }
    return DefaultTimeout
}
```

**注記**: グループレベルのタイムアウトは現在の実装では未サポートです。

#### 3.1.2. 解決順序の詳細

1. **コマンドレベル**: `[[groups.commands]]`の`timeout`
   - 存在する場合: その値を使用（0も含む）
   - 存在しない場合: 次のレベルへ

2. **グローバルレベル**: `[global]`の`timeout`
   - 存在する場合: その値を使用（0も含む）
   - 存在しない場合: 次のレベルへ

3. **デフォルト**: システム定義の`DefaultTimeout`（60秒）

**注記**: グループレベルのタイムアウト（`[[groups]]`の`timeout`）は将来実装予定です。

### 3.2. RuntimeGlobal の変更

#### 3.2.1. Timeout メソッドの更新

```go
// Timeout returns the global timeout from the spec.
// Returns the configured Timeout value, which can be unset, unlimited, or a positive value.
// Use common.ResolveEffectiveTimeout() to resolve the effective timeout with proper fallback logic.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeGlobal).
func (r *RuntimeGlobal) Timeout() common.Timeout {
    if r == nil || r.Spec == nil {
        panic("RuntimeGlobal.Timeout: nil receiver or Spec (programming error - use NewRuntimeGlobal)")
    }
    return r.timeout
}
```

**注記**: `Timeout()`メソッドは`common.Timeout`オブジェクトを返します。実際のタイムアウト値（秒単位）を解決するには、`common.ResolveEffectiveTimeout()`関数を使用してください。

#### 3.2.2. RuntimeCommand の変更

```go
// RuntimeCommand represents runtime command configuration
type RuntimeCommand struct {
    Spec *CommandSpec

    // timeout is the converted Timeout value from Spec.Timeout
    timeout common.Timeout

    // EffectiveTimeout is the resolved timeout value for this command
    // 0 means unlimited execution
    // >0 means timeout after N seconds
    EffectiveTimeout int

    // 他のフィールド
    ExpandedCmd         string
    ExpandedArgs        []string
    ExpandedEnv         map[string]string
    ExpandedVars        map[string]string
    EffectiveWorkDir    string
}

// NewRuntimeCommand creates a new RuntimeCommand with the required spec.
// Returns ErrNilSpec if spec is nil.
// Note: EffectiveTimeout is resolved later during command preparation.
func NewRuntimeCommand(spec *CommandSpec) (*RuntimeCommand, error) {
    if spec == nil {
        return nil, ErrNilSpec
    }

    return &RuntimeCommand{
        Spec:         spec,
        timeout:      common.NewFromIntPtr(spec.Timeout),
        ExpandedArgs: []string{},
        ExpandedEnv:  make(map[string]string),
        ExpandedVars: make(map[string]string),
    }, nil
}

// Timeout returns the command-specific timeout from the spec.
// Use EffectiveTimeout for the fully resolved timeout value.
// Panics if r or r.Spec is nil (programming error - use NewRuntimeCommand).
func (r *RuntimeCommand) Timeout() common.Timeout {
    if r == nil || r.Spec == nil {
        panic("RuntimeCommand.Timeout: nil receiver or Spec (programming error - use NewRuntimeCommand)")
    }
    return r.timeout
}
```

**注記**:
- `NewRuntimeCommand()`はコマンドspecのみを受け取り、`EffectiveTimeout`の解決は後で`common.ResolveEffectiveTimeout()`を使用して行います
- `Timeout()`メソッドは`common.Timeout`オブジェクトを返します

## 4. TOML パース仕様

### 4.1. TOML読み込み処理

TOML の整数値が *int に読み込まれるため、特別な処理は不要です。

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
        expected common.Timeout
        wantErr  bool
    }{
        {
            name:     "unset timeout",
            toml:     `version = "1.0"`,
            expected: common.NewUnsetTimeout(),
            wantErr:  false,
        },
        {
            name:     "zero timeout",
            toml:     `version = "1.0"\ntimeout = 0`,
            expected: common.NewUnlimitedTimeout(),
            wantErr:  false,
        },
        {
            name:     "positive timeout",
            toml:     `version = "1.0"\ntimeout = 300`,
            expected: mustNewTimeout(300),
            wantErr:  false,
        },
        {
            name:     "negative timeout",
            toml:     `version = "1.0"\ntimeout = -1`,
            wantErr:  true,
        },
        {
            name:     "timeout exceeds maximum",
            toml:     `version = "1.0"\ntimeout = 90000`,
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実装
        })
    }
}

func mustNewTimeout(seconds int) common.Timeout {
    t, err := common.NewTimeout(seconds)
    if err != nil {
        panic(err)
    }
    return t
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

- **Timeout型サイズ**: 8バイト（ポインタ1つ分）
- **影響度**: `*int`型と同等のメモリ使用量
- **追加コスト**: 型安全性のための実装であり、性能面でのオーバーヘッドは無視可能

#### 7.1.2. 実行時性能

- **タイムアウト解決**: O(1)の計算量
- **追加オーバーヘッド**: 1マイクロ秒未満
- **TOML読み込み**: 既存性能から5%以内の増加

### 7.2. 最適化

#### 7.2.1. インライン化

```go
// Timeout methods should be inlined for performance
//go:inline
func (t Timeout) IsSet() bool {
    return t.value != nil
}

//go:inline
func (t Timeout) IsUnlimited() bool {
    return t.value != nil && *t.value == 0
}

//go:inline
func (t Timeout) Value() int {
    if t.value == nil {
        panic("Value() called on unset Timeout - check IsSet() before calling Value()")
    }
    return *t.value
}
```

#### 7.2.2. プリコンピューテーション

```go
// Precompute timeout values during command preparation
// Note: NewRuntimeCommand does not resolve effective timeout;
// it is resolved later during command preparation using ResolveEffectiveTimeout
func PrepareCommandTimeout(cmd *RuntimeCommand, globalTimeout common.Timeout) {
    cmd.EffectiveTimeout = common.ResolveEffectiveTimeout(
        cmd.Timeout(),
        globalTimeout,
    )
}
```

**注記**: `NewRuntimeCommand()`はタイムアウトを解決しません。`EffectiveTimeout`の解決は、コマンド準備時に`common.ResolveEffectiveTimeout()`を使用して行います。

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

- [x] `Timeout`型を`internal/common/timeout.go`に定義
- [ ] `GlobalSpec.Timeout` を `common.Timeout` に変更
- [ ] `CommandSpec.Timeout` を `common.Timeout` に変更
- [ ] `RuntimeGlobal.Timeout()` メソッド更新
- [ ] `ResolveTimeout` 関数実装
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
