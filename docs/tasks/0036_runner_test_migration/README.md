# Task 0036: runner_test.go 型移行

## 概要

`internal/runner/runner_test.go`（2569行、21個のテスト関数）を古い型システム（Config, GlobalConfig, CommandGroup, Command）から新しいSpec/Runtime分離型システム（ConfigSpec, GlobalSpec, GroupSpec, CommandSpec, RuntimeCommand）に移行するタスク。

## ドキュメント

1. **[移行ガイド](./01_migration_guide.md)** - 詳細な型変換マッピングと移行手順
2. **[実装計画書](./02_implementation_plan.md)** - 段階的な実装手順とマイルストーン

## クイックスタート

### 前提条件

- Task 0035 Phase 8 が完了していること
- `internal/runner/testing/mocks.go` が存在すること
- すべての既存テストが PASS していること

### 移行手順（要約）

```bash
# 1. 現在のブランチを確認
git status

# 2. 新しい作業ブランチを作成
git checkout -b task/0036-runner-test-migration

# 3. ヘルパーメソッドを追加（手動）
# internal/runner/runner_test.go の MockResourceManager 型エイリアス後に追加
# 詳細は 02_implementation_plan.md の Step 1 を参照

# 4. 最初のテストを移行
# TestNewRunner 関数を修正（詳細は 01_migration_guide.md を参照）

# 5. テスト実行
go test -tags test -v ./internal/runner -run TestNewRunner

# 6. 成功したらコミット
git add internal/runner/runner_test.go
git commit -m "test: migrate TestNewRunner to ConfigSpec"

# 7. 残りのテスト関数を順次移行（Step 4-23）
# ...

# 8. すべてのテストが成功したら skip_integration_tests タグを削除

# 9. 最終検証
make test
make lint

# 10. コミット
git commit -m "test: remove skip_integration_tests tag from runner_test.go"
```

## 進捗トラッキング

### Phase 1: ヘルパー準備
- [ ] ヘルパーメソッド（SetupDefaultMockBehavior等）を追加
- [ ] 型変換ヘルパー関数（createConfigSpec等）を追加
- [ ] コンパイル確認

### Phase 2: テスト関数移行

#### 簡単（3個、推定2-3時間）
- [ ] TestNewRunner
- [ ] TestNewRunnerWithSecurity
- [ ] TestRunner_ExecuteCommand

#### 中程度（9個、推定9-18時間）
- [ ] TestRunner_ExecuteGroup ⚠️ 重要
- [ ] TestRunner_ExecuteAll
- [ ] TestRunner_OutputCapture
- [ ] TestRunner_OutputCaptureEdgeCases
- [ ] TestRunner_OutputSizeLimit
- [ ] TestRunner_CommandTimeout
- [ ] TestRunner_GroupTimeout
- [ ] TestRunner_GlobalTimeout
- [ ] TestRunner_EnvironmentVariables

#### 複雑（9個、推定18-27時間）
- [ ] TestRunner_ExecuteGroup_ComplexErrorScenarios
- [ ] TestRunner_ExecuteAllWithPriority
- [ ] TestRunner_GroupPriority
- [ ] TestRunner_DependencyHandling ⚠️ 最も複雑
- [ ] TestRunner_PrivilegedCommand
- [ ] TestRunner_SecurityValidation
- [ ] TestRunner_OutputCaptureErrorCategorization
- [ ] TestRunner_OutputCaptureErrorHandlingStages
- [ ] TestRunner_SecurityIntegration

### Phase 3: 最終化
- [ ] skip_integration_tests タグの削除
- [ ] 全テスト実行確認
- [ ] Lint確認
- [ ] 古い型定義の削除検討

## よくある質問

### Q1: なぜこんなに大規模な移行が必要なのか？

A: Task 0035（Spec/Runtime分離）で型システムを刷新しましたが、`runner_test.go`は古い型を大量に使用していたため、段階的な移行が必要です。

### Q2: すべてを一度に変更できないのか？

A: 2569行、21個のテスト関数を一度に変更するのはリスクが高すぎます。段階的移行により、各ステップで動作を確認できます。

### Q3: 移行中に既存の機能が壊れないか？

A: 各テスト関数の移行後にテストを実行し、既存の動作が保たれていることを確認します。

### Q4: どのくらいの時間がかかるのか？

A: 推定2-3日（16-24時間）です。詳細は[実装計画書](./02_implementation_plan.md)のマイルストーンを参照してください。

### Q5: 他のタスクと並行して進められるか？

A: 可能ですが、`internal/runner/runner.go`や関連ファイルの変更とコンフリクトする可能性があります。できれば集中して進めることを推奨します。

## トラブルシューティング

### コンパイルエラー: `cannot use config (variable of type *runnertypes.ConfigSpec) as *runnertypes.Config`

**原因**: 古い型が残っている

**解決策**: すべての `Config` を `ConfigSpec` に変更

### テスト失敗: `not enough arguments in call to runner.ExecuteGroup`

**原因**: `ExecuteGroup()` のシグネチャが変更された

**解決策**: `RuntimeGlobal` を追加で渡す（詳細は移行ガイド参照）

### フィールドエラー: `unknown field 'Dir' in struct literal`

**原因**: フィールド名が変更された

**解決策**: `Dir` → `WorkDir` に変更

## 参考コマンド

```bash
# 特定のテストのみ実行
go test -tags test -v ./internal/runner -run TestNewRunner

# すべてのテスト実行
make test

# Lint実行
make lint

# 型の使用箇所を検索
grep -r "runnertypes\.Config[^S]" internal/runner/

# コミット前の差分確認
git diff internal/runner/runner_test.go | head -100

# 進捗確認（変更行数）
git diff --stat internal/runner/runner_test.go
```

## 成功基準

✅ すべてのチェックリストが完了
✅ `make test` で全テスト PASS
✅ `make lint` でエラーなし
✅ `skip_integration_tests` タグが削除されている
✅ カバレッジが低下していない

## 次のステップ

このタスクが完了したら：
1. **Task 0037**: 残りの統合テスト（output_capture_integration_test.go等）の移行
2. **Task 0038**: テストインフラの最終整備

## 関連資料

- [Task 0035: Spec/Runtime Separation](../0035_spec_runtime_separation/)
- [テスト再有効化計画](../0035_spec_runtime_separation/test_reactivation_plan.md)
- [group_executor_test.go 移行例](../../../internal/runner/group_executor_test.go)
