# GroupExecutor リファクタリング - 詳細仕様書

## 1. 仕様概要

### 1.1 目的

`NewDefaultGroupExecutor` 関数の Functional Options パターンによるリファクタリングの詳細な実装仕様を定義する。

### 1.2 適用範囲

- `NewDefaultGroupExecutor` 関数の新しいシグネチャ
- Option 関数群の詳細仕様
- テスト用ヘルパー関数の詳細仕様
- エラーハンドリング仕様
- パフォーマンス要件

## 2. データ型仕様

### 2.1 GroupExecutorOption Type

#### 2.1.1 型定義

```go
// GroupExecutorOption configures a DefaultGroupExecutor during construction.
// Option functions are applied in the order they are provided to NewDefaultGroupExecutor.
//
// Example:
//   ge := NewDefaultGroupExecutor(
//       executor, config, validator, verificationManager, resourceManager, runID,
//       WithNotificationFunc(notifyFunc),
//       WithDryRun(&resource.DryRunOptions{DetailLevel: resource.DetailLevelFull}),
//       WithKeepTempDirs(true),
//   )
type GroupExecutorOption func(*groupExecutorOptions)
```

#### 2.1.2 設計原則

- **Immutability**: Option 関数は設定を変更するが、元の構造体は変更しない
- **Composability**: 複数の Option 関数を組み合わせ可能
- **Type Safety**: コンパイル時の型安全性を保証
- **Null Object Pattern**: nil Option は安全に無視される

### 2.2 groupExecutorOptions Structure

#### 2.2.1 型定義

```go
// groupExecutorOptions holds internal configuration options for DefaultGroupExecutor.
// This struct is not exposed publicly to maintain encapsulation.
type groupExecutorOptions struct {
    // notificationFunc is called when group execution completes.
    // If nil, no notification is sent.
    notificationFunc groupNotificationFunc

    // dryRunOptions contains dry-run configuration.
    // If nil, dry-run mode is disabled.
    dryRunOptions *resource.DryRunOptions

    // keepTempDirs indicates whether to preserve temporary directories
    // for debugging purposes.
    keepTempDirs bool
}
```

#### 2.2.2 フィールド仕様

| フィールド | 型 | デフォルト値 | 説明 |
|------------|----|--------------|----- |
| `notificationFunc` | `groupNotificationFunc` | `nil` | グループ実行完了時の通知関数。nil の場合は通知なし。 |
| `dryRunOptions` | `*resource.DryRunOptions` | `nil` | Dry-run モードの設定。nil の場合は dry-run 無効。 |
| `keepTempDirs` | `bool` | `false` | 一時ディレクトリを保持するか。デバッグ用途。 |

#### 2.2.3 メモリレイアウト

```go
// Expected memory layout (64-bit system):
type groupExecutorOptions struct {
    notificationFunc groupNotificationFunc  // 8 bytes (function pointer)
    dryRunOptions    *resource.DryRunOptions // 8 bytes (pointer)
    keepTempDirs     bool                    // 1 byte
    // Padding: 7 bytes
    // Total: 24 bytes
}
```

## 3. 関数仕様

### 3.1 defaultGroupExecutorOptions Function

#### 3.1.1 シグネチャ

```go
// defaultGroupExecutorOptions returns a new groupExecutorOptions with default values.
// This function ensures consistent default behavior across the codebase.
func defaultGroupExecutorOptions() *groupExecutorOptions
```

#### 3.1.2 実装仕様

```go
func defaultGroupExecutorOptions() *groupExecutorOptions {
    return &groupExecutorOptions{
        notificationFunc: nil,    // No notification by default
        dryRunOptions:    nil,    // Dry-run disabled by default
        keepTempDirs:     false,  // Cleanup temp dirs by default
    }
}
```

#### 3.1.3 戻り値

| 条件 | 戻り値 | 説明 |
|------|--------|------|
| 常に | `*groupExecutorOptions` | デフォルト値で初期化された設定構造体 |

#### 3.1.4 副作用

- **メモリ割り当て**: 新しい `groupExecutorOptions` 構造体を1つ割り当て
- **スレッドセーフティ**: スレッドセーフ（新しいインスタンスを毎回作成）

### 3.2 NewDefaultGroupExecutor Function

#### 3.2.1 新しいシグネチャ

```go
// NewDefaultGroupExecutor creates a new DefaultGroupExecutor with the specified
// configuration and optional settings.
//
// Required parameters:
//   - executor: Command executor interface (can be nil in tests)
//   - config: Configuration specification (must not be nil)
//   - validator: Security validator (can be nil in tests)
//   - verificationManager: File verification manager (can be nil)
//   - resourceManager: Resource management interface (must not be nil)
//   - runID: Unique identifier for this execution (must not be empty)
//
// Optional parameters are specified using option functions:
//   - WithNotificationFunc: Set notification callback
//   - WithDryRun: Enable dry-run mode with specific options
//   - WithKeepTempDirs: Control temporary directory cleanup
//
// Example (production):
//   ge := NewDefaultGroupExecutor(
//       executor, config, validator, verificationManager, resourceManager, runID,
//       WithNotificationFunc(runner.logGroupExecutionSummary),
//       WithDryRun(dryRunOptions),
//   )
//
// Example (test):
//   ge := NewDefaultGroupExecutor(
//       nil, config, nil, nil, mockResourceManager, "test-run-123",
//   )
//
// Panics if config is nil, resourceManager is nil, or runID is empty.
func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor
```

#### 3.2.2 実装仕様

```go
func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    // Input validation
    if config == nil {
        panic("NewDefaultGroupExecutor: config cannot be nil")
    }
    if resourceManager == nil {
        panic("NewDefaultGroupExecutor: resourceManager cannot be nil")
    }
    if runID == "" {
        panic("NewDefaultGroupExecutor: runID cannot be empty")
    }

    // Apply options
    opts := defaultGroupExecutorOptions()
    for _, opt := range options {
        if opt != nil {
            opt(opts)
        }
    }

    // Unpack dry-run options
    isDryRun := opts.dryRunOptions != nil
    var dryRunDetailLevel resource.DetailLevel
    var dryRunShowSensitive bool

    if isDryRun {
        dryRunDetailLevel = opts.dryRunOptions.DetailLevel
        dryRunShowSensitive = opts.dryRunOptions.ShowSensitive
    } else {
        dryRunDetailLevel = resource.DetailLevelSummary // Safe default
    }

    return &DefaultGroupExecutor{
        executor:            executor,
        config:              config,
        validator:           validator,
        verificationManager: verificationManager,
        resourceManager:     resourceManager,
        runID:               runID,
        notificationFunc:    opts.notificationFunc,
        isDryRun:            isDryRun,
        dryRunDetailLevel:   dryRunDetailLevel,
        dryRunShowSensitive: dryRunShowSensitive,
        keepTempDirs:        opts.keepTempDirs,
    }
}
```

#### 3.2.3 パラメータ仕様

| パラメータ | 型 | 必須 | 制約 | デフォルト値 |
|------------|-------|------|------|--------------|
| `executor` | `executor.CommandExecutor` | No | テストでは nil 可 | - |
| `config` | `*runnertypes.ConfigSpec` | Yes | nil 不可 | - |
| `validator` | `security.ValidatorInterface` | No | テストでは nil 可 | - |
| `verificationManager` | `verification.ManagerInterface` | No | nil 可 | - |
| `resourceManager` | `resource.ResourceManager` | Yes | nil 不可 | - |
| `runID` | `string` | Yes | 空文字不可 | - |
| `options` | `...GroupExecutorOption` | No | nil エントリは無視 | 空スライス |

#### 3.2.4 戻り値仕様

| 条件 | 戻り値 | 説明 |
|------|--------|------|
| 正常 | `*DefaultGroupExecutor` | 設定済みの GroupExecutor インスタンス |

#### 3.2.5 例外仕様

| 条件 | 例外 | メッセージ |
|------|------|-----------|
| `config == nil` | `panic` | "NewDefaultGroupExecutor: config cannot be nil" |
| `resourceManager == nil` | `panic` | "NewDefaultGroupExecutor: resourceManager cannot be nil" |
| `runID == ""` | `panic` | "NewDefaultGroupExecutor: runID cannot be empty" |

#### 3.2.6 パフォーマンス仕様

| メトリクス | 要件 | 測定方法 |
|------------|------|----------|
| 実行時間 | < 100ns (options数に比例) | ベンチマークテスト |
| メモリ割り当て | 1-2 allocations | メモリプロファイル |
| CPU使用率 | 無視できるレベル | プロファイル |

## 4. Option 関数仕様

### 4.1 WithNotificationFunc Function

#### 4.1.1 シグネチャ

```go
// WithNotificationFunc sets the notification function that will be called
// when group execution completes.
//
// The notification function receives:
//   - group: The group specification that was executed
//   - result: The execution result including success/failure status
//   - duration: The total execution duration
//
// If fn is nil, no notification will be sent (default behavior).
//
// Example:
//   WithNotificationFunc(func(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
//       log.Printf("Group %s completed in %v", group.Name, duration)
//   })
func WithNotificationFunc(fn groupNotificationFunc) GroupExecutorOption
```

#### 4.1.2 実装仕様

```go
func WithNotificationFunc(fn groupNotificationFunc) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.notificationFunc = fn
    }
}
```

#### 4.1.3 パラメータ仕様

| パラメータ | 型 | 制約 | 説明 |
|------------|-----|------|------|
| `fn` | `groupNotificationFunc` | nil 可 | 通知関数。nil の場合は通知無効化。 |

#### 4.1.4 動作仕様

| 入力 | 設定値 | 動作 |
|------|--------|------|
| 有効な関数 | `fn` | 指定された関数が通知に使用される |
| `nil` | `nil` | 通知が無効化される（デフォルトと同じ） |

#### 4.1.5 使用例

```go
// Production usage
WithNotificationFunc(runner.logGroupExecutionSummary)

// Test usage with custom function
notificationCalled := false
testNotifyFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {
    notificationCalled = true
}
WithNotificationFunc(testNotifyFunc)

// Disable notification explicitly
WithNotificationFunc(nil)
```

### 4.2 WithDryRun Function

#### 4.2.1 シグネチャ

```go
// WithDryRun enables dry-run mode with the specified options.
//
// Dry-run mode executes all validation and preparation steps but does not
// actually run the commands. This is useful for testing and validation.
//
// If options is nil, dry-run mode is disabled (same as not calling WithDryRun).
//
// DryRunOptions fields:
//   - DetailLevel: Controls the level of detail in dry-run output
//   - ShowSensitive: Whether to show sensitive information in dry-run output
//
// Example:
//   WithDryRun(&resource.DryRunOptions{
//       DetailLevel:   resource.DetailLevelFull,
//       ShowSensitive: false,
//   })
func WithDryRun(options *resource.DryRunOptions) GroupExecutorOption
```

#### 4.2.2 実装仕様

```go
func WithDryRun(options *resource.DryRunOptions) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.dryRunOptions = options
    }
}
```

#### 4.2.3 パラメータ仕様

| パラメータ | 型 | 制約 | 説明 |
|------------|-----|------|------|
| `options` | `*resource.DryRunOptions` | nil 可 | Dry-run設定。nil の場合は dry-run 無効化。 |

#### 4.2.4 DryRunOptions フィールド仕様

| フィールド | 型 | 有効値 | デフォルト値 | 説明 |
|------------|-----|--------|--------------|------|
| `DetailLevel` | `resource.DetailLevel` | `DetailLevelSummary`, `DetailLevelFull` | `DetailLevelSummary` | 出力の詳細レベル |
| `ShowSensitive` | `bool` | `true`, `false` | `false` | 機密情報の表示可否 |

#### 4.2.5 動作仕様

| 入力 | 設定値 | 結果 |
|------|--------|------|
| 有効な `DryRunOptions` | `options` | dry-run モード有効、指定された設定を使用 |
| `nil` | `nil` | dry-run モード無効（デフォルトと同じ） |

#### 4.2.6 使用例

```go
// Production usage - basic dry-run
WithDryRun(&resource.DryRunOptions{
    DetailLevel:   resource.DetailLevelSummary,
    ShowSensitive: false,
})

// Test usage - detailed dry-run with sensitive info
WithDryRun(&resource.DryRunOptions{
    DetailLevel:   resource.DetailLevelFull,
    ShowSensitive: true,
})

// Disable dry-run explicitly
WithDryRun(nil)

// Using existing DryRunOptions from configuration
WithDryRun(opts.dryRunOptions)
```

### 4.3 WithKeepTempDirs Function

#### 4.3.1 シグネチャ

```go
// WithKeepTempDirs controls whether temporary directories created during
// group execution are preserved after completion.
//
// When keep is true, temporary directories are not cleaned up, which can
// be useful for debugging purposes. When false (default), temporary
// directories are automatically cleaned up.
//
// Warning: Keeping temporary directories may consume disk space and should
// typically only be used during development and debugging.
//
// Example:
//   WithKeepTempDirs(true)  // Keep temp dirs for debugging
//   WithKeepTempDirs(false) // Clean up temp dirs (default)
func WithKeepTempDirs(keep bool) GroupExecutorOption
```

#### 4.3.2 実装仕様

```go
func WithKeepTempDirs(keep bool) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.keepTempDirs = keep
    }
}
```

#### 4.3.3 パラメータ仕様

| パラメータ | 型 | 制約 | 説明 |
|------------|-----|------|------|
| `keep` | `bool` | なし | 一時ディレクトリを保持するか |

#### 4.3.4 動作仕様

| 入力 | 設定値 | 動作 |
|------|--------|------|
| `true` | `true` | 一時ディレクトリを保持（デバッグ用） |
| `false` | `false` | 一時ディレクトリを削除（通常動作） |

#### 4.3.5 使用例

```go
// Development/debugging
WithKeepTempDirs(true)

// Production (explicit, but not necessary as it's the default)
WithKeepTempDirs(false)

// Test scenarios where temp dirs need inspection
if debugMode {
    WithKeepTempDirs(true)
}
```

## 5. テスト用ヘルパー関数仕様

### 5.1 パッケージ配置

```go
//go:build test

// Package group provides test utilities for DefaultGroupExecutor.
// This file is only compiled during testing due to the build tag.
package group

// Import path: Same as main package ("yourproject/internal/runner/executor/group")
// File: testing_helpers.go
```

**Build Tag 仕様**:
- ファイル先頭に `//go:build test` を配置
- `go build` 時には除外、`go test` 時のみインクルード
- 同一パッケージ内のため、unexported 型や関数にもアクセス可能

**ファイル例 (`testing_helpers.go`)**:
```go
//go:build test

package group

import (
    "github.com/yourusername/yourproject/internal/runner/resource"
    "github.com/yourusername/yourproject/internal/runner/runnertypes"
)

// NewTestGroupExecutor creates a DefaultGroupExecutor with common test defaults.
// This function is only available during testing due to the build tag.
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    return NewDefaultGroupExecutor(
        nil,                // executor: nil for tests
        config,
        nil,                // validator: nil for tests
        nil,                // verificationManager: nil for tests
        resourceManager,
        "test-run-123",     // consistent test runID
        options...,
    )
}
```

**Build Tag の動作保証**:
- `go build ./...`: ヘルパー関数は含まれない（プロダクションビルド）
- `go test ./...`: ヘルパー関数が利用可能（テスト実行）
- IDE のコード補完: テストファイル内でのみ認識される

### 5.2 NewTestGroupExecutor Function

#### 5.2.1 シグネチャ

```go
// NewTestGroupExecutor creates a DefaultGroupExecutor with common test defaults.
// This is a convenience function for tests that don't need custom executors,
// validators, or verification managers.
//
// Default values:
//   - executor: nil (most tests don't need actual command execution)
//   - validator: nil (security validation often skipped in unit tests)
//   - verificationManager: nil (file verification often not needed in unit tests)
//   - runID: "test-run-123" (consistent test identifier)
//
// Use this function when you only need to specify config, resourceManager,
// and optionally some options. For more control over dependencies, use
// NewTestGroupExecutorWithConfig instead.
//
// Example:
//   ge := testing.NewTestGroupExecutor(config, mockResourceManager)
//
//   ge := testing.NewTestGroupExecutor(
//       config,
//       mockResourceManager,
//       WithNotificationFunc(testNotificationFunc),
//   )
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor
```

#### 5.2.2 実装仕様

```go
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    return NewDefaultGroupExecutor(
        nil,                   // executor: nil for tests
        config,
        nil,                   // validator: nil for tests
        nil,                   // verificationManager: nil for tests
        resourceManager,
        "test-run-123",        // consistent test runID
        options...,
    )
}
```

#### 5.2.3 パラメータ仕様

| パラメータ | 型 | 必須 | 制約 | デフォルト値 |
|------------|-----|------|------|--------------|
| `config` | `*runnertypes.ConfigSpec` | Yes | nil 不可 | - |
| `resourceManager` | `resource.ResourceManager` | Yes | nil 不可 | - |
| `options` | `...group.GroupExecutorOption` | No | - | 空スライス |

#### 5.2.4 使用例

```go
// Basic test setup
ge := NewTestGroupExecutor(config, mockResourceManager)

// With notification testing
var notificationReceived bool
notifyFunc := func(_ *runnertypes.GroupSpec, _ *groupExecutionResult, _ time.Duration) {
    notificationReceived = true
}
ge := NewTestGroupExecutor(
    config,
    mockResourceManager,
    WithNotificationFunc(notifyFunc),
)

// With dry-run testing
ge := NewTestGroupExecutor(
    config,
    mockResourceManager,
    WithDryRun(&resource.DryRunOptions{
        DetailLevel:   resource.DetailLevelFull,
        ShowSensitive: true,
    }),
)
```

### 5.3 TestGroupExecutorConfig Structure

#### 5.3.1 型定義

```go
// TestGroupExecutorConfig holds configuration for test group executor creation.
// Use this with NewTestGroupExecutorWithConfig when you need to customize
// specific dependencies that NewTestGroupExecutor doesn't expose.
//
// Unset fields will use test-appropriate defaults:
//   - Executor: nil
//   - Validator: nil
//   - VerificationManager: nil
//   - RunID: "test-run-123"
type TestGroupExecutorConfig struct {
    // Executor is the command executor. If nil, defaults to nil (test default).
    Executor executor.CommandExecutor

    // Config is the configuration specification. Required.
    Config *runnertypes.ConfigSpec

    // Validator is the security validator. If nil, defaults to nil (test default).
    Validator security.ValidatorInterface

    // VerificationManager is the file verification manager. If nil, defaults to nil.
    VerificationManager verification.ManagerInterface

    // ResourceManager is the resource manager interface. Required.
    ResourceManager resource.ResourceManager

    // RunID is the execution identifier. If empty, defaults to "test-run-123".
    RunID string
}
```

#### 5.3.2 フィールド仕様

| フィールド | 型 | 必須 | デフォルト値 | 説明 |
|------------|-----|------|--------------|------|
| `Executor` | `executor.CommandExecutor` | No | `nil` | コマンド実行インターフェース |
| `Config` | `*runnertypes.ConfigSpec` | Yes | - | 設定仕様（必須） |
| `Validator` | `security.ValidatorInterface` | No | `nil` | セキュリティバリデータ |
| `VerificationManager` | `verification.ManagerInterface` | No | `nil` | ファイル検証マネージャー |
| `ResourceManager` | `resource.ResourceManager` | Yes | - | リソースマネージャー（必須） |
| `RunID` | `string` | No | `"test-run-123"` | 実行識別子 |

### 5.4 NewTestGroupExecutorWithConfig Function

#### 5.4.1 シグネチャ

```go
// NewTestGroupExecutorWithConfig creates a DefaultGroupExecutor with custom configuration.
// Use this when you need to customize specific dependencies that NewTestGroupExecutor
// doesn't expose, such as providing a custom executor or validator.
//
// Unset fields in the config will use test-appropriate defaults:
//   - Executor: nil
//   - Validator: nil
//   - VerificationManager: nil
//   - RunID: "test-run-123"
//
// Required fields (Config and ResourceManager) must be set in the config struct.
//
// Example:
//   ge := testing.NewTestGroupExecutorWithConfig(
//       testing.TestGroupExecutorConfig{
//           Config:          config,
//           ResourceManager: mockResourceManager,
//           Executor:        mockExecutor,  // custom executor
//       },
//       WithKeepTempDirs(true),
//   )
func NewTestGroupExecutorWithConfig(
    cfg TestGroupExecutorConfig,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor
```

#### 5.4.2 実装仕様

```go
func NewTestGroupExecutorWithConfig(
    cfg TestGroupExecutorConfig,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    // Apply defaults for unset fields
    if cfg.RunID == "" {
        cfg.RunID = "test-run-123"
    }

    return NewDefaultGroupExecutor(
        cfg.Executor,
        cfg.Config,
        cfg.Validator,
        cfg.VerificationManager,
        cfg.ResourceManager,
        cfg.RunID,
        options...,
    )
}
```

#### 5.4.3 使用例

```go
// Custom executor for integration tests
ge := NewTestGroupExecutorWithConfig(
    TestGroupExecutorConfig{
        Config:          config,
        ResourceManager: mockResourceManager,
        Executor:        realExecutor,
    },
)

// Custom validator for security tests
ge := NewTestGroupExecutorWithConfig(
    TestGroupExecutorConfig{
        Config:          config,
        ResourceManager: mockResourceManager,
        Validator:       mockValidator,
    },
    WithNotificationFunc(securityTestNotifyFunc),
)

// Custom runID for tracing
ge := NewTestGroupExecutorWithConfig(
    TestGroupExecutorConfig{
        Config:          config,
        ResourceManager: mockResourceManager,
        RunID:           "security-test-" + testID,
    },
)
```

## 6. エラーハンドリング仕様

### 6.1 入力検証エラー

#### 6.1.1 必須パラメータ検証

| 条件 | 例外タイプ | メッセージ | 対応方法 |
|------|-----------|-----------|----------|
| `config == nil` | `panic` | "NewDefaultGroupExecutor: config cannot be nil" | 有効な config を指定 |
| `resourceManager == nil` | `panic` | "NewDefaultGroupExecutor: resourceManager cannot be nil" | 有効な resourceManager を指定 |
| `runID == ""` | `panic` | "NewDefaultGroupExecutor: runID cannot be empty" | 空でない runID を指定 |

#### 6.1.2 Option 関数エラー処理

```go
// nil オプションは安全に無視される
for _, opt := range options {
    if opt != nil {  // nil チェックで安全性確保
        opt(opts)
    }
}
```

### 6.2 実行時エラー

#### 6.2.1 DryRun オプション検証

```go
// WithDryRun 内での検証は不要（nil は有効な値）
func WithDryRun(options *resource.DryRunOptions) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        // nil チェック不要 - nil は「dry-run無効」を意味
        opts.dryRunOptions = options
    }
}
```

#### 6.2.2 通知関数エラー処理

```go
// 通知関数内でのエラーは呼び出し側で処理
if ge.notificationFunc != nil {
    defer func() {
        if r := recover(); r != nil {
            // 通知関数のパニックをログに記録し、継続
            log.Printf("Notification function panicked: %v", r)
        }
    }()
    ge.notificationFunc(group, result, duration)
}
```

### 6.3 テスト用ヘルパーエラー処理

#### 6.3.1 NewTestGroupExecutor エラー

```go
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    // 入力検証は NewDefaultGroupExecutor に委譲
    // panic は呼び出し元に伝播
    return NewDefaultGroupExecutor(
        nil,
        config,               // nil チェックは委譲先で実行
        nil,
        nil,
        resourceManager,      // nil チェックは委譲先で実行
        "test-run-123",
        options...,
    )
}
```

## 7. パフォーマンス仕様

### 7.1 実行時パフォーマンス要件

| メトリクス | 現在値 | 目標値 | 許容範囲 |
|------------|--------|--------|----------|
| 関数呼び出し時間 | ~50ns | <100ns | 110%以内 |
| メモリ割り当て | 1 allocation | 1-2 allocations | +1 allocation |
| CPU使用率 | 無視できるレベル | 無視できるレベル | 変化なし |

### 7.2 メモリ使用量仕様

#### 7.2.1 構造体サイズ

```go
// groupExecutorOptions memory layout
type groupExecutorOptions struct {
    notificationFunc groupNotificationFunc  // 8 bytes
    dryRunOptions    *resource.DryRunOptions // 8 bytes
    keepTempDirs     bool                    // 1 byte + 7 bytes padding
}
// Total: 24 bytes on 64-bit systems
```

#### 7.2.2 Option 関数オーバーヘッド

- **関数ポインタサイズ**: 8 bytes per option (64-bit system)
- **スライス オーバーヘッド**: 24 bytes (slice header)
- **推定合計**: 24 + (8 × オプション数) bytes

### 7.3 ベンチマーク仕様

#### 7.3.1 ベンチマークテスト

```go
func BenchmarkNewDefaultGroupExecutor(b *testing.B) {
    config := &runnertypes.ConfigSpec{/* test config */}
    resourceManager := &mockResourceManager{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = NewDefaultGroupExecutor(
            nil,
            config,
            nil,
            nil,
            resourceManager,
            "benchmark-run",
        )
    }
}

func BenchmarkNewDefaultGroupExecutorWithOptions(b *testing.B) {
    config := &runnertypes.ConfigSpec{/* test config */}
    resourceManager := &mockResourceManager{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = NewDefaultGroupExecutor(
            nil,
            config,
            nil,
            nil,
            resourceManager,
            "benchmark-run",
            WithNotificationFunc(dummyNotifyFunc),
            WithDryRun(&resource.DryRunOptions{DetailLevel: resource.DetailLevelFull}),
            WithKeepTempDirs(true),
        )
    }
}
```

## 8. セキュリティ仕様

### 8.1 入力検証

#### 8.1.1 型安全性

- **コンパイル時検証**: Option 関数の型により不正な型の混入を防止
- **実行時検証**: nil チェックにより不正な参照を防止

#### 8.1.2 セキュリティ関連パラメータ

| パラメータ | セキュリティ要件 | 検証方法 |
|------------|------------------|----------|
| `validator` | 本番では非nil推奨 | 呼び出し側で責任を持つ |
| `config` | 必須、信頼できるソース | nil チェック + 呼び出し側検証 |
| `notificationFunc` | 任意の関数だが実行制限 | panic 回復により影響を限定 |

### 8.2 実行時セキュリティ

#### 8.2.1 通知関数の安全実行

```go
// 通知関数は安全に実行され、エラーが他に影響しない
if ge.notificationFunc != nil {
    func() {
        defer func() {
            if r := recover(); r != nil {
                // エラーログ出力のみ、実行は継続
                log.Printf("Notification function error: %v", r)
            }
        }()
        ge.notificationFunc(group, result, duration)
    }()
}
```

#### 8.2.2 一時ディレクトリセキュリティ

```go
// keepTempDirs = false の場合の安全なクリーンアップ
if !ge.keepTempDirs {
    defer func() {
        if err := os.RemoveAll(tempDir); err != nil {
            // クリーンアップエラーをログに記録
            log.Printf("Failed to cleanup temp directory %s: %v", tempDir, err)
        }
    }()
}
```

## 9. 互換性仕様

### 9.1 移行完了状態

#### 9.1.1 最終実装

**ステータス**: ✅ 移行完了 (2025-10-27)

レガシー関数は削除され、新しいFunctional Optionsパターンのみがサポートされています。

```go
// 移行期間中のレガシー関数（一時的）
func NewDefaultGroupExecutorLegacy(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    notificationFunc groupNotificationFunc,
    isDryRun bool,
    dryRunDetailLevel resource.DetailLevel,
    dryRunShowSensitive bool,
    keepTempDirs bool,
) *DefaultGroupExecutor {
    // 新実装への変換
    var dryRunOptions *resource.DryRunOptions
    if isDryRun {
        dryRunOptions = &resource.DryRunOptions{
            DetailLevel:   dryRunDetailLevel,
            ShowSensitive: dryRunShowSensitive,
        }
    }

すべてのプロダクションおよびテストコードは新しいAPIに移行済みです。

### 9.2 動作互換性保証

#### 9.2.1 デフォルト値の検証結果

| 設定 | 旧実装デフォルト | 新実装デフォルト | 互換性 |
|------|-----------------|-----------------|--------|
| `notificationFunc` | `nil` | `nil` | ✅ 完全一致 |
| dry-run mode | `false` | `false` (`nil`) | ✅ 完全一致 |
| `dryRunDetailLevel` | `DetailLevelSummary` | `DetailLevelSummary` | ✅ 完全一致 |
| `dryRunShowSensitive` | `false` | `false` | ✅ 完全一致 |
| `keepTempDirs` | `false` | `false` | ✅ 完全一致 |

**検証結果**: すべてのデフォルト値が完全に一致することを確認済み。

## 10. テスト仕様

### 10.1 ユニットテスト要件

#### 10.1.1 Option 関数テスト

```go
func TestWithNotificationFunc(t *testing.T) {
    tests := []struct {
        name     string
        fn       groupNotificationFunc
        expected groupNotificationFunc
    }{
        {
            name:     "valid function",
            fn:       dummyNotifyFunc,
            expected: dummyNotifyFunc,
        },
        {
            name:     "nil function",
            fn:       nil,
            expected: nil,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            opts := defaultGroupExecutorOptions()
            WithNotificationFunc(tt.fn)(opts)

            // 関数ポインタの比較は直接できないため、実行結果で検証
            if (opts.notificationFunc == nil) != (tt.expected == nil) {
                t.Errorf("notificationFunc mismatch")
            }
        })
    }
}
```

#### 10.1.2 統合テスト

```go
func TestNewDefaultGroupExecutor_Integration(t *testing.T) {
    config := &runnertypes.ConfigSpec{/* test config */}
    mockRM := &mockResourceManager{}

    t.Run("default options", func(t *testing.T) {
        ge := NewDefaultGroupExecutor(
            nil, config, nil, nil, mockRM, "test-run",
        )

        assert.NotNil(t, ge)
        assert.Equal(t, config, ge.config)
        assert.Equal(t, mockRM, ge.resourceManager)
        assert.Equal(t, "test-run", ge.runID)
        assert.Nil(t, ge.notificationFunc)
        assert.False(t, ge.isDryRun)
        assert.False(t, ge.keepTempDirs)
    })
}
```

### 10.2 パフォーマンステスト

```go
func TestNewDefaultGroupExecutor_Performance(t *testing.T) {
    config := &runnertypes.ConfigSpec{/* test config */}
    mockRM := &mockResourceManager{}

    // アロケーション回数のテスト
    allocs := testing.AllocsPerRun(100, func() {
        _ = NewDefaultGroupExecutor(
            nil, config, nil, nil, mockRM, "perf-test",
            WithKeepTempDirs(false),
        )
    })

    // 期待値: 1回のアロケーション (groupExecutorOptions構造体)
    // 許容範囲: 2回以下
    if allocs > 2 {
        t.Errorf("Too many allocations per call: got %.1f, want <= 2", allocs)
    }
}

func BenchmarkNewDefaultGroupExecutor(b *testing.B) {
    config := &runnertypes.ConfigSpec{/* test config */}
    mockRM := &mockResourceManager{}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = NewDefaultGroupExecutor(
            nil, config, nil, nil, mockRM, "bench-test",
            WithNotificationFunc(nil),
            WithDryRun(&resource.DryRunOptions{
                DetailLevel:   resource.DetailLevelFull,
                ShowSensitive: false,
            }),
            WithKeepTempDirs(false),
        )
    }
}
```

---

**文書バージョン**: 1.0
**作成日**: 2025-10-27
**承認日**: [日付]
**次回レビュー予定**: [日付]
