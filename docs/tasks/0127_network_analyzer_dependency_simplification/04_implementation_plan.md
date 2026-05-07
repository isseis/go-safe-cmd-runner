# 実装計画書: NetworkAnalyzer 周辺の依存配線簡素化

## 進捗サマリー

- Phase 1: security.AnalysisDeps 導入
- Phase 2: risk / resource レイヤーの整理
- Phase 3: composition root の簡素化

---

## Phase 1: security.AnalysisDeps 導入と NetworkAnalyzer 更新

### 1-1. AnalysisDeps 型と NewNetworkAnalyzer の変更

- [x] `internal/runner/base/security/network_analyzer.go`
  - [x] `AnalysisDeps` 構造体を追加する
  - [x] `NetworkAnalyzer` 構造体のフィールドを `goos string` + `deps AnalysisDeps` に変更する
  - [x] `NewNetworkAnalyzer(goos string, deps AnalysisDeps)` にシグネチャを変更する
  - [x] `analyzeBinarySignals` 内の `a.store` → `a.deps.NetworkSymbolStore` に置換する
  - [x] `checkAnalysisCache` 内の `a.store` → `a.deps.NetworkSymbolStore` に置換する
  - [x] `checkSyscallCache` 内の `a.syscallStore` → `a.deps.SyscallStore` に置換する
  - [x] `checkDynLibDepsNetwork` 内の `a.depsStore` → `a.deps.DynLibDepsStore`、`a.libAnalysisStore` → `a.deps.LibAnalysisStore` に置換する
  - [x] `followShebangChain` 内の `a.shebangStore` → `a.deps.ShebangStore` に置換する
  - [x] 条件式（`a.store != nil` 等）を新フィールド参照に更新する

### 1-2. テストヘルパーの更新

- [x] `internal/runner/base/security/network_analyzer_test_helpers.go`
  - [x] `newNetworkAnalyzerWithStore` を `AnalysisDeps{NetworkSymbolStore: store}` を使う形に変更する

### 1-3. security パッケージのテスト更新

- [x] `internal/runner/base/security/network_analyzer_test.go`
  - [x] `NewNetworkAnalyzer` を呼ぶ 24 箇所を新シグネチャ（`AnalysisDeps{...}`）に変更する
- [x] `internal/runner/base/security/command_analysis_test.go`
  - [x] `NewNetworkAnalyzer` を呼ぶ 1 箇所（line 2516 付近）を新シグネチャに変更する

### 1-4. ビルド・テスト確認

- [x] `make build` が成功することを確認する
- [x] `make test` が成功することを確認する
- [x] `make lint` が成功することを確認する

---

## Phase 2: risk / resource レイヤーの整理

### 2-1. risk.NewStandardEvaluator のシグネチャ変更

- [x] `internal/runner/base/risk/evaluator.go`
  - [x] `NewStandardEvaluator` の引数を `networkAnalyzer *security.NetworkAnalyzer` 1 つに変更する
  - [x] コンストラクタ本体で `security.NewNetworkAnalyzer(runtime.GOOS, ...)` の呼び出しを削除する
  - [x] 不要になる import（`fileanalysis`、`dynamicanalysis`、`runtime`）を削除する

### 2-2. risk パッケージのテスト更新

- [x] `internal/runner/base/risk/evaluator_test.go`
  - [x] `NewStandardEvaluator(nil, nil, nil, nil, nil)` を `NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))` に変更する（2 箇所）
  - [x] `"runtime"` と `security` の import を追加する

### 2-3. resource.Config の変更

- [x] `internal/runner/resource/default_manager.go`
  - [x] `Config` から `NetworkSymbolStore`、`SyscallStore`、`DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` フィールドを削除する
  - [x] `Config` に `RiskEvaluator risk.Evaluator` フィールドを追加する
  - [x] 不要になる import（`fileanalysis`、`dynamicanalysis`）を削除する

### 2-4. resource/normal_manager.go の変更

- [x] `internal/runner/resource/normal_manager.go`
  - [x] `newNormalManager` の `riskEvaluator` 設定行を `cfg.RiskEvaluator` の直接代入に変更する

### 2-4b. runner.go の中間状態更新

Phase 2 で `resource.Config` のストアフィールドが削除されるため、
`runner.go` を同時にコンパイルが通る中間状態に更新する。
（個別型アサーションはまだ残すが、evaluator を組み立てて Config に渡す形に変更する）

- [x] `internal/runner/runner.go`
  - [x] `createNormalResourceManager` で取得した 5 つのストアから `security.AnalysisDeps{...}` を構築する
  - [x] `security.NewNetworkAnalyzer(runtime.GOOS, deps)` を呼ぶ
  - [x] `risk.NewStandardEvaluator(networkAnalyzer)` を呼ぶ
  - [x] `resource.Config` のストアフィールド 5 個を `RiskEvaluator: evaluator` に置き換える
  - [x] `"runtime"` と `risk` の import を追加する
  - [x] `fileanalysis` と `dynamicanalysis` の import はまだ残す（型アサーションで使用中）

### 2-5. resource テストヘルパーの変更

- [x] `internal/runner/resource/test_helpers.go`
  - [x] `defaultTestEvaluator()` ヘルパー関数を追加する（`risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))` を返す）
  - [x] `NewNormalResourceManagerWithOutput` から `store fileanalysis.NetworkSymbolStore` 引数を削除し、`Config.RiskEvaluator: defaultTestEvaluator()` を設定する
  - [x] `NewNormalResourceManager` から `store` 引数を削除する（内部で `NewNormalResourceManagerWithOutput` を呼ぶため連動する）
  - [x] `NewDefaultResourceManagerForTest` から `symStore fileanalysis.NetworkSymbolStore` 引数を削除し、`Config.RiskEvaluator: defaultTestEvaluator()` を設定する
  - [x] `"runtime"` と `risk`、`security` の import を追加する
  - [x] 不要になる `fileanalysis` の import を削除する

- [x] `internal/runner/resource/testutil/helpers.go`
  - [x] `NewNormalResourceManager` の `store` 引数を削除する（`resource.NewNormalResourceManagerWithOutput` の変更に追随する）
  - [x] `NewDefaultResourceManager` の `symStore fileanalysis.NetworkSymbolStore` 引数を削除する
  - [x] `NewDefaultResourceManager` 内で `Config.RiskEvaluator` に `risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))` を設定する
  - [x] `"runtime"`、`risk`、`security` の import を追加する
  - [x] 不要になる `fileanalysis` の import を削除する

### 2-6. resource テストヘルパー呼び出し元の更新

- [x] `internal/runner/resource/normal_manager_test.go`
  - [x] `NewNormalResourceManagerWithOutput(..., nil)` の末尾 `nil` 引数を削除する（1 箇所）
- [x] `internal/runner/resource/error_scenarios_test.go`
  - [x] `NewNormalResourceManager(...)` の呼び出しを更新する（2 箇所）
  - [x] `NewDefaultResourceManagerForTest(..., nil, 0, nil)` の末尾 `nil` を削除する（複数箇所）
- [x] `internal/runner/resource/default_manager_test.go`
  - [x] `NewDefaultResourceManagerForTest(..., nil)` の末尾 `nil` を削除する（約 15 箇所）
- [x] `internal/runner/resource/performance_test.go`
  - [x] `NewDefaultResourceManagerForTest(..., nil)` の末尾 `nil` を削除する（1 箇所）
- [x] `internal/runner/e2e_slack_redaction_test.go`
  - [x] `resourcetestutil.NewDefaultResourceManager(..., nil)` の末尾 `nil` を削除する（2 箇所）
- [x] `internal/runner/integration_dual_defense_test.go`
  - [x] `resourcetestutil.NewDefaultResourceManager(..., nil)` の末尾 `nil` を削除する（4 箇所）
- [x] `internal/runner/command_output_capture_test.go`
  - [x] `resourcetestutil.NewDefaultResourceManager(..., nil)` の末尾 `nil` を削除する（2 箇所）

### 2-7. ビルド・テスト確認

- [x] `make build` が成功することを確認する
- [x] `make test` が成功することを確認する
- [x] `make lint` が成功することを確認する

---

## Phase 3: verification.Manager の API 追加と runner 簡素化

### 3-1. verification.Manager.GetAnalysisDeps() の追加

- [ ] `internal/verification/manager.go`
  - [ ] `GetAnalysisDeps() security.AnalysisDeps` メソッドを追加する
  - [ ] `security` の import を追加する
  - [ ] `GetNetworkSymbolStore()` を削除する
  - [ ] `GetSyscallAnalysisStore()` を削除する
  - [ ] `GetDynLibAnalysisStore()` を削除する
  - [ ] `GetDynLibDepsStore()` を削除する
  - [ ] `GetShebangInterpreterStore()` を削除する
  - [ ] `fileanalysis` と `dynamicanalysis` の import は他のメソッドで使用されているため削除しない

### 3-2. verification パッケージのテスト追加

- [ ] `internal/verification/manager_test.go`（または既存の verification テストファイル）
  - [ ] `TestManager_GetAnalysisDeps` を追加する（`Manager` のゼロ値で `GetAnalysisDeps()` を呼び、全フィールドが nil であることを確認する）

### 3-3. runner.createNormalResourceManager の簡素化

- [ ] `internal/runner/runner.go`
  - [ ] `createNormalResourceManager` の 5 系統の型アサーション（`networkSymbolStoreProvider` など）を削除する
  - [ ] `analysisDepsProvider` インターフェースを定義し、`GetAnalysisDeps()` の 1 系統の型アサーションに変更する
  - [ ] `security.NewNetworkAnalyzer(runtime.GOOS, deps)` と `risk.NewStandardEvaluator(networkAnalyzer)` を呼ぶ
  - [ ] 組み立てた evaluator を `resource.Config.RiskEvaluator` に設定する
  - [ ] 不要になる import（`fileanalysis`、`dynamicanalysis`）を削除する
  - [ ] 不要になる var 宣言（`networkStore`、`syscallStore` 等）を削除する
  - [ ] `"runtime"` と `risk` の import を追加する（未追加の場合）

### 3-4. runner_test.go の更新

- [ ] `internal/runner/runner_test.go`
  - [ ] `pathResolverWithStore` 構造体と関連メソッドを削除する
  - [ ] `pathResolverWithDeps` 構造体（`ResolvePath` + `GetAnalysisDeps`）を追加する
  - [ ] `TestCreateNormalResourceManager_AnalysisStoresInjected` を `GetAnalysisDeps()` が呼ばれることを確認する形に更新する
  - [ ] 不要になる import（`fileanalysis`）を削除する
  - [ ] `security` の import を追加する（`AnalysisDeps` の使用のため）

### 3-5. ビルド・テスト確認

- [ ] `make build` が成功することを確認する
- [ ] `make test` が成功することを確認する
- [ ] `make lint` が成功することを確認する

---

## 最終確認

- [ ] `AC-1`: `NewNetworkAnalyzer` と `NewStandardEvaluator` のシグネチャが分析依存 5 個を個別引数で受け取らない
- [ ] `AC-2`: `resource.Config` に `NetworkSymbolStore`、`SyscallStore`、`DynLibDepsStore`、`LibAnalysisStore`、`ShebangStore` フィールドが存在しない
- [ ] `AC-3`: `runner.createNormalResourceManager` に型アサーションが 1 系統（`analysisDepsProvider`）のみ残り、5 系統の個別 getter 呼び出しが存在しない
- [ ] `AC-4`: `security` パッケージに hash dir や store dir の具体構築コードが存在しない
- [ ] `AC-5`: `make test` で全テストが通過する
- [ ] `AC-6`: `make test`、`make lint`、`make build` がすべて成功する
- [ ] コード中に日本語が含まれていない（コメント・文字列ともに）
- [ ] 不要 import が残っていない（lint 確認）
