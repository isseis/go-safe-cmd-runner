# Task 0038: クイックリファレンス

よく使うコマンドと手順のクイックリファレンス。

## 環境確認コマンド

### 現在の状態を確認

```bash
# skip_integration_tests タグが付いているファイルを確認
grep -r "skip_integration_tests" --include="*.go" | grep -v ".git"

# 古い型の使用箇所を確認
grep -r "runnertypes\.Config[^S]" --include="*.go" --exclude-dir=".git"
grep -r "runnertypes\.GlobalConfig" --include="*.go" --exclude-dir=".git"
grep -r "runnertypes\.CommandGroup" --include="*.go" --exclude-dir=".git"
grep -r "runnertypes\.Command[^S]" --include="*.go" --exclude-dir=".git"

# PrepareCommand の使用箇所を確認
grep -r "PrepareCommand" --include="*.go" --exclude-dir=".git"
```

## テスト実行コマンド

### 基本的なテスト実行

```bash
# 全テスト実行
make test

# 特定のパッケージのテスト
go test -tags test -v ./internal/runner

# 特定のテスト関数のみ
go test -tags test -v ./internal/runner -run TestNewRunner

# 統合テストを除外して実行（移行中）
go test -tags test ./... -short
```

### パフォーマンステスト

```bash
# パフォーマンステストを含めて実行
go test -tags test -v ./test/performance/...

# ベンチマーク実行
go test -tags test -bench=. -benchmem ./test/performance/...

# 特定のベンチマークのみ
go test -tags test -bench=BenchmarkOutputCapture -benchmem ./test/performance/...
```

### セキュリティテスト

```bash
# セキュリティテスト実行
go test -tags test -v ./test/security/...

# 特定のセキュリティテストのみ
go test -tags test -v ./test/security -run TestOutputSecurity
```

## カバレッジコマンド

### カバレッジレポート生成

```bash
# カバレッジデータ生成
go test -tags test -coverprofile=coverage.out ./...

# HTMLレポート生成（ブラウザで開く）
go tool cover -html=coverage.out -o coverage.html
open coverage.html  # macOS
xdg-open coverage.html  # Linux

# テキスト形式のカバレッジ表示
go tool cover -func=coverage.out

# 総カバレッジ率のみ表示
go tool cover -func=coverage.out | grep total
```

### パッケージ別カバレッジ

```bash
# runner パッケージのカバレッジ
go test -tags test -coverprofile=coverage.out ./internal/runner
go tool cover -func=coverage.out

# 特定のパッケージのカバレッジのみ抽出
go tool cover -func=coverage.out | grep "internal/runner"
```

### カバレッジ比較

```bash
# 移行前のカバレッジを保存
go test -tags test -coverprofile=coverage_before.out ./...
go tool cover -func=coverage_before.out | grep total > coverage_before.txt

# 移行後のカバレッジを保存
go test -tags test -coverprofile=coverage_after.out ./...
go tool cover -func=coverage_after.out | grep total > coverage_after.txt

# 比較
diff coverage_before.txt coverage_after.txt
```

## Lintコマンド

```bash
# Lint実行
make lint

# 特定のパッケージのみ
golangci-lint run --build-tags test ./internal/runner/...

# Lintエラーの詳細表示
golangci-lint run --build-tags test -v
```

## ビルドコマンド

```bash
# 全パッケージのビルド確認
go build ./...

# 特定のパッケージのみ
go build ./internal/runner/...

# テストバイナリのビルド
go test -tags test -c ./internal/runner
```

## Git操作

### ブランチ作成

```bash
# Phase 1.1用のブランチ
git checkout -b task/0038-phase1-runner-test

# Phase 1.2用のブランチ
git checkout -b task/0038-phase1-performance-test

# Phase 1.3用のブランチ
git checkout -b task/0038-phase1-security-test
```

### コミット例

```bash
# Phase 1.1完了時
git add internal/runner/runner_test.go
git commit -m "test: migrate runner_test.go to new type system (Task 0038 Phase 1.1)"

# Phase 2完了時
git add internal/runner/runnertypes/config.go
git commit -m "refactor: remove deprecated types (Config, GlobalConfig, etc.) - Task 0038 Phase 2"

# Phase 5完了時
git add docs/tasks/0035_spec_runtime_separation/test_reactivation_plan.md
git commit -m "docs: update test reactivation plan - Task 0038 complete"
```

## 進捗確認コマンド

### テスト統計

```bash
# テスト関数の数を確認
find . -name "*_test.go" -not -path "./.git/*" | xargs grep -h "^func Test" | wc -l

# パッケージ別テスト数
find ./internal/runner -name "*_test.go" | xargs grep -h "^func Test" | wc -l
```

### ファイル統計

```bash
# Go ファイルの総行数
find . -name "*.go" -not -path "./.git/*" | xargs wc -l | tail -1

# テストファイルの総行数
find . -name "*_test.go" -not -path "./.git/*" | xargs wc -l | tail -1

# 特定ファイルの行数
wc -l internal/runner/runner_test.go
```

## トラブルシューティング

### コンパイルエラーの確認

```bash
# 詳細なエラー出力
go build -v ./...

# 特定のパッケージのエラー
go build -v ./internal/runner/...
```

### テスト失敗時の詳細確認

```bash
# 詳細ログ付きでテスト実行
go test -tags test -v -count=1 ./internal/runner -run TestNewRunner

# タイムアウトを延長
go test -tags test -v -timeout 30m ./...

# 失敗したテストのみ再実行
go test -tags test -v -failfast ./...
```

### キャッシュのクリア

```bash
# テストキャッシュをクリア
go clean -testcache

# すべてのキャッシュをクリア
go clean -cache

# モジュールキャッシュもクリア
go clean -modcache
```

## Phase別の典型的な作業フロー

### Phase 1: テスト移行の典型的フロー

```bash
# 1. 現状確認
go test -tags test -v ./internal/runner -run TestNewRunner

# 2. ファイルを編集（エディタで）
# ...

# 3. コンパイル確認
go build ./internal/runner

# 4. テスト実行
go test -tags test -v ./internal/runner -run TestNewRunner

# 5. 全テスト確認
make test

# 6. Lint確認
make lint

# 7. コミット
git add internal/runner/runner_test.go
git commit -m "test: migrate TestNewRunner to ConfigSpec"
```

### Phase 2: 型削除の典型的フロー

```bash
# 1. 使用箇所が0件であることを確認
grep -r "runnertypes\.Config[^S]" --include="*.go" --exclude-dir=".git"
# （0件であることを確認）

# 2. バックアップ作成（念のため）
cp internal/runner/runnertypes/config.go internal/runner/runnertypes/config.go.bak

# 3. 型定義を削除（エディタで）
# ...

# 4. コンパイル確認
go build ./...

# 5. 全テスト実行
make test

# 6. Lint確認
make lint

# 7. コミット
git add internal/runner/runnertypes/config.go
git commit -m "refactor: remove deprecated Config type"
```

### Phase 4: カバレッジ確認の典型的フロー

```bash
# 1. カバレッジレポート生成
go test -tags test -coverprofile=coverage.out ./...

# 2. 総カバレッジ確認
go tool cover -func=coverage.out | grep total

# 3. パッケージ別確認
go tool cover -func=coverage.out | grep "internal/runner"

# 4. HTMLレポートで詳細確認
go tool cover -html=coverage.out -o coverage.html
open coverage.html

# 5. カバレッジが低い箇所を特定してテスト追加
# ...

# 6. 再度カバレッジ確認
go test -tags test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

## 便利なエイリアス設定（オプション）

以下を `~/.bashrc` または `~/.zshrc` に追加：

```bash
# Go テスト関連
alias gtest='go test -tags test -v'
alias gtestall='make test'
alias gcov='go test -tags test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | grep total'
alias gcovhtml='go test -tags test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out'

# Lint
alias glint='make lint'

# Task 0038 専用
alias t0038='cd ~/git/go-safe-cmd-runner && cat docs/tasks/0038_test_infrastructure_finalization/progress.md'
```

## よく使うgrepパターン

```bash
# テストファイル内の特定のパターンを検索
grep -n "Config{" internal/runner/*_test.go
grep -n "GlobalConfig{" internal/runner/*_test.go
grep -n "CommandGroup{" internal/runner/*_test.go
grep -n "Command{" internal/runner/*_test.go

# skip_integration_tests が残っていないか確認
find . -name "*.go" -exec grep -l "skip_integration_tests" {} \;

# TODO コメントの確認
grep -rn "TODO" --include="*.go" | grep -i "task.*038"
```

## 緊急時のロールバック

```bash
# 最後のコミットを取り消し（ファイルは保持）
git reset --soft HEAD~1

# 最後のコミットを完全に取り消し
git reset --hard HEAD~1

# 特定のファイルのみ元に戻す
git checkout HEAD -- internal/runner/runner_test.go

# ブランチ全体を元に戻す
git reset --hard origin/main
```

## 参考

- [Task 0038 README](./README.md)
- [進捗状況](./progress.md)
- [Task 0036 移行ガイド](../0036_runner_test_migration/01_migration_guide.md)
- [Task 0037 README](../0037_remaining_integration_tests/README.md)
