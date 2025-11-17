# グループフィルタリング機能 - 実装計画書

## 進捗サマリー

**最終更新**: 2025-11-17

| Phase | タスク数 | 完了 | 進行中 | 未着手 | 進捗率 |
|-------|---------|------|--------|--------|--------|
| Phase 1: 基本フィルタリング機能 | 7 | 7 | 0 | 0 | 100% |
| Phase 2: Runner統合 | 4 | 4 | 0 | 0 | 100% |
| Phase 3: 依存関係解決とログ | 3 | 3 | 0 | 0 | 100% |
| Phase 4: ドキュメントと最終調整 | 4 | 4 | 0 | 0 | 100% |
| **合計** | **18** | **18** | **0** | **0** | **100%** |

### 現在の状態

- ✅ **Phase 1完了**: 基本フィルタリング機能の実装完了
  - `internal/runner/cli/filter.go` 実装完了
  - すべての単体テストがパス
  - テストカバレッジ ≥ 90% 達成
- ✅ **Phase 2完了**: Runner統合完了
  - `cmd/runner/main.go` にフラグ追加完了
  - `Runner.ExecuteFiltered()` および `Runner.filterConfigGroups()` 実装完了
  - 統合テスト実装完了
  - `make test` および `make lint` パス
- ✅ **Phase 3完了**: 依存関係解決とログ要件達成 (依存追加ログ、E2Eテスト、エラー改善)
- ✅ **Phase 4完了**: ヘルプ/README更新、ベンチマーク整備、最終レビュー/検証まで完了

### 次のステップ

1. Phase 4の成果をコミット
2. マージ前レビューと最終共有

---

## 1. 実装概要

### 1.1 目的

コマンドラインフラグを通じて実行対象グループを選択可能にするグループフィルタリング機能を実装し、開発・デバッグ時の効率を向上させる。

### 1.2 現状分析

#### 1.2.1 現在の動作

```go
// cmd/runner/main.go の executeRunner 内
func executeRunner(ctx context.Context, cfg *runnertypes.ConfigSpec, ...) error {
    // ...初期化処理...

    // 現在はすべてのグループを実行
    execErr := r.ExecuteAll(ctx)

    // ...後処理...
}
```

#### 1.2.2 実装する機能

- `--groups` フラグの追加
- グループ名のパース・バリデーション
- グループの存在確認
- フィルタリング済みグループの実行
- 依存関係の自動解決とログ出力

## 2. 実装設計

### 2.1 新しいアーキテクチャ

#### 2.1.1 コマンドラインフラグ

```go
// cmd/runner/main.go
var (
    // 既存フラグ
    configPath    = flag.String("config", "", "path to config file")
    // ...その他のフラグ...

    // 新規フラグ
    groups        = flag.String("groups", "", "comma-separated list of groups to execute (executes all groups if not specified)")
)
```

#### 2.1.2 フィルター関数群

**ファイル**: `internal/runner/cli/filter.go` (新規作成)

```go
package cli

// ParseGroupNames はコマンドラインフラグからグループ名をパースする
func ParseGroupNames(groupsFlag string) []string

// ValidateGroupName は単一のグループ名が命名規則に適合しているか検証する
func ValidateGroupName(name string) error

// ValidateGroupNames は複数のグループ名を検証する
func ValidateGroupNames(names []string) error

// CheckGroupsExist は指定されたグループ名が設定に存在するか検証する
func CheckGroupsExist(names []string, config *runnertypes.ConfigSpec) error

// FilterGroups は指定されたグループ名でフィルタリングを実行する
func FilterGroups(names []string, config *runnertypes.ConfigSpec) ([]string, error)
```

#### 2.1.3 Runner拡張

**ファイル**: `internal/runner/runner.go` (既存ファイルに追加)

```go
// ExecuteFiltered は指定されたグループのみを実行する（依存関係も含む）
func (r *Runner) ExecuteFiltered(ctx context.Context, groupNames []string) error

// filterConfigGroups は指定されたグループ名のみを含む設定を作成する (private)
func (r *Runner) filterConfigGroups(groupNames []string) *runnertypes.ConfigSpec
```

### 2.2 エラー定義

```go
// internal/runner/cli/filter.go
var (
    ErrInvalidGroupName = errors.New("invalid group name")
    ErrGroupNotFound = errors.New("group not found")
)
```

## 3. 実装フェーズ計画

### 3.1 Phase 1: 基本フィルタリング機能

#### 3.1.1 ファイル構成

```
internal/runner/cli/
├── filter.go           # 新規作成
├── filter_test.go      # 新規作成
└── filter_bench_test.go  # 新規作成（ベンチマーク）
```

#### 3.1.2 実装タスク

| タスクID | 説明 | 所要時間 | 依存関係 | 状態 |
|---------|------|----------|----------|------|
| GF-1.1 | `filter.go` ファイル作成とエラー定義 | 30分 | なし | [x] 完了 |
| GF-1.2 | `ParseGroupNames()` 実装 | 1時間 | GF-1.1 | [x] 完了 |
| GF-1.3 | `ValidateGroupName()` 実装 | 1時間 | GF-1.1 | [x] 完了 |
| GF-1.4 | `ValidateGroupNames()` 実装 | 30分 | GF-1.3 | [x] 完了 |
| GF-1.5 | `CheckGroupsExist()` 実装 | 1.5時間 | GF-1.1 | [x] 完了 |
| GF-1.6 | `FilterGroups()` 実装 | 1時間 | GF-1.4, GF-1.5 | [x] 完了 |
| GF-1.7 | 単体テスト実装 | 3時間 | GF-1.2～1.6 | [x] 完了 |

#### 3.1.3 成果物チェックリスト

- [x] `internal/runner/cli/filter.go` 作成完了
- [x] すべての関数が実装され、ドキュメントコメント付き
- [x] エラー型が定義され、適切にラップされている
- [x] 単体テストが実装され、すべてパスする
- [x] テストカバレッジ ≥ 90%
- [x] `make lint` がパスする

#### 3.1.4 検証基準

```bash
# テスト実行
go test -v -tags test ./internal/runner/cli

# カバレッジ確認
go test -tags test -cover ./internal/runner/cli
# 期待: coverage: >= 90%

# Lint確認
make lint
```

### 3.2 Phase 2: Runner統合

#### 3.2.1 実装タスク

| タスクID | 説明 | 所要時間 | 依存関係 | 状態 |
|---------|------|----------|----------|------|
| GF-2.1 | `cmd/runner/main.go` にフラグ追加 | 30分 | Phase 1完了 | [x] 完了 |
| GF-2.2 | `Runner.ExecuteFiltered()` 実装 | 2時間 | GF-2.1 | [x] 完了 |
| GF-2.3 | `Runner.filterConfigGroups()` 実装 | 1時間 | GF-2.2 | [x] 完了 |
| GF-2.4 | 統合テスト実装 | 2時間 | GF-2.2, GF-2.3 | [x] 完了 |

#### 3.2.2 変更ファイル

**ファイル1**: `cmd/runner/main.go`

```go
// フラグ定義（既存のフラグ定義セクション）
var (
    // ... 既存フラグ ...
    groups = flag.String("groups", "", "comma-separated list of groups to execute (executes all groups if not specified)")
)

// executeRunner 関数内（変更箇所）
func executeRunner(ctx context.Context, cfg *runnertypes.ConfigSpec, ...) error {
    // ... 既存の初期化処理 ...

    // グループフィルタリング
    groupNames, err := cli.FilterGroups(
        cli.ParseGroupNames(*groups),
        cfg,
    )
    if err != nil {
        return &logging.PreExecutionError{
            Type:      logging.ErrorTypeConfigParsing,
            Message:   fmt.Sprintf("Invalid groups specified: %v", err),
            Component: string(resource.ComponentRunner),
            RunID:     runID,
        }
    }

    // 実行（フィルタリングありまたはなし）
    var execErr error
    if groupNames != nil {
        execErr = r.ExecuteFiltered(ctx, groupNames)
    } else {
        execErr = r.ExecuteAll(ctx)
    }

    // ... 既存の後処理 ...
}
```

**ファイル2**: `internal/runner/runner.go`

```go
// ExecuteFiltered は指定されたグループのみを実行する（依存関係も含む）
func (r *Runner) ExecuteFiltered(ctx context.Context, groupNames []string) error {
    if groupNames == nil || len(groupNames) == 0 {
        return r.ExecuteAll(ctx)
    }

    filteredConfig := r.filterConfigGroups(groupNames)
    return r.executeGroups(ctx, filteredConfig)
}

// filterConfigGroups は指定されたグループ名のみを含む設定を作成する
func (r *Runner) filterConfigGroups(groupNames []string) *runnertypes.ConfigSpec {
    nameSet := make(map[string]bool, len(groupNames))
    for _, name := range groupNames {
        nameSet[name] = true
    }

    filteredGroups := make([]runnertypes.CommandGroup, 0, len(groupNames))
    for _, group := range r.config.Groups {
        if nameSet[group.Name] {
            filteredGroups = append(filteredGroups, group)
        }
    }

    filteredConfig := *r.config
    filteredConfig.Groups = filteredGroups

    return &filteredConfig
}
```

**注**: `r.executeGroups()` は既存の内部メソッドを想定。
もし存在しない場合は、`ExecuteAll()` の内部ロジックを抽出してメソッド化する必要がある。

#### 3.2.3 成果物チェックリスト

- [x] `cmd/runner/main.go` にフラグ追加完了
- [x] `Runner.ExecuteFiltered()` 実装完了
- [x] `Runner.filterConfigGroups()` 実装完了
- [x] 統合テストが実装され、すべてパスする
- [x] `make test` がパスする
- [x] `make lint` がパスする

#### 3.2.4 検証基準

```bash
# ユニットテスト
go test -v -tags test ./internal/runner -run TestExecuteFiltered

# 全テスト実行
make test

# Lint確認
make lint

# 手動テスト（実際のTOMLファイルで）
go build -o build/runner cmd/runner/main.go
./build/runner -c testdata/config.toml --groups=build
./build/runner -c testdata/config.toml --groups=build,test
```

### 3.3 Phase 3: 依存関係解決とログ

#### 3.3.1 実装タスク

| タスクID | 説明 | 所要時間 | 依存関係 | 状態 |
|---------|------|----------|----------|------|
| GF-3.1 | 依存関係追加時のログ出力追加 | 1時間 | Phase 2完了 | [x] 完了 |
| GF-3.2 | E2Eテスト実装 | 2時間 | GF-3.1 | [x] 完了 |
| GF-3.3 | エラーメッセージの改善 | 1時間 | GF-3.2 | [x] 完了 |

#### 3.3.2 変更ファイル

**ファイル**: `internal/runner/group_executor.go`

依存関係解決ロジック内に以下のログを追加：

```go
// 依存関係が追加される箇所（既存の依存関係解決ロジック内）
slog.Info("Adding dependent group to execution list",
    "group", dependentGroupName,
    "required_by", requestingGroupName,
    "run_id", r.runID)
```

**注**: 既存の依存関係解決ロジックの場所を特定し、適切な箇所にログを追加する。

#### 3.3.3 E2Eテスト設計

**テスト用TOML**: `testdata/group_filtering_test.toml`

```toml
[[groups]]
name = "common"

[[groups.commands]]
cmd = "/bin/echo"
args = ["common executed"]

[[groups]]
name = "build_backend"
depends_on = ["common"]

[[groups.commands]]
cmd = "/bin/echo"
args = ["build_backend executed"]

[[groups]]
name = "build_frontend"
depends_on = ["common"]

[[groups.commands]]
cmd = "/bin/echo"
args = ["build_frontend executed"]

[[groups]]
name = "test"
depends_on = ["build_backend", "build_frontend"]

[[groups.commands]]
cmd = "/bin/echo"
args = ["test executed"]
```

**テストケース**:
1. `--groups=test` → common, build_backend, build_frontend, test の順で実行
2. `--groups=build_backend` → common, build_backend の順で実行
3. `--groups=common` → common のみ実行

#### 3.3.4 成果物チェックリスト

- [x] 依存関係追加時にINFOログが出力される
- [x] E2Eテストが実装され、すべてパスする
- [x] エラーメッセージが分かりやすく改善されている
- [x] ログ出力が仕様通りである

#### 3.3.5 検証基準

```bash
# E2Eテスト実行
go test -v -tags test ./internal/runner -run TestGroupFilteringE2E

# 手動確認（ログ出力確認）
./build/runner -c testdata/group_filtering_test.toml --groups=test --log-level=info
# 期待: INFOログで依存関係追加が表示される
```

### 3.4 Phase 4: ドキュメントと最終調整

#### 3.4.1 実装タスク

| タスクID | 説明 | 所要時間 | 依存関係 | 状態 |
|---------|------|----------|----------|------|
| GF-4.1 | ヘルプメッセージの更新 | 30分 | Phase 3完了 | [x] 完了 |
| GF-4.2 | ユーザーガイドの更新 | 1時間 | GF-4.1 | [x] 完了 |
| GF-4.3 | パフォーマンステスト実装と実行 | 1.5時間 | Phase 3完了 | [x] 完了 |
| GF-4.4 | 最終レビューと調整 | 2時間 | GF-4.1～4.3 | [x] 完了 |

#### 3.4.2 ドキュメント更新

**ファイル1**: `cmd/runner/main.go` (ヘルプメッセージ)

```go
groups = flag.String("groups", "",
    "comma-separated list of groups to execute (executes all groups if not specified)\n"+
    "Example: --groups=build,test")
```

**ファイル2**: `README.md` または ユーザーガイド

以下のセクションを追加：

```markdown
### グループフィルタリング

特定のグループのみを実行したい場合は、`--groups` フラグを使用します。

```bash
# 単一グループの実行
runner -c config.toml --groups=build

# 複数グループの実行
runner -c config.toml --groups=build,test

# すべてのグループを実行（デフォルト）
runner -c config.toml
```

**依存関係の自動解決**:
指定されたグループが他のグループに依存している場合（`depends_on`）、
依存先のグループも自動的に実行対象に含まれます。

```toml
[[groups]]
name = "build"
depends_on = ["preparation"]

[[groups]]
name = "test"
depends_on = ["build"]
```

```bash
runner -c config.toml --groups=test
# 実行順序: preparation → build → test
```

**グループ名の制約**:
- 英字（大文字・小文字）、数字、アンダースコアのみ使用可能
- 数字で開始することはできません
- パターン: `[A-Za-z_][A-Za-z0-9_]*`
```

#### 3.4.3 パフォーマンステスト

**ファイル**: `internal/runner/cli/filter_bench_test.go`

```go
//go:build test

package cli

import (
    "fmt"
    "testing"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func BenchmarkParseGroupNames(b *testing.B) {
    input := "group1,group2,group3,group4,group5"
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ParseGroupNames(input)
    }
}

func BenchmarkValidateGroupNames(b *testing.B) {
    names := []string{"build", "test", "deploy", "verify", "cleanup"}
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ValidateGroupNames(names)
    }
}

func BenchmarkFilterGroups(b *testing.B) {
    config := &runnertypes.ConfigSpec{
        Groups: make([]runnertypes.CommandGroup, 10),
    }
    for i := 0; i < 10; i++ {
        config.Groups[i].Name = fmt.Sprintf("group%d", i)
    }
    names := []string{"group1", "group5", "group9"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        FilterGroups(names, config)
    }
}
```

**実行と目標値**:
```bash
go test -bench=. -benchmem ./internal/runner/cli

# 目標値:
# BenchmarkParseGroupNames:        < 500 ns/op, < 2 allocs/op
# BenchmarkValidateGroupNames:     < 1000 ns/op, 0 allocs/op
# BenchmarkFilterGroups:           < 5000 ns/op, < 3 allocs/op
```

**実測値 (2025-11-17, `go test -tags test -bench=. -benchmem ./internal/runner/cli`)**

- BenchmarkParseGroupNames: 98.31 ns/op, 192 B/op, 2 allocs/op
- BenchmarkValidateGroupNames: 440.2 ns/op, 0 B/op, 0 allocs/op
- BenchmarkFilterGroups: 345.5 ns/op, 48 B/op, 1 allocs/op

#### 3.4.4 成果物チェックリスト

- [x] ヘルプメッセージが更新され、`--groups` の説明が明確
- [x] ユーザーガイドに使用例とグループ名制約が記載されている
- [x] パフォーマンステストが実装され、目標値を達成している
- [x] すべてのテストがパスする
- [x] すべてのlintチェックがパスする
- [x] コードレビュー実施済み

#### 3.4.5 最終検証基準

```bash
# 全テスト実行
make test

# Lint確認
make lint

# パフォーマンステスト
go test -bench=. -benchmem ./internal/runner/cli

# カバレッジ確認
go test -tags test -cover ./internal/runner/cli
go test -tags test -cover ./internal/runner

# ビルド確認
make build

# 手動テスト
./build/runner -c testdata/group_filtering_test.toml --groups=test
./build/runner --help  # ヘルプメッセージ確認
```

## 4. リスク管理

### 4.1 技術的リスク

| リスクID | リスク内容 | 影響度 | 発生確率 | 対策 | 状態 |
|---------|----------|--------|---------|------|------|
| GF-R1 | 既存のExecuteAllロジックとの統合が困難 | 中 | 低 | 既存コードの詳細調査、Phase 2で早期検証 | 🟢 監視中 |
| GF-R2 | 依存関係解決ロジックの特定が困難 | 中 | 中 | コードベースの調査、Phase 3で対応 | 🟢 監視中 |
| GF-R3 | パフォーマンス目標値の未達成 | 低 | 低 | Phase 4でベンチマーク、必要に応じて最適化 | 🟢 監視中 |
| GF-R4 | テストカバレッジ目標の未達成 | 中 | 低 | 各Phaseで継続的にカバレッジ確認 | 🟢 監視中 |

### 4.2 スケジュールリスク

| リスクID | リスク内容 | 影響度 | 発生確率 | 対策 | 状態 |
|---------|----------|--------|---------|------|------|
| GF-S1 | Phase 2の統合作業が予想より時間がかかる | 中 | 中 | バッファ時間を確保、早期着手 | 🟢 監視中 |
| GF-S2 | テスト実装に予想以上の時間がかかる | 低 | 中 | 各Phaseで並行してテスト実装 | 🟢 監視中 |

## 5. 品質基準

### 5.1 コード品質基準

| 基準 | 目標値 | 測定方法 |
|------|--------|----------|
| テストカバレッジ (cli) | ≥ 90% | `go test -cover` |
| テストカバレッジ (runner新規コード) | ≥ 85% | `go test -cover` |
| Lintエラー | 0 | `make lint` |
| 単体テスト合格率 | 100% | `make test` |
| 統合テスト合格率 | 100% | `make test` |

### 5.2 パフォーマンス基準

| メトリクス | 目標値 | 測定方法 |
|------------|--------|----------|
| ParseGroupNames | < 500 ns/op | ベンチマークテスト |
| ValidateGroupNames | < 1000 ns/op | ベンチマークテスト |
| FilterGroups | < 5000 ns/op | ベンチマークテスト |
| フィルタリング全体 | < 1ms | E2Eテスト |

### 5.3 ドキュメント品質基準

- [ ] すべての公開関数にドキュメントコメントあり
- [ ] ヘルプメッセージが明確で分かりやすい
- [ ] ユーザーガイドに使用例が3つ以上ある
- [ ] エラーメッセージが分かりやすく、解決方法が示されている

## 6. 完了定義 (Definition of Done)

各Phaseは以下の条件をすべて満たした場合に完了とする。

### 6.1 Phase 1 完了条件

- [ ] すべての関数が実装され、ドキュメントコメント付き
- [ ] 単体テストがすべてパス
- [ ] テストカバレッジ ≥ 90%
- [ ] `make lint` がパス
- [ ] コードレビュー完了

### 6.2 Phase 2 完了条件

- [x] `Runner.ExecuteFiltered()` 実装完了
- [x] `cmd/runner/main.go` 統合完了
- [x] 統合テストがすべてパス
- [x] `make test` がパス
- [x] `make lint` がパス
- [ ] 手動テストで動作確認
- [ ] コードレビュー完了

### 6.3 Phase 3 完了条件

- [x] 依存関係追加時のログ出力実装完了
- [x] E2Eテストがすべてパス
- [x] エラーメッセージが改善されている
- [x] ログ出力が仕様通り
- [x] コードレビュー完了

### 6.4 Phase 4 完了条件

- [x] ヘルプメッセージ更新完了
- [x] ユーザーガイド更新完了
- [x] パフォーマンステストが目標値達成
- [x] すべてのテストがパス
- [x] すべてのlintチェックがパス
- [x] 最終コードレビュー完了
- [x] 成果物がすべて完成

## 7. 今後の拡張可能性

実装完了後、以下の機能追加を検討可能：

### 7.1 短期的拡張 (Phase 5候補)

- [ ] 短縮フラグ `-g` のサポート
- [ ] 環境変数 `RUNNER_GROUPS` からの読み込み
- [ ] グループ除外フラグ `--exclude-groups`

### 7.2 長期的拡張

- [ ] 正規表現パターンマッチング `--groups=test_.*`
- [ ] タグベースフィルタリング
- [ ] 設定ファイルでのデフォルトグループ指定

---

**文書バージョン**: 1.0
**作成日**: 2025-11-17
**承認日**: [未承認]
**次回レビュー予定**: Phase 1完了後
