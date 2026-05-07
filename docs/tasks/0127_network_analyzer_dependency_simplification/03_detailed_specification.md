# 詳細仕様書: NetworkAnalyzer 周辺の依存配線簡素化

## 1. 変更対象ファイル

| ファイル | 変更種別 |
|---------|---------|
| `internal/runner/base/security/network_analyzer.go` | 型追加・シグネチャ変更 |
| `internal/runner/base/security/network_analyzer_test_helpers.go` | テストヘルパー更新 |
| `internal/runner/base/security/network_analyzer_test.go` | コンストラクタ呼び出し更新 |
| `internal/runner/base/security/command_analysis_test.go` | コンストラクタ呼び出し更新 |
| `internal/runner/base/risk/evaluator.go` | シグネチャ変更 |
| `internal/runner/base/risk/evaluator_test.go` | コンストラクタ呼び出し更新 |
| `internal/runner/resource/default_manager.go` | Config フィールド変更 |
| `internal/runner/resource/normal_manager.go` | newNormalManager 変更 |
| `internal/runner/resource/test_helpers.go` | テストヘルパー更新 |
| `internal/runner/resource/testutil/helpers.go` | テストヘルパー更新 |
| `internal/runner/runner.go` | createNormalResourceManager 簡素化 |
| `internal/runner/runner_test.go` | TestCreateNormalResourceManager_* 更新 |
| `internal/verification/manager.go` | GetAnalysisDeps 追加・旧 getter 削除 |

---

## 2. Phase 1: security.AnalysisDeps 導入と NetworkAnalyzer 更新

### 2.1 AnalysisDeps 型定義

`internal/runner/base/security/network_analyzer.go` に追加する。

```go
// AnalysisDeps aggregates the analysis stores consumed by NetworkAnalyzer.
// A nil field disables the corresponding analysis, preserving the existing
// "feature disabled" behavior.
type AnalysisDeps struct {
    NetworkSymbolStore fileanalysis.NetworkSymbolStore
    SyscallStore       fileanalysis.SyscallAnalysisStore
    DynLibDepsStore    fileanalysis.DynLibDepsStore
    LibAnalysisStore   dynamicanalysis.Store
    ShebangStore       fileanalysis.ShebangInterpreterStore
}
```

### 2.2 NetworkAnalyzer 構造体変更

変更前:
```go
type NetworkAnalyzer struct {
    goos             string
    store            fileanalysis.NetworkSymbolStore
    syscallStore     fileanalysis.SyscallAnalysisStore
    depsStore        fileanalysis.DynLibDepsStore
    libAnalysisStore dynamicanalysis.Store
    shebangStore     fileanalysis.ShebangInterpreterStore
}
```

変更後:
```go
type NetworkAnalyzer struct {
    goos string
    deps AnalysisDeps
}
```

### 2.3 NewNetworkAnalyzer シグネチャ変更

変更前:
```go
func NewNetworkAnalyzer(
    goos string,
    symStore fileanalysis.NetworkSymbolStore,
    svcStore fileanalysis.SyscallAnalysisStore,
    depsStore fileanalysis.DynLibDepsStore,
    libAnalysisStore dynamicanalysis.Store,
    shebangStore fileanalysis.ShebangInterpreterStore,
) *NetworkAnalyzer
```

変更後:
```go
func NewNetworkAnalyzer(goos string, deps AnalysisDeps) *NetworkAnalyzer
```

### 2.4 内部フィールド参照の置き換え

`NetworkAnalyzer` のメソッド内での参照を以下の通り一括置換する。

| 変更前 | 変更後 |
|--------|--------|
| `a.store` | `a.deps.NetworkSymbolStore` |
| `a.syscallStore` | `a.deps.SyscallStore` |
| `a.depsStore` | `a.deps.DynLibDepsStore` |
| `a.libAnalysisStore` | `a.deps.LibAnalysisStore` |
| `a.shebangStore` | `a.deps.ShebangStore` |

### 2.5 network_analyzer_test_helpers.go の更新

`newNetworkAnalyzerWithStore` は `AnalysisDeps` を使う形に変更する。
`newNetworkAnalyzer` は構造体リテラルから `deps` フィールドが消えるのみ（変更不要）。

```go
func newNetworkAnalyzerWithStore(goos string, store fileanalysis.NetworkSymbolStore) *NetworkAnalyzer {
    return &NetworkAnalyzer{goos: isec.RequireGOOS(goos), deps: AnalysisDeps{NetworkSymbolStore: store}}
}
```

### 2.6 テスト変更: network_analyzer_test.go と command_analysis_test.go

`NewNetworkAnalyzer` を呼び出している箇所をすべて新シグネチャに変換する。
変換パターンは以下の通り。

| 変換前 | 変換後 |
|--------|--------|
| `NewNetworkAnalyzer(runtime.GOOS, nil, nil, nil, nil, nil)` | `NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{})` |
| `NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, nil, nil, nil)` | `NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{NetworkSymbolStore: symStore, SyscallStore: svcStore})` |
| `NewNetworkAnalyzer(runtime.GOOS, nil, nil, depsStore, libStore, nil)` | `NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{DynLibDepsStore: depsStore, LibAnalysisStore: libStore})` |
| `NewNetworkAnalyzer(runtime.GOOS, symStore, svcStore, depsStore, libStore, shebangStore)` | `NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{NetworkSymbolStore: symStore, SyscallStore: svcStore, DynLibDepsStore: depsStore, LibAnalysisStore: libStore, ShebangStore: shebangStore})` |

`command_analysis_test.go` は package `security` 内のため、`security.` プレフィックスは不要。
`network_analyzer_test.go` も同様。

---

## 3. Phase 2: risk.NewStandardEvaluator と resource.Config の変更

### 3.1 risk.NewStandardEvaluator シグネチャ変更

変更前:
```go
func NewStandardEvaluator(
    symStore fileanalysis.NetworkSymbolStore,
    syscallStore fileanalysis.SyscallAnalysisStore,
    depsStore fileanalysis.DynLibDepsStore,
    libAnalysisStore dynamicanalysis.Store,
    shebangStore fileanalysis.ShebangInterpreterStore,
) Evaluator
```

変更後:
```go
func NewStandardEvaluator(networkAnalyzer *security.NetworkAnalyzer) Evaluator
```

`NewStandardEvaluator` の内部実装:

```go
func NewStandardEvaluator(networkAnalyzer *security.NetworkAnalyzer) Evaluator {
    return &StandardEvaluator{networkAnalyzer: networkAnalyzer}
}
```

`runtime` および分析ストアの import は不要になる。`security` import は既存のまま維持する。

### 3.2 risk/evaluator_test.go の更新

変更前:
```go
evaluator := NewStandardEvaluator(nil, nil, nil, nil, nil)
```

変更後:
```go
evaluator := NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
```

テストファイルに `"runtime"` と `security` の import を追加する。

### 3.3 resource.Config の変更

変更前（`internal/runner/resource/default_manager.go`）:
```go
type Config struct {
    // ...
    NetworkSymbolStore fileanalysis.NetworkSymbolStore
    SyscallStore       fileanalysis.SyscallAnalysisStore
    DynLibDepsStore    fileanalysis.DynLibDepsStore
    LibAnalysisStore   dynamicanalysis.Store
    ShebangStore       fileanalysis.ShebangInterpreterStore
}
```

変更後:
```go
type Config struct {
    Executor         executor.CommandExecutor
    FileSystem       executor.FileSystem
    PrivilegeManager runnertypes.PrivilegeManager
    PathResolver     PathResolver
    Logger           *slog.Logger
    Mode             ExecutionMode
    DryRunOpts       *DryRunOptions
    OutputManager    output.CaptureManager
    MaxOutputSize    int64
    RiskEvaluator    risk.Evaluator
}
```

`default_manager.go` から不要になる import (`fileanalysis`, `dynamicanalysis`) を削除する。

### 3.4 resource/normal_manager.go の変更

`newNormalManager` の `riskEvaluator` 組み立て行を `cfg.RiskEvaluator` の直接代入に変更する。

変更前:
```go
riskEvaluator: risk.NewStandardEvaluator(cfg.NetworkSymbolStore, cfg.SyscallStore, cfg.DynLibDepsStore, cfg.LibAnalysisStore, cfg.ShebangStore),
```

変更後:
```go
riskEvaluator: cfg.RiskEvaluator,
```

`cfg.RiskEvaluator` が nil の場合、`EvaluateRisk` 呼び出し時に nil ポインタパニックとなる。
これは呼び出し側（composition root またはテストヘルパー）の責任で非 nil を保証すること。

### 3.5 runner.go の中間状態（Phase 2）

`resource.Config` からストアフィールドが削除されるため、`runner.go` を同時に更新してコンパイルを通す。
この時点では 5 系統の型アサーションは残るが、取得したストアで `AnalysisDeps` と evaluator を組み立てて
`resource.Config.RiskEvaluator` に渡す中間形態とする。

```go
// Intermediate state after Phase 2 (5 type assertions remain, evaluator assembled here)
deps := security.AnalysisDeps{
    NetworkSymbolStore: networkStore,
    SyscallStore:       syscallStore,
    DynLibDepsStore:    dynLibDepsStore,
    LibAnalysisStore:   dynlibAnalysisStore,
    ShebangStore:       shebangStore,
}
networkAnalyzer := security.NewNetworkAnalyzer(runtime.GOOS, deps)
evaluator := risk.NewStandardEvaluator(networkAnalyzer)

resourceManager, err := resource.NewDefaultResourceManager(resource.Config{
    // ... other fields ...
    RiskEvaluator: evaluator,
})
```

追加 import: `"runtime"`, `risk`。
`fileanalysis`, `dynamicanalysis` の import は Phase 3 まで残す。

### 3.6 resource/test_helpers.go の変更

`NetworkSymbolStore` 引数を削除し、テスト専用の evaluator を内部で生成する。

変更前:
```go
func NewNormalResourceManagerWithOutput(
    exec executor.CommandExecutor,
    fs executor.FileSystem,
    privMgr runnertypes.PrivilegeManager,
    outputMgr output.CaptureManager,
    maxOutputSize int64,
    logger *slog.Logger,
    store fileanalysis.NetworkSymbolStore,
) *NormalResourceManager {
    return newNormalManager(Config{
        Executor:           exec,
        FileSystem:         fs,
        PrivilegeManager:   privMgr,
        MaxOutputSize:      maxOutputSize,
        Logger:             logger,
        NetworkSymbolStore: store,
    }, outputMgr)
}
```

変更後:
```go
func NewNormalResourceManagerWithOutput(
    exec executor.CommandExecutor,
    fs executor.FileSystem,
    privMgr runnertypes.PrivilegeManager,
    outputMgr output.CaptureManager,
    maxOutputSize int64,
    logger *slog.Logger,
) *NormalResourceManager {
    return newNormalManager(Config{
        Executor:         exec,
        FileSystem:       fs,
        PrivilegeManager: privMgr,
        MaxOutputSize:    maxOutputSize,
        Logger:           logger,
        RiskEvaluator:    defaultTestEvaluator(),
    }, outputMgr)
}
```

`NewNormalResourceManager` も同様に更新する。

`NewDefaultResourceManagerForTest` は `symStore fileanalysis.NetworkSymbolStore` パラメータを削除し、
`Config.RiskEvaluator` に `defaultTestEvaluator()` を設定する。

`defaultTestEvaluator` はテスト専用のヘルパー関数として同ファイルに定義する:

```go
func defaultTestEvaluator() risk.Evaluator {
    return risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
}
```

`test_helpers.go` に `runtime`, `risk`, `security` の import を追加する。
`fileanalysis` の import は削除する。

### 3.7 resource/testutil/helpers.go の変更

`symStore fileanalysis.NetworkSymbolStore` パラメータを削除し、
`resource.NewDefaultResourceManager` に渡す `Config.RiskEvaluator` に evaluator を設定する。

`testutil` パッケージは `resource` の公開 API のみ呼べるため、
`resource/test_helpers.go` の `defaultTestEvaluator()` は参照できない。
`testutil/helpers.go` 内で直接 evaluator を生成する。

```go
func NewDefaultResourceManager(
    exec executor.CommandExecutor,
    fs executor.FileSystem,
    privMgr runnertypes.PrivilegeManager,
    pathResolver resource.PathResolver,
    logger *slog.Logger,
    mode resource.ExecutionMode,
    dryRunOpts *resource.DryRunOptions,
    outputMgr output.CaptureManager,
    maxOutputSize int64,
) (*resource.DefaultResourceManager, error) {
    return resource.NewDefaultResourceManager(resource.Config{
        Executor:         exec,
        FileSystem:       fs,
        PrivilegeManager: privMgr,
        PathResolver:     pathResolver,
        Logger:           logger,
        Mode:             mode,
        DryRunOpts:       dryRunOpts,
        OutputManager:    outputMgr,
        MaxOutputSize:    maxOutputSize,
        RiskEvaluator:    risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{})),
    })
}
```

`testutil/helpers.go` に `"runtime"`、`risk`、`security` の import を追加する。
不要になる `fileanalysis` の import を削除する。

### 3.8 テスト呼び出しの更新

`NewNormalResourceManagerWithOutput(exec, fs, priv, outputMgr, maxSize, logger, nil)` → 末尾の `nil` 引数を削除。
`NewDefaultResourceManagerForTest(..., nil, 0, nil)` → 末尾の `nil` (symStore) を削除して `..., nil, 0`。

変更対象ファイル:
- `internal/runner/resource/normal_manager_test.go`
- `internal/runner/resource/error_scenarios_test.go`
- `internal/runner/resource/default_manager_test.go`
- `internal/runner/resource/performance_test.go`
- `internal/runner/e2e_slack_redaction_test.go`
- `internal/runner/integration_dual_defense_test.go`
- `internal/runner/command_output_capture_test.go`

---

## 4. Phase 3: verification.Manager の API 追加と runner 簡素化

### 4.1 verification.Manager.GetAnalysisDeps() の追加

`internal/verification/manager.go` に追加する。

```go
// GetAnalysisDeps returns the aggregated analysis dependencies owned by this Manager.
// Nil fields in the returned AnalysisDeps indicate that the corresponding analysis
// is unavailable (e.g. hash dir absent or store initialization failed).
func (m *Manager) GetAnalysisDeps() security.AnalysisDeps {
    return security.AnalysisDeps{
        NetworkSymbolStore: m.networkSymbolStore,
        SyscallStore:       m.syscallAnalysisStore,
        DynLibDepsStore:    m.dynLibDepsStore,
        LibAnalysisStore:   m.dynlibAnalysisStore,
        ShebangStore:       m.shebangStore,
    }
}
```

`verification` パッケージに `security` の import が追加される。
現時点で `security` → `verification` の依存はないため、循環 import は生じない。

### 4.2 旧 getter の削除

以下のメソッドは `runner.go` からの参照がなくなるため削除する。

- `GetNetworkSymbolStore() fileanalysis.NetworkSymbolStore`
- `GetSyscallAnalysisStore() fileanalysis.SyscallAnalysisStore`
- `GetDynLibAnalysisStore() dynamicanalysis.Store`
- `GetDynLibDepsStore() fileanalysis.DynLibDepsStore`
- `GetShebangInterpreterStore() fileanalysis.ShebangInterpreterStore`

`fileanalysis` および `dynamicanalysis` の import は `Manager` 構造体フィールドや他のメソッドで
引き続き使用されるため、旧 getter 削除後も残す。

### 4.3 runner.createNormalResourceManager の簡素化

Phase 3 完了後の `createNormalResourceManager`:

```go
func createNormalResourceManager(opts *runnerOptions, _ *runnertypes.ConfigSpec, pathResolver resource.PathResolver, validator *security.Validator) error {
    fs := common.NewDefaultFileSystem()
    maxOutputSize := int64(0)

    outputMgr := output.NewDefaultOutputCaptureManager(validator)

    var deps security.AnalysisDeps
    type analysisDepsProvider interface {
        GetAnalysisDeps() security.AnalysisDeps
    }
    if p, ok := pathResolver.(analysisDepsProvider); ok {
        deps = p.GetAnalysisDeps()
    }

    networkAnalyzer := security.NewNetworkAnalyzer(runtime.GOOS, deps)
    evaluator := risk.NewStandardEvaluator(networkAnalyzer)

    resourceManager, err := resource.NewDefaultResourceManager(resource.Config{
        Executor:         opts.executor,
        FileSystem:       fs,
        PrivilegeManager: opts.privilegeManager,
        PathResolver:     pathResolver,
        Logger:           slog.Default(),
        Mode:             resource.ExecutionModeNormal,
        DryRunOpts:       &resource.DryRunOptions{},
        OutputManager:    outputMgr,
        MaxOutputSize:    maxOutputSize,
        RiskEvaluator:    evaluator,
    })
    if err != nil {
        return fmt.Errorf("failed to create default resource manager: %w", err)
    }
    opts.resourceManager = resourceManager
    return nil
}
```

不要になる import: `fileanalysis`, `dynamicanalysis`。
追加が必要な import: `runtime`, `risk`。

### 4.4 runner_test.go の更新

`TestCreateNormalResourceManager_AnalysisStoresInjected` は `GetAnalysisDeps()` が呼ばれることを検証する形に変更する。

変更前（`pathResolverWithStore`）:
```go
type pathResolverWithStore struct {
    networkStoreCalled *bool
    syscallStoreCalled *bool
}

func (p *pathResolverWithStore) GetNetworkSymbolStore() fileanalysis.NetworkSymbolStore { ... }
func (p *pathResolverWithStore) GetSyscallAnalysisStore() fileanalysis.SyscallAnalysisStore { ... }
```

変更後（`pathResolverWithDeps`）:
```go
type pathResolverWithDeps struct {
    called *bool
}

func (p *pathResolverWithDeps) ResolvePath(path string) (string, error) {
    return path, nil
}

func (p *pathResolverWithDeps) GetAnalysisDeps() security.AnalysisDeps {
    *p.called = true
    return security.AnalysisDeps{}
}
```

テスト本体:
```go
func TestCreateNormalResourceManager_AnalysisStoresInjected(t *testing.T) {
    called := false
    resolver := &pathResolverWithDeps{called: &called}

    opts := &runnerOptions{}
    err := createNormalResourceManager(opts, &runnertypes.ConfigSpec{}, resolver, nil)
    require.NoError(t, err)

    assert.True(t, called, "GetAnalysisDeps must be called when pathResolver implements the interface")
    assert.NotNil(t, opts.resourceManager)
}
```

`TestCreateNormalResourceManager_NoStoreWhenResolverLacksInterface` はロジック変更なし（pathResolver が `GetAnalysisDeps` を実装しない場合に空の deps が使われることを確認）。

---

## 5. 受け入れ基準とテストの対応

| AC | 基準 | 検証方法 |
|----|------|---------|
| AC-1 | `NetworkAnalyzer` と `StandardEvaluator` のコンストラクタが分析依存 5 個を個別引数で受け取らない | コンパイル確認（シグネチャ変更後は旧シグネチャが使えない） |
| AC-2 | `resource.Config` から分析ストア群の個別フィールドが除去される | コンパイル確認 |
| AC-3 | `runner.createNormalResourceManager` での分析依存配線が単一の組み立て操作に簡約される | コードレビュー（型アサーション 1 系統のみ） |
| AC-4 | 分析依存の具体生成責務は `verification` に残り、`security` は具体ストア構築を行わない | コードレビュー（`security` パッケージに hash/store dir 構築コードがない） |
| AC-5 | 既存のネットワーク判定系テストと runner 初期化系テストが通過する | `make test` |
| AC-6 | `make test`、`make lint`、`make build` が成功する | CI |

既存テストの補完として以下を確認する。

- `TestCreateNormalResourceManager_AnalysisStoresInjected`: `GetAnalysisDeps()` が呼ばれる
- `verification.Manager.GetAnalysisDeps()` が 5 フィールドすべてを返す（新規テスト）
- 既存の `network_analyzer_test.go` のすべてのシナリオが新シグネチャで通過する

### 新規テスト: TestManager_GetAnalysisDeps

`internal/verification/manager_test.go`（または既存の verification テストファイル）に追加。

```go
func TestManager_GetAnalysisDeps(t *testing.T) {
    m := &Manager{}
    deps := m.GetAnalysisDeps()
    // All fields nil when manager has no stores initialized
    assert.Nil(t, deps.NetworkSymbolStore)
    assert.Nil(t, deps.SyscallStore)
    assert.Nil(t, deps.DynLibDepsStore)
    assert.Nil(t, deps.LibAnalysisStore)
    assert.Nil(t, deps.ShebangStore)
}
```

---

## 6. モック境界の定義（F-127-5 AC-3）

本リファクタリング後の推奨モック境界は以下の通り。

### 単体テスト（security パッケージ）

- `NetworkAnalyzer` の単体テストは `AnalysisDeps` のフィールドに stub store を注入する
- `AnalysisDeps` 全体をゼロ値にすれば、すべての store が無効化された状態を手軽に再現できる
- `NewNetworkAnalyzer(runtime.GOOS, AnalysisDeps{...})` を直接呼ぶ

### 単体テスト（risk パッケージ）

- `risk.Evaluator` インターフェース全体を mock にして `NormalResourceManager` を差し替えることができる
- `NewStandardEvaluator` 自体のテストは `*security.NetworkAnalyzer` を注入する

### 統合テスト（runner パッケージ）

- `pathResolverWithDeps` など最小の struct で `GetAnalysisDeps()` を実装すれば十分
- `WithVerificationManager` Option で本物の `verification.Manager` を差し込む統合シナリオも引き続き利用可能

### テスト用ヘルパーの責任範囲

- `resource/test_helpers.go`: ストア詳細を不要とし、`defaultTestEvaluator()` で標準 evaluator を内包する
- `resource/testutil/helpers.go`: 公開 API 経由で `Config.RiskEvaluator` にデフォルト evaluator を設定する
- テストが特定のリスク判定挙動を検証したい場合は、`risk.Evaluator` を実装した mock を `resource.Config.RiskEvaluator` に直接渡す

---

## 7. import 依存の変化

| 変化 | 方向 | 理由 |
|------|------|------|
| `verification` → `security` | 追加 | `GetAnalysisDeps()` の戻り値型が `security.AnalysisDeps` |
| `runner` → `risk` | 追加 | `risk.NewStandardEvaluator` を呼ぶ |
| `runner` → `fileanalysis`, `dynamicanalysis` | 削除 | 個別型アサーションが不要になる |
| `resource` → `fileanalysis`, `dynamicanalysis` | 削除 | Config フィールドから削除 |
| `risk` → `fileanalysis`, `dynamicanalysis` | 削除 | コンストラクタ引数から削除 |

循環 import の懸念:
- `verification` → `security`: `security` が `verification` を import していないため問題なし
- `runner` → `risk`, `security`: すでに両方 import 済み
