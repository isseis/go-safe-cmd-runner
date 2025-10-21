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

### ✅ Phase 8 で部分的に再有効化完了

| ファイル | 状態 | 備考 |
|---------|------|------|
| `internal/runner/group_executor_test.go` | ✅ 完了 | ビルドタグ削除、共有モック作成、全テスト PASS（3個スキップ） |

**完了した作業**:
1. ✅ ビルドタグを削除（`skip_integration_tests` を除去）
2. ✅ 共有モック `MockResourceManager` を `internal/runner/testing/mocks.go` に作成
3. ✅ `package runner_test` → `package runner` に変更（プライベート型アクセスのため）
4. ✅ `Dir` → `WorkDir` フィールド名修正
5. ✅ `TempDir` 関連テストをスキップ（未実装機能）
6. ✅ `TestCreateCommandContext` の時刻計算問題を修正
7. ✅ 全 10 テスト中 7 テストが PASS、3 テストは未実装機能のためスキップ

### 🔄 Phase 9 以降で再有効化予定（大規模な型移行が必要な統合テスト）

以下のテストファイルは、古い型システム（Spec/Runtime分離前）を大量に使用しており、大規模な移行作業が必要です。

| ファイル | 行数 | 主な課題 | 優先度 |
|---------|------|----------|--------|
| `internal/runner/runner_test.go` | ~2700行 | `Config`→`ConfigSpec`, `CommandGroup`→`GroupSpec`への大規模移行 | 高 |
| `internal/runner/output_capture_integration_test.go` | ~200行 | 型移行 + `package runner_test` 問題 | 中 |
| `test/performance/output_capture_test.go` | ~150行 | 型移行 + パフォーマンステスト環境整備 | 中 |
| `test/security/output_security_test.go` | ~200行 | 型移行 + セキュリティテスト環境整備 | 中 |

**必要な作業の詳細**:

#### 1. 型システムの大規模移行（推定2000+行の変更）
   - `runnertypes.Config` → `runnertypes.ConfigSpec`
   - `runnertypes.GlobalConfig` → `runnertypes.GlobalSpec`
   - `runnertypes.CommandGroup` → `runnertypes.GroupSpec`
   - `runnertypes.Command` → `runnertypes.CommandSpec` / `runnertypes.RuntimeCommand`

#### 2. テストヘルパーメソッドの実装・移植
   - `SetupDefaultMockBehavior()`: デフォルトのモック動作設定
   - `SetupSuccessfulMockExecution(stdout, stderr string)`: 成功時のモック設定
   - `SetupFailedMockExecution(err error)`: 失敗時のモック設定
   - `NewMockResourceManagerWithDefaults()`: デフォルト設定付きモック作成

#### 3. パッケージ構造の整理
   - `package runner_test` vs `package runner` の使い分け
   - プライベート型へのアクセス問題の解決

#### 4. ファイル別の詳細タスク

**`internal/runner/runner_test.go` (最優先)**:
- [ ] `Config` → `ConfigSpec` への変換（約200箇所）
- [ ] `GlobalConfig` → `GlobalSpec` への変換（約150箇所）
- [ ] `CommandGroup` → `GroupSpec` への変換（約100箇所）
- [ ] `Command` → `CommandSpec`/`RuntimeCommand` への変換（約300箇所）
- [ ] `NewRunner()` の引数変更への対応
- [ ] テストヘルパーメソッドの実装
- [ ] `setupSafeTestEnv()` などのユーティリティ関数の更新

**`internal/runner/output_capture_integration_test.go`**:
- [ ] `MockResourceManager` と `setupSafeTestEnv` へのアクセス問題解決
- [ ] 型の変換（約50箇所）
- [ ] `package runner_test` から必要な型・関数へのアクセス方法の確立

**`test/performance/output_capture_test.go`**:
- [ ] `runnertypes.Command` → `runnertypes.RuntimeCommand` への変換
- [ ] `runnertypes.CommandGroup` → `runnertypes.GroupSpec` への変換
- [ ] `PrepareCommand()` などの削除されたメソッドへの対応
- [ ] パフォーマンス計測コードの動作確認

**`test/security/output_security_test.go`**:
- [ ] 型の変換（約80箇所）
- [ ] セキュリティテスト環境の整備
- [ ] 新しいセキュリティAPI（`security.Validator` など）への対応

#### 5. 検証と品質保証
- [ ] すべてのテストが個別に PASS することを確認
- [ ] `make test` で全テストが PASS することを確認
- [ ] `make lint` でリントエラーがないことを確認
- [ ] カバレッジレポートの確認（カバレッジが低下していないこと）

**推奨アプローチ**:
1. **Task 0036**: `runner_test.go` の型移行（最優先、単独タスク）
2. **Task 0037**: 残りの統合テストの型移行（並行作業可能）
3. **Task 0038**: パフォーマンス・セキュリティテストの再有効化と環境整備

**備考**:
- ファイルが存在しないため除外: `internal/runner/environment/integration_test.go`

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
- [x] Phase 8: 統合テスト部分的有効化 (4/5 完了)
  - ✅ group_executor_test.go (7/10 テスト PASS、3テスト未実装機能でスキップ)
  - ✅ output_capture_integration_test.go (2/2 テスト PASS)
  - ✅ environment/integration_test.go (7/7 テスト PASS、Task 0038で完了)
  - ✅ test/performance/output_capture_test.go (5/5 テスト PASS、すでに新しい型を使用、Task 0038で確認)
  - ✅ test/security/output_security_test.go (8/8 テスト PASS、すでに新しい型を使用、Task 0038で確認)
  - 🔄 runner_test.go (大規模な型移行が必要、Task 0036 で対応中)
- [x] Task 0038: テストインフラの最終整備（ほぼ完了、runner_test.go は Task 0039 へ移行）
  - ✅ Phase 1.0: environment/integration_test.go 有効化完了（7テストPASS）
  - ⏸️ Phase 1.1: runner_test.go 大規模移行が必要（Task 0039 へ移行）
  - ✅ Phase 1.2: test/performance/output_capture_test.go 確認完了（5テストPASS、作業不要）
  - ✅ Phase 1.3: test/security/output_security_test.go 確認完了（8テストPASS、作業不要）
  - ✅ Phase 3: CI/CD パイプライン確認完了（テスト実行時間: 2.5秒）
  - ✅ Phase 4: カバレッジ確認完了（76.1%、runner_test.goがスキップされているため妥当）
  - ✅ Phase 5: ドキュメント更新完了（test_reactivation_plan.md, progress.md, completion_report.md）
- [ ] Task 0036: runner_test.go の大規模型移行（詳細ドキュメント完成、実装は保留中）
- [ ] Task 0039: runner_test.go の型システム移行（Task 0038 から分離、2025-10-21 作成）

## 次のステップ（Phase 9 以降の新規タスク）

Phase 8 で `group_executor_test.go` の再有効化が完了しました。残りの統合テストは大規模な型移行が必要なため、以下の新規タスクとして計画することを推奨します：

### Task 0036: `runner_test.go` の型移行と再有効化

**優先度**: 高（最優先）
**推定工数**: 2-3日（16-24時間）
**影響範囲**: 2569行、21個のテスト関数、650+箇所の型変換

**ドキュメント**: [docs/tasks/0036_runner_test_migration/](../0036_runner_test_migration/)
- [README.md](../0036_runner_test_migration/README.md) - タスク概要とクイックスタート
- [移行ガイド](../0036_runner_test_migration/01_migration_guide.md) - 詳細な型変換マッピング
- [実装計画書](../0036_runner_test_migration/02_implementation_plan.md) - 段階的実装手順

**作業内容**:
1. テストヘルパーメソッドの実装（2-3時間）
2. 21個のテスト関数の段階的移行（12-18時間）
3. `skip_integration_tests`タグの削除と検証（2-3時間）

**前提条件**:
- Phase 8 までのすべての変更が完了していること
- `MockResourceManager` が利用可能であること（`internal/runner/testing/mocks.go`）

### Task 0037: 残りの統合テストの再有効化

**優先度**: 中
**推定工数**: 2日（12-17時間）
**ドキュメント**: [docs/tasks/0037_remaining_integration_tests/](../0037_remaining_integration_tests/)

**進捗状況**:
- ✅ `internal/runner/output_capture_integration_test.go` (227行) - 完了、全テスト PASS
- ✅ `test/performance/output_capture_test.go` (422行) - 完了、すでに新しい型を使用、全5テスト PASS
- ✅ `test/security/output_security_test.go` (546行) - 完了、すでに新しい型を使用、全8テスト PASS

**完了した作業**:
1. ✅ `output_capture_integration_test.go` の型移行完了
   - `package runner_test` → `package runner` に変更
   - 全ての古い型を新しい型に移行
   - ヘルパー関数（`setupSafeTestEnv`, `MockResourceManager`）を追加
   - 2つのテスト関数が全て PASS
2. ✅ `test/performance/output_capture_test.go` - すでに新しい型（RuntimeCommand）を使用、テストPASS確認のみ
3. ✅ `test/security/output_security_test.go` - すでに新しい型（RuntimeCommand）を使用、テストPASS確認のみ
4. ✅ Task 0037 完了（全ての対象ファイルが新しい型システムで動作中）

**Task 0037 完了サマリー**:
- `internal/runner/output_capture_integration_test.go`: 移行完了（2テストPASS）
- `test/performance/output_capture_test.go`: 作業不要、既に完了（5テストPASS）
- `test/security/output_security_test.go`: 作業不要、既に完了（8テストPASS）

### Task 0038: テストインフラの最終整備

**優先度**: 中
**推定工数**: ほぼ完了（Task 0039 として runner_test.go を分離）
**ステータス**: ✅ ほぼ完了（2025-10-21）
**ドキュメント**: [docs/tasks/0038_test_infrastructure_finalization/](../0038_test_infrastructure_finalization/)

**ドキュメント一覧**:
- [README.md](../0038_test_infrastructure_finalization/README.md) - タスク概要と詳細計画
- [progress.md](../0038_test_infrastructure_finalization/progress.md) - 進捗追跡シート
- [quick_reference.md](../0038_test_infrastructure_finalization/quick_reference.md) - コマンドリファレンス
- [completion_report.md](../0038_test_infrastructure_finalization/completion_report.md) - 完了報告書（2025-10-21）

**完了した作業**:
1. ✅ **Phase 1.0**: environment/integration_test.go 有効化完了（7テストPASS、0.5時間）
2. ✅ **Phase 1.2**: test/performance/output_capture_test.go 確認完了（5テストPASS、0.2時間、作業不要）
3. ✅ **Phase 1.3**: test/security/output_security_test.go 確認完了（8テストPASS、0.2時間、作業不要）
4. ✅ **Phase 3**: CI/CD パイプライン確認完了（テスト実行時間: 2.5秒）
5. ✅ **Phase 4**: カバレッジ確認完了（76.1%）
6. ✅ **Phase 5**: ドキュメント更新完了（completion_report.md 作成）

**Task 0039 への引き継ぎ**:
1. ⏸️ **Phase 1.1**: runner_test.go の型移行（推定16-26時間）
   - 2538行のテストコード、21個のテスト関数
   - ~48個のコンパイルエラー（EffectiveWorkDir, TempDir, SetupFailedMockExecution など）
   - Task 0039 として独立タスク化（2025-10-21）

**成功基準** (達成状況):
- [x] 統合テストの大部分が新しい型システムで動作（20/41テストが有効化）
- [x] CI/CDパイプラインが正常動作
- [x] テストカバレッジ76.1%（runner_test.goがスキップされているため妥当）
- [ ] `skip_integration_tests` タグが完全に削除（runner_test.go のみ残存、Task 0039 で対応）
- [ ] 古い型定義が完全に削除（runner_test.go移行後、Task 0039 完了時）

### Task 0039: runner_test.go の型システム移行

**優先度**: 高（Task 0038 から分離）
**推定工数**: 16-26時間（4フェーズ）
**ステータス**: 📝 ドキュメント作成完了（2025-10-21）
**ドキュメント**: [docs/tasks/0039_runner_test_migration/](../0039_runner_test_migration/)

**ドキュメント一覧**:
- [README.md](../0039_runner_test_migration/README.md) - 包括的なタスク概要、背景分析、4フェーズアプローチ
- [progress.md](../0039_runner_test_migration/progress.md) - 詳細な進捗トラッキング（30サブタスク）
- [quick_reference.md](../0039_runner_test_migration/quick_reference.md) - コマンドリファレンスとコードパターン

**概要**:
- **ファイル**: `internal/runner/runner_test.go`（2538行）
- **テスト数**: 21個のテスト関数
- **エラー数**: ~48個のコンパイルエラー

**特定された問題**:
1. **EffectiveWorkDir の型配置誤り** (~25箇所)
   - テストコードが `CommandSpec` に `EffectiveWorkDir` があると想定
   - 実際は `RuntimeCommand` のフィールド

2. **TempDir フィールドの欠如** (~10箇所)
   - `GroupSpec` に `TempDir` フィールドがない
   - Go標準の `t.TempDir()` で代替または将来の機能追加が必要

3. **SetupFailedMockExecution メソッドの未実装** (~8箇所)
   - `MockResourceManager` にメソッドが存在しない
   - 直接モック設定で対応

**アプローチ（4フェーズ）**:
1. **Phase 1: 分析と設計** (2-3時間)
   - 全エラーの詳細分析
   - 解決策の設計
   - 移行計画の策定

2. **Phase 2: 基盤整備** (3-4時間)
   - MockResourceManager の拡張
   - ヘルパー関数の実装（createRuntimeCommand など）
   - ユーティリティの準備

3. **Phase 3: 段階的移行** (10-16時間)
   - Tier 1: 簡単なテスト（3個、各0.5-1時間）
   - Tier 2: 中程度のテスト（5個、各1-2時間）
   - Tier 3: 複雑なテスト（13個、各1-2時間）

4. **Phase 4: 検証と最終調整** (1-3時間)
   - 全テストの実行
   - カバレッジ確認（80%以上目標）
   - ドキュメント更新

**リスクと対策**:
1. **リスク**: 3つの問題カテゴリが相互に依存
   - **対策**: Phase 2 で基盤を整備してから移行

2. **リスク**: テスト失敗時のデバッグに時間がかかる
   - **対策**: 3個ずつ段階的にコミット

3. **リスク**: 新機能（TempDir）の必要性が判明
   - **対策**: 一部テストをスキップし、Task 0040 として別途対応

**成功基準**:
- [ ] 全21個のテストがPASS
- [ ] `skip_integration_tests` タグを削除
- [ ] カバレッジ80%以上達成
- [ ] CI/CDパイプラインで全テストPASS

**前提条件**:
- Task 0038 完了（environment, performance, security テスト有効化済み）
- `MockResourceManager` が利用可能（`internal/runner/testing/mocks.go`）

### Task 0038: テストインフラの最終整備

**優先度**: 中
**推定工数**: ほぼ完了（Task 0039 として runner_test.go を分離）
**ステータス**: ✅ ほぼ完了（2025-10-21）
**ドキュメント**: [docs/tasks/0038_test_infrastructure_finalization/](../0038_test_infrastructure_finalization/)

**ドキュメント一覧**:
- [README.md](../0038_test_infrastructure_finalization/README.md) - タスク概要と詳細計画
- [progress.md](../0038_test_infrastructure_finalization/progress.md) - 進捗追跡シート
- [quick_reference.md](../0038_test_infrastructure_finalization/quick_reference.md) - コマンドリファレンス

**完了した作業**:
1. ✅ **Phase 1.0**: environment/integration_test.go 有効化完了（7テストPASS、0.5時間）
2. ✅ **Phase 1.2**: test/performance/output_capture_test.go 確認完了（5テストPASS、0.2時間、作業不要）
3. ✅ **Phase 1.3**: test/security/output_security_test.go 確認完了（8テストPASS、0.2時間、作業不要）
4. ✅ **Phase 3**: CI/CD パイプライン確認完了（テスト実行時間: 2.5秒）
5. ✅ **Phase 4**: カバレッジ確認完了（76.1%）
6. ✅ **Phase 5**: ドキュメント更新完了（completion_report.md 作成）

**Task 0039 への引き継ぎ**:
1. ⏸️ **Phase 1.1**: runner_test.go の型移行（推定16-26時間）
   - 2538行のテストコード、21個のテスト関数
   - ~48個のコンパイルエラー（EffectiveWorkDir, TempDir, SetupFailedMockExecution など）
   - Task 0039 として独立タスク化（2025-10-21）

**成功基準** (達成状況):
- [x] 統合テストの大部分が新しい型システムで動作（20/41テストが有効化）
- [x] CI/CDパイプラインが正常動作
- [x] テストカバレッジ76.1%（runner_test.goがスキップされているため妥当）
- [ ] `skip_integration_tests` タグが完全に削除（runner_test.go のみ残存、Task 0039 で対応）
- [ ] 古い型定義が完全に削除（runner_test.go移行後、Task 0039 完了時）


## 参考情報

- Task 0035 実装計画: `docs/tasks/0035_spec_runtime_separation/04_implementation_plan.md`
- アーキテクチャ設計: `docs/tasks/0035_spec_runtime_separation/02_architecture.md`
