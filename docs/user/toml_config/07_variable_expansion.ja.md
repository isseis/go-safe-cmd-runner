# 第7章: 変数展開機能

## 7.1 変数展開の概要

変数展開機能は、コマンドやその引数に変数を埋め込み、実行時に実際の値に置き換える機能です。go-safe-cmd-runnerでは、**内部変数**(TOML展開専用)と**プロセス環境変数**(子プロセスに渡される環境変数)を明確に分離し、セキュリティと明確性を向上させています。

### 主な利点

1. **セキュリティ向上**: 内部変数とプロセス環境変数を分離し、意図しない情報漏洩を防止
2. **動的なコマンド構築**: 実行時に値を決定できる
3. **設定の再利用**: 同じ変数を複数の場所で使用
4. **環境の切り替え**: 開発/本番環境などの切り替えが容易
5. **保守性の向上**: 変更箇所を一箇所に集約

### 変数の種類

go-safe-cmd-runnerでは、2種類の変数を扱います:

| 変数の種類 | 用途 | 参照構文 | 定義方法 | 子プロセスへの影響 |
|-----------|------|---------|---------|------------------|
| **内部変数** | TOML設定ファイル内での展開専用 | `%{VAR}` | `vars`, `from_env` | なし(デフォルト) |
| **プロセス環境変数** | 子プロセスの環境変数として設定 | - | `env` | あり |

### 使用可能な場所

変数展開は以下の場所で使用できます:

- **cmd**: 実行するコマンドのパス(`%{VAR}` を使用)
- **args**: コマンドの引数(`%{VAR}` を使用)
- **env**: プロセス環境変数の値(`%{VAR}` を使用可能)
- **verify_files**: 検証対象ファイルパス(`%{VAR}` を使用)
- **vars**: 内部変数の定義(`%{VAR}` で他の内部変数を参照可能)

## 7.2 変数展開の文法

### 内部変数の参照構文

内部変数は `%{変数名}` の形式で記述します:

```toml
cmd = "%{VARIABLE_NAME}"
args = ["%{ARG1}", "%{ARG2}"]
env = ["VAR=%{VALUE}"]
```

### 変数名のルール

- 英字(大文字・小文字)、数字、アンダースコア(`_`)が使用可能
- 推奨は小文字とアンダースコアを使用(例: `my_variable`, `app_dir`)
- 先頭は英字またはアンダースコアで開始
- 大文字小文字を区別する(`home` と `HOME` は別の変数)
- 予約プレフィックス `__runner_` で始まる変数名は使用不可

```toml
# 有効な変数名
"%{path}"
"%{my_tool}"
"%{_private_var}"
"%{var123}"
"%{HOME}"

# 無効な変数名
"%{123var}"         # 数字で開始
"%{my-var}"         # ハイフンは使用不可
"%{my.var}"         # ドットは使用不可
"%{__runner_test}"  # 予約プレフィックス
```

## 7.3 内部変数の定義

### 7.3.1 `vars` フィールドによる内部変数定義

#### 概要

`vars` フィールドを使用して、TOML展開専用の内部変数を定義できます。これらの変数は子プロセスの環境変数には影響しません。

#### 設定形式

```toml
[global]
vars = [
    "app_dir=/opt/myapp",
    "log_level=info"
]

[[groups]]
name = "backup"
vars = [
    "backup_dir=%{app_dir}/backups",
    "retention_days=30"
]

[[groups.commands]]
name = "backup_db"
vars = [
    "timestamp=20250114",
    "output_file=%{backup_dir}/dump_%{timestamp}.sql"
]
cmd = "/usr/bin/pg_dump"
args = ["-f", "%{output_file}", "mydb"]
```

#### スコープと継承

| レベル | スコープ | 継承ルール |
|--------|---------|-----------|
| **Global.vars** | すべてのグループとコマンドから参照可能 | - |
| **Group.vars** | そのグループ内のコマンドから参照可能 | Global.vars とマージ(Group が優先) |
| **Command.vars** | そのコマンド内でのみ参照可能 | Global + Group + Command をマージ |

#### 参照構文

- `%{変数名}` の形式で参照
- `cmd`, `args`, `verify_files`, `env` の値、および他の `vars` 定義内で使用可能

#### 基本的な例

```toml
version = "1.0"

[global]
vars = ["base_dir=/opt"]

[[groups]]
name = "prod_backup"
vars = ["db_tools=%{base_dir}/db-tools"]

[[groups.commands]]
name = "db_dump"
vars = [
    "timestamp=20250114",
    "output_file=%{base_dir}/dump_%{timestamp}.sql"
]
cmd = "%{db_tools}/dump.sh"
args = ["-o", "%{output_file}"]
```

### 7.3.2 `from_env` によるシステム環境変数の取り込み

#### 概要

`from_env` フィールドを使用して、システム環境変数を内部変数として取り込むことができます。

#### 設定形式

```toml
[global]
env_allowlist = ["HOME", "PATH", "USER"]
from_env = [
    "home=HOME",
    "user_path=PATH",
    "username=USER"
]

[[groups]]
name = "example"
from_env = [
    "custom=CUSTOM_VAR"  # このグループ専用の取り込み
]
```

#### 構文

`内部変数名=システム環境変数名` の形式で記述します:

- **左辺**: 内部変数名(推奨は小文字、例: `home`, `user_path`)
- **右辺**: システム環境変数名(通常は大文字、例: `HOME`, `PATH`)

#### セキュリティ制約

- `from_env` で参照するシステム環境変数は必ず `env_allowlist` に含まれている必要があります
- `env_allowlist` にない変数を参照するとエラーになります

#### 継承ルール

| レベル | 継承動作 |
|--------|---------|
| **Global.from_env** | すべてのグループから継承される(デフォルト) |
| **Group.from_env** | 定義されている場合は Global.from_env を**上書き**(Override) |
| **Group.from_env が nil** | Global.from_env を継承 |
| **Group.from_env が []** | 空のマッピング(どの環境変数も取り込まない) |

#### 例: システム環境変数の取り込み

```toml
version = "1.0"

[global]
env_allowlist = ["HOME", "PATH"]
from_env = [
    "home=HOME",
    "user_path=PATH"
]

[[groups]]
name = "file_operations"

[[groups.commands]]
name = "list_home"
cmd = "/bin/ls"
args = ["-la", "%{home}"]
# %{home} は /home/username などに展開される
```

### 7.3.3 内部変数のネスト

内部変数の値には、他の内部変数への参照を含めることができます。

#### 基本例

```toml
[global]
vars = [
    "base=/opt",
    "app_dir=%{base}/myapp",
    "log_dir=%{app_dir}/logs"
]

[[groups.commands]]
name = "show_log_dir"
cmd = "/bin/echo"
args = ["Log directory: %{log_dir}"]
# 実際: Log directory: /opt/myapp/logs
```

#### 展開順序

変数は定義順に展開されます:

1. `base` → `/opt`
2. `app_dir` → `%{base}/myapp` → `/opt/myapp`
3. `log_dir` → `%{app_dir}/logs` → `/opt/myapp/logs`

### 7.3.4 循環参照の検出

循環参照はエラーとして検出されます:

```toml
[[groups.commands]]
name = "circular"
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # エラー: 循環参照
]
cmd = "/bin/echo"
args = ["%{var1}"]
```

## 7.4 プロセス環境変数の定義

### 7.4.1 `env` フィールドによる環境変数設定

#### 概要

`env` フィールドで定義された環境変数は、コマンド実行時に子プロセスに渡されます。この値には内部変数(`%{VAR}`)を使用できます。

#### 設定形式

```toml
[global]
env = [
    "LOG_LEVEL=info",
    "APP_ENV=production"
]

[[groups]]
name = "app_tasks"
env = [
    "DB_HOST=localhost",
    "DB_PORT=5432"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
env = [
    "CONFIG_FILE=%{config_path}"  # 内部変数を使用可能
]
vars = ["config_path=/etc/myapp/config.yml"]
```

#### 継承とマージ

`env` フィールドは以下のようにマージされます:

1. Global.env
2. Group.env (Global と結合)
3. Command.env (Global + Group と結合)

同じ名前の環境変数が複数レベルで定義された場合、より具体的なレベル(Command > Group > Global)が優先されます。

#### 内部変数との関係

- `env` の値には `%{VAR}` 形式で内部変数を参照できます
- `env` で定義された環境変数は、デフォルトでは子プロセスにのみ渡され、内部変数としては使用できません
- 内部変数として使いたい場合は、`vars` フィールドで定義してください

#### 例: 内部変数を使ったプロセス環境変数の設定

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "log_dir=%{app_dir}/logs"
]
env = [
    "APP_HOME=%{app_dir}",
    "LOG_PATH=%{log_dir}/app.log"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--verbose"]
# 子プロセスは APP_HOME=/opt/myapp, LOG_PATH=/opt/myapp/logs/app.log を受け取る
```

## 7.5 使用可能な場所の詳細

### 7.5.1 cmd での変数展開

コマンドパスに内部変数を使用できます。

#### 例1: 基本的なコマンドパス展開

```toml
[[groups.commands]]
name = "docker_version"
cmd = "%{docker_cmd}"
args = ["version"]
vars = ["docker_cmd=/usr/bin/docker"]
```

実行時:
- `%{docker_cmd}` → `/usr/bin/docker` に展開
- 実際の実行: `/usr/bin/docker version`

#### 例2: バージョン管理されたツール

```toml
[[groups.commands]]
name = "gcc_compile"
cmd = "%{toolchain_dir}/gcc-%{version}/bin/gcc"
args = ["-o", "output", "main.c"]
vars = [
    "toolchain_dir=/opt/toolchains",
    "version=11.2.0"
]
```

実行時:
- `%{toolchain_dir}` → `/opt/toolchains` に展開
- `%{version}` → `11.2.0` に展開
- 実際の実行: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 7.5.2 args での変数展開

コマンド引数に内部変数を使用できます。

#### 例1: ファイルパスの構築

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["%{source_file}", "%{dest_file}"]
vars = [
    "source_file=/data/original.txt",
    "dest_file=/backups/backup.txt"
]
```

#### 例2: 複数の変数を1つの引数に含める

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["%{user}@%{host}:%{port}"]
vars = [
    "user=admin",
    "host=server01.example.com",
    "port=22"
]
```

実行時:
- `%{user}@%{host}:%{port}` → `admin@server01.example.com:22` に展開

#### 例3: 設定ファイルの切り替え

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/%{env_type}.yml"]
vars = [
    "config_dir=/etc/myapp/configs",
    "env_type=production"
]
```

実行時:
- `%{config_dir}/%{env_type}.yml` → `/etc/myapp/configs/production.yml` に展開

### 7.5.3 複数変数の組み合わせ

複数の変数を組み合わせて、複雑なパスや文字列を構築できます。

#### 例1: タイムスタンプ付きバックアップパス

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "%{backup_root}/%{date}/%{user}/data"]
vars = [
    "backup_root=/var/backups",
    "date=2025-10-02",
    "user=admin"
]
```

実行時:
- `%{backup_root}/%{date}/%{user}/data` → `/var/backups/2025-10-02/admin/data` に展開

#### 例2: データベース接続文字列

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"]
vars = [
    "db_user=appuser",
    "db_pass=secret123",
    "db_host=localhost",
    "db_port=5432",
    "db_name=myapp_db"
]
```

実行時:
- 接続文字列が完全に展開される
- `postgresql://appuser:secret123@localhost:5432/myapp_db`
## 7.6 実践例

### 7.6.1 コマンドパスの動的構築

環境に応じてコマンドパスを切り替える例:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH"]
vars = [
    "python_root=/usr/local"
]

[[groups]]
name = "python_tasks"

# Python 3.10 を使用
[[groups.commands]]
name = "run_with_py310"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]
vars = ["py_version=3.10"]

# Python 3.11 を使用
[[groups.commands]]
name = "run_with_py311"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]
vars = ["py_version=3.11"]
```

### 7.6.2 引数の動的生成

Docker コンテナの起動パラメータを動的に構築:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH"]
vars = ["docker_bin=/usr/bin/docker"]

[[groups]]
name = "docker_deployment"

[[groups.commands]]
name = "start_container"
cmd = "%{docker_bin}"
args = [
    "run",
    "-d",
    "--name", "%{container_name}",
    "-v", "%{host_path}:%{container_path}",
    "-p", "%{host_port}:%{container_port}",
    "%{image_name}:%{image_tag}"
]
vars = [
    "container_name=myapp-prod",
    "host_path=/opt/myapp/data",
    "container_path=/app/data",
    "host_port=8080",
    "container_port=80",
    "image_name=myapp",
    "image_tag=v1.2.3"
]
env = ["APP_ENV=production"]
```

実行されるコマンド:
```bash
/usr/bin/docker run -d \
  --name myapp-prod \
  -v /opt/myapp/data:/app/data \
  -p 8080:80 \
  myapp:v1.2.3
```

### 7.6.3 環境別設定の切り替え

開発環境と本番環境で異なる設定を使用:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH"]
vars = [
    "app_bin=/opt/myapp/bin/myapp",
    "config_dir=/etc/myapp/configs"
]

# 開発環境グループ
[[groups]]
name = "development"
vars = [
    "env_type=development",
    "log_level=debug",
    "db_url=postgresql://localhost/dev_db"
]

[[groups.commands]]
name = "run_dev"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}"
]

# 本番環境グループ
[[groups]]
name = "production"
vars = [
    "env_type=production",
    "log_level=info",
    "db_url=postgresql://prod-server/prod_db"
]

[[groups.commands]]
name = "run_prod"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}"
]
```

### 7.6.4 システム環境変数の活用

`from_env` を使用してシステム環境変数を安全に取り込む例:

```toml
version = "1.0"

[global]
env_allowlist = ["HOME", "USER", "PATH"]
from_env = [
    "home=HOME",
    "username=USER"
]
vars = [
    "config_file=%{home}/.myapp/config.yml",
    "log_file=/var/log/myapp/%{username}.log"
]

[[groups]]
name = "user_tasks"

[[groups.commands]]
name = "show_config"
cmd = "/bin/cat"
args = ["%{config_file}"]

[[groups.commands]]
name = "show_logs"
cmd = "/bin/tail"
args = ["-f", "%{log_file}"]
```

## 7.7 エスケープシーケンス

リテラル(文字通りの)`%` や `\` を使用したい場合、エスケープが必要です。

### パーセント記号のエスケープ

`\%` でリテラルのパーセント記号を表現:

```toml
[[groups.commands]]
name = "percentage_display"
cmd = "/bin/echo"
args = ["Progress: 50\\%"]
```

出力: `Progress: 50%`

```toml
[[groups.commands]]
name = "price_display"
cmd = "/bin/echo"
args = ["Price: \\$100 USD"]
```

出力: `Price: $100 USD`

### バックスラッシュのエスケープ

`\\` でリテラルのバックスラッシュを表現:

```toml
[[groups.commands]]
name = "windows_path"
cmd = "/bin/echo"
args = ["Path: C:\\\\Users\\\\%{user}"]
vars = ["user=JohnDoe"]
```

出力: `Path: C:\Users\JohnDoe`

### 混在した例

```toml
[[groups.commands]]
name = "mixed_escape"
cmd = "/bin/echo"
args = ["Literal \\% is different from %{percent}"]
vars = ["percent=100"]
```

出力: `Literal % is different from 100`

## 7.8 自動変数

### 7.8.1 概要

システムは各コマンド実行時に以下の内部変数を自動的に設定します:

- **`__runner_datetime`**: 実行時刻（UTC）をYYYYMMDDHHmmSS.msec形式で表現
- **`__runner_pid`**: runnerプロセスのプロセスID

これらの変数は、**内部変数として利用可能**であり、`%{__runner_datetime}` や `%{__runner_pid}` の形式で参照できます。

### 7.8.2 使用例

#### タイムスタンプ付きバックアップ

```toml
[[groups.commands]]
name = "backup_with_timestamp"
description = "タイムスタンプ付きバックアップの作成"
cmd = "/usr/bin/tar"
args = [
    "czf",
    "/tmp/backup/data-%{__runner_datetime}.tar.gz",
    "/data"
]
```

実行例:
- 実行時刻が 2025-10-05 14:30:22.123 UTC の場合
- バックアップファイル名: `/tmp/backup/data-20251005143022.123.tar.gz`

#### PIDを使用したロックファイル

```toml
[[groups.commands]]
name = "create_lock_file"
description = "PIDを含むロックファイルの作成"
cmd = "/bin/sh"
args = [
    "-c",
    "echo %{__runner_pid} > /var/run/myapp-%{__runner_pid}.lock"
]
```

実行例:
- PIDが 12345 の場合
- ロックファイル: `/var/run/myapp-12345.lock`（内容: 12345）

#### 実行ログの記録

```toml
[[groups.commands]]
name = "log_execution"
description = "実行時刻とPIDをログに記録"
cmd = "/bin/sh"
args = [
    "-c",
    "echo 'Executed at %{__runner_datetime} by PID %{__runner_pid}' >> /var/log/executions.log"
]
```

出力例:
```
Executed at 20251005143022.123 by PID 12345
```

#### 複数の自動変数の組み合わせ

```toml
[[groups.commands]]
name = "timestamped_report"
description = "タイムスタンプとPID付きレポート"
cmd = "/opt/myapp/bin/report"
args = [
    "--output", "/reports/%{__runner_datetime}-%{__runner_pid}.html",
    "--title", "Report %{__runner_datetime}"
]
```

実行例:
- 出力ファイル: `/reports/20251005143022.123-12345.html`
- レポートタイトル: `Report 20251005143022.123`

### 7.8.3 日時フォーマット

`__runner_datetime` のフォーマット仕様:

| 部分 | 説明 | 例 |
|-----|------|-----|
| YYYY | 西暦4桁 | 2025 |
| MM | 月2桁（01-12） | 10 |
| DD | 日2桁（01-31） | 05 |
| HH | 時2桁（00-23、UTC） | 14 |
| mm | 分2桁（00-59） | 30 |
| SS | 秒2桁（00-59） | 45 |
| .msec | ミリ秒3桁（000-999） | .123 |

完全な例: `20251005143045.123` = 2025年10月5日 14時30分45秒.123（UTC）

**注意**: タイムゾーンは常にUTCです。ローカルタイムゾーンではありません。

### 7.8.4 予約プレフィックス

プレフィックス `__runner_` は自動変数用に予約されており、ユーザー定義の変数では使用できません。

#### エラーになる例

```toml
[[groups.commands]]
name = "invalid_var"
cmd = "/bin/echo"
args = ["%{__runner_custom}"]
vars = ["__runner_custom=value"]  # エラー: 予約プレフィックスの使用
```

エラーメッセージ:
```
variable "__runner_custom" uses reserved prefix "__runner_";
this prefix is reserved for automatically generated variables
```

#### 正しい例

```toml
[[groups.commands]]
name = "valid_var"
cmd = "/bin/echo"
args = ["%{my_custom_var}"]
vars = ["my_custom_var=value"]  # OK: 予約プレフィックスを使用していない
```

### 7.8.5 変数生成のタイミング

自動変数（`__runner_datetime` と `__runner_pid`）は、設定ファイルのロード時に一度だけ生成され、各コマンドの実行時には生成されません。すべてのグループのすべてのコマンドは、runner実行全体を通じて完全に同じ値を共有します。

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "/tmp/backup/db-%{__runner_datetime}.sql", "mydb"]

[[groups.commands]]
name = "backup_files"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/files-%{__runner_datetime}.tar.gz", "/data"]
```

**重要なポイント**: 両コマンドは完全に同じタイムスタンプを使用します。これは `__runner_datetime` が実行時ではなく、設定ロード時にサンプリングされるためです:
- `/tmp/backup/db-20251005143022.123.sql`
- `/tmp/backup/files-20251005143022.123.tar.gz`

これにより、コマンドが異なる時刻に実行される場合や、異なるグループに属している場合でも、単一のrunner実行内のすべてのコマンド間で一貫性が保証されます。

## 7.9 セキュリティ考慮事項

### 7.9.1 内部変数とプロセス環境変数の分離

内部変数(vars, from_env)とプロセス環境変数(env)は明確に分離されています:

```toml
[global]
vars = [
    "app_dir=/opt/myapp",
    "config_path=%{app_dir}/config.yml"
]
env = [
    "APP_HOME=%{app_dir}"  # 子プロセスに渡される
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_path}"]  # 内部変数を使用
# 子プロセスは APP_HOME 環境変数を受け取るが、app_dir や config_path は受け取らない
```

### 7.9.2 from_env のセキュリティ制約

`from_env` で取り込めるシステム環境変数は、`env_allowlist` で明示的に許可されたもののみです:

```toml
[global]
env_allowlist = ["HOME", "USER"]
from_env = [
    "home=HOME",      # OK: HOME は allowlist に含まれている
    "user=USER",      # OK: USER は allowlist に含まれている
    "path=PATH"       # エラー: PATH は allowlist に含まれていない
]
```

### 7.9.3 絶対パスの要件

展開後のコマンドパスは絶対パスである必要があります:

```toml
# 正しい: 絶対パスに展開される
[[groups.commands]]
name = "valid"
cmd = "%{tool_dir}/mytool"
vars = ["tool_dir=/opt/tools"]  # 絶対パス

# 誤り: 相対パスに展開される
[[groups.commands]]
name = "invalid"
cmd = "%{tool_dir}/mytool"
vars = ["tool_dir=./tools"]  # 相対パス - エラー
```

### 7.9.4 機密情報の扱い

機密情報は内部変数として定義し、必要な場合のみプロセス環境変数として渡します:

```toml
[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
args = [
    "-H", "Authorization: Bearer %{api_token}",
    "%{api_endpoint}/data"
]
vars = [
    "api_token=sk-1234567890abcdef",
    "api_endpoint=https://api.example.com"
]
# api_token と api_endpoint は内部変数のみで、子プロセスには渡されない
```

### 7.9.5 変数名の検証

変数名は POSIX 準拠の命名規則に従う必要があり、予約プレフィックス `__runner_` は使用できません:

```toml
# 有効な変数名
vars = [
    "app_dir=/opt/app",
    "log_level=info",
    "_private=value"
]

# 無効な変数名
vars = [
    "__runner_custom=value",  # エラー: 予約プレフィックス
    "123invalid=value",        # エラー: 数字で開始
    "my-var=value"             # エラー: ハイフン使用不可
]
```

## 7.10 トラブルシューティング

### 未定義変数

内部変数が定義されていない場合、エラーになります:

```toml
[[groups.commands]]
name = "undefined_var"
cmd = "/bin/echo"
args = ["Value: %{UNDEFINED}"]
# UNDEFINED が vars に定義されていない → エラー
```

**解決方法**: 必要な変数を `vars` または `from_env` で定義する

### 循環参照

内部変数が互いに参照し合う場合、エラーになります:

```toml
[[groups.commands]]
name = "circular"
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # 循環参照 → エラー
]
cmd = "/bin/echo"
args = ["%{var1}"]
```

**解決方法**: 変数の依存関係を整理する

### allowlist エラー

`from_env` で参照するシステム環境変数が `env_allowlist` にない場合、エラーになります:

```toml
[global]
env_allowlist = ["HOME"]
from_env = ["path=PATH"]  # エラー: PATH が allowlist にない
```

**解決方法**: `env_allowlist` に必要な環境変数を追加する

```toml
[global]
env_allowlist = ["HOME", "PATH"]
from_env = ["path=PATH"]  # OK
```

### 展開後のパス検証エラー

展開後のパスが不正な場合、エラーになります:

```toml
[[groups.commands]]
name = "invalid_path"
cmd = "%{tool}"
vars = ["tool=../tool"]  # 相対パス → エラー
```

**解決方法**: 絶対パスを使用する

```toml
[[groups.commands]]
name = "valid_path"
cmd = "%{tool}"
vars = ["tool=/opt/tools/tool"]  # 絶対パス → OK
```

## 実践的な総合例

以下は、変数展開機能を活用した実践的な設定例です:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "application_deployment"
description = "アプリケーションのデプロイメント処理"

# ステップ1: 設定ファイルの配置
[[groups.commands]]
name = "deploy_config"
description = "環境別設定ファイルの配置"
cmd = "/bin/cp"
args = [
    "${CONFIG_SOURCE}/${ENV_TYPE}/app.yml",
    "${CONFIG_DEST}/app.yml",
]
env = [
    "CONFIG_SOURCE=/opt/configs/templates",
    "CONFIG_DEST=/etc/myapp",
    "ENV_TYPE=production",
]

# ステップ2: データベースマイグレーション
[[groups.commands]]
name = "db_migration"
description = "データベーススキーマのマイグレーション"
cmd = "${APP_BIN}/migrate"
args = [
    "--database", "${DB_URL}",
    "--migrations", "${MIGRATION_DIR}",
]
env = [
    "APP_BIN=/opt/myapp/bin",
    "DB_URL=postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}",
    "DB_USER=appuser",
    "DB_PASS=secret123",
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_NAME=myapp_prod",
    "MIGRATION_DIR=/opt/myapp/migrations",
]
timeout = 600

# ステップ3: アプリケーションの起動
[[groups.commands]]
name = "start_application"
description = "アプリケーションサーバーの起動"
cmd = "${APP_BIN}/server"
args = [
    "--config", "${CONFIG_DEST}/app.yml",
    "--port", "${APP_PORT}",
    "--workers", "${WORKER_COUNT}",
]
env = [
    "APP_BIN=/opt/myapp/bin",
    "CONFIG_DEST=/etc/myapp",
    "APP_PORT=8080",
    "WORKER_COUNT=4",
]

# ステップ4: ヘルスチェック
[[groups.commands]]
name = "health_check"
description = "アプリケーションのヘルスチェック"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "${HEALTH_URL}",
]
env = [
    "HEALTH_URL=http://localhost:8080/health",
]
timeout = 30
```

## 7.11 verify_files での変数展開

### 7.11.1 概要

`verify_files` フィールドでも環境変数展開を使用できます。これにより、ファイル検証パスを動的に構築し、環境に応じた柔軟な検証設定が可能になります。

### 7.11.2 対象フィールド

変数展開は以下の `verify_files` フィールドで使用できます:

- **グローバルレベル**: `[global]` セクションの `verify_files`
- **グループレベル**: `[[groups]]` セクションの `verify_files`

### 7.11.3 基本例

#### グローバルレベルでの展開

```toml
version = "1.0"

[global]
env_allowlist = ["HOME"]
verify_files = [
    "${HOME}/config.toml",
    "${HOME}/data.txt",
]

[[groups]]
name = "example"

[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["hello"]
```

展開結果（`HOME=/home/user` の場合）:
- `${HOME}/config.toml` → `/home/user/config.toml`
- `${HOME}/data.txt` → `/home/user/data.txt`

#### グループレベルでの展開

```toml
version = "1.0"

[global]
env_allowlist = ["APP_ROOT"]

[[groups]]
name = "app_group"
env_allowlist = ["APP_ROOT"]
verify_files = [
    "${APP_ROOT}/config/app.yml",
    "${APP_ROOT}/bin/server",
]

[[groups.commands]]
name = "start"
cmd = "/bin/echo"
## 7.11 実践的な総合例

以下は、変数展開機能を活用した実践的な設定例です:

```toml
version = "1.0"

[global]
timeout = 300
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]
from_env = [
    "home=HOME",
    "username=USER"
]
vars = [
    "app_root=/opt/myapp",
    "config_dir=%{app_root}/config",
    "bin_dir=%{app_root}/bin"
]

[[groups]]
name = "application_deployment"
description = "アプリケーションのデプロイメント処理"
vars = [
    "env_type=production",
    "log_dir=%{app_root}/logs"
]

# ステップ1: 設定ファイルの配置
[[groups.commands]]
name = "deploy_config"
description = "環境別設定ファイルの配置"
cmd = "/bin/cp"
args = [
    "%{config_dir}/templates/%{env_type}/app.yml",
    "%{config_dir}/app.yml"
]

# ステップ2: データベースマイグレーション
[[groups.commands]]
name = "db_migration"
description = "データベーススキーマのマイグレーション"
cmd = "%{bin_dir}/migrate"
args = [
    "--database", "%{db_url}",
    "--migrations", "%{migration_dir}"
]
vars = [
    "db_user=appuser",
    "db_pass=secret123",
    "db_host=localhost",
    "db_port=5432",
    "db_name=myapp_prod",
    "db_url=postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}",
    "migration_dir=%{app_root}/migrations"
]
timeout = 600

# ステップ3: アプリケーションの起動
[[groups.commands]]
name = "start_application"
description = "アプリケーションサーバーの起動"
cmd = "%{bin_dir}/server"
args = [
    "--config", "%{config_dir}/app.yml",
    "--port", "%{app_port}",
    "--workers", "%{worker_count}"
]
vars = [
    "app_port=8080",
    "worker_count=4"
]
env = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{log_dir}/app.log"
]

# ステップ4: ヘルスチェック
[[groups.commands]]
name = "health_check"
description = "アプリケーションのヘルスチェック"
cmd = "/usr/bin/curl"
args = ["-f", "%{health_url}"]
vars = ["health_url=http://localhost:%{app_port}/health"]
timeout = 30
```

## 7.12 まとめ

### 変数システムの全体像

go-safe-cmd-runnerの変数システムは、以下の3つのコンポーネントで構成されています:

1. **内部変数** (`vars`, `from_env`)
   - TOML設定ファイル内での展開専用
   - `%{VAR}` 構文で参照
   - 子プロセスには渡されない(デフォルト)

2. **プロセス環境変数** (`env`)
   - 子プロセスに渡される環境変数
   - 内部変数 `%{VAR}` を値に使用可能

3. **自動変数** (`__runner_datetime`, `__runner_pid`)
   - システムが自動生成
   - 内部変数として利用可能

### ベストプラクティス

1. **内部変数を活用する**: パスやURLなど、TOML展開にのみ必要な値は `vars` で定義
2. **from_env で明示的に取り込む**: システム環境変数は `from_env` で明示的に取り込み、意図を明確に
3. **env は必要最小限に**: 子プロセスに渡す環境変数は必要最小限に抑える
4. **セキュリティを考慮**: 機密情報は慎重に扱い、不要な環境変数は渡さない
5. **命名規則を統一**: 内部変数は小文字とアンダースコア、環境変数は大文字を推奨

### 次のステップ

次章では、これらの変数展開機能を含む実践的な例を紹介します。
```bash
export DEPLOY_ENV=production
export APP_ROOT=/opt/myapp
export CONFIG_ROOT=/etc/myapp/config
```

この設定により、以下のファイルが検証されます:
- `/etc/myapp/config/production/global.yml`
- `/etc/myapp/config/production/secrets.enc`
- `/opt/myapp/web/nginx.conf`
- `/opt/myapp/web/ssl/cert.pem`
- `/opt/myapp/web/ssl/key.pem`
- `/opt/myapp/db/schema.sql`
- `/opt/myapp/db/migrations/production/`

### 7.11.11 制限事項

1. **絶対パスの要件**: 展開後のパスは絶対パスである必要があります
2. **システム環境変数のみ**: verify_files では Command.Env の変数は使用できません
3. **展開タイミング**: 設定ロード時に1度だけ展開されます（実行時ではありません）

## 次のステップ

次章では、これまで学んだ設定を組み合わせた実践的な例を紹介します。実際のユースケースに基づいた設定ファイルの作成方法を学びます。
