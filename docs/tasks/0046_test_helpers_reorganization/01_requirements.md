# テストヘルパファイル統一化

## 背景

現在、テスト専用のヘルパ関数やモックが様々な命名規則とディレクトリ配置で管理されている:

- `_test_helpers.go`, `_test_helper.go`, `_testing.go`, `test_helpers.go` など命名が不統一
- 一部は `testing/` サブディレクトリにあるが、多くはテスト対象と同じディレクトリに配置
- すべてのファイルに `//go:build test` タグが付与されている

## 目的

テストヘルパファイルの命名規則と配置場所を統一し、コードベースの保守性を向上させる。

## 採用ルール: `testing/` サブディレクトリ方式

### ディレクトリ構造
```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── testing/
    ├── mocks.go              # 軽量なモック実装
    ├── testify_mocks.go      # testify ベースのモック
    ├── mocks_test.go         # モックのテスト
    └── helpers.go            # テストヘルパ関数
```

### ファイル命名規則
- **`testing/mocks.go`**: シンプルなモック実装(外部ライブラリ依存なし)
- **`testing/testify_mocks.go`**: testify フレームワークを使った高度なモック
- **`testing/mocks_test.go`**: モック実装のユニットテスト
- **`testing/helpers.go`**: 共通のテストユーティリティ関数

### パッケージ命名
- すべてのテストユーティリティは `package testing` を使用
- インポート: `<module>/internal/<package>/testing`

### ビルドタグ
- すべてのファイルに `//go:build test` を付与

## 既存ファイルの移行対象

### 移行が必要なファイル(9個)

1. `cmd/runner/main_test_helper.go`
   - → `cmd/runner/testing/helpers.go`
   - 内容: テスト用の一時ハッシュディレクトリ作成関数

2. `internal/verification/manager_testing.go`
   - → `internal/verification/testing/helpers.go`
   - 内容: テスト用のマネージャ作成関数

3. `internal/runner/group_executor_test_helpers.go`
   - → `internal/runner/testing/helpers.go` にマージ
   - 内容: グループ実行のテストヘルパ

4. `internal/runner/test_helpers.go`
   - → `internal/runner/testing/helpers.go` にマージ
   - 内容: ランナーのテストヘルパ

5. `internal/runner/runnertypes/config_test_helpers.go`
   - → `internal/runner/runnertypes/testing/helpers.go`
   - 内容: 設定のテストヘルパ

6. `internal/runner/variable/auto_test_helper.go`
   - → `internal/runner/variable/testing/helpers.go`
   - 内容: 変数展開のテストヘルパ

7. `internal/runner/bootstrap/verification_test_helper.go`
   - → `internal/runner/bootstrap/testing/helpers.go`
   - 内容: ブートストラップのテストヘルパ

8. `internal/runner/executor/testing/mocks.go`
   - → そのまま(ビルドタグを `test` に統一)
   - 現在: `//go:build test || !prod`

9. `internal/runner/privilege/testing/mocks.go`
   - → そのまま(既に正しい配置)

### 既に正しい配置のファイル

以下のファイルは既に `testing/` サブディレクトリにあり、変更不要:

- `internal/common/testing/mocks.go`
- `internal/common/testing/mocks_test.go`
- `internal/runner/executor/testing/testify_mocks.go`
- `internal/runner/privilege/testing/mocks.go`
- `internal/runner/testing/mocks.go`
- `internal/runner/security/testing/testify_mocks.go`
- `internal/runner/security/testing/testify_mocks_test.go`
- `internal/verification/testing/testify_mocks.go`
- `internal/verification/testing/testify_mocks_test.go`

## 影響範囲

### 影響を受けるテストファイル

移行したファイルをインポートしているすべてのテストファイルで、インポートパスの更新が必要。

### 作業手順

1. 各パッケージに `testing/` サブディレクトリを作成
2. ヘルパファイルを移動し、`helpers.go` にリネーム
3. パッケージ名を `testing` に変更
4. ビルドタグを統一(`//go:build test`)
5. インポートパスを更新
6. テストを実行して動作確認
7. 元のファイルを削除

## 完了基準

- [ ] すべてのテストヘルパファイルが `testing/` サブディレクトリに配置されている
- [ ] ファイル名が統一ルールに従っている(`helpers.go`, `mocks.go` など)
- [ ] すべてのテストが成功する(`make test`)
- [ ] リンターが成功する(`make lint`)
- [ ] CLAUDE.md の Mock File Organization セクションとの一貫性が保たれている
