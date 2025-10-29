# テストヘルパファイル移行手順書

## 作業概要

テストヘルパファイルを `testing/` サブディレクトリに移行し、命名規則を統一する。

## 移行作業チェックリスト

### Phase 1: ディレクトリ構造の準備

- [ ] `cmd/runner/testing/` ディレクトリを作成
- [ ] `internal/verification/testing/` ディレクトリを作成
- [ ] `internal/runner/runnertypes/testing/` ディレクトリを作成
- [ ] `internal/runner/variable/testing/` ディレクトリを作成
- [ ] `internal/runner/bootstrap/testing/` ディレクトリを作成

### Phase 2: ファイル移動とリネーム

#### 2.1 cmd/runner パッケージ

- [ ] `cmd/runner/main_test_helper.go` を `cmd/runner/testing/helpers.go` に移動
  - パッケージ名を `testing` に変更
  - ビルドタグを確認(`//go:build test`)

#### 2.2 internal/verification パッケージ

- [ ] `internal/verification/manager_testing.go` を `internal/verification/testing/helpers.go` に移動
  - パッケージ名を `testing` に変更
  - ビルドタグを確認

#### 2.3 internal/runner パッケージ

- [ ] `internal/runner/test_helpers.go` の内容を確認
- [ ] `internal/runner/group_executor_test_helpers.go` の内容を確認
- [ ] 両ファイルを `internal/runner/testing/helpers.go` にマージ
  - パッケージ名を `testing` に変更
  - 重複する関数がないか確認
  - ビルドタグを確認

#### 2.4 internal/runner/runnertypes パッケージ

- [ ] `internal/runner/runnertypes/config_test_helpers.go` を `internal/runner/runnertypes/testing/helpers.go` に移動
  - パッケージ名を `testing` に変更
  - ビルドタグを確認

#### 2.5 internal/runner/variable パッケージ

- [ ] `internal/runner/variable/auto_test_helper.go` を `internal/runner/variable/testing/helpers.go` に移動
  - パッケージ名を `testing` に変更
  - ビルドタグを確認

#### 2.6 internal/runner/bootstrap パッケージ

- [ ] `internal/runner/bootstrap/verification_test_helper.go` を `internal/runner/bootstrap/testing/helpers.go` に移動
  - パッケージ名を `testing` に変更
  - ビルドタグを確認

#### 2.7 internal/runner/executor パッケージ

- [ ] `internal/runner/executor/testing/mocks.go` のビルドタグを `//go:build test` に統一
  - 現在: `//go:build test || !prod`

### Phase 3: インポートパスの更新

各パッケージのテストファイルで、新しいインポートパスに更新する。

#### 3.1 cmd/runner パッケージのテスト

- [ ] `cmd/runner/main_test.go` のインポートを更新
  - 旧: `main_test_helper.go` の関数を直接使用(同一パッケージ)
  - 新: `import testing "github.com/isseis/go-safe-cmd-runner/cmd/runner/testing"`

#### 3.2 internal/verification パッケージのテスト

- [ ] `internal/verification/manager_test.go` などのインポートを確認・更新
  - 検索: `manager_testing.go` を使用しているファイル
  - 新: `import vtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"`

#### 3.3 internal/runner パッケージのテスト

- [ ] `internal/runner/group_executor_test.go` のインポートを更新
- [ ] `internal/runner` パッケージの他のテストファイルを確認
  - 検索: `test_helpers.go`, `group_executor_test_helpers.go` を使用しているファイル
  - 新: `import rtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"`

#### 3.4 internal/runner/runnertypes パッケージのテスト

- [ ] `internal/runner/runnertypes` パッケージのテストファイルを確認
  - 検索: `config_test_helpers.go` を使用しているファイル
  - 新: `import rttesting "github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes/testing"`

#### 3.5 internal/runner/variable パッケージのテスト

- [ ] `internal/runner/variable/auto_test.go` などのインポートを更新
  - 検索: `auto_test_helper.go` を使用しているファイル
  - 新: `import vartesting "github.com/isseis/go-safe-cmd-runner/internal/runner/variable/testing"`

#### 3.6 internal/runner/bootstrap パッケージのテスト

- [ ] `internal/runner/bootstrap` パッケージのテストファイルを確認
  - 検索: `verification_test_helper.go` を使用しているファイル
  - 新: `import btesting "github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap/testing"`

#### 3.7 internal/runner/executor パッケージのテスト

- [ ] `internal/runner/executor` パッケージのテストファイルを確認
  - ビルドタグ変更の影響を確認

### Phase 4: 動作確認

- [ ] `make test` を実行してすべてのテストが成功することを確認
- [ ] `make lint` を実行してリンターエラーがないことを確認
- [ ] `make build` を実行してビルドが成功することを確認

### Phase 5: 古いファイルの削除

Phase 4 が成功したら、元のファイルを削除する。

- [ ] `cmd/runner/main_test_helper.go` を削除
- [ ] `internal/verification/manager_testing.go` を削除
- [ ] `internal/runner/group_executor_test_helpers.go` を削除
- [ ] `internal/runner/test_helpers.go` を削除
- [ ] `internal/runner/runnertypes/config_test_helpers.go` を削除
- [ ] `internal/runner/variable/auto_test_helper.go` を削除
- [ ] `internal/runner/bootstrap/verification_test_helper.go` を削除

### Phase 6: 最終確認

- [ ] `make test` を再実行
- [ ] `make lint` を再実行
- [ ] `make build` を再実行
- [ ] Git status を確認し、すべての変更がコミット可能な状態であることを確認

## 注意事項

1. **パッケージ名の変更**: 移動後のファイルはすべて `package testing` にする
2. **インポートエイリアス**: 同じ名前の `testing` パッケージが複数あるため、適切なエイリアスを使用
   - 例: `import rtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/testing"`
3. **ビルドタグの統一**: すべてのファイルで `//go:build test` を使用
4. **マージ時の注意**: `internal/runner/testing/helpers.go` は2つのファイルをマージするため、関数名の重複に注意

## ロールバック手順

移行中に問題が発生した場合:

1. `git status` で変更ファイルを確認
2. `git checkout .` で変更を破棄
3. `git clean -fd` で新規ファイル/ディレクトリを削除
4. 問題を分析し、修正してから再度移行を試みる

## 完了確認

すべてのチェックボックスにチェックが入り、以下が確認できたら完了:

- ✅ すべてのテストが成功
- ✅ リンターエラーなし
- ✅ ビルド成功
- ✅ 古いファイルが削除されている
- ✅ CLAUDE.md のルールに準拠
