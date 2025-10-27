# NewDefaultGroupExecutor リファクタリング分析と提案

## 1. 現状分析

### 1.1 現在の関数シグネチャ

```go
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

**引数の数**: 11個（非常に多い）

### 1.2 各引数の使用状況分析

#### 1.2.1 コア依存性（必須）

| 引数 | 型 | デフォルト値の有無 | 分析結果 |
|------|----|--------------------|----------|
| `executor` | `executor.CommandExecutor` | なし | コマンド実行に必須。プロダクションでは `nil` にならない。テストでは `nil` の場合あり。 |
| `config` | `*runnertypes.ConfigSpec` | なし | 設定に必須。常に非nil。 |
| `validator` | `security.ValidatorInterface` | なし | セキュリティ検証に必須。プロダクションでは非nil、一部テストでは `nil`。 |
| `verificationManager` | `verification.ManagerInterface` | nilが妥当 | ファイル検証機能。機能的にはオプショナル。プロダクションでは通常設定される。 |
| `resourceManager` | `resource.ResourceManager` | なし | リソース管理に必須。常に非nil。 |
| `runID` | `string` | 自動生成可能 | 実行追跡用ID。空文字列チェックが必要だが、自動生成も可能。 |

#### 1.2.2 機能的オプション

| 引数 | 型 | デフォルト値 | プロダクション値 | テスト値 | 分析結果 |
|------|----|--------------|--------------------|----------|----------|
| `notificationFunc` | `groupNotificationFunc` | `nil` | `runner.logGroupExecutionSummary` | テストによる（`nil` または専用関数） | プロダクションでは常に同じ関数。テストでは検証用関数または `nil`。 |
| `isDryRun` | `bool` | `false` | `opts.dryRun` | 多くは `false`、一部 `true` | デフォルト値は `false` が妥当。 |
| `dryRunDetailLevel` | `resource.DetailLevel` | `DetailLevelSummary` | `opts.dryRunOptions.DetailLevel` | 大半が `DetailLevelSummary` | デフォルト値は `DetailLevelSummary` が妥当。 |
| `dryRunShowSensitive` | `bool` | `false` | `opts.dryRunOptions.ShowSensitive` | 大半が `false` | デフォルト値は `false` が妥当。 |
| `keepTempDirs` | `bool` | `false` | `opts.keepTempDirs` | 大半が `false`、一部 `true` | デバッグ用オプション。デフォルト値は `false` が妥当。 |

### 1.3 コールサイト分析

#### プロダクションコード（1箇所）

**場所**: `internal/runner/runner.go:318`

```go
runner.groupExecutor = NewDefaultGroupExecutor(
    opts.executor,
    configSpec,
    validator,
    opts.verificationManager,
    opts.resourceManager,
    opts.runID,
    runner.logGroupExecutionSummary,  // 常にこのメソッド
    opts.dryRun,
    detailLevel,        // opts.dryRunOptions から取得
    showSensitive,      // opts.dryRunOptions から取得
    opts.keepTempDirs,
)
```

**特徴**:
- `notificationFunc` は常に `runner.logGroupExecutionSummary` メソッド
- dry-run 関連の3つの引数は `opts.dryRunOptions` から取得される
- `keepTempDirs` はトップレベルオプション

#### テストコード（22箇所）

**パターン1: 基本的なテスト（大多数）**

```go
ge := NewDefaultGroupExecutor(
    nil,                         // executor (テストでは nil が多い)
    config,
    nil,                         // validator (多くは nil)
    nil,                         // verificationManager (多くは nil)
    mockRM,
    "test-run-123",              // 固定の runID
    nil,                         // notificationFunc (テストでは nil が多い)
    false,                       // isDryRun
    resource.DetailLevelSummary, // dryRunDetailLevel (固定値)
    false,                       // dryRunShowSensitive (固定値)
    false,                       // keepTempDirs (通常は false)
)
```

**パターン2: 通知機能をテストする場合**

```go
notificationFunc := func(_ *runnertypes.GroupSpec, result *groupExecutionResult, _ time.Duration) {
    // テスト検証ロジック
}

ge := NewDefaultGroupExecutor(
    // ... 他の引数 ...
    notificationFunc,  // 専用の検証関数
    // ... 残りの引数 ...
)
```

**パターン3: dry-run モードをテストする場合**

```go
ge := NewDefaultGroupExecutor(
    // ... 他の引数 ...
    true,                        // isDryRun = true
    resource.DetailLevelFull,    // 詳細レベルを変更
    true,                        // showSensitive = true
    // ... 残りの引数 ...
)
```

**パターン4: keepTempDirs をテストする場合**

```go
ge := NewDefaultGroupExecutor(
    // ... 他の引数 ...
    true,  // keepTempDirs = true
)
```

### 1.4 問題点の整理

1. **引数の数が多すぎる（11個）**: 可読性が低く、メンテナンスが困難
2. **関連する引数のグループ化がない**: dry-run 関連の3つの引数がバラバラ
3. **デフォルト値が存在するのに毎回指定が必要**: 特にテストコードで冗長
4. **プロダクションコードで常に同じ値を設定する引数がある**: `notificationFunc`
5. **テストでのボイラープレートコードが多い**: 固定値を毎回指定

## 2. リファクタリング案

### 案1: Options パターン（Functional Options）

#### 概要

Functional Options パターンを使用し、必須の引数のみ位置引数として受け取り、オプションは可変長引数で受け取る。

#### 実装例

```go
// GroupExecutorOption は GroupExecutor の設定オプションを表す
type GroupExecutorOption func(*groupExecutorOptions)

// groupExecutorOptions は内部的な設定を保持する
type groupExecutorOptions struct {
    notificationFunc groupNotificationFunc
    dryRunOptions    *resource.DryRunOptions // nil の場合は dry-run 無効
    keepTempDirs     bool
}

// デフォルト値を返す
func defaultGroupExecutorOptions() *groupExecutorOptions {
    return &groupExecutorOptions{
        notificationFunc: nil,  // デフォルトは通知なし
        dryRunOptions:    nil,  // デフォルトは dry-run 無効
        keepTempDirs:     false,
    }
}

// WithNotificationFunc は通知関数を設定する
func WithNotificationFunc(fn groupNotificationFunc) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.notificationFunc = fn
    }
}

// WithDryRun は dry-run モードを設定する
// dryRunOptions が nil の場合は dry-run を無効化する
func WithDryRun(dryRunOptions *resource.DryRunOptions) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.dryRunOptions = dryRunOptions
    }
}

// WithKeepTempDirs は一時ディレクトリを保持するかを設定する
func WithKeepTempDirs(keep bool) GroupExecutorOption {
    return func(opts *groupExecutorOptions) {
        opts.keepTempDirs = keep
    }
}

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    opts := defaultGroupExecutorOptions()
    for _, opt := range options {
        opt(opts)
    }

    // Unpack dryRunOptions
    isDryRun := opts.dryRunOptions != nil
    var dryRunDetailLevel resource.DetailLevel
    var dryRunShowSensitive bool
    if isDryRun {
        dryRunDetailLevel = opts.dryRunOptions.DetailLevel
        dryRunShowSensitive = opts.dryRunOptions.ShowSensitive
    } else {
        dryRunDetailLevel = resource.DetailLevelSummary
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

#### 使用例

**プロダクションコード**:

```go
runner.groupExecutor = NewDefaultGroupExecutor(
    opts.executor,
    configSpec,
    validator,
    opts.verificationManager,
    opts.resourceManager,
    opts.runID,
    WithNotificationFunc(runner.logGroupExecutionSummary),
    WithDryRun(opts.dryRunOptions), // *resource.DryRunOptions を直接渡す
    WithKeepTempDirs(opts.keepTempDirs),
)
```

**テストコード（シンプルなケース）**:

```go
// デフォルト値で十分な場合
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
)
```

**テストコード（通知機能をテスト）**:

```go
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    WithNotificationFunc(notificationFunc),
)
```

**テストコード（dry-run モードをテスト）**:

```go
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    WithDryRun(&resource.DryRunOptions{
        DetailLevel:   resource.DetailLevelFull,
        ShowSensitive: true,
    }),
)
```

#### Pros & Cons

**Pros**:
- ✅ **可読性が大幅に向上**: オプション名が明示的
- ✅ **拡張性が高い**: 新しいオプションの追加が容易で、既存コードへの影響が小さい
- ✅ **デフォルト値が効果的に機能**: 必要な設定だけを指定
- ✅ **テストコードが簡潔**: 多くのテストでオプション指定が不要
- ✅ **型の再利用**: `resource.DryRunOptions` を再利用し、コードベース全体で一貫性を確保
- ✅ **シンプルなインターフェース**: `WithDryRun` は単一の引数のみを受け取る
- ✅ **Go のベストプラクティス**: 標準ライブラリでも採用されているパターン

**Cons**:
- ⚠️ **コード量が増加**: Option 関数の定義が必要
- ⚠️ **学習コストがやや高い**: Functional Options パターンに不慣れな開発者には初見で分かりにくい可能性
- ⚠️ **実行時エラーの可能性**: コンパイル時にオプションの型チェックができない部分がある

### 案2: Config 構造体パターン

#### 概要

設定用の構造体を定義し、1つの引数として渡す。

#### 実装例

```go
// GroupExecutorConfig は DefaultGroupExecutor の設定を保持する
type GroupExecutorConfig struct {
    // 必須フィールド
    Executor            executor.CommandExecutor
    Config              *runnertypes.ConfigSpec
    Validator           security.ValidatorInterface
    VerificationManager verification.ManagerInterface
    ResourceManager     resource.ResourceManager
    RunID               string

    // オプショナルフィールド（ゼロ値がデフォルト）
    NotificationFunc    groupNotificationFunc
    IsDryRun            bool
    DryRunDetailLevel   resource.DetailLevel
    DryRunShowSensitive bool
    KeepTempDirs        bool
}

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(cfg GroupExecutorConfig) *DefaultGroupExecutor {
    return &DefaultGroupExecutor{
        executor:            cfg.Executor,
        config:              cfg.Config,
        validator:           cfg.Validator,
        verificationManager: cfg.VerificationManager,
        resourceManager:     cfg.ResourceManager,
        runID:               cfg.RunID,
        notificationFunc:    cfg.NotificationFunc,
        isDryRun:            cfg.IsDryRun,
        dryRunDetailLevel:   cfg.DryRunDetailLevel,
        dryRunShowSensitive: cfg.DryRunShowSensitive,
        keepTempDirs:        cfg.KeepTempDirs,
    }
}
```

#### 使用例

**プロダクションコード**:

```go
runner.groupExecutor = NewDefaultGroupExecutor(GroupExecutorConfig{
    Executor:            opts.executor,
    Config:              configSpec,
    Validator:           validator,
    VerificationManager: opts.verificationManager,
    ResourceManager:     opts.resourceManager,
    RunID:               opts.runID,
    NotificationFunc:    runner.logGroupExecutionSummary,
    IsDryRun:            opts.dryRun,
    DryRunDetailLevel:   detailLevel,
    DryRunShowSensitive: showSensitive,
    KeepTempDirs:        opts.keepTempDirs,
})
```

**テストコード（シンプルなケース）**:

```go
ge := NewDefaultGroupExecutor(GroupExecutorConfig{
    Config:          config,
    ResourceManager: mockRM,
    RunID:           "test-run-123",
})
```

#### Pros & Cons

**Pros**:
- ✅ **シンプルで直感的**: 構造体リテラルは Go 開発者に馴染みがある
- ✅ **フィールド名が明示的**: どの値が何に使われるか明確
- ✅ **ゼロ値がデフォルト**: 設定不要なフィールドは省略可能
- ✅ **一度に全設定を確認可能**: 構造体定義を見れば全オプションが分かる
- ✅ **IDE のサポートが良い**: フィールド補完が効く

**Cons**:
- ⚠️ **必須フィールドの強制ができない**: 必須フィールドの設定忘れをコンパイル時に検出できない
- ⚠️ **フィールド順序が自由**: 読みやすさに影響する可能性
- ⚠️ **デフォルト値のカスタマイズが困難**: ゼロ値以外のデフォルト値を設定しにくい
- ⚠️ **拡張時の影響が大きい**: 新しいフィールド追加時、既存の構造体リテラルへの影響が大きい

### 案3: Builder パターン

#### 概要

Builder パターンを使用して、段階的に設定を構築する。

#### 実装例

```go
// GroupExecutorBuilder は DefaultGroupExecutor を構築するビルダー
type GroupExecutorBuilder struct {
    executor            executor.CommandExecutor
    config              *runnertypes.ConfigSpec
    validator           security.ValidatorInterface
    verificationManager verification.ManagerInterface
    resourceManager     resource.ResourceManager
    runID               string
    notificationFunc    groupNotificationFunc
    isDryRun            bool
    dryRunDetailLevel   resource.DetailLevel
    dryRunShowSensitive bool
    keepTempDirs        bool
}

// NewGroupExecutorBuilder は新しいビルダーを作成する
func NewGroupExecutorBuilder() *GroupExecutorBuilder {
    return &GroupExecutorBuilder{
        // デフォルト値
        isDryRun:          false,
        dryRunDetailLevel: resource.DetailLevelSummary,
    }
}

// WithExecutor は executor を設定する
func (b *GroupExecutorBuilder) WithExecutor(executor executor.CommandExecutor) *GroupExecutorBuilder {
    b.executor = executor
    return b
}

// WithConfig は config を設定する
func (b *GroupExecutorBuilder) WithConfig(config *runnertypes.ConfigSpec) *GroupExecutorBuilder {
    b.config = config
    return b
}

// ... 他のフィールド用の With メソッド ...

// Build は DefaultGroupExecutor を構築する
func (b *GroupExecutorBuilder) Build() (*DefaultGroupExecutor, error) {
    // 必須フィールドのバリデーション
    if b.config == nil {
        return nil, errors.New("config is required")
    }
    if b.resourceManager == nil {
        return nil, errors.New("resourceManager is required")
    }
    if b.runID == "" {
        return nil, errors.New("runID is required")
    }

    return &DefaultGroupExecutor{
        executor:            b.executor,
        config:              b.config,
        validator:           b.validator,
        verificationManager: b.verificationManager,
        resourceManager:     b.resourceManager,
        runID:               b.runID,
        notificationFunc:    b.notificationFunc,
        isDryRun:            b.isDryRun,
        dryRunDetailLevel:   b.dryRunDetailLevel,
        dryRunShowSensitive: b.dryRunShowSensitive,
        keepTempDirs:        b.keepTempDirs,
    }, nil
}
```

#### 使用例

**プロダクションコード**:

```go
ge, err := NewGroupExecutorBuilder().
    WithExecutor(opts.executor).
    WithConfig(configSpec).
    WithValidator(validator).
    WithVerificationManager(opts.verificationManager).
    WithResourceManager(opts.resourceManager).
    WithRunID(opts.runID).
    WithNotificationFunc(runner.logGroupExecutionSummary).
    WithDryRun(opts.dryRun, detailLevel, showSensitive).
    WithKeepTempDirs(opts.keepTempDirs).
    Build()
if err != nil {
    return nil, err
}
runner.groupExecutor = ge
```

**テストコード**:

```go
ge, err := NewGroupExecutorBuilder().
    WithConfig(config).
    WithResourceManager(mockRM).
    WithRunID("test-run-123").
    Build()
require.NoError(t, err)
```

#### Pros & Cons

**Pros**:
- ✅ **段階的な構築**: 設定を段階的に構築できる
- ✅ **必須フィールドの検証**: `Build()` メソッドで必須フィールドをチェック可能
- ✅ **メソッドチェーン**: 流れるような記述が可能
- ✅ **エラーハンドリング**: 構築時のエラーを明示的に処理できる

**Cons**:
- ❌ **コード量が最も多い**: Builder クラスとすべての With メソッドの定義が必要
- ❌ **複雑性が高い**: 他の案と比べて実装が複雑
- ❌ **エラーハンドリングの追加**: `Build()` の戻り値としてエラーを扱う必要がある
- ❌ **このユースケースには過剰**: 設定の複雑さがそこまで高くない

### 案4: ハイブリッドアプローチ（必須引数 + DryRunOptions 構造体）

#### 概要

必須の引数は位置引数として残し、dry-run 関連のオプションを既存の `resource.DryRunOptions` 構造体として受け取る。

#### 実装例

```go
// GroupExecutorOptions は GroupExecutor のオプション設定
type GroupExecutorOptions struct {
    NotificationFunc groupNotificationFunc
    DryRunOptions    *resource.DryRunOptions // nil の場合は dry-run 無効
    KeepTempDirs     bool
}

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    opts *GroupExecutorOptions,
) *DefaultGroupExecutor {
    // デフォルト値の適用
    if opts == nil {
        opts = &GroupExecutorOptions{}
    }

    isDryRun := opts.DryRunOptions != nil
    var detailLevel resource.DetailLevel
    var showSensitive bool
    if isDryRun {
        detailLevel = opts.DryRunOptions.DetailLevel
        showSensitive = opts.DryRunOptions.ShowSensitive
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
        notificationFunc:    opts.NotificationFunc,
        isDryRun:            isDryRun,
        dryRunDetailLevel:   detailLevel,
        dryRunShowSensitive: showSensitive,
        keepTempDirs:        opts.KeepTempDirs,
    }
}
```

#### 使用例

**プロダクションコード**:

```go
runner.groupExecutor = NewDefaultGroupExecutor(
    opts.executor,
    configSpec,
    validator,
    opts.verificationManager,
    opts.resourceManager,
    opts.runID,
    &GroupExecutorOptions{
        NotificationFunc: runner.logGroupExecutionSummary,
        DryRunOptions:    opts.dryRunOptions, // nil または設定済み
        KeepTempDirs:     opts.keepTempDirs,
    },
)
```

**テストコード（シンプルなケース）**:

```go
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    nil, // すべてデフォルト値
)
```

**テストコード（dry-run モードをテスト）**:

```go
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    &GroupExecutorOptions{
        DryRunOptions: &resource.DryRunOptions{
            DetailLevel:   resource.DetailLevelFull,
            ShowSensitive: true,
        },
    },
)
```

#### Pros & Cons

**Pros**:
- ✅ **既存の型を再利用**: `resource.DryRunOptions` を活用
- ✅ **必須引数が明確**: 位置引数で必須パラメータを強制
- ✅ **シンプル**: Functional Options より実装が単純
- ✅ **nil チェックで簡単にデフォルト適用**: `opts == nil` でデフォルト動作

**Cons**:
- ⚠️ **まだ引数が多い**: 7個の引数（11個から4個減少）
- ⚠️ **構造体の順序問題**: `GroupExecutorOptions` のフィールド順序が任意
- ⚠️ **拡張性がやや低い**: 新しいオプション追加時に構造体を拡張する必要

## 3. 推奨案

### 推奨: **案1 - Options パターン（Functional Options）**

#### 選定理由

1. **可読性と明示性のバランスが最良**
   - オプション名が明示的で、何を設定しているか一目で分かる
   - 必須引数は位置引数として残り、オプション引数は名前付きで指定

2. **拡張性が高い**
   - 新しいオプションを追加しても既存コードへの影響が最小限
   - 後方互換性を保ちやすい

3. **Go のイディオム**
   - 標準ライブラリや有名な OSS プロジェクトで採用されている
   - Go コミュニティで広く受け入れられているパターン

4. **テストコードの簡潔化**
   - デフォルト値が効果的に機能し、テストが簡潔になる
   - 必要な設定だけを指定できる

5. **型の再利用による一貫性**
   - `resource.DryRunOptions` を再利用し、コードベース全体で統一された表現を使用
   - dry-run 関連の設定を単一の構造体として扱える
   - プロダクションコードでは既に使用されている型なので、追加の学習コストがない

#### 実装の優先順位

**Phase 1: 基本実装**
1. `groupExecutorOptions` 型の定義
2. デフォルト値を返す `defaultGroupExecutorOptions()` 関数
3. 各オプション関数（`WithNotificationFunc`, `WithDryRun`, `WithKeepTempDirs`）
4. `NewDefaultGroupExecutor` のシグネチャ変更

**Phase 2: 段階的な移行**
1. プロダクションコード (`runner.go`) の更新
2. テストコードの更新（ファイルごとに段階的に）
   - まず `group_executor_test.go` を更新
   - 他のテストファイルを順次更新

**Phase 3: クリーンアップ**
1. 旧実装の削除（移行が完了した後）
2. ドキュメントの更新

#### 代替案の検討

**案4（ハイブリッドアプローチ）を次点として検討**

もし以下の条件に当てはまる場合、案4も有力な選択肢：
- Functional Options パターンへの抵抗がある
- より単純な実装を好む
- `resource.DryRunOptions` の再利用を重視する

ただし、拡張性と可読性の観点から、案1（Functional Options）を推奨。

## 4. 移行計画

### 4.1 段階的移行ステップ

#### Step 1: 新しいインターフェースの追加（破壊的変更なし）

1. 新しい Option 型と関数を追加
2. 既存の `NewDefaultGroupExecutor` を `NewDefaultGroupExecutorLegacy` にリネーム
3. 新しい `NewDefaultGroupExecutor` を実装（内部で `NewDefaultGroupExecutorLegacy` を呼び出す）

```go
// 新しい実装
func NewDefaultGroupExecutor(
    executor executor.CommandExecutor,
    config *runnertypes.ConfigSpec,
    validator security.ValidatorInterface,
    verificationManager verification.ManagerInterface,
    resourceManager resource.ResourceManager,
    runID string,
    options ...GroupExecutorOption,
) *DefaultGroupExecutor {
    opts := defaultGroupExecutorOptions()
    for _, opt := range options {
        opt(opts)
    }

    // 既存の実装を呼び出す（移行期間中）
    return NewDefaultGroupExecutorLegacy(
        executor,
        config,
        validator,
        verificationManager,
        resourceManager,
        runID,
        opts.notificationFunc,
        opts.isDryRun,
        opts.dryRunDetailLevel,
        opts.dryRunShowSensitive,
        opts.keepTempDirs,
    )
}
```

#### Step 2: プロダクションコードの移行

`internal/runner/runner.go` の呼び出しを新しいインターフェースに変更。

#### Step 3: テストコードの移行

各テストファイルを順次更新：
1. `group_executor_test.go`（22箇所）
2. その他のテストファイル（もしあれば）

#### Step 4: レガシーコードの削除

すべての移行が完了したら、`NewDefaultGroupExecutorLegacy` を削除。

### 4.2 リスク管理

#### 低リスク
- 段階的移行により、各ステップで動作確認が可能
- 既存のテストが動作し続ける

#### 潜在的なリスクと対策

| リスク | 対策 |
|--------|------|
| デフォルト値の不一致 | テストで現在の動作と新しい実装の動作を比較 |
| オプション指定の誤り | コンパイル時に型チェック、実行時テストで確認 |
| 移行漏れ | grep やツールで旧実装の使用箇所を検索 |

### 4.3 テスト戦略

1. **ユニットテスト**: 各 Option 関数の動作を検証
2. **統合テスト**: 新しいインターフェースでの GroupExecutor の動作を検証
3. **リグレッションテスト**: 既存のテストがすべて成功することを確認

## 5. 期待される効果

### 5.1 定量的効果

- **引数の数**: 11個 → 6個（必須）+ 可変長（オプション）
- **テストコードの行数削減**: 推定30-40%削減（多くのテストでオプション指定が不要に）
- **プロダクションコードの可読性**: オプション名による明示化

### 5.2 定性的効果

- **メンテナンス性の向上**: 新しいオプションの追加が容易
- **可読性の向上**: 各設定の意図が明確
- **テスタビリティの向上**: テストに必要な設定だけを指定可能
- **拡張性の向上**: 後方互換性を保ちながら機能追加が可能

## 6. テスト用ヘルパー関数

### 6.1 必要性

Options パターンを採用した場合でも、テストコードでは多くの場合、同じ引数パターンが繰り返されます：

```go
// 頻出パターン
ge := NewDefaultGroupExecutor(
    nil,           // executor
    config,
    nil,           // validator
    nil,           // verificationManager
    mockRM,
    "test-run-123",
    // オプションはテストケースにより異なる
)
```

この繰り返しを削減し、テストの可読性を向上させるため、テスト用ヘルパー関数を提供します。

### 6.2 実装案

#### 6.2.1 基本ヘルパー関数

```go
// Package testing provides test utilities for group executor
package testing

import (
    "github.com/yourusername/yourproject/internal/runner/executor/group"
    "github.com/yourusername/yourproject/internal/runner/resource"
    "github.com/yourusername/yourproject/internal/runner/runnertypes"
)

// NewTestGroupExecutor creates a DefaultGroupExecutor with common test defaults.
// This is a convenience function for tests that don't need custom executors,
// validators, or verification managers.
//
// Default values:
//   - executor: nil
//   - validator: nil
//   - verificationManager: nil
//   - runID: "test-run-123"
//
// Example:
//
//	ge := testing.NewTestGroupExecutor(config, mockRM)
func NewTestGroupExecutor(
    config *runnertypes.ConfigSpec,
    resourceManager resource.ResourceManager,
    options ...group.GroupExecutorOption,
) *group.DefaultGroupExecutor {
    return group.NewDefaultGroupExecutor(
        nil,    // executor
        config,
        nil,    // validator
        nil,    // verificationManager
        resourceManager,
        "test-run-123", // standard test runID
        options...,
    )
}
```

#### 6.2.2 カスタマイズ可能なヘルパー関数

特定の引数をカスタマイズしたい場合のためのヘルパー：

```go
// TestGroupExecutorConfig holds configuration for test group executor creation
type TestGroupExecutorConfig struct {
    Executor            executor.CommandExecutor
    Config              *runnertypes.ConfigSpec
    Validator           security.ValidatorInterface
    VerificationManager verification.ManagerInterface
    ResourceManager     resource.ResourceManager
    RunID               string
}

// NewTestGroupExecutorWithConfig creates a DefaultGroupExecutor with custom configuration.
// Use this when you need to customize specific dependencies that NewTestGroupExecutor doesn't expose.
//
// Unset fields will use test-appropriate defaults:
//   - Executor: nil
//   - Validator: nil
//   - VerificationManager: nil
//   - RunID: "test-run-123"
//
// Example:
//
//	ge := testing.NewTestGroupExecutorWithConfig(testing.TestGroupExecutorConfig{
//	    Config:          config,
//	    ResourceManager: mockRM,
//	    Executor:        mockExecutor, // custom executor
//	}, options...)
func NewTestGroupExecutorWithConfig(
    cfg TestGroupExecutorConfig,
    options ...group.GroupExecutorOption,
) *group.DefaultGroupExecutor {
    // Apply defaults for unset fields
    if cfg.RunID == "" {
        cfg.RunID = "test-run-123"
    }

    return group.NewDefaultGroupExecutor(
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

### 6.3 使用例

#### 6.3.1 シンプルなテストケース

```go
// Before: 冗長な引数指定
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
)

// After: 簡潔なヘルパー使用
ge := testing.NewTestGroupExecutor(config, mockRM)
```

#### 6.3.2 オプションを指定するテストケース

```go
// Before: 冗長な引数指定 + オプション
ge := NewDefaultGroupExecutor(
    nil,
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
    WithNotificationFunc(notificationFunc),
    WithDryRun(&resource.DryRunOptions{
        DetailLevel:   resource.DetailLevelFull,
        ShowSensitive: true,
    }),
)

// After: 簡潔なヘルパー使用 + オプション
ge := testing.NewTestGroupExecutor(
    config,
    mockRM,
    WithNotificationFunc(notificationFunc),
    WithDryRun(&resource.DryRunOptions{
        DetailLevel:   resource.DetailLevelFull,
        ShowSensitive: true,
    }),
)
```

#### 6.3.3 カスタムexecutorを使うテストケース

```go
// Before: すべての引数を指定
ge := NewDefaultGroupExecutor(
    mockExecutor, // カスタム
    config,
    nil,
    nil,
    mockRM,
    "test-run-123",
)

// After: カスタマイズ可能なヘルパー使用
ge := testing.NewTestGroupExecutorWithConfig(
    testing.TestGroupExecutorConfig{
        Config:          config,
        ResourceManager: mockRM,
        Executor:        mockExecutor,
    },
)
```

### 6.4 ヘルパー関数の配置

テスト用ヘルパー関数は、以下のディレクトリ構造で提供します：

```
internal/runner/executor/group/
├── group_executor.go              # 本体実装
├── group_executor_test.go         # テスト
├── options.go                     # Option 関数の定義
└── testing/
    ├── helpers.go                 # テスト用ヘルパー関数
    └── helpers_test.go            # ヘルパー関数のテスト
```

**パッケージ名**: `testing` (import path: `.../group/testing`)

### 6.5 利点

1. **テストコードの簡潔化**: 繰り返しパターンの削減
2. **可読性の向上**: 重要な部分（config, resourceManager, options）に焦点
3. **保守性の向上**: デフォルト値の変更が1箇所で済む
4. **学習曲線の緩和**: 新しい開発者がテストを書きやすくなる
5. **一貫性の確保**: テスト間で共通のデフォルト値を使用

### 6.6 ヘルパー関数の設計原則

1. **テスト専用**: プロダクションコードでは使用しない
2. **明確な命名**: `NewTest*` プレフィックスでテスト用であることを明示
3. **適切なデフォルト値**: テストで最も頻繁に使用される値を選択
4. **柔軟性の維持**: オプション引数でカスタマイズ可能
5. **段階的な提供**: シンプルなヘルパーと詳細なヘルパーの両方を提供

## 7. 結論

`NewDefaultGroupExecutor` のリファクタリングは、コードベースの保守性と可読性を大幅に向上させる重要な改善です。

**推奨**: **案1（Functional Options パターン）**を採用し、**テスト用ヘルパー関数**を追加し、段階的な移行を実施する。

この提案により、以下を実現できます：
- 引数の数を削減（11個 → 実質6個 + オプション）
- デフォルト値の効果的な活用
- テストコードの簡潔化（ヘルパー関数により更に簡潔に）
- 将来の拡張性の確保

次のステップとして、この分析に基づき以下のドキュメントを作成することを推奨します：
1. 詳細な設計仕様書（各 Option 関数のシグネチャと動作、テスト用ヘルパー関数の仕様）
2. 実装計画書（具体的な実装手順とスケジュール）
3. テスト計画書（移行に伴うテストケースの更新計画）
