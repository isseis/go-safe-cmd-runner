# テストヘルパーファイルの整理

テストヘルパーファイルは、スコープと依存関係に基づく2段階の分類システムに従う：

## 分類 A：`testutil/` サブディレクトリ（クロスパッケージヘルパー）

**使用場面**：複数のパッケージ間で使用されるテストヘルパーやモック、またはパブリック API のみを使用するもの

```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── testutil/
    ├── mocks.go              # 軽量モック（外部依存なし）
    ├── testify_mocks.go      # testify ベースのモック（複雑なシナリオ向け）
    ├── mocks_test.go         # モック実装のテスト
    └── helpers.go            # テストユーティリティ関数
```

**ファイル命名規則：**
- **`testutil/mocks.go`**：外部ライブラリ依存のないシンプルなモック実装
- **`testutil/testify_mocks.go`**：stretchr/testify フレームワークを使用した高度なモック
- **`testutil/mocks_test.go`**：モック実装のユニットテスト
- **`testutil/helpers.go`**：共通のテストユーティリティ関数とセットアップヘルパー

**パッケージ命名：**
- `testutil/` サブディレクトリ内ではドメインプレフィックス付きのパッケージ名を使用する：`package <domain>testutil`
  - 例：`package commontestutil`、`package securitytestutil`、`package verificationtestutil`
- エイリアスなしでインポートする：`<module>/internal/<package>/testutil`
- 一意のパッケージ名により、呼び出し側でのインポートエイリアスが不要となり、コードベース全体でのエイリアスの乱立を防ぐ

**例外：** リポジトリルートの `internal/testutil` パッケージは、可読性のために `package tu` を使用する。そのヘルパー（例：`tu.Int32Ptr`）はインラインのテストデータ構築に多用されるため。

## 分類 B：パッケージレベルの `test_helpers.go`（内部ヘルパー）

**使用場面**：以下の理由により同一パッケージに配置する必要があるテストヘルパー：
- パッケージ内部の型へのメソッド追加
- エクスポートされていない（プライベートな）パッケージ API の使用
- 循環依存の回避

```
<package>/
├── <implementation>.go
├── <implementation>_test.go
└── test_helpers.go           # パッケージ内部のテストヘルパー
```

**ファイル命名規則：**
- **`test_helpers.go`**：パッケージ内部テストヘルパーの単一ファイル
- 複数のヘルパーカテゴリが必要な場合：`test_helpers_<category>.go`（例：`test_helpers_group.go`）

**パッケージ命名：**
- プロダクションコードと同じパッケージ名を使用する
- 常に `//go:build test` ビルドタグを含める

## 新しいテストヘルパーのガイドライン

新しいテストヘルパーコードを追加する際は、以下のデシジョンツリーに従う：

1. **ヘルパーはパブリック API のみを使用するか？**
   - はい → ステップ 2 に進む（分類 A）
   - いいえ → ステップ 4 に進む（分類 B の可能性が高い）

2. **作成するテストヘルパーの種類は？**（分類 A — `testutil/` サブディレクトリ）
   - **モック実装** → 複雑さに基づいて選択する：
     - シンプルなモック（外部依存なし）→ `testutil/mocks.go`
     - 複雑なモック（testify/mock 使用）→ `testutil/testify_mocks.go`
   - **ヘルパー関数**（セットアップ、ユーティリティ、フィクスチャ）→ `testutil/helpers.go`
   - **モックのテスト** → `testutil/mocks_test.go`

3. **ヘルパーは他のパッケージのテストで使用されるか？**
   - はい → パブリック API のみを使用することを確認し、適切な `testutil/` ファイルに配置する（ステップ 2）
   - いいえ → ステップ 4 に進む

4. **パッケージ内部の考慮事項**（分類 B — `test_helpers.go`）
   以下の場合は `test_helpers.go` に配置する：
   - パッケージ内部の型にメソッドを追加する
   - エクスポートされていない（プライベートな）パッケージ API を使用する
   - `testutil/` サブディレクトリに配置すると循環依存が生じる
   - 複数のヘルパーカテゴリが存在する場合：`test_helpers_<category>.go` を使用する（例：`test_helpers_group.go`）

**ビルドタグ：**
- すべてのテストヘルパーファイルの先頭に `//go:build test` を含める
- これにより、テストビルド時にのみコンパイルされ、プロダクションバイナリには含まれない

**例：**
- モックインターフェース実装 → `testutil/mocks.go` または `testutil/testify_mocks.go`
- テストセットアップヘルパー関数 → `testutil/helpers.go`
- 内部型へのメソッド → `test_helpers.go`
- プライベートコンストラクタを使用するファクトリ関数 → `test_helpers.go`
