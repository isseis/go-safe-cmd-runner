# コマンドテンプレート機能

## 概要

コマンドテンプレート機能を使うと、再利用可能なコマンド定義を作成し、異なるパラメータで複数回実行できます。これにより、設定ファイルの重複を減らし、保守性を向上させることができます。

## 基本的な使い方

### テンプレートの定義

テンプレートは `[command_templates.テンプレート名]` セクションで定義します：

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]
```

### テンプレートの使用

コマンド定義で `template` フィールドを指定し、`params` でパラメータ値を渡します：

```toml
[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "/backup/repo"
```

## プレースホルダー構文

テンプレート内では、以下のプレースホルダー構文が使用できます：

### 必須パラメータ: `${param}`

値が必ず提供される必要があるパラメータ。省略するとエラーになります。

```toml
[command_templates.example]
cmd = "echo"
args = ["${message}"]  # message は必須
```

### オプショナルパラメータ: `${?param}`

値が省略可能なパラメータ。値が空文字列または未指定の場合、その引数全体が削除されます。

```toml
[command_templates.example]
cmd = "restic"
args = ["${?verbose}", "backup", "${path}"]
# verbose が空の場合、args は ["backup", "/data"] になる
```

### 配列パラメータ: `${@param}`

配列値を複数の引数として展開します。

```toml
[command_templates.example]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]

# params.flags = ["-v", "-q"] の場合
# 展開結果: ["restic", "-v", "-q", "backup", "/data"]
```

### エスケープシーケンス

リテラルの `$` を使用したい場合は `\$` でエスケープします（TOMLでは `\\$`）：

```toml
[command_templates.example]
cmd = "echo"
args = ["Price: \\$100"]  # "Price: $100" として展開
```

## パラメータ型

テンプレートパラメータは以下の型をサポートします：

- **文字列**: `params.name = "value"`
- **配列**: `params.flags = ["-v", "-q"]`（`${@param}` でのみ使用可能）

## 変数展開との組み合わせ

パラメータ値内で `%{var}` 構文を使用して、グループ変数を参照できます：

```toml
[groups.vars]
backup_root = "/data/backups"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "%{backup_root}/repo"  # "/data/backups/repo" に展開
```

**展開順序**:
1. テンプレート展開（`${param}` → パラメータ値）
2. 変数展開（`%{var}` → 変数値）
3. セキュリティ検証

## テンプレートで使用可能なフィールド

テンプレート定義では、以下のフィールドが使用できます：

| フィールド | 型 | 必須 | 説明 |
|-----------|-----|------|------|
| `cmd` | string | ✓ | コマンドパス |
| `args` | []string | | コマンド引数 |
| `env` | []string | | 環境変数（KEY=VALUE形式） |
| `workdir` | string | | 作業ディレクトリ |
| `timeout` | int32 | | タイムアウト（秒） |
| `output_size_limit` | int64 | | 出力サイズ制限（バイト） |
| `risk_level` | string | | リスクレベル（low/medium/high） |

**注意**: テンプレート定義内に `name` フィールドを含めることはできません。コマンド名はテンプレート使用時に指定します。

## セキュリティ制約

### テンプレート定義内での制約

テンプレート定義（`cmd`, `args`, `env`, `workdir`）では、**`%{var}` 構文は禁止**されています。これは展開順序の曖昧さを避けるためです。

```toml
# ❌ エラー: テンプレート定義内で %{var} は使用不可
[command_templates.bad_example]
cmd = "%{root}/bin/restic"  # エラー
args = ["backup", "${path}"]
```

### パラメータ値内での使用

パラメータ値（`params.*`）では、`%{var}` 構文を使用できます：

```toml
# ✅ OK: params 内では %{var} が使用可能
[[groups.commands]]
template = "restic_backup"

[groups.commands.params]
path = "%{backup_root}/data"  # OK
```

### フィールドの排他性

コマンド定義で `template` を使用する場合、以下のフィールドは同時に指定できません：
- `cmd`
- `args`
- `env`
- `workdir`

```toml
# ❌ エラー: template と cmd を同時に指定
[[groups.commands]]
name = "backup"
template = "restic_backup"
cmd = "restic"  # エラー
```

## テンプレート名の命名規則

テンプレート名は以下の規則に従う必要があります：

- 英字またはアンダースコア（`_`）で始まる
- 英数字とアンダースコアのみ使用可能
- `__`（2つのアンダースコア）で始まる名前は予約済み

```toml
# ✅ 有効な名前
[command_templates.restic_backup]
[command_templates.backup_v2]
[command_templates._internal]

# ❌ 無効な名前
[command_templates.123backup]      # 数字で始まる
[command_templates.backup-name]    # ハイフン使用
[command_templates.__reserved]     # 予約済みプレフィックス
```

## 実践例

### 例1: 基本的なバックアップテンプレート

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "/data/volumes"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_database"
template = "restic_backup"

[groups.commands.params]
path = "/data/database"
repo = "/backup/repo"
```

### 例2: オプショナルパラメータの活用

```toml
[command_templates.restic_backup_flexible]
cmd = "restic"
args = ["${?verbose}", "backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

[[groups.commands]]
name = "backup_verbose"
template = "restic_backup_flexible"

[groups.commands.params]
verbose = "-v"  # verboseモード
path = "/data"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_quiet"
template = "restic_backup_flexible"

[groups.commands.params]
# verbose は省略（引数から削除される）
path = "/data"
repo = "/backup/repo"
```

### 例3: 配列パラメータによる柔軟な引数指定

```toml
[command_templates.restic_backup_advanced]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

[[groups.commands]]
name = "backup_full"
template = "restic_backup_advanced"

[groups.commands.params]
flags = ["-v", "--exclude-caches", "--one-file-system"]
path = "/home"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_simple"
template = "restic_backup_advanced"

[groups.commands.params]
flags = []  # フラグなし
path = "/home"
repo = "/backup/repo"
```

### 例4: グループ変数との組み合わせ

```toml
[global.vars]
backup_root = "/data/backups"

[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

[[groups]]
name = "daily_backup"

[groups.vars]
data_dir = "/data"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
path = "%{data_dir}/volumes"           # グループ変数参照
repo = "%{backup_root}/repo"           # グローバル変数参照
```

## エラーメッセージ

一般的なエラーとその対処法：

### `template "xxx" not found`
- 指定したテンプレート名が存在しません
- テンプレート名のスペルを確認してください

### `required parameter "xxx" missing`
- 必須パラメータが提供されていません
- `params` セクションに該当パラメータを追加してください

### `forbidden pattern %{ in template definition`
- テンプレート定義内で `%{var}` 構文を使用しています
- `params` 側で変数展開を行うように変更してください

### `cannot use both template and cmd fields`
- `template` と `cmd` が同時に指定されています
- どちらか一方のみを使用してください

### `array parameter ${@xxx} cannot be used in mixed context`
- 配列パラメータが文字列と混在しています
- 配列パラメータは単独の引数として使用してください

## ベストプラクティス

### 1. テンプレート名は説明的に

```toml
# ✅ Good
[command_templates.restic_backup_with_excludes]

# ❌ Bad
[command_templates.rb]
```

### 2. 必須パラメータは最小限に

オプショナルパラメータを活用して、柔軟性を確保：

```toml
[command_templates.flexible_backup]
cmd = "restic"
args = ["${?verbose}", "${@extra_flags}", "backup", "${path}"]
```

### 3. 変数展開はパラメータ側で

テンプレート定義は汎用的に保ち、環境固有の値はパラメータで注入：

```toml
# テンプレート: 汎用的
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

# 使用側: 環境固有
[groups.commands.params]
repo = "%{backup_root}/repo"  # 環境変数を参照
```

### 4. テンプレートの責務を明確に

1つのテンプレートが多くの機能を持たないように：

```toml
# ✅ Good: 責務が明確
[command_templates.restic_backup]
[command_templates.restic_restore]
[command_templates.restic_check]

# ❌ Bad: 1つのテンプレートで全て
[command_templates.restic_all_in_one]
```

## 参考情報

- サンプル設定: `sample/command_template_example.toml`
- 詳細仕様: `docs/tasks/0062_command_templates/03_detailed_spec.md`
- アーキテクチャ: `docs/tasks/0062_command_templates/02_architecture.md`
