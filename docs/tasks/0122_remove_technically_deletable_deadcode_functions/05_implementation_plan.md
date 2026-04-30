# 次フェーズ削除作業計画書

## 0. 目的

`01_requirements.md` 第2回調査で確定した dead code 候補を削除し、追加調査が必要な候補の方針を決定する。

本計画は次を満たすことを目的とする。

1. 削除推奨（優先度 A）13 シンボルをコードベースから除去する
2. 要確認（優先度 B）2 件の削除可否を追加調査で確定する
3. 削除後の `make build`、`make lint`、`go test -tags test ./...` を成功させる

## 1. 対象スコープ

### 1.1 Phase A: 削除推奨（本番コードで未使用・修正コスト低）

**エラー型（型ごと削除）**

| シンボル | ファイル |
|---|---|
| `ErrInvalidParamName` + `Error()` | `internal/runner/config/template_errors.go` |
| `ErrInvalidVarsFormatDetail` + `Error()` + `Unwrap()` | `internal/runner/config/errors.go` |
| `ErrForbiddenPatternInTemplate` + `Error()` | `internal/runner/config/template_errors.go` |
| `ErrUndefinedGlobalVariable` + `Error()` | `internal/runner/variable/scope.go` |
| `ErrUndefinedLocalVariable` + `Error()` | `internal/runner/variable/scope.go` |

**便利メソッド・関数（メソッド/関数を削除）**

| シンボル | ファイル |
|---|---|
| `Component.String()` | `internal/runner/resource/types.go` |
| `RuntimeGroup.Name()` | `internal/runner/runnertypes/runtime.go` |
| `RuntimeGroup.WorkDir()` | `internal/runner/runnertypes/runtime.go` |
| `KnownNetworkLibraryCount()` | `internal/security/binaryanalyzer/known_network_libs.go` |
| `CreationMode.String()` | `internal/verification/types.go` |
| `SecurityLevel.String()` | `internal/verification/types.go` |
| `WithGroupMembershipProvider()` | `internal/runner/runner.go` |

注記: エラー型は `error` インターフェースの実装（`Error()` メソッド）を含む。削除時は型の定義、メソッド、および型を参照するテストコードを合わせて削除する。

### 1.2 Phase B: 追加調査（要確認・中規模）

| シンボル | ファイル | テスト参照数 | 調査方針 |
|---|---|---|---|
| `NewStandardEvaluator()` | `internal/runner/risk/evaluator.go` | 8 | `NewStandardEvaluatorWithStores(store, nil)` への置換コストを評価 |
| `NewELFSyscallStoreAdapter()` | `internal/runner/security/syscall_store_adapter.go` | 3 | アダプターとしての価値を評価。削除時は `fileanalysisSyscallStoreAdapter.LoadSyscallAnalysis` も同時削除 |

### 1.3 非対象

- `WithFileSystem`（36+ テスト参照）、`NewStandardELFAnalyzerWithSyscallStore`（11 件）、`NewSyscallAnalyzerWithConfig`（12 件）: 削除コストが高い
- `fileanalysisSyscallStoreAdapter.LoadSyscallAnalysis`: インターフェース実装
- 構造体フィールドおよびインターフェース実装の未使用判定

## 2. 実施フェーズ

### 2.1 Phase A-1: エラー型 5 件の削除

対象:
- `ErrInvalidParamName`
- `ErrInvalidVarsFormatDetail`
- `ErrForbiddenPatternInTemplate`
- `ErrUndefinedGlobalVariable`
- `ErrUndefinedLocalVariable`

作業:
- [x] 各型の参照箇所を `rg --glob '*.go'` で最終確認
- [x] 型定義・メソッド・関連コメントを削除
- [x] 型を参照するテストコード（インスタンス化、`errors.As` ターゲット）を削除
- [x] `make fmt` 実行
- [x] `go build ./...` 実行
- [x] `make lint` 実行
- [x] `go test -tags test ./...` 実行

完了条件:
- [x] 5 型がコードベースから削除されている
- [x] 品質ゲート（build/lint/test）が成功

### 2.2 Phase A-2: 便利メソッド・関数 8 件の削除

対象:
- `Component.String()`
- `RuntimeGroup.Name()` / `RuntimeGroup.WorkDir()`
- `KnownNetworkLibraryCount()`
- `CreationMode.String()` / `SecurityLevel.String()`
- `WithGroupMembershipProvider()`

作業:
- [ ] 各シンボルの参照箇所を `rg --glob '*.go'` で最終確認
- [ ] `RuntimeGroup.Name()` / `WorkDir()`: テスト側を `r.Spec.Name` / `r.Spec.WorkDir` に置換してからメソッド削除
- [ ] `WithGroupMembershipProvider()`: `runner_test.go:93` をインライン化（Option 関数リテラルを直接渡す）してから関数削除
- [ ] 残り 5 件（`Component.String`、`KnownNetworkLibraryCount`、`CreationMode.String`、`SecurityLevel.String`）: 関数削除、関連テストも削除
- [ ] `make fmt` 実行
- [ ] `go build ./...` 実行
- [ ] `make lint` 実行
- [ ] `go test -tags test ./...` 実行

完了条件:
- [ ] 8 シンボルがコードベースから削除されている
- [ ] テスト修正は最小差分（重複ロジック追加なし）
- [ ] 品質ゲート（build/lint/test）が成功

### 2.3 Phase B: 追加調査と削除判断

**B-1: `NewStandardEvaluator` の調査**

- [ ] `evaluator_test.go` の 8 件参照を確認し、`NewStandardEvaluatorWithStores(nil, nil)` への置換が意図を損なわないか検証
- [ ] 置換コスト（行数・複雑さ）を評価
- [ ] 削除する場合: テストを置換してから関数削除 → `make fmt` → `go build` → `make lint` → `go test`
- [ ] 削除しない場合: 理由を本ドキュメントに記録

完了条件:
- [ ] 削除 or 保留の判断が記録されている
- [ ] 削除した場合は品質ゲートが成功

**B-2: `NewELFSyscallStoreAdapter` の調査**

- [ ] `syscall_store_adapter_test.go` の 3 件参照を確認し、アダプターを直接構築（`&fileanalysisSyscallStoreAdapter{inner: store}`）でテストを書き換えられるか検証
- [ ] `fileanalysisSyscallStoreAdapter` 型が本番コードの他の箇所で参照されていないか再確認
- [ ] 削除する場合: テストを置換してから関数・型・メソッドを削除 → 品質ゲート実行
- [ ] 削除しない場合: 理由を本ドキュメントに記録

完了条件:
- [ ] 削除 or 保留の判断が記録されている
- [ ] 削除した場合は品質ゲートが成功

### 2.4 Phase C: 事後精査と結果更新

- [ ] `go vet -tags=test ./...` 実行
- [ ] `go run honnef.co/go/tools/cmd/staticcheck@latest -tags=test ./...` 実行
- [ ] `go run golang.org/x/tools/cmd/deadcode@latest ./cmd/...` 実行
- [ ] 結果を本タスク配下に記録（新たな dead code 候補があれば `01_requirements.md` に追記）

完了条件:
- [ ] Phase A で削除した 13 シンボルが再検出されない
- [ ] 新規候補リストが更新されている

## 3. コミット戦略

- [ ] Commit 1: Phase A-1（エラー型 5 件）
- [ ] Commit 2: Phase A-2（便利メソッド・関数 8 件）
- [ ] Commit 3: Phase B（追加調査結果。削除した場合はコード変更を含む）
- [ ] Commit 4: Phase C（結果更新ドキュメント）

ルール:
- 1 フェーズ完了ごとにコミットする
- 失敗で実施しなかった項目は `[-]` を付ける
- コミットメッセージは英語、1 行サマリ + 箇条書き本文

## 4. リスクと対策

- エラー型削除によるコンパイルエラー:
  - 対策: `rg` で参照箇所を網羅してから削除する。`go build` を必須ゲートとする
- テスト削除による意図の損失:
  - 対策: 削除するテストが「型の挙動確認」ではなく「dead code のテスト」であることを確認してから削除する
- `errors.As` ターゲット削除によるテスト論理の変化:
  - 対策: `ErrForbiddenPatternInTemplate` が本番コードで生成されていないことを再確認し、関連テストを削除する
- `WithGroupMembershipProvider` インライン化後の可読性低下:
  - 対策: インライン後のテストが「カスタム GroupMembership 注入のテスト」意図を維持していることを確認する

## 5. 進捗管理

### 5.1 ステータス

- [ ] Phase A-1 実施中
- [x] Phase A-1 完了
- [ ] Phase A-2 実施中
- [ ] Phase A-2 完了
- [ ] Phase B 実施中
- [ ] Phase B 完了
- [ ] Phase C 実施中
- [ ] Phase C 完了

### 5.2 実行ログ（追記用）

- 実施日:
- ブランチ:
- 実施者:
- 実行コマンド:
- 結果サマリ:
- 課題/ブロッカー:
- 次アクション:

## 6. レビュー観点

- 削除対象は本当に本番経路に未接続か
- テスト削除は「dead code のテスト削除」であり「有効なテストの削除」ではないか
- Phase B の判断理由が記録されているか
- dead code 再検出結果が計画と一致しているか
