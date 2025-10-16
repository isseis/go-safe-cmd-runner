# 第6章: コマンドレベル設定 [[groups.commands]]

## 概要

`[[groups.commands]]` セクションは、実際に実行するコマンドを定義します。各グループには1つ以上のコマンドが必要です。コマンドはグループ内で定義された順序で実行されます。

## 6.1 コマンドの基本設定

### 6.1.1 name - コマンド名

#### 概要

コマンドを識別するための一意な名前を指定します。

#### 文法

```toml
[[groups.commands]]
name = "コマンド名"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | 必須 |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | 英数字、アンダースコア、ハイフン |
| **一意性** | グループ内で一意である必要がある |

#### 設定例

```toml
version = "1.0"

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups.commands]]
name = "backup_config"
cmd = "/usr/bin/tar"
args = ["-czf", "config.tar.gz", "/etc/myapp"]
```

### 6.1.2 description - 説明

#### 概要

コマンドの目的や役割を説明する人間が読むためのテキストです。

#### 文法

```toml
[[groups.commands]]
name = "example"
description = "コマンドの説明"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション(推奨) |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | 任意の文字列 |

#### 設定例

```toml
[[groups.commands]]
name = "daily_backup"
description = "PostgreSQL データベースの日次完全バックアップ(全テーブル)"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]
```

### 6.1.3 cmd - 実行コマンド

#### 概要

実行するコマンドのパスまたは名前を指定します。これはコマンドの最も重要なパラメータです。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンドパス"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | 必須 |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | 絶対パス、または PATH 上のコマンド名 |
| **変数展開** | %{VAR} 形式の変数展開が可能(第7章参照) |

#### 設定例

#### 例1: 絶対パスの指定

```toml
[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"
args = ["-la"]
```

#### 例2: PATH 上のコマンド名

```toml
[[groups.commands]]
name = "list_files"
cmd = "ls"  # PATH から検索される
args = ["-la"]
```

#### 例3: 変数展開を使用

```toml
[[groups.commands]]
name = "custom_tool"
cmd = "%{tool_dir}/my-script"
vars = ["tool_dir=/opt/tools"]
# 実際には /opt/tools/my-script が実行される
```

#### セキュリティ上の注意

1. **絶対パスの推奨**: セキュリティのため、絶対パスを使用することを推奨
2. **PATH 依存の危険性**: PATH 上のコマンドを使用する場合、意図しないコマンドが実行される可能性
3. **検証の重要性**: `verify_files` でコマンドの整合性を検証

```toml
# 推奨: 絶対パスと検証
[global]
verify_files = ["/usr/bin/pg_dump"]

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/pg_dump"  # 絶対パス
args = ["mydb"]

# 非推奨: PATH 依存
[[groups.commands]]
name = "backup"
cmd = "pg_dump"  # どの pg_dump が実行されるか不明確
args = ["mydb"]
```

### 6.1.4 args - 引数

#### 概要

コマンドに渡す引数を配列で指定します。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = ["引数1", "引数2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **デフォルト値** | [] (引数なし) |
| **有効な値** | 任意の文字列のリスト |
| **変数展開** | %{VAR} 形式の変数展開が可能(第7章参照) |

#### 設定例

#### 例1: 基本的な引数

```toml
[[groups.commands]]
name = "echo_message"
cmd = "echo"
args = ["Hello, World!"]
```

#### 例2: 複数の引数

```toml
[[groups.commands]]
name = "copy_file"
cmd = "/bin/cp"
args = ["-v", "/source/file.txt", "/dest/file.txt"]
```

#### 例3: 引数なし

```toml
[[groups.commands]]
name = "show_date"
cmd = "date"
args = []  # または省略
```

#### 例4: 変数展開を含む引数

```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "%{backup_file}", "%{source_dir}"]
vars = [
    "backup_file=/backups/backup.tar.gz",
    "source_dir=/data",
]
```

#### 重要な注意事項

##### 1. 引数のセキュリティ

各引数は個別の配列要素として指定します。シェルのクォーティングやエスケープは不要です。

```toml
# 正しい: 引数を個別に指定
[[groups.commands]]
name = "find_files"
cmd = "/usr/bin/find"
args = ["/var/log", "-name", "*.log", "-type", "f"]

# 誤り: スペース区切りで1つの文字列にしない
[[groups.commands]]
name = "find_files"
cmd = "/usr/bin/find"
args = ["/var/log -name *.log -type f"]  # これは1つの引数として扱われる
```

##### 2. シェル機能は使用不可

go-safe-cmd-runner はシェルを介さずに直接コマンドを実行します。以下のシェル機能は使用できません:

```toml
# 誤り: パイプは使用不可
[[groups.commands]]
name = "grep_and_count"
cmd = "grep"
args = ["ERROR", "app.log", "|", "wc", "-l"]  # パイプは機能しない

# 誤り: リダイレクトは使用不可
[[groups.commands]]
name = "save_output"
cmd = "echo"
args = ["test", ">", "output.txt"]  # リダイレクトは機能しない

# 正しい: output パラメータを使用
[[groups.commands]]
name = "save_output"
cmd = "echo"
args = ["test"]
output = "output.txt"  # これが正しい方法
```

##### 3. スペースを含む引数

スペースを含む引数も配列要素として自然に扱えます:

```toml
[[groups.commands]]
name = "echo_message"
cmd = "echo"
args = ["This is a message with spaces"]  # スペースを含むがそのまま1つの引数
```

## 6.2 変数と環境設定

### 6.2.1 vars - 内部変数

#### 概要

TOML ファイル内で変数展開に使用される内部変数を `KEY=VALUE` 形式で指定します。コマンドレベルで定義された `vars` は、グローバルレベルおよびグループレベルの `vars` とマージされます(Union による結合)。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
vars = ["key1=value1", "key2=value2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ、コマンド |
| **デフォルト値** | [] |
| **形式** | "KEY=VALUE" |
| **変数名の制約** | POSIX 準拠 (英数字とアンダースコア、数字で開始不可)、`__runner_` プレフィックスは予約済み |
| **継承動作** | マージ (Union) - 下位レベルが上位レベルを上書き |

#### 役割

- **TOML 内での変数展開**: `cmd`、`args`、`env` の値で `%{VAR}` 形式で参照可能
- **プロセス環境への非伝搬**: 子プロセスの環境変数には含まれない
- **階層的なマージ**: グローバル → グループ → コマンドの順でマージ

#### 設定例

#### 例1: コマンド固有の変数

```toml
[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/pg_dump"
vars = [
    "db_name=production_db",
    "backup_dir=/var/backups/postgres",
]
args = ["-d", "%{db_name}", "-f", "%{backup_dir}/%{db_name}.sql"]
```

#### 例2: 階層的なマージ

```toml
[global]
vars = ["base_dir=/opt/app", "log_level=info"]

[[groups]]
name = "admin_tasks"
vars = ["log_level=debug"]  # グローバルの log_level を上書き

[[groups.commands]]
name = "task1"
cmd = "/bin/task"
vars = ["task_id=42"]  # base_dir, log_level を継承、task_id を追加
args = ["--dir", "%{base_dir}", "--log", "%{log_level}", "--id", "%{task_id}"]
# 最終的な変数: base_dir=/opt/app, log_level=debug, task_id=42
```

#### 重要な注意事項

##### 1. プロセス環境への非伝搬

`vars` で定義した変数は、子プロセスの環境変数には設定されません:

```toml
[[groups.commands]]
name = "print_vars"
cmd = "/bin/sh"
vars = ["my_var=hello"]
args = ["-c", "echo $my_var"]  # my_var は空文字列 (環境変数に存在しない)
```

子プロセスに環境変数を渡すには、`env` パラメータを使用します:

```toml
[[groups.commands]]
name = "print_vars"
cmd = "/bin/sh"
vars = ["my_var=hello"]
env = ["MY_VAR=%{my_var}"]  # vars の値を env で環境変数に変換
args = ["-c", "echo $MY_VAR"]  # MY_VAR=hello が出力される
```

##### 2. 変数名の制約

変数名は以下のルールに従う必要があります:

- **POSIX 準拠**: 英数字とアンダースコアのみ使用可能、数字で開始不可
- **予約プレフィックス**: `__runner_` で始まる名前は自動変数用に予約済み

```toml
# 正しい例
vars = ["my_var=value", "VAR_123=value", "_private=value"]

# 誤った例
vars = [
    "123var=value",      # 数字で開始
    "my-var=value",      # ハイフンは使用不可
    "__runner_custom=x", # 予約プレフィックス
]
```

##### 3. 自動変数

Runner は以下の自動変数を提供します(上書き不可):

- `__RUNNER_DATETIME`: コマンド実行時刻 (ISO 8601 形式)
- `__RUNNER_PID`: Runner プロセスの PID

```toml
[[groups.commands]]
name = "log_execution"
cmd = "/usr/bin/logger"
args = ["Executed at %{__RUNNER_DATETIME} by PID %{__RUNNER_PID}"]
```

### 6.2.2 from_env - システム環境変数のインポート

#### 概要

Runner プロセスが動作しているシステム環境変数を TOML 内の変数展開用にインポートする変数名を指定します。コマンドレベルの `from_env` は、グローバルおよびグループレベルの `from_env` を完全に上書きします(Override 動作)。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
from_env = ["VAR1", "VAR2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ、コマンド |
| **デフォルト値** | [] |
| **形式** | 変数名のみ (VALUE は不要) |
| **セキュリティ制約** | `env_allowlist` に含まれる変数のみインポート可能 |
| **継承動作** | 上書き (Override) - 下位レベルが上位レベルを完全に置き換え |

#### 役割

- **システム環境変数の取り込み**: Runner が動作する環境の変数を TOML 内で利用可能にする
- **TOML 内での変数展開**: インポートした変数を `%{VAR}` 形式で参照可能
- **セキュリティ管理**: `env_allowlist` による制御

#### 設定例

#### 例1: 基本的なインポート

```toml
[global]
env_allowlist = ["HOME", "USER", "PATH"]

[[groups.commands]]
name = "show_user_info"
cmd = "/bin/echo"
from_env = ["USER", "HOME"]
args = ["User: %{USER}, Home: %{HOME}"]
```

#### 例2: Override 動作

```toml
[global]
env_allowlist = ["HOME", "USER", "PATH", "LANG"]
from_env = ["HOME", "USER"]  # グローバルレベル

[[groups]]
name = "intl_tasks"
from_env = ["LANG"]  # グループレベル: グローバルの from_env を上書き

[[groups.commands]]
name = "task1"
cmd = "/bin/echo"
# from_env を指定しないため、グループの from_env が適用される
args = ["Language: %{LANG}"]  # HOME, USER は利用不可

[[groups.commands]]
name = "task2"
cmd = "/bin/echo"
from_env = ["HOME", "PATH"]  # コマンドレベル: グループの from_env を上書き
args = ["Path: %{PATH}"]  # LANG は利用不可、HOME と PATH のみ利用可能
```

#### 重要な注意事項

##### 1. env_allowlist との関係

`from_env` でインポートする変数は、必ず `env_allowlist` に含まれている必要があります:

```toml
[global]
env_allowlist = ["HOME", "USER"]

[[groups.commands]]
name = "example"
cmd = "/bin/echo"
from_env = ["HOME", "PATH"]  # エラー: PATH は env_allowlist に含まれていない
args = ["%{HOME}"]
```

##### 2. Override による置き換え

コマンドレベルで `from_env` を指定すると、グループおよびグローバルの `from_env` は完全に無視されます:

```toml
[global]
from_env = ["HOME", "USER", "PATH"]

[[groups]]
name = "tasks"
from_env = ["LANG", "LC_ALL"]

[[groups.commands]]
name = "task1"
cmd = "/bin/echo"
from_env = ["PWD"]  # HOME, USER, PATH, LANG, LC_ALL はすべて無視される
args = ["%{PWD}"]   # PWD のみ利用可能
```

##### 3. 存在しない変数のインポート

`from_env` で指定した変数がシステム環境に存在しない場合、その変数は空文字列として扱われます:

```toml
[[groups.commands]]
name = "example"
cmd = "/bin/echo"
from_env = ["NONEXISTENT_VAR"]
args = ["Value: %{NONEXISTENT_VAR}"]  # "Value: " と出力される
```

### 6.2.3 env - プロセス環境変数

#### 概要

コマンド実行時に子プロセスに設定する環境変数を `KEY=VALUE` 形式で指定します。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
env = ["KEY1=value1", "KEY2=value2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **デフォルト値** | [] |
| **形式** | "KEY=VALUE" |
| **変数展開** | VALUE 部分で %{VAR} 形式の変数展開が可能 |

#### 役割

- **プロセス環境変数**: 子プロセスの環境変数として設定される
- **コマンド設定**: コマンドの動作を環境変数で制御
- **認証情報**: データベース接続情報などの設定
- **動作モード**: デバッグモードなどの切り替え

#### 設定例

#### 例1: 基本的な環境変数

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env = [
    "LOG_LEVEL=debug",
    "PORT=8080",
    "CONFIG_FILE=/etc/app/config.yaml",
]
```

#### 例2: データベース接続情報

```toml
[[groups.commands]]
name = "db_migration"
cmd = "/opt/app/migrate"
args = []
env = [
    "DATABASE_URL=postgresql://localhost:5432/mydb",
    "DB_USER=appuser",
    "DB_PASSWORD=secret123",
]
```

#### 例3: 変数展開を使用

```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []
vars = [
    "backup_dir=/var/backups",
    "date=2025-01-15",
]
env = [
    "BACKUP_FILE=%{backup_dir}/backup-%{date}.tar.gz",
]
# BACKUP_FILE は /var/backups/backup-2025-01-15.tar.gz に展開される
```

#### 重要な注意事項

##### 1. env_allowlist との関係

設定した環境変数は、グループまたはグローバルの `env_allowlist` に含まれている必要があります:

```toml
[global]
env_allowlist = ["PATH", "LOG_LEVEL", "DATABASE_URL"]

[[groups]]
name = "app_group"

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/server"
args = []
env = [
    "LOG_LEVEL=debug",      # OK: env_allowlist に含まれる
    "DATABASE_URL=...",     # OK: env_allowlist に含まれる
    "UNAUTHORIZED_VAR=x",   # エラー: env_allowlist に含まれない
]
```

##### 2. 形式のルール

- `KEY=VALUE` 形式が必須
- `=` が含まれない場合はエラー
- VALUE が空でも `KEY=` と記述が必要

```toml
# 正しい
env = [
    "KEY=value",
    "EMPTY_VAR=",  # 空の値
]

# 誤り
env = [
    "KEY",         # エラー: = がない
    "KEY value",   # エラー: = がない
]
```

##### 3. 重複の禁止

同じキーを複数回定義することはできません:

```toml
# 誤り: LOG_LEVEL が重複
env = [
    "LOG_LEVEL=debug",
    "LOG_LEVEL=info",  # エラー: 重複
]
```

### 6.2.4 dir - 実行ディレクトリ

#### 概要

このコマンド専用の実行ディレクトリを指定します。

> **注意**: 現在のバージョンでは `dir` パラメータは実装されていません。作業ディレクトリはグループレベルの `workdir` またはグローバルの `workdir` で制御してください。

## 6.3 タイムアウト設定

### 6.3.1 timeout - コマンド固有タイムアウト

#### 概要

このコマンド専用のタイムアウト時間を秒単位で指定します。グローバルの `timeout` をオーバーライドします。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
timeout = 秒数
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 整数 (int) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、コマンド |
| **デフォルト値** | グローバルの timeout |
| **有効な値** | 正の整数(秒単位) |
| **オーバーライド** | グローバル設定をオーバーライド |

#### 設定例

#### 例1: 長時間実行コマンド

```toml
[global]
timeout = 60  # デフォルト: 60秒

[[groups]]
name = "mixed_tasks"

[[groups.commands]]
name = "quick_check"
cmd = "ping"
args = ["-c", "3", "localhost"]
# timeout 未指定 → グローバルの 60秒

[[groups.commands]]
name = "long_backup"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]
output = "full_backup.sql"
timeout = 1800  # 30分 = 1800秒
```

#### 例2: タイムアウトの段階的設定

```toml
[global]
timeout = 300  # デフォルト: 5分

[[groups]]
name = "backup_tasks"

[[groups.commands]]
name = "small_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["small_db"]
timeout = 60  # 1分で十分

[[groups.commands]]
name = "medium_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["medium_db"]
# グローバルの 300秒(5分)を使用

[[groups.commands]]
name = "large_db_backup"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
timeout = 3600  # 1時間
```

## 6.4 権限管理

### 6.4.1 run_as_user - 実行ユーザー

#### 概要

コマンドを特定のユーザー権限で実行します。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
run_as_user = "ユーザー名"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | システムに存在するユーザー名 |
| **前提条件** | go-safe-cmd-runner が root 権限で実行されている必要がある |

#### 設定例

#### 例1: root 権限でのコマンド実行

```toml
[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
# root 権限が必要なパッケージ更新
```

#### 例2: 特定ユーザーでのコマンド実行

```toml
[[groups.commands]]
name = "user_backup"
cmd = "/home/appuser/backup.sh"
args = []
run_as_user = "appuser"
# appuser の権限でスクリプトを実行
```

#### セキュリティ上の注意

1. **最小権限の原則**: 必要最小限の権限で実行
2. **root の使用を最小化**: root 権限が本当に必要な場合のみ使用
3. **監査ログ**: 権限昇格は自動的に監査ログに記録される

### 6.4.2 run_as_group - 実行グループ

#### 概要

コマンドを特定のグループ権限で実行します。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
run_as_group = "グループ名"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | システムに存在するグループ名 |
| **前提条件** | go-safe-cmd-runner が適切な権限で実行されている必要がある |

#### 設定例

```toml
[[groups.commands]]
name = "read_log"
cmd = "/usr/bin/cat"
args = ["/var/log/app/app.log"]
run_as_group = "loggroup"
# loggroup グループの権限でログを読み取り
```

#### 組み合わせの例

```toml
[[groups.commands]]
name = "privileged_operation"
cmd = "/opt/admin/tool"
args = []
run_as_user = "admin"
run_as_group = "admingroup"
# admin ユーザーおよび admingroup グループの権限で実行
```

## 6.5 リスク管理

### 6.5.1 max_risk_level - 最大リスクレベル

#### 概要

コマンドに許容される最大のリスクレベルを指定します。コマンドのリスクが指定されたレベルを超える場合、実行が拒否されます。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
max_risk_level = "リスクレベル"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **デフォルト値** | "low" |
| **有効な値** | "low", "medium", "high" |

### 6.5.2 リスクレベルの種類

#### リスクレベルの定義

| レベル | 説明 | 例 |
|--------|------|-----|
| **low** | 低リスク | 読み取り専用コマンド、情報取得 |
| **medium** | 中リスク | ファイル作成・変更、ネットワークアクセス |
| **high** | 高リスク | システム設定変更、パッケージインストール |

#### リスク評価の仕組み

go-safe-cmd-runner は以下の要素からコマンドのリスクを自動評価します:

1. **コマンドの種類**: rm, chmod, chown などの危険なコマンド
2. **引数パターン**: 再帰削除(-rf)、強制実行(-f)など
3. **権限昇格**: run_as_user, run_as_group の使用
4. **ネットワークアクセス**: curl, wget などのネットワークコマンド

#### 設定例

#### 例1: 低リスクコマンド

```toml
[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"
args = ["-la"]
max_risk_level = "low"  # 読み取り専用なので低リスク
```

#### 例2: 中リスクコマンド

```toml
[[groups.commands]]
name = "create_backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
max_risk_level = "medium"  # ファイル作成なので中リスク
```

#### 例3: 高リスクコマンド

```toml
[[groups.commands]]
name = "install_package"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "package-name"]
run_as_user = "root"
max_risk_level = "high"  # システム変更と権限昇格なので高リスク
```

#### 例4: リスクレベル違反時の挙動

```toml
[[groups.commands]]
name = "dangerous_operation"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
max_risk_level = "low"  # rm -rf は中リスク以上
# このコマンドは実行拒否される(リスクレベル超過)
```

#### セキュリティのベストプラクティス

```toml
# 推奨: 適切なリスクレベルの設定
[[groups]]
name = "safe_operations"

[[groups.commands]]
name = "read_config"
cmd = "/bin/cat"
args = ["/etc/app/config.yaml"]
max_risk_level = "low"  # 読み取りのみ

[[groups.commands]]
name = "backup_data"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
max_risk_level = "medium"  # ファイル作成

[[groups.commands]]
name = "system_update"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
max_risk_level = "high"  # システム変更と権限昇格
```

## 6.6 出力管理

### 6.6.1 output - 標準出力キャプチャ

#### 概要

コマンドの標準出力をファイルに保存します。

#### 文法

```toml
[[groups.commands]]
name = "example"
cmd = "コマンド"
args = []
output = "ファイルパス"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | コマンドのみ |
| **有効な値** | 相対パスまたは絶対パス |
| **サイズ制限** | グローバルの max_output_size による制限 |
| **ディレクトリ作成** | 必要に応じて自動作成 |

#### 役割

- **ログ保存**: コマンド出力の永続化
- **結果の記録**: 処理結果をファイルとして保存
- **監査証跡**: 実行履歴の証拠として保管

#### 設定例

#### 例1: 相対パスでの出力

```toml
[[groups]]
name = "data_export"
workdir = "/var/app/output"

[[groups.commands]]
name = "export_users"
cmd = "/opt/app/export"
args = ["--table", "users"]
output = "users.csv"
# /var/app/output/users.csv に保存
```

#### 例2: 絶対パスでの出力

```toml
[[groups.commands]]
name = "system_report"
cmd = "/usr/bin/systemctl"
args = ["status"]
output = "/var/log/reports/system_status.txt"
# 絶対パスで保存
```

#### 例3: サブディレクトリへの出力

```toml
[[groups]]
name = "log_export"
workdir = "/var/app"

[[groups.commands]]
name = "export_logs"
cmd = "/opt/app/export-logs"
args = []
output = "logs/export/output.txt"
# /var/app/logs/export/ ディレクトリが自動作成され、
# /var/app/logs/export/output.txt に保存
```

#### 例4: 複数コマンドの出力

```toml
[[groups]]
name = "system_info"
workdir = "/tmp/reports"

[[groups.commands]]
name = "disk_usage"
cmd = "/bin/df"
args = ["-h"]
output = "disk_usage.txt"

[[groups.commands]]
name = "memory_info"
cmd = "/usr/bin/free"
args = ["-h"]
output = "memory_info.txt"

[[groups.commands]]
name = "process_list"
cmd = "/bin/ps"
args = ["aux"]
output = "processes.txt"
```

#### 重要な注意事項

##### 1. サイズ制限

出力サイズは `max_output_size` (グローバル設定)によって制限されます:

```toml
[global]
max_output_size = 1048576  # 1MB

[[groups.commands]]
name = "large_export"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output = "dump.sql"
# 出力が 1MB を超える場合、警告が記録される
```

##### 2. パーミッション

出力ファイルのパーミッションは以下のように設定されます:
- ファイル: 0600 (所有者のみ読み書き可能)
- ディレクトリ: 0700 (所有者のみアクセス可能)

##### 3. 既存ファイルの扱い

同名のファイルが存在する場合、上書きされます:

```toml
[[groups.commands]]
name = "daily_report"
cmd = "/opt/app/report"
args = []
output = "daily.txt"
# 既存の daily.txt は上書きされる
```

##### 4. 標準エラー出力

`output` パラメータは標準出力(stdout)のみをキャプチャします。標準エラー出力(stderr)は通常のログに記録されます。

## コマンド設定の全体例

以下は、コマンドレベルの設定を組み合わせた実践的な例です:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/app"
log_level = "info"
env_allowlist = ["PATH", "HOME", "DATABASE_URL", "BACKUP_DIR"]
max_output_size = 10485760  # 10MB
verify_files = ["/bin/sh"]

[[groups]]
name = "database_operations"
description = "データベース関連の操作"
priority = 10
workdir = "/var/backups/db"
env_allowlist = ["PATH", "DATABASE_URL", "BACKUP_DIR"]
verify_files = ["/usr/bin/pg_dump", "/usr/bin/psql"]

# コマンド1: データベースバックアップ
[[groups.commands]]
name = "full_backup"
description = "PostgreSQL 全データベースのバックアップ"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases", "--verbose"]
env = ["DATABASE_URL=postgresql://localhost/postgres"]
output = "full_backup.sql"
timeout = 1800  # 30分
max_risk_level = "medium"

# コマンド2: バックアップの検証
[[groups.commands]]
name = "verify_backup"
description = "バックアップファイルの整合性確認"
cmd = "/usr/bin/psql"
args = ["--dry-run", "-f", "full_backup.sql"]
env = ["DATABASE_URL=postgresql://localhost/testdb"]
output = "verification.log"
timeout = 600  # 10分
max_risk_level = "low"

# コマンド3: 古いバックアップの削除
[[groups.commands]]
name = "cleanup_old_backups"
description = "30日以上前のバックアップファイルを削除"
cmd = "/usr/bin/find"
args = [".", "-name", "*.sql", "-mtime", "+30", "-delete"]
timeout = 300  # 5分
max_risk_level = "medium"

[[groups]]
name = "system_maintenance"
description = "システムメンテナンスタスク"
priority = 20
workdir = "/tmp"
env_allowlist = []  # 環境変数なし

# コマンド4: ディスク使用量レポート
[[groups.commands]]
name = "disk_report"
description = "ディスク使用量のレポート生成"
cmd = "/bin/df"
args = ["-h", "/var"]
output = "/var/log/disk_usage.txt"
timeout = 60
max_risk_level = "low"

# コマンド5: システムアップデート(root権限)
[[groups.commands]]
name = "system_update"
description = "システムパッケージの更新"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
timeout = 600
max_risk_level = "high"
```

## 次のステップ

次章では、変数展開機能について詳しく解説します。`%{VAR}` 形式の変数を使用して、動的なコマンド構築を行う方法を学びます。
