# グループフィルタリング機能 - 要件定義書

## 1. 概要

### 1.1 背景

現在、runner は TOML 設定ファイルに定義されたすべてのグループを実行する。しかし、大規模な設定ファイルでは複数のグループが定義されており、開発やデバッグ時には特定のグループのみを実行したい場合がある。

```toml
[[groups]]
name = "preparation"
# ... commands ...

[[groups]]
name = "build"
depends_on = ["preparation"]
# ... commands ...

[[groups]]
name = "test"
depends_on = ["build"]
# ... commands ...

[[groups]]
name = "deploy"
depends_on = ["test"]
# ... commands ...
```

上記のような設定で、`build` グループのみを実行したい場合や、`test` と `deploy` のみを実行したい場合に対応できない。

### 1.2 目的

コマンドラインフラグを通じて実行対象グループを選択可能にし、開発・デバッグ時の効率を向上させる。

### 1.3 スコープ

- **対象**: runner コマンドのコマンドラインインターフェース拡張
- **実装範囲**: グループフィルタリング機能、依存関係解決、エラーハンドリング
- **注**: 既存の設定ファイル形式は変更しない

## 2. 機能要件

### 2.1 コマンドラインインターフェース

#### 2.1.1 フラグ仕様

**フラグ名**: `--groups` または `-g`

**型**: 文字列（カンマ区切りのグループ名リスト）

**デフォルト値**: 空文字列（すべてのグループを実行）

**例**:
```bash
# すべてのグループを実行（デフォルト）
runner -c config.toml

# 特定のグループのみ実行
runner -c config.toml --groups=build

# 複数のグループを実行
runner -c config.toml --groups=build,test

# 短縮形
runner -c config.toml -g build,test
```

#### 2.1.2 グループ名の制約

グループ名は環境変数名と同じ命名規則に従う：

- **正規表現**: `^[A-Za-z_][A-Za-z0-9_]*$`
- **開始文字**: 英字（大文字・小文字）またはアンダースコア `_`
- **以降の文字**: 英数字またはアンダースコア
- **禁止文字**: カンマ `,`、スペース、特殊記号

**有効な例**:
- `build`
- `test_integration`
- `Deploy_Prod`
- `_internal`

**無効な例**:
- `build,test` (カンマを含む)
- `123test` (数字で開始)
- `test-unit` (ハイフンを含む)
- `build test` (スペースを含む)

### 2.2 フィルタリング動作

#### 2.2.1 フラグ未指定時

**条件**: `--groups` フラグが指定されていない、または空文字列

**動作**: 設定ファイルに定義されたすべてのグループを実行

**例**:
```bash
runner -c config.toml
# → すべてのグループを実行
```

#### 2.2.2 単一グループ指定時

**条件**: `--groups=group_name` の形式で単一グループを指定

**動作**: 指定されたグループのみを実行対象とする（依存関係は2.3を参照）

**例**:
```bash
runner -c config.toml --groups=build
# → build グループのみを実行対象
```

#### 2.2.3 複数グループ指定時

**条件**: `--groups=group1,group2,group3` の形式で複数グループを指定

**動作**: カンマで区切られたすべてのグループを実行対象とする

**例**:
```bash
runner -c config.toml --groups=build,test
# → build と test グループを実行対象
```

**パース規則**:
- カンマ `,` で分割
- 各グループ名の前後の空白は除去
- 空の要素は無視

**例**:
```bash
--groups=build,test,deploy    # → ["build", "test", "deploy"]
--groups=build, test, deploy  # → ["build", "test", "deploy"] (空白除去)
--groups=build,,test          # → ["build", "test"] (空要素無視)
```

### 2.3 依存関係の自動解決

#### 2.3.1 基本動作

指定されたグループが `depends_on` フィールドで他のグループに依存している場合、依存先のグループも自動的に実行対象に含める。

**例**:
```toml
[[groups]]
name = "preparation"

[[groups]]
name = "build"
depends_on = ["preparation"]

[[groups]]
name = "test"
depends_on = ["build"]
```

```bash
runner -c config.toml --groups=test
# 実際の実行順序: preparation → build → test
```

#### 2.3.2 依存関係の再帰的解決

依存関係は再帰的に解決される。

**例**:
```toml
[[groups]]
name = "setup"

[[groups]]
name = "prepare"
depends_on = ["setup"]

[[groups]]
name = "build"
depends_on = ["prepare"]

[[groups]]
name = "test"
depends_on = ["build"]
```

```bash
runner -c config.toml --groups=test
# 実際の実行順序: setup → prepare → build → test
```

#### 2.3.3 複数グループ指定時の依存関係

複数のグループを指定した場合、各グループの依存関係をすべて解決する。

**例**:
```toml
[[groups]]
name = "common"

[[groups]]
name = "build_backend"
depends_on = ["common"]

[[groups]]
name = "build_frontend"
depends_on = ["common"]

[[groups]]
name = "test_backend"
depends_on = ["build_backend"]

[[groups]]
name = "test_frontend"
depends_on = ["build_frontend"]
```

```bash
runner -c config.toml --groups=test_backend,test_frontend
# 実際の実行順序: common → build_backend → build_frontend → test_backend → test_frontend
# (common は両方の依存先として一度だけ実行される)
```

#### 2.3.4 依存関係追加のログ出力

依存関係により自動的に追加されたグループは INFO レベルでログに記録する。

**ログ例**:
```
INFO: Group 'test' depends on 'build', adding to execution list
INFO: Group 'build' depends on 'preparation', adding to execution list
INFO: Executing groups: preparation, build, test
```

### 2.4 エラーハンドリング

#### 2.4.1 存在しないグループ名

**条件**: 指定されたグループ名が設定ファイルに存在しない

**動作**: エラーメッセージを出力して終了（終了コード: 1）

**エラーメッセージ例**:
```
Error: group 'nonexistent' specified in --groups does not exist in configuration
Available groups: preparation, build, test, deploy
```

#### 2.4.2 無効なグループ名

**条件**: 指定されたグループ名が命名規則に違反している

**動作**: エラーメッセージを出力して終了（終了コード: 1）

**エラーメッセージ例**:
```
Error: invalid group name '123test' in --groups flag
Group names must match the pattern: [A-Za-z_][A-Za-z0-9_]*
```

#### 2.4.3 依存関係エラー

依存関係の循環参照などのエラーは、既存のグループ実行時のエラーハンドリングと同様に処理される（本機能では新たなエラーケースは追加されない）。

### 2.5 既存機能との互換性

#### 2.5.1 設定ファイル形式

既存の TOML 設定ファイル形式は変更しない。グループ定義に新しいフィールドは追加しない。

#### 2.5.2 デフォルト動作

`--groups` フラグを指定しない場合の動作は、現在の動作（すべてのグループを実行）と完全に同一である。

#### 2.5.3 他のフラグとの併用

既存のすべてのフラグ（`--config`, `--dry-run` など）と併用可能。

**例**:
```bash
runner -c config.toml --groups=test --dry-run
```

## 3. 非機能要件

### 3.1 パフォーマンス

#### NFR-1: フィルタリングのオーバーヘッド

グループフィルタリングによる実行時間のオーバーヘッドは無視できる程度（1ms未満）とする。

### 3.2 保守性

#### NFR-2: コードの明瞭性

グループフィルタリングのロジックは、既存のグループ実行ロジックから明確に分離し、保守しやすい設計とする。

#### NFR-3: テストカバレッジ

以下のテストを実装する：
- フラグパースのユニットテスト
- グループ名バリデーションのユニットテスト
- 依存関係解決のユニットテスト
- エラーケースのユニットテスト
- 統合テスト（実際の設定ファイルを使用）

### 3.3 ユーザビリティ

#### NFR-4: エラーメッセージの明瞭性

エラーが発生した場合、ユーザーが問題を理解し、修正できるような明確なメッセージを提供する。

#### NFR-5: ヘルプメッセージ

`--help` フラグで表示されるヘルプメッセージに、`--groups` フラグの使用方法を明記する。

## 4. 制約事項

### 4.1 技術的制約

- Go 1.23.10 以上
- 既存の CLI フレームワーク（flag パッケージまたは使用中のライブラリ）を使用
- 既存のグループ実行ロジック（`internal/runner/executor`）との統合

### 4.2 互換性制約

- 既存の設定ファイルはすべて変更なしで動作する
- 既存のコマンドラインインターフェースは変更しない（新しいフラグの追加のみ）

## 5. 成功基準

### 5.1 機能の完全性

- [ ] `--groups` フラグでグループ名を指定できる
- [ ] カンマ区切りで複数のグループを指定できる
- [ ] フラグ未指定時はすべてのグループが実行される
- [ ] 依存関係が自動的に解決される
- [ ] 依存関係追加時に INFO ログが出力される
- [ ] 存在しないグループ名でエラーが発生する
- [ ] 無効なグループ名でエラーが発生する

### 5.2 テストカバレッジ

- [ ] ユニットテストで全てのフィルタリングパターンをカバー
- [ ] エラーケースのテストを作成
- [ ] 統合テストで実際の TOML ファイルを使用した検証

### 5.3 ドキュメント

- [ ] ヘルプメッセージの更新
- [ ] ユーザーガイドの更新（使用例を含む）
- [ ] サンプル実行コマンドの追加

## 6. 想定される利用シナリオ

### 6.1 開発時の特定グループのみ実行

開発者が `build` グループのみを繰り返し実行してデバッグする。

```bash
runner -c config.toml --groups=build
```

### 6.2 テストグループのみ実行

CI/CD の特定ステージで、テスト関連のグループのみを実行する。

```bash
runner -c config.toml --groups=test_unit,test_integration
```

### 6.3 デプロイメントグループの実行

本番環境へのデプロイ時に、デプロイ関連のグループのみを実行する（依存関係は自動解決される）。

```bash
runner -c config.toml --groups=deploy_production
# 依存する build や test グループも自動的に実行される
```

### 6.4 ドライランとの組み合わせ

特定のグループをドライランモードで実行する。

```bash
runner -c config.toml --groups=deploy --dry-run
```

## 7. 実装の考慮事項

### 7.1 フェーズ

実装は以下のフェーズに分割することを推奨する：

**Phase 1: 基本フィルタリング**
- コマンドラインフラグのパース
- グループ名のバリデーション
- 基本的なフィルタリング機能

**Phase 2: 依存関係解決**
- 依存関係の自動追加
- ログ出力の実装

**Phase 3: エラーハンドリング強化**
- エラーメッセージの改善
- エッジケースの処理

### 7.2 既存コードとの統合ポイント

- **CLI パース**: `internal/runner/cli` パッケージ
- **グループ実行**: `internal/runner/executor` パッケージ
- **設定管理**: `internal/runner/config` パッケージ

### 7.3 データ構造

グループフィルタリングの状態を保持するための構造を検討：

```go
type GroupFilter struct {
    // 指定されたグループ名（パース後）
    SpecifiedGroups []string

    // 依存関係解決後の実行対象グループ（順序付き）
    ExecutionGroups []string
}
```

## 8. 今後の拡張可能性

### 8.1 将来的な機能追加

以下の機能は現時点では対象外だが、将来的に検討可能：

- **グループの除外**: `--exclude-groups` フラグで特定グループを除外
- **正規表現サポート**: `--groups=test_.*` のようなパターンマッチング
- **タグベースフィルタリング**: グループにタグを付与し、タグで絞り込み
- **環境変数からの読み込み**: `RUNNER_GROUPS=build,test` のような指定

### 8.2 設定ファイルでのデフォルト指定

設定ファイルでデフォルトの実行対象グループを指定できる機能：

```toml
[global]
default_groups = ["build", "test"]
```

## 9. 参照

- 既存のグループ実行機能: `internal/runner/executor`
- CLI インターフェース: `internal/runner/cli`
- グループ依存関係: `internal/runner/config` の `depends_on` フィールド

---

**文書バージョン**: 1.0
**作成日**: 2025-11-17
**承認日**: [未承認]
**次回レビュー予定**: [実装完了後]
