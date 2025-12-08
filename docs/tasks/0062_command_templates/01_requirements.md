# コマンドテンプレート機能 - 要件定義書

## 1. 概要

### 1.1 背景

現在の TOML 設定では、複数のグループで同じコマンド定義を繰り返し記述する必要がある。

```toml
[[groups]]
name = "group1"
[[groups.commands]]
name = "restic_prune"
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]

[[groups]]
name = "group2"
[[groups.commands]]
name = "restic_prune"
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]
```

この重複は以下の問題を引き起こす：

1. **保守性の低下**: 同じコマンド定義を複数箇所で修正する必要がある
2. **一貫性の欠如**: コピー時の誤りにより、グループ間でコマンド定義が不一致になる可能性
3. **可読性の低下**: 設定ファイルが冗長になり、本質的な差分が見えにくい

### 1.2 目的

テンプレート機能を導入し、共通のコマンド定義を一カ所で管理できるようにする。

```toml
# テンプレート定義
[command_templates.restic_prune]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]

# グループでテンプレートを使用
[[groups]]
name = "group1"
[[groups.commands]]
template = "restic_prune"

[[groups]]
name = "group2"
[[groups.commands]]
template = "restic_prune"
```

### 1.3 スコープ

#### 対象範囲 (In Scope)

- `[[groups.commands]]` のテンプレート化
- テンプレートへのパラメータ渡し（文字列、配列）
- テンプレート内でのパラメータ展開（`${param}`, `${?param}`, `${@list}`）
- params 内での既存変数参照（`%{group_root}` など）
- 後方互換性の維持（テンプレート機能はオプショナル）

#### 対象外 (Out of Scope)

- テンプレートの継承・拡張機能
- `[[groups.commands]]` 以外の要素のテンプレート化
- 条件付きテンプレート展開（環境変数による分岐など）
- テンプレートのネスト（テンプレートから別のテンプレートを参照）

## 2. 機能要件

### 2.1 テンプレート定義

#### F-001: テンプレート定義セクション

**概要**: TOML ファイルにテンプレート定義セクションを追加する。

**フォーマット**:
```toml
[command_templates.<template_name>]
cmd = "<command>"
args = ["<arg1>", "<arg2>", ...]
env = ["<key>=<value>", ...]
workdir = "<working_directory>"
# その他のコマンド実行に関するフィールド
```

**必須フィールド**:
- `cmd`: コマンドパス（必須）

**オプショナルフィールド**:
- `args`: 引数配列（省略時は空配列）
- `env`: 環境変数配列（省略時は空配列）
- `workdir`: 作業ディレクトリ（省略時は未設定）
- その他 `[[groups.commands]]` で利用可能な実行関連フィールド

**禁止フィールド**:
- `name`: テンプレートではコマンド名を定義できない（呼び出し側で指定）
- `template`: テンプレート内でテンプレートを参照できない（ネストは禁止）

**制約**:
- テンプレート名は英字またはアンダースコアで始まり、英数字とアンダースコアのみ使用可能（既存の変数名規則と同一、`ValidateVariableName` を使用）
- テンプレート定義は設定ファイルの先頭（`[[groups]]` より前）に配置
- 同名のテンプレートが複数定義された場合はエラー
- 予約済みのテンプレート名（将来の拡張用）: `__` で始まる名前は予約済み
- テンプレート定義に `name` フィールドが含まれる場合はエラー

**例**:
```toml
[command_templates.restic_check]
cmd = "restic"
args = ["check", "--read-data-subset=${subset_percentage}"]

[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]
```

### 2.2 パラメータ展開

#### F-002: 文字列パラメータ展開 `${param}`

**概要**: テンプレート内の `${param}` を文字列値で置換する。

**動作**:
- `${param}` はパラメータ値で置換される
- 空文字列でも要素として保持される

**例**:
```toml
[command_templates.example]
args = ["--option", "${value}"]

[[groups.commands]]
template = "example"
params.value = "test"
# 結果: args = ["--option", "test"]
```

#### F-003: オプショナル文字列パラメータ展開 `${?param}`

**概要**: テンプレート内の `${?param}` を文字列値で置換し、空文字列の場合は要素を削除する。

**動作**:
- `${?param}` はパラメータ値で置換される
- パラメータが空文字列の場合、その要素を配列から削除する

**例**:
```toml
[command_templates.example]
args = ["backup", "${?optional_flag}", "${path}"]

# optional_flag が空文字列の場合
[[groups.commands]]
template = "example"
params.optional_flag = ""
params.path = "/data"
# 結果: args = ["backup", "/data"]

# optional_flag が指定された場合
[[groups.commands]]
template = "example"
params.optional_flag = "--verbose"
params.path = "/data"
# 結果: args = ["backup", "--verbose", "/data"]
```

#### F-004: 配列パラメータ展開 `${@list}`

**概要**: テンプレート内の `${@list}` を配列で置換し、その要素を展開する。

**動作**:
- `${@list}` は配列の全要素で置換される
- 空配列の場合、要素を追加しない（無駄な空文字列が入らない）

**例**:
```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${path}"]

# verbose_flags が空配列の場合
[[groups.commands]]
template = "restic_backup"
params.verbose_flags = []
params.path = "/data"
# 結果: args = ["backup", "/data"]

# verbose_flags が指定された場合
[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q", "--no-cache"]
params.path = "/data"
# 結果: args = ["-q", "--no-cache", "backup", "/data"]
```

#### F-008: 非再帰的展開

**概要**: パラメータ展開は1回のみ行われ、再帰的な展開は行わない。

**動作**:
- パラメータ値に含まれる `${...}` や `${@...}` は文字列リテラルとして扱われる
- これにより、無限ループや DoS 攻撃（Billion Laughs attack 類似）を防止する

**例**:
```toml
[command_templates.echo]
args = ["${msg}"]

[[groups.commands]]
template = "echo"
params.msg = "${other_param}"
# 結果: args = ["${other_param}"]
# （"${other_param}" は展開されず、そのまま文字列として渡される）
```

### 2.3 テンプレート使用

#### F-005: グループコマンドでのテンプレート参照

**概要**: `[[groups.commands]]` でテンプレートを参照し、パラメータを渡す。

**フォーマット**:
```toml
[[groups.commands]]
name = "<command_name>"  # 必須（グループ内でユニーク）
template = "<template_name>"
params.<param1> = "<value1>"
params.<param2> = ["<value2a>", "<value2b>"]
```

**必須フィールド**:
- `name`: コマンド名（グループ内でユニークである必要がある）
- `template`: テンプレート名

**排他的フィールド（エラー）**:
`template` が指定された場合、以下のフィールドは指定できない:
- `cmd`: コマンドパス（テンプレートで定義）
- `args`: 引数配列（テンプレートで定義）
- `env`: 環境変数配列（テンプレートで定義）
- `workdir`: 作業ディレクトリ（テンプレートで定義）
- その他テンプレートで定義可能な全てのコマンド実行関連フィールド

**併用可能なフィールド**:
`template` と併用可能なフィールド:
- `name`: コマンド名（必須、同じテンプレートから複数のコマンドを区別するために使用）
- `params`: パラメータ指定（テンプレート展開に使用）

**制約**:
- `name` は必須であり、グループ内でユニークである必要がある（既存の仕様と同じ）
- 存在しないテンプレート名を指定した場合はエラー
- テンプレートで定義されていないパラメータを渡した場合は警告（エラーではない）
- テンプレートで使用されているパラメータが未指定の場合はエラー
- `template` と `cmd`/`args`/`env`/`workdir` を同時に指定した場合はエラー

**例**:
```toml
# 基本的な使用例
[[groups.commands]]
name = "daily_backup"  # 必須
template = "restic_backup"
params.verbose_flags = ["-q"]
params.backup_path = "/data/group1/volumes"

# 同じテンプレートから複数のコマンドを作成
[[groups.commands]]
name = "backup_volumes"  # 異なる name で区別
template = "restic_backup"
params.verbose_flags = ["-v", "-v"]
params.backup_path = "/data/group1/volumes"

[[groups.commands]]
name = "backup_config"  # 異なる name で区別
template = "restic_backup"
params.verbose_flags = []
params.backup_path = "/etc"

# エラー例: template と cmd の併用
[[groups.commands]]
name = "test"
template = "restic_backup"
cmd = "foo"  # ❌ エラー: template と cmd は排他的
```

#### F-007: リテラル `$` のエスケープ

**概要**: テンプレート定義内でリテラルの `$` 文字を使用したい場合のエスケープ機構を提供する。

**動作**:
- `\$` は展開処理でリテラルの `$` に変換される
- 既存の `\%` エスケープと同じバックスラッシュベースの記法を採用
- TOMLファイル内では `\\$` と記述する（TOML のエスケープルールにより `\$` になる）

**例**:
```toml
[command_templates.example]
args = ["--cost=\\$100", "${path}"]

[[groups.commands]]
template = "example"
params.path = "/data"
# 結果: args = ["--cost=$100", "/data"]
```

**一貫性**:
- 変数展開の `\%` エスケープと同じ記法
- 例: `\%{var}` → `%{var}` (展開されない)
- 例: `\$` → `$` (リテラル)

### 2.4 変数展開の順序

#### F-006: 展開タイミング

**概要**: テンプレート展開と既存の変数展開（`%{...}`）の順序を定義する。

**展開順序**:
1. テンプレートに params を適用（`${...}`, `${?...}`, `${@...}` を展開）
2. 結果として得られた `[[groups.commands]]` に対して `%{...}` を展開

**重要**: params 内の `%{...}` は Step 1 では展開されず、Step 2 で展開される。
これにより、params は「テンプレートに渡す式」として機能し、その式にはリテラル値だけでなく変数参照も含めることができる。

**例**:
```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]

[[groups]]
name = "group1"

[groups.vars]
group_root = "/data/group1"

[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q"]
params.backup_path = "%{group_root}/volumes"

# 展開プロセス:
# Step 1: テンプレート適用（${...} を params 値で置換）
#   args = ["-q", "backup", "%{group_root}/volumes"]
# Step 2: 結果の %{} を展開
#   args = ["-q", "backup", "/data/group1/volumes"]
```

**理由**:
- params でグループ固有の変数（`%{group_root}` など）を使用できる
- params 内で変数参照を明示的に指定することで、どの変数が使用されるか明確
- `%{...}` の展開は1回のみでシンプル
- テンプレート定義は変数に依存せず、再利用性が高い
- 実装がシンプル（既存の変数展開ロジックを再利用）

**メンタルモデル**:
- `params.backup_path = "%{group_root}/volumes"` は「値」ではなく「式」を渡している
- テンプレート展開は「式の代入」、変数展開は「式の評価」

**セキュリティ上の注意**:
- テンプレート定義（`cmd`, `args`, `env`, `workdir`）に `%{` が含まれる場合はエラーとして拒否する（NF-006）
- これにより、テンプレートは異なるコンテキストで安全に再利用できる
- 変数参照は各コマンド定義で `params` 経由で明示的に行う

**使用例**:
```toml
# OK: テンプレート定義には変数参照なし
[command_templates.restic_backup]
args = ["backup", "${path}"]

# OK: params 内で明示的に変数を参照
[[groups.commands]]
template = "restic_backup"
params.path = "%{group_root}/volumes"  # ローカル変数参照は許可
```

## 3. 非機能要件

### 3.1 互換性 (Compatibility)

#### NF-001: 後方互換性の維持

**要件**: 既存の TOML 設定ファイルが変更なしで動作すること。

**確認項目**:
- テンプレート機能を使用しない設定ファイルが従来通り動作する
- 全ての既存テストケースが変更なしで通る
- サンプル設定ファイルが正常に動作する

### 3.2 保守性 (Maintainability)

#### NF-002: 明確なエラーメッセージとデバッグ情報

**要件**: テンプレート関連のエラーは明確で分かりやすいメッセージを表示すること。

**エラーメッセージ例**:
- `template "restic_backup" not found`
- `parameter "backup_path" is required but not provided in template "restic_backup"`
- `cannot specify both "template" and "cmd" fields in command definition`
- `cannot specify both "template" and "args" fields in command definition`
- `cannot specify both "template" and "env" fields in command definition`
- `cannot specify both "template" and "workdir" fields in command definition`
- `template definition cannot contain "name" field`
- `unused parameter "extra_param" in template "restic_backup"`（警告）
- `variable "group_root" is not defined in group "group1", referenced by template parameter "backup_path" in template "restic_backup" (command #2)`（変数未定義エラー）

**変数未定義エラーの要件**:
- テンプレート使用時には、どの params フィールドから参照されたかを明記する
- グループ名、コマンド番号、テンプレート名を含める
- params の値（展開前の式）を表示する
- 可能であれば、修正のヒント（Hint）を表示する

**デバッグログ出力**:
- テンプレート展開時に params の値（展開前）をログ出力
- 変数展開時に最終的な値（展開後）をログ出力
- デバッグモードでは展開の各ステップを追跡可能にする

**ログ出力例**:
```
DEBUG: Expanding template "restic_backup" with params: {backup_path: "%{group_root}/volumes"}
DEBUG: Template expansion result: args = ["backup", "%{group_root}/volumes"]
DEBUG: Variable expansion result: args = ["backup", "/data/group1/volumes"]
```

#### NF-003: テストカバレッジ

**要件**: テンプレート機能の全ての分岐をカバーするテストを作成すること。

**確認項目**:
- 正常系: 各種パラメータ展開の動作確認
- 異常系: 存在しないテンプレート、未指定パラメータなど
- エッジケース: 空文字列、空配列、特殊文字など

### 3.3 性能 (Performance)

#### NF-004: オーバーヘッドの最小化

**要件**: テンプレート展開によるオーバーヘッドを最小限に抑えること。

**期待値**:
- テンプレート展開は設定ファイル読み込み時に1回のみ実行
- 実行時のパフォーマンスに影響しない
- メモリ使用量の増加は無視できるレベル

### 3.4 セキュリティ (Security)

#### NF-005: 展開後のセキュリティ検証

**要件**: テンプレート展開後に生成されたコマンド定義に対して、既存のセキュリティ検証を適用すること。

**検証タイミング**: テンプレートパラメータ展開後、`%{...}` 変数展開後

**検証項目**:
- **コマンドパス検証**: 展開後の `cmd` が `cmd_allowed` / `AllowedCommands` に含まれること
- **コマンドインジェクション検出**: 展開後の `cmd`, `args` に対する危険パターン検出（`;`, `|`, `&&`, `||`, `$()`, バッククォート等）
- **パストラバーサル検出**: 展開後のパスに対する `../` パターン検出
- **環境変数検証**: 展開後の `env` に対する `ValidateAllEnvironmentVars` 検証
- **パラメータ名の検証**: パラメータ名は `ValidateVariableName` で検証（展開前にチェック）

**実装方針**:
- テンプレート展開は既存のコマンド定義と同じ `CommandSpec` を生成する
- 展開後は通常のコマンド定義と同様の検証パスを通過させる
- 特別な例外処理やバイパスを設けない
- これにより、パラメータ値が最終的にどのフィールド（`cmd`, `args`, `env`）に使用されるかに応じた適切な検証が自動的に適用される

#### NF-006: 変数参照のセキュリティ境界

**要件**: テンプレート定義内での変数参照を禁止し、params内での変数参照を許可する。

**実装方針**:
- **テンプレート定義を禁止**: `command_templates` セクションの `cmd`, `args`, `env`, `workdir` に `%{` が含まれる場合、エラーとして拒否する
- **params を許可**: `params` での `%{...}` 使用は許可する（ローカル変数参照のため）

**理由**:
1. **テンプレートは複数のグループで再利用される**: 異なるコンテキストで同じ変数名が異なる意味を持つ可能性がある
2. **コンテキスト依存の危険性**: グループAでは安全でも、グループBでは機密変数を参照してしまう可能性がある
3. **明示性の向上**: 変数参照は各コマンド定義で `params` 経由で明示的に行うことで、どの変数が使用されるか明確になる
4. **責任の分離**: テンプレート作成者は変数の詳細を知る必要がなく、コマンド定義者が使用する変数を選択する

**攻撃シナリオの例**:
```toml
# 危険なテンプレート定義（このような定義は禁止される）
[command_templates.echo_var]
cmd = "echo"
args = ["%{secret_password}"]  # エラー: テンプレート定義で %{ 禁止

# グループAでは無害に見える
[[groups]]
name = "development"
[groups.vars]
secret_password = "dev_message"

[[groups.commands]]
template = "echo_var"  # 開発環境では問題なし

# しかしグループBで機密情報が漏洩
[[groups]]
name = "production"
[groups.vars]
secret_password = "prod_db_pass_xyz123"  # 漏洩！

[[groups.commands]]
template = "echo_var"  # 同じテンプレートで機密情報露出
```

**安全な使用方法**:
```toml
# 安全なテンプレート定義（変数参照なし）
[command_templates.echo_msg]
cmd = "echo"
args = ["${message}"]  # パラメータのみ

# 各グループで明示的に変数を渡す
[[groups]]
name = "production"
[groups.vars]
prod_message = "Production ready"

[[groups.commands]]
template = "echo_msg"
params.message = "%{prod_message}"  # 明示的、安全、許可される
```

**エラーメッセージ例**:
```
template "echo_var" contains forbidden pattern "%{" in args[0]: variable references are not allowed in template definitions for security reasons
```

#### NF-007: Dry-run モードでのテンプレート展開可視化

**要件**: `--dry-run` モードでテンプレート展開のプロセスを可視化すること。

**表示内容**:
- 使用されたテンプレート名
- params の値（展開前の式）
- 変数展開後の最終値
- 最終的に展開されたコマンド定義

**表示例**:
```
Group: group1
Command: restic_backup (from template)
  Template parameters:
    verbose_flags = ["-q"]
    backup_path = "%{group_root}/volumes" → "/data/group1/volumes"

  Expanded command:
    cmd: restic
    args: ["-q", "backup", "/data/group1/volumes"]
```

**目的**:
- ユーザーがテンプレート展開の結果を事前に確認できる
- デバッグ時に展開プロセスを追跡できる


## 4. 代替案の検討

### 4.1 検討した代替案

#### 案1: 単純なテンプレート定義 + パラメータ展開（ベースライン）

```toml
[command_templates.restic_check]
cmd = "restic"
args = ["check", "--read-data-subset=${subset_percentage}"]

[[groups.commands]]
template = "restic_check"
params = { subset_percentage = "2.5%" }
```

**Pros**:
- シンプルで直感的
- TOML の標準的な構造を使用
- パラメータ名が明示的

**Cons**:
- 空文字列 `""` を args に含めると無駄な空引数が渡される
- リストをパラメータとして渡しにくい

**不採用理由**: リスト展開のサポートが不十分

---

#### 案2: リスト展開のみをサポート（`${...name}` 記法）

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${...verbose_flags}", "backup", "${backup_path}"]

[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q"]
params.backup_path = "/data"
```

**Pros**:
- リストのパラメータを自然に扱える
- 空配列を渡せば要素が消える

**Cons**:
- `${...name}` という記法が直感的でない（スプレッド演算子風だが一貫性がない）
- 文字列パラメータで空文字列を削除する機能がない

**不採用理由**: 記法が分かりにくく、オプショナル文字列パラメータの機能がない

---

#### 案3: 条件付き要素（環境変数ベース）

```toml
[command_templates.restic_backup]
cmd = "restic"
args = [
    { value = "-q", if_env_not = "DEBUG" },
    "backup",
    "${backup_path}"
]

[[groups.commands]]
template = "restic_backup"
params.backup_path = "/data"
```

**Pros**:
- 環境変数による動的な制御が可能
- パラメータ渡しがシンプル
- デバッグ時の切り替えが環境変数で完結

**Cons**:
- TOML の構造が複雑（インライン配列と辞書の混在）
- 実装コストが高い
- YAGNI に反する（現時点で環境変数による条件分岐は不要）

**不採用理由**: 複雑性が高く、YAGNI の原則に反する

---

#### 案4: ハイブリッド（採用案）

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${?optional_flag}", "${backup_path}"]

[[groups.commands]]
template = "restic_backup"
params.verbose_flags = ["-q"]
params.optional_flag = ""
params.backup_path = "/data"
# 結果: args = ["-q", "backup", "/data"]
```

**Pros**:
- 文字列と配列の両方を扱える
- `${}`, `${?}`, `${@}` の3つの記法で直感的
- 空配列・空文字列で要素を消せる
- YAGNI に従いシンプルながら、必須要件をカバー

**Cons**:
- 3種類の記法を覚える必要がある

**採用理由**:
- シンプルながら必要十分な機能を提供
- Ruby/shell の `$@` に似た記法で直感的
- 将来的に継承機能を追加する際も構造を変えずに拡張可能

### 4.2 記法の詳細比較

| 記法 | 用途 | 空の場合の動作 | 例 |
|------|------|----------------|-----|
| `${param}` | 文字列置換 | 空文字列として保持 | `${path}` → `""` |
| `${?param}` | オプショナル文字列置換 | 要素を削除 | `${?flag}` → （削除） |
| `${@list}` | 配列展開 | 何も追加しない | `${@flags}` → （削除） |

## 5. 実装計画

### 5.1 実装フェーズ

#### Phase 1: パース機能の追加

**対象ファイル**:
- `internal/runner/runnertypes/spec.go` - テンプレート定義の型追加
- `internal/runner/config/loader.go` - TOML パース時のテンプレート読み込み

**実装内容**:
- `CommandTemplate` 型の定義
- TOML ファイルから `[command_templates.*]` セクションの読み込み
- テンプレート名の検証（重複チェック、命名規則チェック）

**テスト**:
- 正常系: テンプレート定義が正しく読み込まれる
- 異常系: 重複テンプレート名、不正なテンプレート名

#### Phase 2: テンプレート展開機能の実装

**対象ファイル**:
- `internal/runner/config/template_expansion.go`（新規作成）

**実装内容**:
- `${param}`, `${?param}`, `${@list}` のパーサー
- パラメータ置換ロジック
- テンプレート展開関数
- テンプレート定義値のセキュリティ検証 (NF-006: %{ パターン禁止)
- params 値の基本的な検証 (NF-005)
- リテラル `$` のエスケープ処理（`\$` → `$`）

**テスト**:
- 各記法のパラメータ展開テスト
- 空文字列・空配列の扱い
- エスケープ処理（`\$` のリテラル表記）
- セキュリティ検証のテスト（危険パターンの検出）
- テンプレート定義内の `%{` 検出テスト (NF-006)
- params 内の `%{` 使用テスト（許可されることを確認）

#### Phase 3: コマンド定義への統合

**対象ファイル**:
- `internal/runner/config/expansion.go` - コマンド展開ロジックの修正

**実装内容**:
- `[[groups.commands]]` で `template` フィールドが指定された場合の処理
- テンプレート展開 → `%{...}` 変数展開の順序制御
- エラーハンドリング（未定義テンプレート、未指定パラメータなど）
- 展開後のセキュリティ検証が既存のパスを通過することの確認（NF-007）

**テスト**:
- テンプレート使用時のコマンド展開テスト
- 変数展開の順序テスト（`%{...}` との組み合わせ）
- エラーケースのテスト
- セキュリティ検証統合テスト

#### Phase 4: エンドツーエンドテスト

**対象ファイル**:
- `internal/runner/config/loader_test.go` - 統合テスト

**実装内容**:
- TOML 読み込み → テンプレート展開 → コマンド実行の全フロー
- サンプル設定ファイルを使用したテスト
- 既存テストの互換性確認

### 5.2 リスクと対策

#### Risk-001: 既存の変数展開との干渉

**リスク**: `${...}` 記法が既存の何らかの機能と衝突する可能性。

**対策**:
- 既存のコードベースで `${` の使用箇所を検索
- 既存の変数展開は `%{...}` 記法を使用しているため、衝突の可能性は低い
- テンプレート展開は明示的に `template` フィールドが指定された場合のみ実行

**確認結果**:
```bash
grep -r '\${' --include='*.go' --include='*.toml' internal/ sample/
# 既存の使用箇所がないことを確認
```

**補足**: 仮に既存の TOML で `${...}` が使用されていた場合、テンプレート機能を使用しない限り影響はない（`template` フィールドが指定されたコマンドのみがテンプレート展開の対象となる）。

#### Risk-002: 複雑な展開順序によるバグ

**リスク**: `%{...}` と `${...}` の展開順序が複雑で、バグが混入する可能性。

**対策**:
- 展開順序を明確にドキュメント化
- 各ステップでの中間結果をログ出力（デバッグモード）
- 展開順序を検証する専用のテストケースを作成
- テンプレート定義での変数参照を禁止 (NF-006)

#### Risk-003: パラメータ値によるセキュリティバイパス

**リスク**: 悪意のある params 値により、セキュリティ検証をバイパスされる可能性。

**対策**:
- 展開後のコマンドに対する既存セキュリティ検証の適用（NF-005）
- テンプレート定義に `%{` パターンを含めることの禁止 (NF-006)

#### Risk-004: パフォーマンスへの影響

**リスク**: テンプレート展開処理が重く、設定ファイル読み込みが遅くなる可能性。

**対策**:
- 展開は設定ファイル読み込み時に1回のみ実行
- ベンチマークテストで性能測定
- 通常のユースケース（数個〜数十個のテンプレート）では影響が小さいことを確認

#### Risk-005: 配列パラメータ展開の悪用

**リスク**: `${@list}` 展開で大量の要素を注入し、コマンドライン長制限を超過させる、または意図しない引数を混入させる可能性。

**対策**:
- 展開後の引数配列に対する既存の検証を適用（NF-005）
- 配列要素数の上限設定（オプション：将来の拡張として検討）

## 6. セキュリティ考慮事項

### 6.1 脅威モデル

テンプレート機能において想定する脅威：

1. **設定ファイル改ざん**: 攻撃者が TOML 設定ファイルを改ざんし、悪意のあるテンプレートやパラメータを注入する
2. **パラメータインジェクション**: 正規の設定ファイル内で、params の値を通じて意図しないコマンドや引数を注入する
3. **テンプレート定義での変数参照**: テンプレート定義に `%{...}` を含めることで、異なるグループコンテキストで意図しない変数（機密情報など）を参照させる
4. **セキュリティバイパス**: テンプレート展開を通じて、既存のセキュリティ検証（`cmd_allowed`, `AllowedCommands` 等）をバイパスする

### 6.2 セキュリティ設計原則

1. **Defense in Depth（多層防御）**:
   - 展開時検証（テンプレート定義での変数参照禁止）
   - 出力時検証（展開後のコマンドに対する既存セキュリティ検証）

2. **Fail-Safe Defaults（安全側への失敗）**:
   - 不正な入力はエラーとして拒否
   - 曖昧なケースは許可ではなく拒否

3. **Least Privilege（最小権限の原則）**:
   - テンプレート展開は設定ファイル読み込み時の静的処理
   - 実行時の動的なパラメータ変更は不可

### 6.3 検証チェックリスト

実装時に確認すべきセキュリティ項目：

- [ ] テンプレート名のバリデーション（`ValidateVariableName` 使用）
- [ ] パラメータ名のバリデーション（`ValidateVariableName` 使用）
- [ ] テンプレート定義に `name` フィールドが含まれる場合の拒否
- [ ] テンプレート定義（`cmd`, `args`, `env`, `workdir`）に `%{` パターンが含まれる場合の拒否
- [ ] `template` フィールドと排他的フィールド（`cmd`, `args`, `env`, `workdir`）の同時指定の拒否
- [ ] パラメータ展開が非再帰的であること（params 値内の `${...}` が展開されないこと）
- [ ] params 値内の `%{...}` はそのまま保持され、後続の変数展開フェーズで処理されること
- [ ] 展開後の `cmd` が `cmd_allowed` / `AllowedCommands` を通過すること
- [ ] 展開後の `cmd`, `args` に対する危険パターン検出（コマンドインジェクション、パストラバーサル等）
- [ ] 展開後の `env` に対する `ValidateAllEnvironmentVars` 検証
- [ ] リテラル `$` のエスケープ処理（`\$` → `$`）
- [ ] テンプレート展開後の `%{...}` 変数展開における再帰展開の深さ制限（既存の `MaxRecursionDepth` と同様）

## 7. 用語集

- **テンプレート (Template)**: 再利用可能なコマンド定義
- **パラメータ (Parameter)**: テンプレートに渡す変数
- **パラメータ式 (Parameter Expression)**: params で指定する値。リテラル文字列、リテラル配列、または変数参照（`%{...}`）を含む式
- **テンプレート展開 (Template Expansion)**: テンプレート内の `${...}` パラメータ参照をパラメータ式で置換する処理（変数展開は行わない）
- **変数展開 (Variable Expansion)**: `%{variable}` 形式の変数参照を実際の変数値で置き換える処理（テンプレート展開後に実行）
- **文字列パラメータ**: `${param}` 形式で参照される文字列型のパラメータ
- **オプショナルパラメータ**: `${?param}` 形式で参照され、空の場合に要素が削除されるパラメータ
- **配列パラメータ**: `${@list}` 形式で参照される配列型のパラメータ
- **変数参照**: `%{variable}` 形式でローカル変数を参照する機能。params内で使用可能だが、テンプレート定義内では禁止

## 8. 参考資料

### 8.1 関連ファイル

- `internal/runner/runnertypes/spec.go` - 設定ファイルの型定義
- `internal/runner/runnertypes/runtime.go` - 実行時展開済みデータ構造（`ExpandedArrayVars` 等）
- `internal/runner/config/loader.go` - TOML 読み込み処理
- `internal/runner/config/expansion.go` - 変数展開ロジック
- `internal/runner/security/validator.go` - セキュリティ検証
- `internal/runner/security/environment_validation.go` - 環境変数検証
- `sample/risk-based-control.toml` - サンプル設定ファイル

### 8.2 関連タスク

- Task 0030: ファイル変数展開の検証
- Task 0061: グループ展開時のコマンド事前展開
- Task 0063: vars テーブル形式への変更（配列変数サポートの基盤）

### 8.3 設計上の議論

この要件定義書は、以下の議論に基づいて作成された：

1. **問題の発見**: 複数グループで同じコマンド定義を繰り返し記述する必要がある
2. **解決策の検討**: テンプレート機能の導入
3. **記法の選択**: 複数の代替案を比較し、ハイブリッド案（`${}`, `${?}`, `${@}`）を採用
4. **展開順序の決定**:
   - テンプレート展開 → `%{...}` 変数展開の2段階展開（params 内では `%{...}` 展開を行わない）
   - 案1（params を先に展開）vs 案2（テンプレート展開を先に実行）を比較し、案2を採用
   - 採用理由: 実装がシンプル（既存の変数展開ロジックを再利用）、一貫性が高い、セキュリティ検証が容易
5. **デバッグ性の向上**: 案2の課題（params の値が分かりにくい、エラーメッセージが複雑）に対する対策を追加
   - デバッグログでの展開前後の値表示（NF-002）
   - dry-run モードでの可視化（NF-007）
   - エラーメッセージへのコンテキスト情報追加（NF-002）
6. **YAGNI の原則**: 継承機能や条件分岐は現時点では不要と判断し、スコープ外とした
