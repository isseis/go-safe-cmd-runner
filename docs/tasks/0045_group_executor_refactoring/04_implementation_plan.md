# GroupExecutor リファクタリング - 実装計画書

## 進捗サマリー

**最終更新**: 2025-10-27

| Phase | タスク数 | 完了 | 進行中 | 未着手 | 進捗率 |
|-------|---------|------|--------|--------|--------|
| Phase 1: 基盤実装 | 6 | 6 | 0 | 0 | 100% ✅ |
| Phase 2: プロダクション移行 | 4 | 2 | 2 | 0 | 50% 🔄 |
| Phase 3: テストコード移行 | 5 | 0 | 0 | 5 | 0% 📝 |
| Phase 4: クリーンアップ | 4 | 0 | 0 | 4 | 0% 📝 |
| **合計** | **19** | **8** | **2** | **9** | **42%** |

### 現在の状態

- ✅ **Phase 1完了**: Functional Optionsパターンの実装が完了
  - 新しいAPI、テストヘルパー、ユニットテストがすべて実装済み
- 🔄 **Phase 2進行中**: プロダクションコードの移行が完了、検証中
  - runner.goの移行完了、全テストパス
  - パフォーマンステストと動作確認が要確認
- 📝 **Phase 3未着手**: テストコードは現在レガシー関数を使用中
  - 22箇所のテストコードが移行待ち

### 次のステップ

1. Phase 2の検証作業を完了する（GE-2.3, GE-2.4）
2. Phase 3のテストコード移行を開始する（GE-3.1から）

---

## 1. 実装概要

### 1.1 目的

`NewDefaultGroupExecutor` 関数の11個の引数を Functional Options パターンでリファクタリングし、可読性と保守性を大幅に改善する。

### 1.2 現状分析

#### 1.2.1 現在の実装

```go
// 現在のシグネチャ（11引数）
func NewDefaultGroupExecutor(
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
) *DefaultGroupExecutor
```

#### 1.2.2 使用状況

- **プロダクションコード**: 1箇所（`internal/runner/runner.go:318`）
- **テストコード**: 22箇所（すべて `group_executor_test.go`）

#### 1.2.3 共通パターン分析

**プロダクションコード**:
- `notificationFunc`: `runner.logGroupExecutionSummary` （常に同じ値）
- dry-run設定: `opts.dryRunOptions` から取得して分解
- その他: 実際の値を設定

**テストコード**:
- `executor`: 大部分が `nil`（実際のコマンド実行不要）
- `validator`: 大部分が `nil` または `mockValidator`
- `verificationManager`: 大部分が `nil` または `mockVerificationManager`
- `runID`: 大部分が `"test-run-123"`
- `notificationFunc`: テスト固有の関数または `nil`
- dry-run設定: ほぼデフォルト値（`false`, `DetailLevelSummary`, `false`）
- `keepTempDirs`: ほぼ `false`

## 2. 実装設計

### 2.1 新しいアーキテクチャ

#### 2.1.1 必須引数（位置引数）

```go
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

#### 2.1.2 オプション関数

```go
type GroupExecutorOption func(*groupExecutorOptions)

func WithNotificationFunc(fn groupNotificationFunc) GroupExecutorOption
func WithDryRun(options *resource.DryRunOptions) GroupExecutorOption
func WithKeepTempDirs(keep bool) GroupExecutorOption
```

#### 2.1.3 内部構造体

```go
type groupExecutorOptions struct {
    notificationFunc groupNotificationFunc
    dryRunOptions    *resource.DryRunOptions  // nil = disabled
    keepTempDirs     bool
}
```

### 2.2 テストヘルパー関数

#### 2.2.1 基本ヘルパー

```go
//go:build test

func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    return NewDefaultGroupExecutor(
        nil,                    // executor
        config,
        nil,                    // validator
        nil,                    // verificationManager
        resourceManager,
        "test-run-123",         // runID
        options...,
    )
}
```

#### 2.2.2 カスタマイズ可能ヘルパー

```go
type TestGroupExecutorConfig struct {
    Executor            executor.CommandExecutor
    Config              *runnertypes.ConfigSpec
    Validator           security.ValidatorInterface
    VerificationManager verification.ManagerInterface
    ResourceManager     resource.ResourceManager
    RunID               string
}

func NewTestGroupExecutorWithConfig(
    cfg TestGroupExecutorConfig,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor
```

## 3. 実装フェーズ計画

### 3.1 Phase 1: 基盤実装（Week 1）

#### 3.1.1 ファイル構成

```
internal/runner/
├── group_executor.go              # 既存ファイル（修正）
├── group_executor_options.go      # 新規作成
└── group_executor_test_helpers.go # 新規作成（Build Tag）
```

#### 3.1.2 実装タスク

| タスク | 説明 | 所要時間 | 依存関係 | 状態 |
|--------|------|----------|----------|------|
| GE-1.1 | `group_executor_options.go` 作成 | 2h | なし | ✅ 完了 |
| GE-1.2 | Option 関数実装 | 4h | GE-1.1 | ✅ 完了 |
| GE-1.3 | 新しい `NewDefaultGroupExecutor` 実装 | 3h | GE-1.2 | ✅ 完了 |
| GE-1.4 | 既存関数をレガシー関数へリネーム | 1h | GE-1.3 | ✅ 完了 |
| GE-1.5 | テストヘルパー関数実装 | 3h | GE-1.3 | ✅ 完了 |
| GE-1.6 | ユニットテスト作成 | 4h | GE-1.5 | ✅ 完了 |

#### 3.1.3 成果物

- 新しい Option 関数群
- レガシー関数との並行サポート
- テスト用ヘルパー関数
- 包括的なユニットテスト

### 3.2 Phase 2: プロダクションコード移行（Week 2）

#### 3.2.1 移行対象

**ファイル**: `internal/runner/runner.go:318`

**変更前**:
```go
runner.groupExecutor = NewDefaultGroupExecutor(
    opts.executor,
    configSpec,
    validator,
    opts.verificationManager,
    opts.resourceManager,
    opts.runID,
    runner.logGroupExecutionSummary,
    opts.dryRun,
    detailLevel,
    showSensitive,
    opts.keepTempDirs,
)
```

**変更後**:
```go
var groupOptions []GroupExecutorOption
groupOptions = append(groupOptions, WithNotificationFunc(runner.logGroupExecutionSummary))

if opts.dryRunOptions != nil {
    groupOptions = append(groupOptions, WithDryRun(opts.dryRunOptions))
}

if opts.keepTempDirs {
    groupOptions = append(groupOptions, WithKeepTempDirs(true))
}

runner.groupExecutor = NewDefaultGroupExecutor(
    opts.executor,
    configSpec,
    validator,
    opts.verificationManager,
    opts.resourceManager,
    opts.runID,
    groupOptions...,
)
```

#### 3.2.2 実装タスク

| タスク | 説明 | 所要時間 | 依存関係 | 状態 |
|--------|------|----------|----------|------|
| GE-2.1 | プロダクションコード移行 | 2h | Phase 1完了 | ✅ 完了 |
| GE-2.2 | 統合テスト実行・修正 | 3h | GE-2.1 | ✅ 完了 |
| GE-2.3 | パフォーマンステスト | 2h | GE-2.2 | ⏳ 要確認 |
| GE-2.4 | プロダクション動作確認 | 1h | GE-2.3 | ⏳ 要確認 |

### 3.3 Phase 3: テストコード移行（Week 3-4）

#### 3.3.1 移行戦略

22箇所のテストコードを効率的に移行:

**パターン1: 標準パターン（15箇所程度）**
```go
// 変更前
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    nil,
    false,                       // isDryRun
    resource.DetailLevelSummary, // dryRunDetailLevel
    false,                       // dryRunShowSensitive
    false,                       // keepTempDirs
)

// 変更後
ge := NewTestGroupExecutor(config, mockRM)
```

**パターン2: 通知関数有り（5箇所程度）**
```go
// 変更前
ge := NewDefaultGroupExecutor(
    nil, config, mockValidator, mockVerificationManager, mockRM,
    "test-run-123", notificationFunc,
    false, resource.DetailLevelSummary, false, false,
)

// 変更後
ge := NewTestGroupExecutor(
    config, mockRM,
    WithNotificationFunc(notificationFunc),
)
```

**パターン3: カスタム設定（2箇所程度）**
```go
// 変更前
ge := NewDefaultGroupExecutor(
    mockExecutor, config, validator, verificationManager, mockRM,
    "custom-run-id", notificationFunc,
    true, resource.DetailLevelFull, true, true,
)

// 変更後
ge := NewTestGroupExecutorWithConfig(
    TestGroupExecutorConfig{
        Executor:            mockExecutor,
        Config:              config,
        Validator:           validator,
        VerificationManager: verificationManager,
        ResourceManager:     mockRM,
        RunID:               "custom-run-id",
    },
    WithNotificationFunc(notificationFunc),
    WithDryRun(&resource.DryRunOptions{
        DetailLevel:   resource.DetailLevelFull,
        ShowSensitive: true,
    }),
    WithKeepTempDirs(true),
)
```

#### 3.3.2 実装タスク

| タスク | 説明 | 所要時間 | 依存関係 | 状態 |
|--------|------|----------|----------|------|
| GE-3.1 | パターン1移行（15箇所） | 6h | Phase 2完了 | 📝 未着手 |
| GE-3.2 | パターン2移行（5箇所） | 3h | GE-3.1 | 📝 未着手 |
| GE-3.3 | パターン3移行（2箇所） | 3h | GE-3.2 | 📝 未着手 |
| GE-3.4 | 全テスト実行・修正 | 4h | GE-3.3 | 📝 未着手 |
| GE-3.5 | テスト品質確認 | 2h | GE-3.4 | 📝 未着手 |

### 3.4 Phase 4: クリーンアップ（Week 5）

#### 3.4.1 実装タスク

| タスク | 説明 | 所要時間 | 依存関係 | 状態 |
|--------|------|----------|----------|------|
| GE-4.1 | レガシー関数削除 | 1h | Phase 3完了 | 📝 未着手 |
| GE-4.2 | ドキュメント更新 | 3h | GE-4.1 | 📝 未着手 |
| GE-4.3 | 最終テスト実行 | 2h | GE-4.2 | 📝 未着手 |
| GE-4.4 | コードレビュー対応 | 3h | GE-4.3 | 📝 未着手 |

## 4. 技術的詳細

### 4.1 ファイル別実装詳細

#### 4.1.1 group_executor_options.go

```go
package runner

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
    "time"
)

// GroupExecutorOption configures a DefaultGroupExecutor during construction.
type GroupExecutorOption func(*groupExecutorOptions)

// groupExecutorOptions holds internal configuration options for DefaultGroupExecutor.
type groupExecutorOptions struct {
    notificationFunc groupNotificationFunc
    dryRunOptions    *resource.DryRunOptions
    keepTempDirs     bool
}

// defaultGroupExecutorOptions returns a new groupExecutorOptions with default values.
func defaultGroupExecutorOptions() *groupExecutorOptions {
    return &groupExecutorOptions{
        notificationFunc: nil,
        dryRunOptions:    nil,    // dry-run disabled
        keepTempDirs:     false,
    }
}

// WithNotificationFunc sets the notification function.
func WithNotificationFunc(fn groupNotificationFunc) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.notificationFunc = fn
    }
}

// WithDryRun enables dry-run mode with the specified options.
func WithDryRun(options *resource.DryRunOptions) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.dryRunOptions = options
    }
}

// WithKeepTempDirs controls temporary directory cleanup.
func WithKeepTempDirs(keep bool) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.keepTempDirs = keep
    }
}
```

#### 4.1.2 group_executor.go の変更

```go
// NewDefaultGroupExecutor creates a new DefaultGroupExecutor with the specified
// configuration and optional settings.
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

    // Extract dry-run settings
    isDryRun := opts.dryRunOptions != nil
    var detailLevel resource.DetailLevel
    var showSensitive bool

    if isDryRun {
        detailLevel = opts.dryRunOptions.DetailLevel
        showSensitive = opts.dryRunOptions.ShowSensitive
    } else {
        detailLevel = resource.DetailLevelSummary
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
        dryRunDetailLevel:   detailLevel,
        dryRunShowSensitive: showSensitive,
        keepTempDirs:        opts.keepTempDirs,
    }
}
```

#### 4.1.3 group_executor_test_helpers.go

```go
//go:build test

package runner

import (
    "github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security"
    "github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// NewTestGroupExecutor creates a DefaultGroupExecutor with common test defaults.
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    return NewDefaultGroupExecutor(
        nil,                    // executor
        config,
        nil,                    // validator
        nil,                    // verificationManager
        resourceManager,
        "test-run-123",         // runID
        options...,
    )
}

// TestGroupExecutorConfig holds configuration for test group executor creation.
type TestGroupExecutorConfig struct {
    Executor            executor.CommandExecutor
    Config              *runnertypes.ConfigSpec
    Validator           security.ValidatorInterface
    VerificationManager verification.ManagerInterface
    ResourceManager     resource.ResourceManager
    RunID               string
}

// NewTestGroupExecutorWithConfig creates a DefaultGroupExecutor with custom configuration.
func NewTestGroupExecutorWithConfig(
    cfg TestGroupExecutorConfig,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    // Apply defaults for unset fields
    executor := cfg.Executor
    validator := cfg.Validator
    verificationManager := cfg.VerificationManager
    runID := cfg.RunID
    if runID == "" {
        runID = "test-run-123"
    }

    return NewDefaultGroupExecutor(
        executor,
        cfg.Config,
        validator,
        verificationManager,
        cfg.ResourceManager,
        runID,
        options...,
    )
}
```

### 4.2 移行スクリプト

効率的な移行のため、半自動化スクリプトを準備:

```bash
#!/bin/bash
# migrate_test_calls.sh

# パターン1: 標準パターンの置換
sed -i.bak -E 's/ge := NewDefaultGroupExecutor\(\s*nil,\s*([^,]+),\s*nil,\s*nil,\s*([^,]+),\s*"test-run-123",\s*nil,\s*false,\s*resource\.DetailLevelSummary,\s*false,\s*false,?\s*\)/ge := NewTestGroupExecutor(\1, \2)/g' group_executor_test.go

# パターン2: 通知関数有りの場合の識別
grep -n "NewDefaultGroupExecutor.*notificationFunc" group_executor_test.go
```

## 5. 品質保証計画

### 5.1 テスト戦略

#### 5.1.1 ユニットテスト

```go
func TestGroupExecutorOptions(t *testing.T) {
    tests := []struct {
        name    string
        options []GroupExecutorOption
        want    groupExecutorOptions
    }{
        {
            name:    "default options",
            options: nil,
            want: groupExecutorOptions{
                notificationFunc: nil,
                dryRunOptions:    nil,
                keepTempDirs:     false,
            },
        },
        {
            name: "with notification func",
            options: []GroupExecutorOption{
                WithNotificationFunc(testNotificationFunc),
            },
            want: groupExecutorOptions{
                notificationFunc: testNotificationFunc,
                dryRunOptions:    nil,
                keepTempDirs:     false,
            },
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            opts := defaultGroupExecutorOptions()
            for _, opt := range tt.options {
                opt(opts)
            }

            // Compare results
            if !reflect.DeepEqual(*opts, tt.want) {
                t.Errorf("got %+v, want %+v", *opts, tt.want)
            }
        })
    }
}
```

#### 5.1.2 統合テスト

```go
func TestNewDefaultGroupExecutor_Integration(t *testing.T) {
    config := &runnertypes.ConfigSpec{
        Global: runnertypes.GlobalSpec{
            Timeout: common.IntPtr(30),
        },
    }
    mockRM := new(runnertesting.MockResourceManager)

    // Test with options
    ge := NewDefaultGroupExecutor(
        nil, config, nil, nil, mockRM, "test-run-123",
        WithNotificationFunc(testNotificationFunc),
        WithDryRun(&resource.DryRunOptions{
            DetailLevel:   resource.DetailLevelFull,
            ShowSensitive: true,
        }),
        WithKeepTempDirs(true),
    )

    // Verify configuration
    assert.NotNil(t, ge.notificationFunc)
    assert.True(t, ge.isDryRun)
    assert.Equal(t, resource.DetailLevelFull, ge.dryRunDetailLevel)
    assert.True(t, ge.dryRunShowSensitive)
    assert.True(t, ge.keepTempDirs)
}
```

#### 5.1.3 パフォーマンステスト

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

### 5.2 品質基準

#### 5.2.1 パフォーマンス基準

| メトリクス | 現在値 | 目標値 | 許容範囲 |
|------------|--------|--------|----------|
| 関数呼び出し時間 | ~50ns | <100ns | 110%以内 |
| メモリ割り当て | 1 allocation | 1-2 allocations | +1 allocation |
| テストコード行数 | 基準値 | 30-40%削減 | 25%以上削減 |

#### 5.2.2 品質基準

| 項目 | 基準 | 測定方法 |
|------|------|----------|
| テストカバレッジ | ≥85% | `go test -cover` |
| 循環複雑度 | ≤15 | `gocyclo` |
| 引数数 | ≤6+options | 静的解析 |
| ドキュメント完成度 | 100% | レビュー |

### 5.3 リスク管理

#### 5.3.1 技術的リスク

| リスク | 確率 | 影響度 | 対策 |
|--------|------|--------|------|
| デフォルト値不一致 | 低 | 中 | 詳細テストとレビュー |
| パフォーマンス劣化 | 低 | 中 | ベンチマークテスト |
| 移行時のバグ | 中 | 高 | 段階的移行とテスト |

#### 5.3.2 スケジュールリスク

| リスク | 確率 | 影響度 | 対策 |
|--------|------|--------|------|
| テスト移行の遅延 | 中 | 中 | 並行作業と自動化 |
| レビュー時間延長 | 中 | 低 | 事前の設計確認 |

## 6. 展開計画

### 6.1 デプロイメント戦略

#### 6.1.1 段階的展開

1. **Phase 1**: 内部テスト環境での検証
2. **Phase 2**: ステージング環境での統合テスト
3. **Phase 3**: 本番環境への段階的展開
4. **Phase 4**: レガシー関数の完全削除

#### 6.1.2 ロールバック計画

各フェーズでの問題発生時:
- **即座のロールバック**: Git レベルでの巻き戻し
- **部分ロールバック**: レガシー関数への一時的復帰
- **緊急対応**: ホットフィックスの適用

### 6.2 監視・メトリクス

#### 6.2.1 成功指標

- テストコード行数削減: 30-40%
- 新しいテストの作成時間短縮: 50%
- コードレビュー時間短縮: 20%
- バグ報告数: 変化なしまたは減少

#### 6.2.2 技術指標

- ビルド時間: 変化なし
- テスト実行時間: +5%以内
- メモリ使用量: +5%以内
- 実行時間: +10%以内

## 7. 実施スケジュール

### 7.1 全体スケジュール

```
Week 1: Phase 1 - 基盤実装
├── Day 1-2: Option関数とヘルパー実装
├── Day 3-4: 新しいコンストラクタ実装
└── Day 5: テスト作成と検証

Week 2: Phase 2 - プロダクション移行
├── Day 1-2: 本番コード移行
├── Day 3-4: 統合テストと動作確認
└── Day 5: パフォーマンステスト

Week 3: Phase 3a - テスト移行(前半)
├── Day 1-2: パターン1移行(15箇所)
├── Day 3-4: パターン2移行(5箇所)
└── Day 5: 中間テスト実行

Week 4: Phase 3b - テスト移行(後半)
├── Day 1-2: パターン3移行(2箇所)
├── Day 3-4: 全体テスト・修正
└── Day 5: 品質確認

Week 5: Phase 4 - クリーンアップ
├── Day 1: レガシー関数削除
├── Day 2-3: ドキュメント整備
├── Day 4: 最終テスト
└── Day 5: リリース準備
```

### 7.2 マイルストーン

| マイルストーン | 日付 | 成果物 |
|----------------|------|--------|
| MS1: 基盤完成 | Week 1 End | 新実装とテスト |
| MS2: 本番移行完了 | Week 2 End | プロダクション動作確認 |
| MS3: テスト移行完了 | Week 4 End | 全テスト移行 |
| MS4: リリース | Week 5 End | 完全なリファクタリング |

### 7.3 リソース計画

#### 7.3.1 人員配置

- **主担当**: 1名（フルタイム）
- **レビューア**: 1名（パートタイム、各フェーズで2-3時間）
- **テスター**: 1名（Phase 2-3で各2-3時間）

#### 7.3.2 環境要件

- 開発環境: Go 1.19+
- テスト環境: CI/CD パイプライン
- 静的解析ツール: golangci-lint, gocyclo

## 8. 承認・レビュー

### 8.1 レビューポイント

#### 8.1.1 設計レビュー

- [ ] Functional Options パターンの適用妥当性
- [ ] デフォルト値の妥当性
- [ ] テストヘルパー関数の設計

#### 8.1.2 実装レビュー

- [ ] Option 関数の型安全性
- [ ] エラーハンドリングの適切性
- [ ] パフォーマンスへの影響

#### 8.1.3 テストレビュー

- [ ] テストカバレッジの充足性
- [ ] エッジケースのカバレッジ
- [ ] パフォーマンステストの妥当性

### 8.2 承認プロセス

1. **設計レビュー**: アーキテクトによる承認
2. **実装レビュー**: 開発チームリードによる承認
3. **品質レビュー**: QAチームによる承認
4. **最終承認**: プロジェクトオーナーによる承認

---

**文書バージョン**: 1.0
**作成日**: 2025-10-27
**承認日**: [日付]
**次回レビュー予定**: [日付]
