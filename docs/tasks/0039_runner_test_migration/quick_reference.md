# Task 0039: クイックリファレンス

Task 0039 (runner_test.go型移行) でよく使うコマンドと手順のクイックリファレンス。

## 基本的な確認コマンド

### ファイル情報の確認

```bash
# ファイルサイズと行数
wc -l internal/runner/runner_test.go

# テスト関数の数
grep -c "^func Test" internal/runner/runner_test.go

# テスト関数の一覧
grep "^func Test" internal/runner/runner_test.go

# ビルドタグの確認
head -5 internal/runner/runner_test.go
```

### コンパイルエラーの確認

```bash
# ビルドタグを無視してコンパイル（エラー確認用）
go test -tags test ./internal/runner -run TestNewRunner 2>&1 | head -50

# 全エラーをファイルに保存
go test -tags test ./internal/runner 2>&1 > compile_errors.log

# エラー数をカウント
go test -tags test ./internal/runner 2>&1 | grep -c "has the problem"
```

### 特定のパターンの検索

```bash
# EffectiveWorkDir の使用箇所
grep -n "EffectiveWorkDir" internal/runner/runner_test.go

# TempDir の使用箇所
grep -n "TempDir" internal/runner/runner_test.go

# SetupFailedMockExecution の使用箇所
grep -n "SetupFailedMockExecution" internal/runner/runner_test.go

# CommandSpec の使用箇所
grep -n "CommandSpec{" internal/runner/runner_test.go

# GroupSpec の使用箇所
grep -n "GroupSpec{" internal/runner/runner_test.go
```

## Phase 1: 分析と設計

### エラー分類スクリプト

```bash
# コンパイルエラーを種類別に分類
go test -tags test ./internal/runner 2>&1 | \
  grep "has the problem" | \
  awk '{print $NF}' | \
  sort | uniq -c | sort -rn

# EffectiveWorkDir関連のエラー
go test -tags test ./internal/runner 2>&1 | \
  grep "EffectiveWorkDir"

# TempDir関連のエラー
go test -tags test ./internal/runner 2>&1 | \
  grep "TempDir"

# SetupFailedMockExecution関連のエラー
go test -tags test ./internal/runner 2>&1 | \
  grep "SetupFailedMockExecution"
```

### バックアップの作成

```bash
# 作業前のバックアップ
cp internal/runner/runner_test.go internal/runner/runner_test.go.$(date +%Y%m%d_%H%M%S)

# または
git stash save "Before Task 0039 - runner_test.go backup"
```

## Phase 2: 基盤整備

### モックの拡張

```bash
# MockResourceManager の確認
cat internal/runner/testing/mocks.go | grep -A 10 "type MockResourceManager"

# メソッド一覧
grep "^func (m \*MockResourceManager)" internal/runner/testing/mocks.go
```

### ヘルパー関数の実装場所

```bash
# runner_test.go 内にヘルパー関数を追加
# 例: createRuntimeCommand(), createTestConfigSpec() など

# 実装例
cat << 'EOF' >> internal/runner/runner_test.go

// Helper functions for test migration

func createRuntimeCommand(spec *runnertypes.CommandSpec) *runnertypes.RuntimeCommand {
    return &runnertypes.RuntimeCommand{
        Spec:             spec,
        ExpandedCmd:      spec.Cmd,
        ExpandedArgs:     spec.Args,
        ExpandedEnv:      make(map[string]string),
        ExpandedVars:     make(map[string]string),
        EffectiveWorkDir: "",
        EffectiveTimeout: 30,
    }
}
EOF
```

## Phase 3: 段階的移行

### 特定のテストのみ実行

```bash
# 1つのテスト関数のみ実行
go test -tags test -v ./internal/runner -run TestNewRunner

# 複数のテスト関数を実行（正規表現）
go test -tags test -v ./internal/runner -run "TestNewRunner|TestNewRunnerWithSecurity"

# 詳細なログ付き
go test -tags test -v ./internal/runner -run TestNewRunner 2>&1 | tee test_output.log
```

### 段階的なビルドタグ削除

```bash
# ビルドタグを一時的に削除（テスト用）
sed -i '1,2d' internal/runner/runner_test.go  # 最初の2行を削除

# 元に戻す
git checkout -- internal/runner/runner_test.go
```

### 型変換の一括置換（慎重に使用）

```bash
# EffectiveWorkdir を EffectiveWorkDir に修正
sed -i 's/EffectiveWorkdir/EffectiveWorkDir/g' internal/runner/runner_test.go

# 確認
git diff internal/runner/runner_test.go | head -50
```

### コミット戦略

```bash
# 3つのテストごとにコミット
git add internal/runner/runner_test.go
git commit -m "test: migrate TestNewRunner to new type system (Task 0039 Phase 3.1)"

# Phase完了時にコミット
git commit -m "test: complete Phase 3.1 - simple test migration (Task 0039)"
```

## Phase 4: 検証と最終調整

### 全テスト実行

```bash
# runner パッケージの全テスト
go test -tags test -v ./internal/runner

# 全プロジェクトのテスト
make test

# タイムアウトを延長
go test -tags test -v -timeout 30m ./internal/runner
```

### リント確認

```bash
# Lint実行
make lint

# runner_test.go のみ
golangci-lint run --build-tags test ./internal/runner/runner_test.go

# 詳細表示
golangci-lint run --build-tags test -v ./internal/runner/
```

### カバレッジ確認

```bash
# カバレッジレポート生成
make coverage

# 総カバレッジ確認
go tool cover -func=coverage.out | grep total

# runner パッケージのカバレッジ
go tool cover -func=coverage.out | grep "internal/runner/"

# HTMLレポート
go tool cover -html=coverage.out -o coverage.html
xdg-open coverage.html  # または open coverage.html (macOS)
```

### skip_integration_tests タグの完全削除

```bash
# タグが残っているファイルを確認（0件であるべき）
grep -r "skip_integration_tests" --include="*.go" | grep -v ".git"

# runner_test.go からタグを削除
sed -i '1,2d' internal/runner/runner_test.go

# または手動で編集
# 1行目の "//go:build skip_integration_tests" を削除
```

## トラブルシューティング

### コンパイルエラーが多すぎる場合

```bash
# エラーを段階的に修正
# 1. EffectiveWorkDir のエラーのみ修正
# 2. TempDir のエラーのみ修正
# 3. SetupFailedMockExecution のエラーのみ修正

# 各段階でコンパイル確認
go build ./internal/runner
```

### テストが失敗する場合

```bash
# 詳細なログ出力
go test -tags test -v ./internal/runner -run TestNewRunner 2>&1 | tee debug.log

# 特定のエラーをgrep
cat debug.log | grep "FAIL"
cat debug.log | grep "Error"

# スタックトレース確認
go test -tags test -v ./internal/runner -run TestNewRunner -trace=trace.out
go tool trace trace.out
```

### キャッシュクリア

```bash
# テストキャッシュクリア
go clean -testcache

# ビルドキャッシュクリア
go clean -cache

# 全てクリア
go clean -cache -testcache -modcache
```

## よく使う編集パターン

### EffectiveWorkDir の修正

```go
// Before (間違い)
expectedCmd := runnertypes.CommandSpec{
    Name: "test",
    Cmd: "echo",
    EffectiveWorkDir: "/tmp",  // CommandSpec にはこのフィールドがない
}

// After (正しい)
spec := runnertypes.CommandSpec{
    Name: "test",
    Cmd: "echo",
    WorkDir: "/tmp",  // Spec では WorkDir を使用
}
runtimeCmd := createRuntimeCommand(spec)
runtimeCmd.EffectiveWorkDir = "/tmp"  // RuntimeCommand で設定
```

### TempDir の代替実装

```go
// Before (TempDirフィールドは存在しない)
group := runnertypes.GroupSpec{
    Name: "test",
    TempDir: true,  // エラー
}

// After Option 1: テストをスキップ
t.Skip("TempDir feature not yet implemented (Task 0040)")

// After Option 2: WorkDir で代替
group := runnertypes.GroupSpec{
    Name: "test",
    WorkDir: t.TempDir(),  // Go標準のTempDirを使用
}
```

### SetupFailedMockExecution の実装

```go
// Before (未定義メソッド)
mockRM.SetupFailedMockExecution(errors.New("test error"))

// After (直接モック設定)
mockRM.On("ExecuteCommand", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
    Return(nil, errors.New("test error"))
```

## 進捗管理

### 進捗確認

```bash
# 完了したテスト数
grep "✅" docs/tasks/0039_runner_test_migration/progress.md | wc -l

# 残りのテスト数
grep "⏸️" docs/tasks/0039_runner_test_migration/progress.md | wc -l

# 現在のPhase
grep "状態.*進行中" docs/tasks/0039_runner_test_migration/progress.md
```

### 進捗記録

```bash
# progress.md を更新
# 各テスト完了後、Phase完了後に更新

# コミット
git add docs/tasks/0039_runner_test_migration/progress.md
git commit -m "docs: update Task 0039 progress - Phase X.Y completed"
```

## 緊急時のロールバック

```bash
# 最後のコミットを取り消し（ファイルは保持）
git reset --soft HEAD~1

# 最後のコミットを完全に取り消し
git reset --hard HEAD~1

# 特定のファイルのみ元に戻す
git checkout HEAD -- internal/runner/runner_test.go

# stash から復元
git stash list
git stash pop stash@{0}
```

## 便利なエイリアス（オプション）

```bash
# ~/.bashrc または ~/.zshrc に追加

# Task 0039 関連
alias t0039='cd ~/git/go-safe-cmd-runner && cat docs/tasks/0039_runner_test_migration/progress.md'
alias t0039test='cd ~/git/go-safe-cmd-runner && go test -tags test -v ./internal/runner -run'
alias t0039errors='cd ~/git/go-safe-cmd-runner && go test -tags test ./internal/runner 2>&1 | grep -A 2 "has the problem"'

# 一般的なテストコマンド
alias gtest='go test -tags test -v'
alias gtestall='make test'
alias gcov='make coverage && go tool cover -func=coverage.out | grep total'
```

## チェックリスト

### Phase開始前

- [ ] `docs/tasks/0039_runner_test_migration/README.md` を読んだ
- [ ] `docs/tasks/0039_runner_test_migration/progress.md` を確認した
- [ ] 前のPhaseが完了している
- [ ] バックアップを取得した

### Phase完了時

- [ ] 該当Phaseの全タスクが完了
- [ ] テストが全てPASS
- [ ] `make lint` でエラーなし
- [ ] `progress.md` を更新した
- [ ] 変更をコミットした

### Task完了時

- [ ] 全21個のテストがPASS
- [ ] `skip_integration_tests` タグを削除した
- [ ] `make test` で全テストPASS
- [ ] カバレッジ確認（80%以上）
- [ ] ドキュメント更新完了

## 参考

- [Task 0039 README](./README.md)
- [Task 0039 進捗状況](./progress.md)
- [Task 0038 クイックリファレンス](../0038_test_infrastructure_finalization/quick_reference.md)
