# Task 0038: テストインフラの最終整備

## 概要

Task 0035（Spec/Runtime分離）、Task 0036（runner_test.go型移行）、Task 0037（統合テスト型移行）で実施した変更を統合し、テストインフラを最終的に整備するタスク。

## 目的

1. すべての統合テストが新しい型システムで動作する状態にする
2. 古い型定義を完全に削除する
3. CI/CDパイプラインでのテスト実行を確認する
4. テストカバレッジとドキュメントを最新化する

## 前提条件

以下のタスクが完了していること：

- [x] Task 0035: Spec/Runtime分離（Phase 1-8）
- [x] Task 0036: runner_test.go の型移行（詳細ドキュメント完成、実装は進行中）
- [x] Task 0037: 残りの統合テストの型移行（1/3完了）

## 現状分析

### 完了済みテストファイル

| ファイル | 行数 | 状態 | テスト数 |
|---------|------|------|---------|
| `internal/runner/group_executor_test.go` | 696 | ✅ 完了 | 7/10 PASS |
| `internal/runner/output_capture_integration_test.go` | 265 | ✅ 完了 | 2/2 PASS |

### 移行作業中のファイル

| ファイル | 行数 | 状態 | 推定残工数 |
|---------|------|------|-----------|
| `internal/runner/runner_test.go` | 2569 | 📋 ガイド完成 | 16-24時間 |
| `test/performance/output_capture_test.go` | 411 | 📋 ガイド完成 | 4-6時間 |
| `test/security/output_security_test.go` | 535 | 📋 ガイド完成 | 6-8時間 |

### skip_integration_tests タグの状態

```bash
# 現在のタグ使用状況
$ grep -r "skip_integration_tests" --include="*.go" | grep -v ".git"
internal/runner/runner_test.go://go:build skip_integration_tests
test/performance/output_capture_test.go://go:build skip_integration_tests
test/security/output_security_test.go://go:build skip_integration_tests
```

**残り3ファイル**がタグ付き（型移行が必要）

### 古い型定義の状態

`internal/runner/runnertypes/config.go`に以下の古い型が残存：

- `type Config struct`
- `type GlobalConfig struct`
- `type CommandGroup struct`
- `type Command struct`

これらは`runner_test.go`等で使用されているため、削除前に移行が必要。

## 作業計画

### Phase 1: 残りの統合テストの完全移行（26-38時間）

#### 1.1 runner_test.go の型移行（16-24時間、最優先）

**参照**: [Task 0036 詳細ドキュメント](../0036_runner_test_migration/)

**手順**:
1. ヘルパーメソッドの実装（2-3時間）
2. 21個のテスト関数を段階的に移行（12-18時間）
3. `skip_integration_tests`タグの削除（1時間）
4. 検証（2-3時間）

**成功基準**:
- [ ] 全21個のテスト関数が新しい型を使用
- [ ] `make test` で全テスト PASS
- [ ] `make lint` でエラーなし

#### 1.2 test/performance/output_capture_test.go の型移行（4-6時間）

**参照**: [Task 0037 詳細ドキュメント](../0037_remaining_integration_tests/)

**主な変更**:
- `PrepareCommand()` 削除への対応
- `Command` → `RuntimeCommand`
- `CommandGroup` → `GroupSpec`
- パフォーマンス測定コードの動作確認

**成功基準**:
- [ ] 全パフォーマンステストが PASS
- [ ] ベンチマーク結果が以前と同等
- [ ] メモリリークがない

#### 1.3 test/security/output_security_test.go の型移行（6-8時間）

**参照**: [Task 0037 詳細ドキュメント](../0037_remaining_integration_tests/)

**主な変更**:
- `Command` → `RuntimeCommand`（100-150箇所）
- セキュリティバリデーションAPIの更新
- セキュリティテストの動作確認

**成功基準**:
- [ ] 全セキュリティテストが PASS
- [ ] セキュリティバリデーションが正しく機能
- [ ] 脆弱性テストケースが期待通り動作

### Phase 2: 古い型定義の削除（2-4時間）

#### 2.1 使用箇所の最終確認

```bash
# Config の使用箇所を確認
grep -r "runnertypes\.Config[^S]" --include="*.go" --exclude-dir=".git"

# GlobalConfig の使用箇所を確認
grep -r "runnertypes\.GlobalConfig" --include="*.go" --exclude-dir=".git"

# CommandGroup の使用箇所を確認
grep -r "runnertypes\.CommandGroup" --include="*.go" --exclude-dir=".git"

# Command の使用箇所を確認（Spec除外）
grep -r "runnertypes\.Command[^S]" --include="*.go" --exclude-dir=".git"
```

**期待結果**: すべての検索で0件

#### 2.2 古い型の削除

`internal/runner/runnertypes/config.go`から以下を削除：

```go
// 削除対象
type Config struct { ... }
type GlobalConfig struct { ... }
type CommandGroup struct { ... }
type Command struct { ... }

// 関連するメソッドも削除
func PrepareCommand(...) { ... }
func (c *Config) Validate() error { ... }
// など
```

**注意**:
- 削除前に必ずバックアップを取る
- 段階的にコミット（型ごとに分けて削除可能）

#### 2.3 検証

```bash
# コンパイル確認
go build ./...

# テスト実行
make test

# Lint確認
make lint
```

### Phase 3: CI/CD パイプラインの整備（4-6時間）

#### 3.1 GitHub Actions ワークフローの確認

現在の`.github/workflows/`ディレクトリを確認し、統合テストの実行が含まれているか確認。

**確認項目**:
- [ ] `make test` が実行されているか
- [ ] テストタイムアウトが適切か
- [ ] 並列実行の設定が適切か

#### 3.2 テストタグの整理

`skip_integration_tests`タグが完全に削除されたことを確認：

```bash
# タグの検索（0件であるべき）
grep -r "skip_integration_tests" --include="*.go"

# build タグのみのファイルを確認
grep -r "//go:build" --include="*.go" | grep -v "test"
```

#### 3.3 テスト実行時間の最適化

統合テスト実行時間を測定し、必要に応じて最適化：

```bash
# テスト実行時間の測定
go test -v ./... 2>&1 | grep "PASS\|FAIL" | awk '{print $NF}' | sort -rn | head -20

# 遅いテストの特定と最適化検討
```

### Phase 4: テストカバレッジの確認と改善（2-4時間）

#### 4.1 カバレッジレポートの生成

```bash
# カバレッジレポート生成
go test -tags test -coverprofile=coverage.out ./...

# HTML レポート生成
go tool cover -html=coverage.out -o coverage.html

# カバレッジ率の確認
go tool cover -func=coverage.out | grep total
```

**目標カバレッジ**: 80%以上

#### 4.2 カバレッジ低下箇所の特定

型移行によってカバレッジが低下していないか確認：

```bash
# パッケージ別カバレッジ
go test -tags test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -E "internal/runner"
```

**カバレッジが低下している場合**:
- 削除されたテストがないか確認
- 新しい型でカバーされていないパスがないか確認
- 必要に応じてテストケースを追加

#### 4.3 カバレッジバッジの更新

README.md等にカバレッジバッジがある場合は更新。

### Phase 5: ドキュメントの最終更新（2-3時間）

#### 5.1 テスト関連ドキュメントの更新

以下のドキュメントを最新化：

- [ ] `README.md` - テスト実行手順
- [ ] `docs/testing.md`（存在する場合）
- [ ] `CONTRIBUTING.md`（存在する場合）

#### 5.2 移行完了の記録

`docs/tasks/0035_spec_runtime_separation/test_reactivation_plan.md`を最終更新：

```markdown
## 進捗状況

- [x] Phase 5: types_test.go 有効化
- [x] Phase 6: Resource Manager テスト有効化 (10/10 完了)
- [x] Phase 7: Executor テスト有効化 (2/2 完了)
- [x] Phase 8: 統合テスト完全有効化 (5/5 完了)
  - ✅ group_executor_test.go
  - ✅ output_capture_integration_test.go
  - ✅ runner_test.go
  - ✅ test/performance/output_capture_test.go
  - ✅ test/security/output_security_test.go
- [x] Task 0036: runner_test.go の型移行（完了）
- [x] Task 0037: 残りの統合テストの型移行（完了）
- [x] Task 0038: テストインフラの最終整備（完了）

## 最終状態

すべての統合テストが新しい型システム（Spec/Runtime分離）で動作しています。
古い型定義（Config, GlobalConfig, CommandGroup, Command）は完全に削除されました。
```

#### 5.3 CHANGELOG.md の更新（存在する場合）

```markdown
## [Unreleased]

### Changed
- **[BREAKING]** Spec/Runtime分離による型システム刷新
  - `Config` → `ConfigSpec`
  - `GlobalConfig` → `GlobalSpec`
  - `CommandGroup` → `GroupSpec`
  - `Command` → `CommandSpec` / `RuntimeCommand`
- すべての統合テストを新しい型システムに移行

### Removed
- 古い型定義（Config, GlobalConfig, CommandGroup, Command）
- `PrepareCommand()` メソッド
- `skip_integration_tests` ビルドタグ
```

## チェックリスト

### Phase 1: 統合テストの完全移行
- [ ] `runner_test.go` の型移行完了
- [ ] `test/performance/output_capture_test.go` の型移行完了
- [ ] `test/security/output_security_test.go` の型移行完了
- [ ] すべてのテストが PASS
- [ ] `skip_integration_tests` タグをすべて削除

### Phase 2: 古い型定義の削除
- [ ] 古い型の使用箇所が0件であることを確認
- [ ] `Config`, `GlobalConfig`, `CommandGroup`, `Command` を削除
- [ ] 関連するメソッド（`PrepareCommand()`等）を削除
- [ ] コンパイル成功
- [ ] 全テスト PASS

### Phase 3: CI/CD パイプライン
- [ ] GitHub Actions ワークフローを確認・更新
- [ ] テストタグの整理完了
- [ ] テスト実行時間を測定・最適化

### Phase 4: カバレッジ
- [ ] カバレッジレポート生成
- [ ] カバレッジ80%以上を達成
- [ ] カバレッジ低下箇所がないことを確認

### Phase 5: ドキュメント
- [ ] README.md 更新
- [ ] test_reactivation_plan.md 最終更新
- [ ] CHANGELOG.md 更新（存在する場合）
- [ ] 移行完了を記録

## 推定スケジュール

| Phase | 内容 | 推定工数 | 累積 |
|-------|------|---------|------|
| Phase 1 | 統合テストの完全移行 | 26-38時間 | 26-38h |
| Phase 2 | 古い型定義の削除 | 2-4時間 | 28-42h |
| Phase 3 | CI/CD パイプライン整備 | 4-6時間 | 32-48h |
| Phase 4 | カバレッジ確認と改善 | 2-4時間 | 34-52h |
| Phase 5 | ドキュメント最終更新 | 2-3時間 | 36-55h |
| **合計** | | **36-55時間** | **5-7日** |

## リスクと対策

### リスク1: テスト移行中の予期しない問題

**対策**:
- 段階的移行（1ファイルずつ）
- 各ステップで動作確認
- 問題発生時は一旦元に戻して原因調査

### リスク2: カバレッジの低下

**対策**:
- 移行前後でカバレッジを比較
- 低下箇所を特定してテストケース追加
- カバレッジ目標（80%）を明確に設定

### リスク3: CI/CDの実行時間増加

**対策**:
- テスト実行時間を監視
- 並列実行の活用
- 遅いテストの特定と最適化

## 成功基準

1. ✅ すべての統合テストが新しい型システムで動作
2. ✅ `skip_integration_tests` タグが完全に削除
3. ✅ 古い型定義が完全に削除
4. ✅ `make test` で全テスト PASS
5. ✅ `make lint` でエラーなし
6. ✅ テストカバレッジ80%以上
7. ✅ CI/CDパイプラインが正常動作
8. ✅ ドキュメントが最新状態

## 次のステップ

Task 0038完了後：

1. **リリース準備**:
   - バージョン番号の更新
   - リリースノートの作成
   - マイグレーションガイドの作成（ユーザー向け）

2. **パフォーマンス最適化**:
   - プロファイリング実施
   - ボトルネックの特定と改善

3. **新機能開発**:
   - TempDir機能の実装
   - その他積み残し機能

## 参考資料

- [Task 0035: Spec/Runtime分離](../0035_spec_runtime_separation/)
- [Task 0036: runner_test.go型移行](../0036_runner_test_migration/)
- [Task 0037: 残りの統合テスト型移行](../0037_remaining_integration_tests/)
- [テスト再有効化計画](../0035_spec_runtime_separation/test_reactivation_plan.md)
