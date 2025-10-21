# Task 0039: runner_test.go の型移行

## 概要

`internal/runner/runner_test.go`を古い型システムから新しい型システム（Spec/Runtime分離）に移行するタスク。このファイルはTask 0038で保留となった最後の大規模統合テストファイルです。

## 背景

Task 0038の実行中に、`runner_test.go`は以下の理由で大規模な移行が必要と判明しました：

1. **CommandSpecに必要なフィールドがない**
   - `EffectiveWorkDir` フィールドが存在しない（`RuntimeCommand`にはある）
   - テストコードで`CommandSpec`に直接`EffectiveWorkDir`を設定しようとしている

2. **GroupSpecに必要なフィールドがない**
   - `TempDir` フィールドが存在しない
   - 約10箇所で`TempDir`を使用している

3. **未定義のモックメソッド**
   - `SetupFailedMockExecution` メソッドが`MockResourceManager`に存在しない
   - 約8箇所で使用されている

4. **コンパイルエラーの規模**
   - 約48個のコンパイルエラー
   - 2538行のファイルで大規模な変更が必要

## 現状分析

### ファイル情報
- **ファイルパス**: `internal/runner/runner_test.go`
- **総行数**: 2538行
- **テスト関数数**: 21個
- **現在の状態**: `skip_integration_tests` ビルドタグ付き（無効化中）

### 主な問題箇所

1. **EffectiveWorkDir の誤使用** (約25箇所)
   ```go
   // 誤: CommandSpec に EffectiveWorkDir を設定
   expectedCmd.EffectiveWorkDir = config.Global.WorkDir

   // 正: RuntimeCommand に設定すべき
   ```

2. **TempDir フィールドの使用** (約10箇所)
   ```go
   // 誤: GroupSpec に TempDir フィールドを設定
   GroupSpec{
       TempDir: true,
       // ...
   }

   // 正: TempDir は WorkDir として扱うか、別の方法で実装
   ```

3. **未定義のモックメソッド** (約8箇所)
   ```go
   // 誤: 存在しないメソッド
   mockRM.SetupFailedMockExecution(errors.New("test error"))

   // 正: 直接 On() を使用
   mockRM.On("ExecuteCommand", ...).Return(nil, errors.New("test error"))
   ```

## 目標

1. ✅ `skip_integration_tests` ビルドタグを削除
2. ✅ 全21個のテスト関数が新しい型システムで動作
3. ✅ 全テストがPASS
4. ✅ コンパイルエラーが0件
5. ✅ `make test` で全テストがPASS

## アプローチ

### Phase 1: 分析と設計（推定: 2-3時間）

1. **現状の詳細分析**
   - 全てのコンパイルエラーをリストアップ
   - 各エラーの原因を分類
   - 修正パターンを特定

2. **設計方針の決定**
   - `EffectiveWorkDir` の扱い方
   - `TempDir` の代替実装方法
   - モックメソッドの追加/修正方針

3. **移行計画の詳細化**
   - テスト関数の優先順位付け
   - 段階的な移行手順の策定

### Phase 2: 基盤整備（推定: 2-4時間）

1. **モックの拡張**
   - `SetupFailedMockExecution` メソッドの実装
   - その他必要なヘルパーメソッドの追加

2. **ヘルパー関数の実装**
   - `CommandSpec` → `RuntimeCommand` 変換ヘルパー
   - テストデータ作成ヘルパー
   - アサーション用ヘルパー

3. **テスト用ユーティリティの整備**
   - `TempDir` 機能のモック/スタブ実装
   - テスト環境のセットアップ関数

### Phase 3: 段階的移行（推定: 10-16時間）

#### 3.1 簡単なテストから開始（推定: 3-4時間）

優先度高、変更箇所少ない：
- `TestNewRunner` (行114-178)
- `TestNewRunnerWithSecurity` (行180-221)
- `TestRunner_ExecuteCommand` (行989-1097)

#### 3.2 中程度のテスト（推定: 4-6時間）

優先度中、中程度の変更：
- `TestRunner_ExecuteGroup` (行223-331)
- `TestRunner_ExecuteAll` (行455-585)
- `TestRunner_EnvironmentVariables` (行2036-2186)
- `TestRunner_GroupPriority` (行713-817)
- `TestRunner_ExecuteAllWithPriority` (行587-711)

#### 3.3 複雑なテスト（推定: 3-6時間）

優先度中低、複雑な変更：
- `TestRunner_OutputCapture` (行1099-1244)
- `TestRunner_OutputCaptureEdgeCases` (行1246-1398)
- `TestRunner_DependencyHandling` (行819-987)
- その他のタイムアウト・セキュリティ関連テスト

### Phase 4: 検証と最終調整（推定: 2-3時間）

1. **統合テスト実行**
   - 全テストを個別に実行
   - `make test` で全体実行
   - エラーの修正

2. **コード品質チェック**
   - `make lint` でリント確認
   - コードレビュー
   - リファクタリング

3. **ドキュメント更新**
   - `test_reactivation_plan.md` 更新
   - Task 0039 完了記録
   - カバレッジレポート確認

## 推定工数

| Phase | 内容 | 推定工数 |
|-------|------|---------|
| Phase 1 | 分析と設計 | 2-3時間 |
| Phase 2 | 基盤整備 | 2-4時間 |
| Phase 3 | 段階的移行 | 10-16時間 |
| Phase 4 | 検証と最終調整 | 2-3時間 |
| **合計** | | **16-26時間** |

**推奨作業期間**: 3-4日（1日4-6時間作業）

## 前提条件

- ✅ Task 0038 完了（Phase 1-5、runner_test.go除く）
- ✅ 他の統合テストファイルが全てPASS
- ✅ `MockResourceManager` が利用可能（`internal/runner/testing/mocks.go`）
- ✅ 新しい型システムが完全に動作

## 成功基準

1. ✅ `skip_integration_tests` ビルドタグが削除されている
2. ✅ コンパイルエラーが0件
3. ✅ 全21個のテスト関数がPASS
4. ✅ `make test` で全テストがPASS
5. ✅ `make lint` でエラーなし
6. ✅ カバレッジが低下していない（目標: 80%以上）

## リスクと対策

### リスク1: TempDir機能が未実装

**影響**: TempDirを使用するテストが動作しない可能性

**対策**:
- TempDir関連のテストを一旦スキップ
- 別タスク（Task 0040）でTempDir機能を実装
- または、ワークディレクトリで代替実装

### リスク2: 想定外のコンパイルエラー

**影響**: 移行作業が遅延する可能性

**対策**:
- Phase 1で全エラーを事前分析
- エラーパターンを分類して対処方針を決定
- 不明なエラーは都度調査・対応

### リスク3: テストロジックの変更が必要

**影響**: 単純な型変換では対応できない場合

**対策**:
- テストの意図を理解してから修正
- 必要に応じてテストロジックを見直し
- 元のテスト意図を損なわないよう注意

## 参考資料

- [Task 0036: runner_test.go型移行](../0036_runner_test_migration/) - 初期の移行ガイド（参考用）
- [Task 0038: テストインフラの最終整備](../0038_test_infrastructure_finalization/)
- [テスト再有効化計画](../0035_spec_runtime_separation/test_reactivation_plan.md)

## 関連Issue/PR

- Task 0035: Spec/Runtime分離
- Task 0038: テストインフラの最終整備

## 次のステップ

1. このREADME.mdを読む
2. [progress.md](./progress.md) で進捗を追跡
3. [quick_reference.md](./quick_reference.md) でコマンドを参照
4. Phase 1から開始
