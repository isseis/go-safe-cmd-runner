# 要件定義書：技術的に削除可能な dead code 関数の削除（フォローアップ）

## 1. 概要

**目的:** タスク 0122 の Phase D 精査で検出された「残件 dead code 候補」について現状調査を実施し、次フェーズの削除対象を確定する。

**背景:**
`go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` の出力をもとに 0122 計画書に記録された候補のうち、一部はその後のコミット（`issei/deadcode-removal-03`）で削除済みとなった。残存候補について参照状況を再確認し、削除可能なものを整理する。

## 2. 調査実施日

2026-04-30

## 3. 残件候補の現状調査（第1回）

### 3.1 削除済みの確認

以下は 0122 計画書の表に記載されていたが、現ブランチ（`issei/deadcode-removal-03`）のコミットで既に削除されている。

| シンボル | ファイル | 削除コミット |
|---|---|---|
| `SafeWriteFile` | `internal/safefileio/safe_file.go` | `refactor: remove dead code SafeWriteFile` |
| `SafeAtomicMoveFile` | `internal/safefileio/safe_file.go` | `refactor: remove dead code SafeAtomicMoveFile` |
| `NewRegistry` | `internal/runner/variable/registry.go` | `refactor: removed dead code Registry` |
| `ScanSVCAddrs` | `internal/security/machoanalyzer/svc_scanner.go` | `refactor: removed dead code ScanSVCAddrs` |

### 3.2 存在しないと確認されたシンボル

計画書の表に記載されていたが、現時点でコードベースに存在しない（既に別途削除済みか、表の記載が不正確）。

| シンボル | 備考 |
|---|---|
| `BackwardScanX0` | `arm64util.go` には `BackwardScanX16`、`BackwardScanStackTrap` は存在するが `BackwardScanX0` は存在しない |
| `newLoaderInternal` | `loader.go` 内に実装なし（コメント内で言及のみ） |
| `ValidateParams` | `template_expansion.go` に存在しない |
| `ValidateEnvImport` | `validation.go` に存在しない |
| `NewNetworkAnalyzer`（大文字 N） | `network_analyzer.go` に存在しない（小文字の `newNetworkAnalyzer` のみ） |
| `NewNetworkAnalyzerWithStore`（大文字 N、単数形） | 存在しない（`NewNetworkAnalyzerWithStores` 複数形は本番コードで使用中） |

### 3.3 削除不可と判定されたシンボル（第1回）

| シンボル | ファイル | 理由 |
|---|---|---|
| `NewLoaderForTest` | `internal/runner/config/test_helpers.go` | 15 以上のテストファイルから参照。テスト専用ヘルパーとして有効活用中 |
| `WithExecutor` | `internal/runner/test_helpers.go` | `//go:build test` ファイルに定義。テストコードから 3 件以上参照 |
| `WithResourceManager` | `internal/runner/test_helpers.go` | `//go:build test` ファイルに定義。テストコードから複数参照 |
| `NewAuditLoggerWithCustom` | `internal/runner/audit/test_helpers.go` | `//go:build test` ファイルに定義。複数テストファイルから参照 |
| `NewNormalResourceManagerWithOutput` | `internal/runner/resource/test_helpers.go` | `//go:build test` ファイルに定義。4 件のテストファイルから参照 |

注記: 上記は `deadcode` ツールで検出されるが、プロダクション entry point（`cmd/...`）から到達できないためである。テストから積極的に参照されており、実際の dead code ではない。

---

## 4. 残件候補の現状調査（第2回）

`go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` を再実行し、以下の 19 シンボルが検出された（2026-04-30）。各シンボルの参照状況を調査した結果を記録する。

### 4.1 削除推奨（本番コードでの使用なし・修正コスト低）

#### 4.1.1 型自体が完全未使用のエラー型

本番コードで型のインスタンス化箇所がなく、テストのみで参照されているか、あるいはどこからも参照されていないエラー型。

| シンボル | ファイル | 本番参照 | テスト参照 | 削除方法 |
|---|---|---|---|---|
| `ErrInvalidParamName` + `Error()` | `internal/runner/config/template_errors.go:213` | なし | **なし** | 型ごと削除（関連テストなし） |
| `ErrInvalidVarsFormatDetail` + `Error()` + `Unwrap()` | `internal/runner/config/errors.go:304` | なし | `errors_test.go` に 1 件 | 型ごと削除（関連テストも削除） |
| `ErrForbiddenPatternInTemplate` + `Error()` | `internal/runner/config/template_errors.go:132` | なし（インスタンス化なし） | `errors.As` ターゲットとして 4 件 | 型ごと削除（テストの `wantErrType` も削除） |
| `ErrUndefinedGlobalVariable` + `Error()` | `internal/runner/variable/scope.go:190` | なし | `scope_test.go` に 2 件 | 型ごと削除（関連テストも削除）。本番は `ErrUndefinedGlobalVariableInTemplate` を使用 |
| `ErrUndefinedLocalVariable` + `Error()` | `internal/runner/variable/scope.go:199` | なし | `scope_test.go` に 2 件 | 型ごと削除（関連テストも削除） |

#### 4.1.2 本番コードで使用されていない便利メソッド・関数

| シンボル | ファイル | 本番参照 | テスト参照 | 削除方法 |
|---|---|---|---|---|
| `RuntimeGroup.Name()` | `internal/runner/runnertypes/runtime.go:212` | なし（本番は `r.Spec.Name` か `ExtractGroupName()` を使用） | `runtime_test.go` に 2 件 | メソッド削除、テストも削除 |
| `RuntimeGroup.WorkDir()` | `internal/runner/runnertypes/runtime.go:221` | なし（本番は `r.Spec.WorkDir` を使用） | `runtime_test.go` に 2 件 | メソッド削除、テストも削除 |
| `KnownNetworkLibraryCount()` | `internal/security/binaryanalyzer/known_network_libs.go:104` | なし | `known_network_libs_test.go` に 1 件 | 関数削除、テストも削除。コメントに「for tests and documentation」と明記 |
| `Component.String()` | `internal/runner/resource/types.go:363` | なし | なし | 関数削除 |
| `CreationMode.String()` | `internal/verification/types.go:19` | なし | `manager_test.go` に 2 件 | メソッド削除、テストも削除 |
| `SecurityLevel.String()` | `internal/verification/types.go:41` | なし | `manager_test.go` に 2 件 | メソッド削除、テストも削除 |
| `WithGroupMembershipProvider()` | `internal/runner/runner.go:136`（本番ファイル、build tag なし） | なし | `runner_test.go` に 1 件 | インライン化して削除 |

### 4.2 要確認（修正コスト中・削除の妥当性を要検討）

| シンボル | ファイル | テスト参照 | 検討事項 |
|---|---|---|---|
| `NewStandardEvaluator()` | `internal/runner/risk/evaluator.go:25` | `evaluator_test.go` に 8 件 | `NewStandardEvaluatorWithStores(store, nil)` への置換で削除可能。修正量は中程度 |
| `NewELFSyscallStoreAdapter()` | `internal/runner/security/syscall_store_adapter.go:22` | `syscall_store_adapter_test.go` に 3 件 | アダプターパターンとしての価値を要評価。削除する場合は `fileanalysisSyscallStoreAdapter.LoadSyscallAnalysis` も同時削除 |

### 4.3 削除不可

| シンボル | ファイル | 理由 |
|---|---|---|
| `WithFileSystem()` | `internal/runner/security/validator.go:79` | テストから 36 件以上参照。テスト基盤として不可欠 |
| `NewStandardELFAnalyzerWithSyscallStore()` | `internal/security/elfanalyzer/standard_analyzer.go:76` | `analyzer_test.go` に 11 件。削除コスト高 |
| `NewSyscallAnalyzerWithConfig()` | `internal/security/elfanalyzer/syscall_analyzer.go:177` | `syscall_analyzer_test.go` に 12 件。削除コスト高 |
| `fileanalysisSyscallStoreAdapter.LoadSyscallAnalysis()` | `internal/runner/security/syscall_store_adapter.go:29` | `secelfanalyzer.SyscallAnalysisStore` インターフェースの実装。`NewELFSyscallStoreAdapter` が存在する限り必須 |

## 5. 次フェーズの削除スコープ（案）

### 優先度 A（削除推奨・小規模）

- `ErrInvalidParamName` 型と `Error()` メソッド
- `ErrInvalidVarsFormatDetail` 型と `Error()` + `Unwrap()` メソッド（関連テストも削除）
- `ErrForbiddenPatternInTemplate` 型と `Error()` メソッド（関連テストも削除）
- `ErrUndefinedGlobalVariable` / `ErrUndefinedLocalVariable` 型と `Error()` メソッド（関連テストも削除）
- `Component.String()` メソッド（参照なし）
- `RuntimeGroup.Name()` / `RuntimeGroup.WorkDir()` メソッド（関連テストも削除）
- `KnownNetworkLibraryCount()` 関数（関連テストも削除）
- `CreationMode.String()` / `SecurityLevel.String()` メソッド（関連テストも削除）
- `WithGroupMembershipProvider()` 関数（テスト 1 件をインライン化）

### 優先度 B（要確認・中規模）

- `NewStandardEvaluator()` 関数：8 件のテストを `NewStandardEvaluatorWithStores(nil, nil)` に置換
- `NewELFSyscallStoreAdapter()` 関数：アダプターとしての存在意義を再評価

## 6. 非機能要件

- 削除後に `make build`、`make lint`、`make test` がすべて成功すること
- 削除対象以外に余分な差分を含まないこと
- 削除の根拠（参照調査）をコミットメッセージまたは PR に記録すること

## 7. スコープ外

- `WithFileSystem`、`NewStandardELFAnalyzerWithSyscallStore`、`NewSyscallAnalyzerWithConfig`: テストで広範に使用されており削除コストが高い
- `NewLoaderForTest`: 参照数が多く削除コストが高い
- `fileanalysisSyscallStoreAdapter.LoadSyscallAnalysis`: インターフェース実装として必須
- 構造体フィールドおよびインターフェース実装の未使用判定
