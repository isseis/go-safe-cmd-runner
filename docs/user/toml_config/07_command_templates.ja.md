# 第7章: コマンドテンプレート機能

## 7.1 コマンドテンプレートの概要

コマンドテンプレート機能は、共通のコマンド定義を一カ所で管理し、複数のグループで再利用できるようにする機能です。

### 背景と目的

複数のグループで同じコマンド定義を繰り返し記述すると、以下の問題が発生します：

1. **保守性の低下**: 同じコマンド定義を複数箇所で修正する必要がある
2. **一貫性の欠如**: コピー時の誤りにより、グループ間でコマンド定義が不一致になる可能性
3. **可読性の低下**: 設定ファイルが冗長になり、本質的な差分が見えにくい

```toml
# テンプレート機能を使わない場合（冗長）
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

テンプレート機能を使用すると、共通のコマンド定義を一カ所にまとめ、各グループからパラメータを変えて参照できます：

```toml
# テンプレート定義
[command_templates.restic_prune]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]

# グループでテンプレートを使用
[[groups]]
name = "group1"
[[groups.commands]]
name = "restic_prune"
template = "restic_prune"

[[groups]]
name = "group2"
[[groups.commands]]
name = "restic_prune"
template = "restic_prune"
```

### 主な利点

1. **DRY原則の実現**: 同じ定義を繰り返さない
2. **保守性の向上**: 変更箇所が一カ所に集約される
3. **一貫性の確保**: 全てのグループで同じ定義が使用される
4. **可読性の向上**: グループ固有の設定（パラメータ）のみが明示される
5. **パラメータ化**: 共通部分を維持しながら、グループごとに異なる値を指定可能

## 7.2 テンプレート定義

### 7.2.1 基本構文

テンプレートは `[command_templates.<テンプレート名>]` セクションで定義します。

```toml
[command_templates.<template_name>]
cmd = "<command>"
args = ["<arg1>", "<arg2>", ...]
# その他のコマンド実行に関するフィールド
```

### 7.2.2 使用可能なフィールド

テンプレート定義では、`[[groups.commands]]` で使用可能な実行関連フィールドのほとんどを指定できます。

| フィールド | 説明 | 必須 |
|-----------|------|------|
| `cmd` | 実行するコマンドのパス | 必須 |
| `args` | コマンドに渡す引数の配列 | オプション |
| `env_vars` | 環境変数の配列 | オプション |
| `workdir` | 作業ディレクトリ | オプション |
| `timeout` | タイムアウト（秒） | オプション |
| `run_as_user` | 実行ユーザー | オプション |
| `run_as_group` | 実行グループ | オプション |
| `risk_level` | リスクレベル | オプション |
| `output_file` | 出力ファイル | オプション |

### 7.2.3 使用禁止のフィールド

以下のフィールドはテンプレート定義で使用できません：

| フィールド | 理由 |
|-----------|------|
| `name` | コマンド名は呼び出し側で指定する |
| `template` | テンプレートのネスト（テンプレートから別のテンプレートを参照）は禁止 |

### 7.2.4 テンプレート名の規則

テンプレート名は以下のルールに従う必要があります：

- 英字またはアンダースコアで始まる
- 英数字とアンダースコアのみ使用可能
- `__`（アンダースコア2つ）で始まる名前は予約済み

```toml
# 有効なテンプレート名
[command_templates.restic_backup]
[command_templates.daily_cleanup]
[command_templates._internal_task]

# 無効なテンプレート名
[command_templates.123_task]        # 数字で開始
[command_templates.my-template]     # ハイフンは使用不可
[command_templates.__reserved]      # 予約済みプレフィックス
```

### 7.2.5 設定例

#### 例1: シンプルなテンプレート

```toml
[command_templates.disk_check]
cmd = "/bin/df"
args = ["-h"]
timeout = 30
risk_level = "low"
```

#### 例2: 複数の引数を持つテンプレート

```toml
[command_templates.restic_forget]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "7", "--keep-weekly", "5", "--keep-monthly", "3"]
timeout = 3600
risk_level = "medium"
```

## 7.3 パラメータ展開

テンプレートにパラメータを定義し、呼び出し時に値を渡すことで、柔軟なコマンド定義が可能です。

### 7.3.1 パラメータ展開の種類

go-safe-cmd-runner では、3種類のパラメータ展開構文を提供しています：

| 記法 | 名称 | 用途 | 空の場合の動作 |
|------|------|------|----------------|
| `${param}` | 文字列パラメータ | 必須の文字列値 | 空文字列として保持 |
| `${?param}` | オプショナルパラメータ | 省略可能な文字列値 | 要素を削除 |
| `${@list}` | 配列パラメータ | 複数の値を展開 | 何も追加しない |

### 7.3.2 文字列パラメータ展開 `${param}`

テンプレート内の `${param}` は、指定された文字列値で置換されます。

```toml
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups.commands]]
name = "backup_data"
template = "backup"
params.path = "/data/important"
# 結果: args = ["backup", "/data/important"]
```

**特徴**:
- パラメータが空文字列 `""` の場合でも、配列要素として保持される
- 必須のパラメータに適している

```toml
[[groups.commands]]
name = "backup_empty"
template = "backup"
params.path = ""
# 結果: args = ["backup", ""]  ← 空文字列が引数として渡される
```

### 7.3.3 オプショナルパラメータ展開 `${?param}`

テンプレート内の `${?param}` は、空文字列の場合にその要素を配列から削除します。

```toml
[command_templates.backup_with_option]
cmd = "restic"
args = ["backup", "${?verbose_flag}", "${path}"]

# verbose_flag を指定した場合
[[groups.commands]]
name = "backup_verbose"
template = "backup_with_option"
params.verbose_flag = "--verbose"
params.path = "/data"
# 結果: args = ["backup", "--verbose", "/data"]

# verbose_flag を空にした場合
[[groups.commands]]
name = "backup_quiet"
template = "backup_with_option"
params.verbose_flag = ""
params.path = "/data"
# 結果: args = ["backup", "/data"]  ← "--verbose" が削除される
```

**特徴**:
- 省略可能なフラグやオプションに適している
- 空文字列で要素を削除できる

### 7.3.4 配列パラメータ展開 `${@list}`

テンプレート内の `${@list}` は、配列の全要素で展開されます。

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${path}"]

# 複数のフラグを指定
[[groups.commands]]
name = "backup_debug"
template = "restic_backup"
params.verbose_flags = ["-v", "-v", "--no-cache"]
params.path = "/data"
# 結果: args = ["-v", "-v", "--no-cache", "backup", "/data"]

# フラグなし（空配列）
[[groups.commands]]
name = "backup_silent"
template = "restic_backup"
params.verbose_flags = []
params.path = "/data"
# 結果: args = ["backup", "/data"]  ← verbose_flags の位置に何も追加されない
```

**特徴**:
- 複数のフラグやオプションを一度に指定できる
- 空配列 `[]` で要素を追加しない

### 7.3.5 パラメータ名の規則

パラメータ名は変数名と同じ規則に従います：

- 英字またはアンダースコアで始まる
- 英数字とアンダースコアのみ使用可能
- `__runner_` プレフィックスは予約済み

```toml
# 有効なパラメータ名
params.backup_path = "/data"
params.verbose_level = "2"
params._internal = "value"

# 無効なパラメータ名
params.123path = "/data"           # 数字で開始
params.backup-path = "/data"       # ハイフンは使用不可
params.__runner_test = "value"     # 予約済みプレフィックス
```

## 7.4 テンプレートの使用

### 7.4.1 基本的な使用方法

`[[groups.commands]]` で `template` フィールドを指定してテンプレートを参照します。

```toml
[[groups.commands]]
name = "<command_name>"      # 必須
template = "<template_name>"
params.<param1> = "<value1>"
params.<param2> = ["<value2a>", "<value2b>"]
```

### 7.4.2 必須フィールド

| フィールド | 説明 |
|-----------|------|
| `name` | コマンド名（グループ内でユニーク） |
| `template` | 参照するテンプレート名 |

### 7.4.3 排他的フィールド

`template` を指定した場合、以下のフィールドは指定できません（エラーになります）：

- `cmd`
- `args`
- `env_vars`
- `workdir`
- `timeout`
- `run_as_user`
- `run_as_group`
- `risk_level`
- `output_file`

これらのフィールドはテンプレート側で定義します。

```toml
# エラー例: template と cmd の併用
[[groups.commands]]
name = "test"
template = "restic_backup"
cmd = "foo"  # エラー: template と cmd は排他的
```

### 7.4.4 併用可能なフィールド

`template` と併用可能なフィールド：

| フィールド | 説明 |
|-----------|------|
| `name` | コマンド名（必須） |
| `params` | パラメータ指定 |
| `description` | コマンドの説明 |

### 7.4.5 同じテンプレートから複数のコマンドを作成

同じテンプレートから異なる `name` で複数のコマンドを定義できます：

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${path}"]

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "backup_data"
template = "restic_backup"
params.path = "/data"

[[groups.commands]]
name = "backup_config"
template = "restic_backup"
params.path = "/etc"

[[groups.commands]]
name = "backup_home"
template = "restic_backup"
params.path = "/home"
```

## 7.5 変数展開との組み合わせ

### 7.5.1 展開順序

テンプレート展開と変数展開（`%{...}`）は以下の順序で処理されます：

1. **テンプレート展開**: `${...}`, `${?...}`, `${@...}` を params の値で置換
2. **変数展開**: 結果に含まれる `%{...}` を展開

### 7.5.2 params 内での変数参照

`params` の値には変数参照（`%{...}`）を含めることができます。これにより、グループ固有の変数をテンプレートに渡すことができます。

```toml
[command_templates.restic_backup]
cmd = "restic"
args = ["backup", "${backup_path}"]

[[groups]]
name = "group1"

[groups.vars]
group_root = "/data/group1"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"
params.backup_path = "%{group_root}/volumes"

# 展開プロセス:
# Step 1: テンプレート展開（${...} を params 値で置換）
#   args = ["backup", "%{group_root}/volumes"]
# Step 2: 変数展開（%{...} を展開）
#   args = ["backup", "/data/group1/volumes"]
```

### 7.5.3 テンプレート定義での変数参照

テンプレート定義では、**グローバル変数のみ**参照できます。グローバル変数は大文字で開始する変数です（例：`%{BackupDir}`）。

#### 許可される変数参照

```toml
# グローバル変数の定義
[global.vars]
BackupDir = "/data/backups"
ToolsPath = "/opt/tools"

# OK: テンプレートでグローバル変数を参照
[command_templates.backup_tool]
cmd = "%{ToolsPath}/backup"
args = ["--output", "%{BackupDir}", "${path}"]
```

#### 禁止される変数参照

**ローカル変数**（小文字またはアンダースコアで開始）はテンプレート定義で参照できません：

```toml
[groups.vars]
backup_date = "20250101"  # ローカル変数

# エラー: テンプレートでローカル変数を参照
[command_templates.bad_template]
cmd = "echo"
args = ["%{backup_date}"]  # エラー: ローカル変数は参照不可
```

**理由**:
- テンプレートは複数のグループで再利用される
- ローカル変数はグループごとに異なる値を持つ可能性がある
- グローバル変数のみに制限することで、予測可能で安全な動作を保証

#### 推奨パターン

ローカル変数を使用したい場合は、`params` 経由で渡します：

```toml
# テンプレート定義: グローバル変数とパラメータを使用
[command_templates.backup_with_date]
cmd = "%{ToolsPath}/backup"
args = ["--output", "%{BackupDir}/${date}", "${path}"]

# グループレベル: ローカル変数を定義
[groups.vars]
backup_date = "20250101"

# コマンド: paramsでローカル変数を渡す
[[groups.commands]]
name = "daily_backup"
template = "backup_with_date"
[groups.commands.params]
date = "%{backup_date}"  # paramsでローカル変数を参照
path = "/data/volumes"
```

## 7.6 エスケープシーケンス

### 7.6.1 リテラル `$` の記述

テンプレート定義内でリテラルの `$` 文字を使用したい場合は、`\$` でエスケープします。

> **注意**: TOML ファイルでは `\\$` と記述する必要があります（TOML のエスケープルールにより `\$` になります）。

```toml
[command_templates.cost_report]
cmd = "echo"
args = ["Cost: \\$100", "${item}"]

[[groups.commands]]
name = "report"
template = "cost_report"
params.item = "Widget"
# 結果: args = ["Cost: $100", "Widget"]
```

### 7.6.2 既存のエスケープとの一貫性

このエスケープ記法は、変数展開の `\%` エスケープと同じ方式です：

- `\%{var}` → `%{var}` （変数展開されない）
- `\$` → `$` （リテラル）

## 7.7 エラーと検証

### 7.7.1 よくあるエラー

#### 存在しないテンプレートの参照

```toml
[[groups.commands]]
name = "test"
template = "nonexistent_template"  # エラー: template "nonexistent_template" not found
```

#### 必須パラメータの未指定

```toml
[command_templates.backup]
cmd = "restic"
args = ["backup", "${path}"]  # path は必須

[[groups.commands]]
name = "backup_test"
template = "backup"
# エラー: parameter "path" is required but not provided in template "backup"
```

#### template と cmd の併用

```toml
[[groups.commands]]
name = "test"
template = "backup"
cmd = "/bin/echo"  # エラー: cannot specify both "template" and "cmd" fields
```

#### テンプレート定義での name フィールド

```toml
[command_templates.bad_template]
name = "should_not_be_here"  # エラー: template definition cannot contain "name" field
cmd = "echo"
```

#### テンプレート定義での変数参照

```toml
[command_templates.dangerous]
cmd = "echo"
args = ["%{secret}"]  # エラー: template contains forbidden pattern "%{" in args
```

### 7.7.2 警告

#### 未使用のパラメータ

テンプレートで使用されていないパラメータを渡した場合、警告が出力されます（エラーではありません）：

```toml
[command_templates.simple]
cmd = "echo"
args = ["hello"]  # パラメータを使用していない

[[groups.commands]]
name = "test"
template = "simple"
params.unused = "value"  # 警告: unused parameter "unused" in template "simple"
```

## 7.8 実践的な設定例

### 7.8.1 バックアップタスクの共通化

```toml
version = "1.0"

# テンプレート定義
[command_templates.restic_backup]
cmd = "restic"
args = ["${@verbose_flags}", "backup", "${backup_path}"]
timeout = 3600
risk_level = "medium"

[command_templates.restic_forget]
cmd = "restic"
args = ["forget", "--prune", "--keep-daily", "${keep_daily}", "--keep-weekly", "${keep_weekly}", "--keep-monthly", "${keep_monthly}"]
timeout = 1800
risk_level = "medium"

# グループ1: 重要データ（詳細ログ、長期保存）
[[groups]]
name = "important_data"

[groups.vars]
data_root = "/data/important"

[[groups.commands]]
name = "backup"
template = "restic_backup"
params.verbose_flags = ["-v", "-v"]
params.backup_path = "%{data_root}"

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
params.keep_daily = "14"
params.keep_weekly = "8"
params.keep_monthly = "12"

# グループ2: 一時データ（静音、短期保存）
[[groups]]
name = "temp_data"

[groups.vars]
data_root = "/data/temp"

[[groups.commands]]
name = "backup"
template = "restic_backup"
params.verbose_flags = []  # 静音モード
params.backup_path = "%{data_root}"

[[groups.commands]]
name = "cleanup"
template = "restic_forget"
params.keep_daily = "3"
params.keep_weekly = "1"
params.keep_monthly = "0"
```

### 7.8.2 データベース操作の共通化

```toml
version = "1.0"

[command_templates.pg_dump]
cmd = "/usr/bin/pg_dump"
args = ["${?verbose}", "-U", "${db_user}", "-d", "${database}", "-f", "${output_file}"]
timeout = 1800
risk_level = "medium"

[command_templates.pg_restore]
cmd = "/usr/bin/pg_restore"
args = ["${?verbose}", "-U", "${db_user}", "-d", "${database}", "${input_file}"]
timeout = 3600
risk_level = "high"

[[groups]]
name = "database_backup"

[groups.vars]
backup_dir = "/var/backups/postgres"

[[groups.commands]]
name = "backup_main_db"
template = "pg_dump"
params.verbose = "--verbose"
params.db_user = "postgres"
params.database = "main_production"
params.output_file = "%{backup_dir}/main_db.dump"

[[groups.commands]]
name = "backup_logs_db"
template = "pg_dump"
params.verbose = ""  # 静音モード
params.db_user = "postgres"
params.database = "logs"
params.output_file = "%{backup_dir}/logs_db.dump"
```

### 7.8.3 システム監視タスクの共通化

```toml
version = "1.0"

[command_templates.check_disk]
cmd = "/bin/df"
args = ["-h", "${mount_point}"]
timeout = 30
risk_level = "low"

[command_templates.check_service]
cmd = "/usr/bin/systemctl"
args = ["status", "${service_name}"]
timeout = 30
risk_level = "low"

[[groups]]
name = "system_monitoring"

[[groups.commands]]
name = "check_root_disk"
template = "check_disk"
params.mount_point = "/"

[[groups.commands]]
name = "check_data_disk"
template = "check_disk"
params.mount_point = "/data"

[[groups.commands]]
name = "check_nginx"
template = "check_service"
params.service_name = "nginx"

[[groups.commands]]
name = "check_postgres"
template = "check_service"
params.service_name = "postgresql"
```

## 7.9 ベストプラクティス

### 7.9.1 テンプレート設計のガイドライン

1. **単一責任**: 各テンプレートは1つの目的に集中する
2. **適切なパラメータ化**: 変更される可能性が高い部分をパラメータ化する
3. **意味のある名前**: テンプレート名から目的が分かるようにする
4. **デフォルト値の考慮**: オプショナルパラメータ（`${?...}`）を活用する

### 7.9.2 パラメータ設計のガイドライン

1. **必須 vs オプショナル**: 常に必要な値は `${param}`、省略可能な値は `${?param}` を使用
2. **配列の活用**: 複数のフラグやオプションは `${@list}` で配列として渡す
3. **変数との組み合わせ**: グループ固有の値は `params` 内で `%{var}` を使用して参照

### 7.9.3 セキュリティのガイドライン

1. **テンプレート定義に変数参照を含めない**: 常に `params` 経由で明示的に渡す
2. **パラメータ値の検証**: 展開後のコマンドは自動的にセキュリティ検証される
3. **最小権限の原則**: テンプレートで `run_as_user` や `risk_level` を適切に設定

## 次のステップ

コマンドテンプレート機能を理解したら、以下の章も参照してください：

- [第6章: コマンドレベル設定](06_command_level.ja.md) - テンプレートなしでのコマンド定義
- [第8章: 変数展開機能](08_variable_expansion.ja.md) - `%{var}` 形式の変数展開
- [第9章: 実践的な設定例](09_practical_examples.ja.md) - より多くの設定例
