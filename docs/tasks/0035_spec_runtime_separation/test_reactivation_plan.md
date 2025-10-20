# テスト再有効化計画

## 概要

Task 0035 (Spec/Runtime Separation) の進行に伴い、一時的に `skip_integration_tests` ビルドタグで無効化されているテストファイルがあります。本ドキュメントでは、各テストファイルの再有効化タイミングとその条件を記載します。

## 現在の状況

- **Phase 4 完了**: ConfigSpec/GlobalSpec/GroupSpec/CommandSpec/RuntimeGlobal/RuntimeGroup/RuntimeCommand の導入
- **Phase 5 完了**: ExpandGlobal() の from_env 処理実装

## テストファイル一覧と再有効化計画

### ✅ Phase 5 で再有効化済み

| ファイル | 状態 | 備考 |
|---------|------|------|
| `internal/runner/resource/types_test.go` | ✅ 有効化済み | 型定義のみ使用、問題なし |

### 🔄 Phase 6 で再有効化予定（Resource Manager の RuntimeCommand 対応）

以下のテストは、Resource Manager が `RuntimeCommand` を使用するように修正が必要です。

| ファイル | 状態 | 備考 |
|---------|------|------|
| `internal/runner/resource/normal_manager_test.go` | ✅ 完了 | ヘルパー関数を追加し、全テストケースを `RuntimeCommand` に変換 |
| `internal/runner/resource/default_manager_test.go` | ✅ 完了 | normal_manager_test.go のヘルパー関数を使用 |
| `internal/runner/resource/dryrun_manager_test.go` | ✅ 完了 | `CommandSpec` → `RuntimeCommand` 変換を実装 |
| `internal/runner/resource/error_scenarios_test.go` | ✅ 完了 | テストケース構造体を `CommandSpec`/`GroupSpec` に変更 |
| `internal/runner/resource/integration_test.go` | ✅ 完了 | `CommandSpec`/`GroupSpec` を使用、テスト実行確認済み |
| `internal/runner/resource/performance_test.go` | ✅ 完了 | `CommandSpec`/`GroupSpec` を使用、ベンチマーク実行確認済み |
| `internal/runner/resource/security_test.go` | ✅ 完了 | `CommandSpec` を使用、テスト実行確認済み |
| `internal/runner/resource/formatter_test.go` | ✅ 完了 | ビルドタグ削除、テスト実行確認済み（新型への依存なし） |
| `internal/runner/resource/manager_test.go` | ✅ 完了 | ビルドタグ削除、テスト実行確認済み（新型への依存なし） |
| `internal/runner/resource/usergroup_dryrun_test.go` | ✅ 完了 | ビルドタグ削除、テスト実行確認済み |

**完了した作業**:
1. ✅ `executor.CommandExecutor` インターフェースの `Execute()` メソッドを `RuntimeCommand` を受け取るように変更
2. ✅ `MockExecutor` の実装を更新
3. ✅ テストコード内で `CommandSpec` → `RuntimeCommand` への変換処理を追加（ヘルパー関数 `createRuntimeCommand()` を実装）
4. ✅ usergroup_dryrun_test.go: ビルドタグを削除し、テスト実行確認

### ✅ Phase 6 完了（Verification Manager の RuntimeGlobal 対応）

| ファイル | 状態 | 備考 |
|---------|------|------|
| `internal/verification/manager_test.go` | ✅ 完了 | `RuntimeGlobal`/`GroupSpec` を使用するヘルパー関数を追加、テスト実行確認済み |

**完了した作業**:
1. ✅ ビルドタグを削除（`skip_integration_tests` を除去）
2. ✅ ヘルパー関数 `createRuntimeGlobal()` と `createGroupSpec()` を実装
3. ✅ 全ての `GlobalConfig` 使用箇所を `RuntimeGlobal` に変換
4. ✅ 全ての `CommandGroup` 使用箇所を `GroupSpec` に変換
5. ✅ テスト実行確認（全テスト PASS）

### ✅ Phase 7 完了（Executor の RuntimeCommand 対応）

| ファイル | 状態 | 備考 |
|---------|------|------|
| `internal/runner/executor/environment_test.go` | ✅ 完了 | ビルドタグ削除、`RuntimeGlobal`/`RuntimeCommand` を使用、テスト実行確認済み |
| `internal/runner/executor/executor_test.go` | ✅ 完了 | ビルドタグ削除、`RuntimeCommand` を使用、テスト実行確認済み |

**完了した作業**:
1. ✅ ビルドタグを削除（`skip_integration_tests` を除去）
2. ✅ `environment_test.go`: `BuildProcessEnvironment` が `RuntimeGlobal`/`RuntimeCommand` を受け取るように変更されたため、ヘルパー関数を実装してテストケースを更新
3. ✅ `executor_test.go`: `Execute()` メソッドが `RuntimeCommand` を受け取るように変更されたため、ヘルパー関数 `createRuntimeCommand()` と `createRuntimeCommandWithName()` を実装
4. ✅ 全ての `Command` 使用箇所を `RuntimeCommand` に変換
5. ✅ テスト実行確認（全テスト PASS）

### 🔄 Phase 8 で再有効化予定（Group Executor の完全な統合テスト）

| ファイル | 理由 | 必要な修正 |
|---------|------|----------|
| `internal/runner/group_executor_test.go` | GroupExecutor の完全な統合テスト | 全ての型変更が完了後に有効化 |
| `internal/runner/environment/integration_test.go` | Environment の統合テスト | 同上 |
| `internal/runner/output_capture_integration_test.go` | Output capture の統合テスト | 同上 |
| `internal/runner/runner_test.go` | Runner の統合テスト | 同上 |
| `test/performance/output_capture_test.go` | パフォーマンステスト | 同上 |
| `test/security/output_security_test.go` | セキュリティテスト | 同上 |

**必要な作業**:
1. Phase 6, 7 の変更が完了していることを確認
2. テスト内で使用されている型をすべて新しい型に変更
3. 統合テストの実行環境を整備

## 再有効化の手順

各 Phase でテストを再有効化する際は、以下の手順に従います：

1. **ビルドタグの変更**
   ```go
   // Before
   //go:build test && skip_integration_tests
   // +build test,skip_integration_tests

   // After
   //go:build test
   // +build test
   ```

2. **テストの実行と確認**
   ```bash
   go test -tags test -v ./path/to/package
   ```

3. **エラーの修正**
   - コンパイルエラーがある場合は、型の変更に対応
   - テスト失敗がある場合は、ロジックの修正

4. **全テストの実行**
   ```bash
   go test -tags test ./...
   ```

5. **コミット**
   - 各 Phase でテスト再有効化をコミット

## 注意事項

- テストの再有効化は段階的に行い、各 Phase で完全に動作することを確認してからコミットします
- 予期しないエラーが発生した場合は、一旦 `skip_integration_tests` に戻し、問題を修正してから再度有効化します
- 全テストが有効化された後、`skip_integration_tests` ビルドタグを使用しているコードは削除します

## 進捗状況

- [x] Phase 5: types_test.go 有効化
- [x] Phase 6: Resource Manager テスト有効化 (10/10 完了)
  - ✅ normal_manager_test.go
  - ✅ default_manager_test.go
  - ✅ dryrun_manager_test.go
  - ✅ error_scenarios_test.go
  - ✅ integration_test.go
  - ✅ performance_test.go
  - ✅ security_test.go
  - ✅ usergroup_dryrun_test.go
  - ✅ formatter_test.go
  - ✅ manager_test.go
- [x] Phase 6: Verification Manager テスト有効化
  - ✅ manager_test.go
- [x] Phase 7: Executor テスト有効化 (2/2 完了)
  - ✅ environment_test.go
  - ✅ executor_test.go
- [ ] Phase 8: 統合テスト有効化

## 参考情報

- Task 0035 実装計画: `docs/tasks/0035_spec_runtime_separation/04_implementation_plan.md`
- アーキテクチャ設計: `docs/tasks/0035_spec_runtime_separation/02_architecture.md`
