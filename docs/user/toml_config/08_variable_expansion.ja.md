# 第8章: 変数展開機能

## 8.1 変数展開の概要

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
| **内部変数** | TOML設定ファイル内での展開専用 | `%{var}` | `vars`, `env_import` | なし(デフォルト) |
| **プロセス環境変数** | 子プロセスの環境変数として設定 | - | `env_vars` | あり |

### 使用可能な場所

変数展開は以下の場所で使用できます:

- **cmd**: 実行するコマンドのパス(`%{var}` を使用)
- **args**: コマンドの引数(`%{var}` を使用)
- **env_vars**: プロセス環境変数の値(`%{var}` を使用可能)
- **verify_files**: 検証対象ファイルパス(`%{var}` を使用)
- **vars**: 内部変数の定義(`%{var}` で他の内部変数を参照可能)

## 8.2 変数展開の文法

### 内部変数の参照構文

内部変数は `%{変数名}` の形式で記述します:

```toml
cmd = "%{variable_name}"
args = ["%{arg1}", "%{arg2}"]
env_vars = ["VAR=%{value}"]
```

### 変数名のルール

#### 基本的な文字ルール

- 英字(大文字・小文字)、数字、アンダースコア(`_`)が使用可能
- 先頭は英字またはアンダースコアで開始
- 大文字小文字を区別する(`home` と `HOME` は別の変数)
- 予約プレフィックス `__` (二重アンダースコア)で始まる変数名は使用不可(システム用に予約)

#### スコープ別の命名規則

変数のスコープによって、変数名の先頭文字に制約があります:

| スコープ | 定義場所 | 命名規則 | 例 |
|---------|---------|---------|---|
| **グローバル変数** | `[global.vars]` | **大文字(A-Z)で開始**する必要がある | `BackupDir`, `MaxRetries`, `Environment` |
| **ローカル変数** | `[groups.vars]`<br>`[groups.commands.vars]` | **小文字(a-z)またはアンダースコア(_)で開始**する必要がある | `backup_date`, `_temp_file`, `retry_count` |

この命名規則により、変数のスコープが名前から明確に判別でき、設定ミスを防止できます。

```
# グローバル変数の例(大文字で開始)
[global.vars]
BackupDir = "/data/backups"      # ✓ 有効
MaxRetries = "3"                  # ✓ 有効
Environment = "production"        # ✓ 有効

backup_dir = "/data/backups"      # ✗ エラー: グローバルスコープでは大文字で開始する必要がある

# ローカル変数の例(小文字またはアンダースコアで開始)
[groups.vars]
backup_date = "20250101"          # ✓ 有効
_temp_file = "/tmp/backup.tmp"    # ✓ 有効
retry_count = "5"                 # ✓ 有効

BackupDate = "20250101"           # ✗ エラー: ローカルスコープでは小文字または_で開始する必要がある

# 無効な変数名の例
[global.vars]
123var = "value"                  # ✗ 数字で開始
my-var = "value"                  # ✗ ハイフンは使用不可
my.var = "value"                  # ✗ ドットは使用不可
__custom_var = "value"            # ✗ 予約プレフィックス(__)
```

#### システム予約変数

`__runner_` プレフィックスで始まる変数は、システムが自動的に提供する変数です:
- `__runner_datetime`: 実行開始時刻(UTC)
- `__runner_pid`: runnerプロセスのプロセスID
- `__runner_workdir`: グループの作業ディレクトリ

これらの変数名をユーザーが定義することはできません。

## 8.3 内部変数の定義

### 8.3.1 `vars` フィールドによる内部変数定義

#### 概要

`vars` フィールドを使用して、TOML展開専用の内部変数を定義できます。これらの変数は子プロセスの環境変数には影響しません。

#### 設定形式

```toml
# グローバル変数(大文字で開始)
[global.vars]
AppDir = "/opt/myapp"

[[groups]]
name = "backup"

# ローカル変数(小文字で開始)
[groups.vars]
backup_dir = "%{AppDir}/backups"
retention_days = "30"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "%{output_file}", "mydb"]

# ローカル変数(小文字で開始)
[groups.commands.vars]
timestamp = "20250114"
output_file = "%{backup_dir}/dump_%{timestamp}.sql"
```

#### スコープと継承

| レベル | スコープ | 継承ルール |
|--------|---------|-----------|
| **Global.vars** | すべてのグループとコマンドから参照可能 | - |
| **Group.vars** | そのグループ内のコマンドから参照可能 | Global.vars とマージ(Group が優先) |
| **Command.vars** | そのコマンド内でのみ参照可能 | Global + Group + Command をマージ |

#### 参照構文

- `%{変数名}` の形式で参照
- `cmd`, `args`, `verify_files`, `env_vars` の値、および他の `vars` 定義内で使用可能

#### 基本的な例

```toml
version = "1.0"

# グローバル変数(大文字で開始)
[global.vars]
BaseDir = "/opt"

[[groups]]
name = "prod_backup"

# ローカル変数(小文字で開始)
[groups.vars]
db_tools = "%{BaseDir}/db-tools"

[[groups.commands]]
name = "db_dump"
cmd = "%{db_tools}/dump.sh"
args = ["-o", "%{output_file}"]

# ローカル変数(小文字で開始)
[groups.commands.vars]
timestamp = "20250114"
output_file = "%{BaseDir}/dump_%{timestamp}.sql"
```

### 8.3.2 `env_import` によるシステム環境変数の取り込み

#### 概要

`env_import` フィールドを使用して、システム環境変数を内部変数として取り込むことができます。

#### 設定形式

```toml
[global]
env_allowed = ["HOME", "PATH", "USER"]
env_import = [
    "home=HOME",
    "user_path=PATH",
    "username=USER"
]

[[groups]]
name = "example"
env_import = [
    "custom=CUSTOM_VAR"  # このグループ専用の取り込み
]
```

#### 構文

`内部変数名=システム環境変数名` の形式で記述します:

- **左辺**: 内部変数名
  - Global.env_import: 大文字で開始(例: `Home`, `UserPath`)
  - Group/Command.env_import: 小文字または_で開始(例: `home`, `user_path`)
- **右辺**: システム環境変数名(通常は大文字、例: `HOME`, `PATH`)

#### セキュリティ制約

- `env_import` で参照するシステム環境変数は必ず `env_allowed` に含まれている必要があります
- `env_allowed` にない変数を参照するとエラーになります

#### 継承ルール

| レベル | 継承動作 |
|--------|---------|
| **Global.env_import** | すべてのグループ・コマンドから継承される(デフォルト) |
| **Group.env_import** | 定義されている場合は Global.env_import と**マージ**(Merge) |
| **Command.env_import** | 定義されている場合は Global + Group の env_import と**マージ**(Merge) |
| **未定義** | 上位レベルの env_import を継承 |

#### 例: システム環境変数の取り込み

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "PATH"]
env_import = [
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

### 8.3.3 内部変数のネスト

内部変数の値には、他の内部変数への参照を含めることができます。

#### 基本例

```toml
# グローバル変数(大文字で開始)
[global.vars]
Base = "/opt"
AppDir = "%{Base}/myapp"
LogDir = "%{AppDir}/logs"

[[groups.commands]]
name = "show_log_dir"
cmd = "/bin/echo"
args = ["Log directory: %{LogDir}"]
# 実際: Log directory: /opt/myapp/logs
```

#### 展開順序

変数は定義順に展開されます:

1. `Base` → `/opt`
2. `AppDir` → `%{Base}/myapp` → `/opt/myapp`
3. `LogDir` → `%{AppDir}/logs` → `/opt/myapp/logs`

### 8.3.4 循環参照の検出

循環参照はエラーとして検出されます:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["%{var1}"]

[groups.commands.vars]
var1 = "%{var2}"
var2 = "%{var1}"  # エラー: 循環参照
```

## 8.4 プロセス環境変数の定義

### 8.4.1 `env_vars` フィールドによる環境変数設定

#### 概要

`env_vars` フィールドで定義された環境変数は、コマンド実行時に子プロセスに渡されます。この値には内部変数(`%{var}`)を使用できます。

#### 設定形式

```toml
[global]
env_vars = [
    "LOG_LEVEL=info",
    "APP_ENV=production"
]

[[groups]]
name = "app_tasks"
env_vars = [
    "DB_HOST=localhost",
    "DB_PORT=5432"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
env_vars = [
    "CONFIG_FILE=%{config_path}"  # 内部変数を使用可能
]
vars = ["config_path=/etc/myapp/config.yml"]
```

#### 継承とマージ

`env_vars` フィールドは以下のようにマージされます:

1. Global.env_vars
2. Group.env_vars (Global と結合)
3. Command.env_vars (Global + Group と結合)

同じ名前の環境変数が複数レベルで定義された場合、より具体的なレベル(Command > Group > Global)が優先されます。

#### 内部変数との関係

- `env_vars` の値には `%{var}` 形式で内部変数を参照できます
- `env_vars` で定義された環境変数は、デフォルトでは子プロセスにのみ渡され、内部変数としては使用できません
- 内部変数として使いたい場合は、`vars` フィールドで定義してください

#### 例: 内部変数を使ったプロセス環境変数の設定

```toml
version = "1.0"

# グローバル変数(大文字で開始)
[global.vars]
AppDir = "/opt/myapp"
LogDir = "%{AppDir}/logs"

[global]
env_vars = [
    "APP_HOME=%{AppDir}",
    "LOG_PATH=%{LogDir}/app.log"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--verbose"]
# 子プロセスは APP_HOME=/opt/myapp, LOG_PATH=/opt/myapp/logs/app.log を受け取る
```

## 8.5 使用可能な場所の詳細

### 8.5.1 cmd での変数展開

コマンドパスに内部変数を使用できます。

#### 例1: 基本的なコマンドパス展開

```toml
[[groups.commands]]
name = "docker_version"
cmd = "%{docker_cmd}"
args = ["version"]

[groups.commands.vars]
docker_cmd = "/usr/bin/docker"
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

[groups.commands.vars]
toolchain_dir = "/opt/toolchains"
version = "11.2.0"
```

実行時:
- `%{toolchain_dir}` → `/opt/toolchains` に展開
- `%{version}` → `11.2.0` に展開
- 実際の実行: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 8.5.2 args での変数展開

コマンド引数に内部変数を使用できます。

#### 例1: ファイルパスの構築

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["%{source_file}", "%{dest_file}"]

[groups.commands.vars]
source_file = "/data/original.txt"
dest_file = "/backups/backup.txt"
```

#### 例2: 複数の変数を1つの引数に含める

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["%{user}@%{host}:%{port}"]

[groups.commands.vars]
user = "admin"
host = "server01.example.com"
port = "22"
```

実行時:
- `%{user}@%{host}:%{port}` → `admin@server01.example.com:22` に展開

#### 例3: 設定ファイルの切り替え

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/%{env_type}.yml"]

[groups.commands.vars]
config_dir = "/etc/myapp/configs"
env_type = "production"
```

実行時:
- `%{config_dir}/%{env_type}.yml` → `/etc/myapp/configs/production.yml` に展開

### 8.5.3 複数変数の組み合わせ

複数の変数を組み合わせて、複雑なパスや文字列を構築できます。

#### 例1: タイムスタンプ付きバックアップパス

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "%{backup_root}/%{date}/%{user}/data"]

[groups.commands.vars]
backup_root = "/var/backups"
date = "2025-10-02"
user = "admin"
```

実行時:
- `%{backup_root}/%{date}/%{user}/data` → `/var/backups/2025-10-02/admin/data` に展開

#### 例2: データベース接続文字列

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"]

[groups.commands.vars]
db_user = "appuser"
db_pass = "secret123"
db_host = "localhost"
db_port = "5432"
db_name = "myapp_db"
```

実行時:
- 接続文字列が完全に展開される
- `postgresql://appuser:secret123@localhost:5432/myapp_db`
## 8.6 実践例

### 8.6.1 コマンドパスの動的構築

環境に応じてコマンドパスを切り替える例:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "PYTHON_ROOT", "PY_VERSION"]

[[groups]]
name = "python_tasks"

# Python 3.10 を使用
[[groups.commands]]
name = "run_with_py310"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]

[groups.commands.vars]
python_root = "/usr/local"
py_version = "3.10"

# Python 3.11 を使用
[[groups.commands]]
name = "run_with_py311"
cmd = "%{python_root}/python%{py_version}/bin/python"
args = ["-V"]

[groups.commands.vars]
python_root = "/usr/local"
py_version = "3.11"
```

### 8.6.2 引数の動的生成

Docker コンテナの起動パラメータを動的に構築:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "DOCKER_BIN"]

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
    "-e", "APP_ENV=%{app_env}",
    "-p", "%{host_port}:%{container_port}",
    "%{image_name}:%{image_tag}",
]

[groups.commands.vars]
docker_bin = "/usr/bin/docker"
container_name = "myapp-prod"
host_path = "/opt/myapp/data"
container_path = "/app/data"
app_env = "production"
host_port = "8080"
container_port = "80"
image_name = "myapp"
image_tag = "v1.2.3"
```

実行されるコマンド:
```bash
/usr/bin/docker run -d \
  --name myapp-prod \
  -v /opt/myapp/data:/app/data \
  -e APP_ENV=production \
  -p 8080:80 \
  myapp:v1.2.3
```

### 8.6.3 環境別設定の切り替え

開発環境と本番環境で異なる設定を使用:

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "LOG_LEVEL", "DB_URL"]

# 開発環境グループ
[[groups]]
name = "development"

[[groups.commands]]
name = "run_dev"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}",
]

[groups.commands.vars]
app_bin = "/opt/myapp/bin/myapp"
config_dir = "/etc/myapp/configs"
env_type = "development"
db_url = "postgresql://localhost/dev_db"

# 本番環境グループ
[[groups]]
name = "production"

[[groups.commands]]
name = "run_prod"
cmd = "%{app_bin}"
args = [
    "--config", "%{config_dir}/%{env_type}.yml",
    "--log-level", "%{log_level}",
    "--db", "%{db_url}",
]

[groups.commands.vars]
app_bin = "/opt/myapp/bin/myapp"
config_dir = "/etc/myapp/configs"
env_type = "production"
db_url = "postgresql://prod-server/prod_db"
```

### 8.6.4 システム環境変数の活用

`env_import` を使用してシステム環境変数を安全に取り込む例:

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER", "PATH"]
env_import = [
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

## 8.7 エスケープシーケンス

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

## 8.8 変数のネスト

変数の値には他の変数を含めることができます。

### 基本例

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: %{full_msg}"]

[groups.commands.vars]
full_msg = "Hello, %{user}!"
user = "Alice"
```

展開の順序:
1. `%{user}` → `Alice` に展開
2. `%{full_msg}` → `Hello, Alice!` に展開
3. 最終的な引数: `Message: Hello, Alice!`

### 複雑なパス構築

```toml
[[groups.commands]]
name = "complex_path"
cmd = "/bin/echo"
args = ["Config path: %{config_path}"]

[groups.commands.vars]
config_path = "%{base_dir}/%{env_type}/config.yml"
base_dir = "/opt/myapp"
env_type = "production"
```

展開の順序:
1. `%{base_dir}` → `/opt/myapp` に展開
2. `%{env_type}` → `production` に展開
3. `%{config_path}` → `/opt/myapp/production/config.yml` に展開

## 8.9 変数の自己参照

変数の自己参照は、環境変数を拡張する際によく使用される重要な機能です。特に `PATH` のような環境変数で、既存の値に新しい値を追加したい場合に便利です。

### 自己参照の仕組み

`PATH=/custom/bin:%{path}` のような式では、`%{path}` は **`env_import` でインポートされたシステム環境変数**または内部変数を参照します。これは循環参照ではなく、意図的にサポートされている機能です。

### 基本例: PATH の拡張

```toml
[global]
env_allowed = ["PATH"]
env_import = ["path=PATH"]

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["PATH is: %{path}"]
env_vars = ["PATH=/opt/mytools/bin:%{path}"]
```

展開プロセス:
1. システム環境変数 `PATH` を `%{path}` としてインポート（例: `/usr/bin:/bin`）
2. `%{path}` → `/usr/bin:/bin` に展開
3. 最終値: `/opt/mytools/bin:/usr/bin:/bin`

### 実践例: カスタムツールディレクトリの追加

```toml
[global]
env_allowed = ["PATH"]
env_import = ["path=PATH"]

[[groups.commands]]
name = "use_custom_tools"
cmd = "%{custom_tool}"
args = ["--version"]
env_vars = [
    "PATH=%{tool_dir}/bin:%{path}"
]

[groups.commands.vars]
tool_dir = "/opt/custom-tools"
custom_tool = "%{tool_dir}/bin/mytool"
```

この設定により:
- `%{custom_tool}` は拡張された `PATH` から見つけられます（フルパスでなくコマンド名だけで指定しても）
- 既存のシステム `PATH` は保持されます

### 他の環境変数での自己参照

`PATH` だけでなく、他の環境変数でも自己参照を使用できます:

```toml
[global]
env_allowed = ["LD_LIBRARY_PATH", "PYTHONPATH"]
env_import = [
    "ld_library_path=LD_LIBRARY_PATH",
    "pythonpath=PYTHONPATH"
]

[[groups.commands]]
name = "extend_lib_path"
cmd = "/opt/myapp/bin/app"
args = []
env_vars = [
    "LD_LIBRARY_PATH=/opt/myapp/lib:%{ld_library_path}",
    "PYTHONPATH=/opt/myapp/python:%{pythonpath}"
]
```

### 自己参照と循環参照の違い

**自己参照（正常）**: `env_import` でインポートしたシステム環境変数または内部変数を参照
```toml
env_vars = ["PATH=/custom/bin:%{path}"]  # %{path} はシステム環境変数を参照
```

**循環参照（エラー）**: vars 内の変数が互いに循環参照
```toml
[global.vars]
var1 = "%{var2}"
var2 = "%{var1}"  # エラー: 循環参照
```

### 重要な注意事項

1. **システム環境変数が存在しない場合**: `env_import` で参照するシステム環境変数が存在しない場合、エラーが発生します
2. **allowlist との関係**: `env_import` でシステム環境変数を参照する場合、その変数は `env_allowed` に含まれている必要があります

```toml
[global]
env_allowed = ["PATH", "HOME"]  # PATH と HOME のインポートを許可
env_import = ["system_path=PATH"]  # OK: PATH は allowlist に含まれている

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["%{path}"]
env_vars = ["PATH=%{path}"]

[groups.commands.vars]
path = "PATH_PREFIX:/custom:%{system_path}"
```

## 8.10 自動変数

### 8.10.1 概要

システムは以下の内部変数を自動的に設定します:

- **`__runner_datetime`**: runner実行開始時刻（UTC）をYYYYMMDDHHmmSS.msec形式で表現（グローバル変数）
- **`__runner_pid`**: runnerプロセスのプロセスID（グローバル変数）
- **`__runner_workdir`**: グループの作業ディレクトリ（グループ実行時に設定される変数、コマンドレベルでのみ利用可能）

これらの変数は、**内部変数として利用可能**であり、`%{__runner_datetime}`、`%{__runner_pid}`、`%{__runner_workdir}` の形式で参照できます。

### 8.10.2 使用例

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

#### 作業ディレクトリの参照

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "create_backup"
description = "作業ディレクトリにバックアップファイルを作成"
cmd = "/usr/bin/tar"
args = ["czf", "%{__runner_workdir}/backup.tar.gz", "/data"]
```

実行例:
- グループの作業ディレクトリが `/tmp/scr-backup_group-XXXXXX` の場合
- バックアップファイル: `/tmp/scr-backup_group-XXXXXX/backup.tar.gz`

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

### 8.10.3 日時フォーマット

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

### 8.10.4 予約プレフィックス

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

### 8.10.5 変数生成のタイミング

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

## 8.11 セキュリティ考慮事項

### 8.11.1 内部変数とプロセス環境変数の分離

内部変数(vars, env_import)とプロセス環境変数(env_vars)は明確に分離されています:

```toml
[global.vars]
app_dir = "/opt/myapp"
config_path = "%{app_dir}/config.yml"

[global]
env_vars = [
    "APP_HOME=%{app_dir}"  # 子プロセスに渡される
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_path}"]  # 内部変数を使用
# 子プロセスは APP_HOME 環境変数を受け取るが、app_dir や config_path は受け取らない
```

### 8.11.2 env_import のセキュリティ制約

`env_import` で取り込めるシステム環境変数は、`env_allowed` で明示的に許可されたもののみです:

```toml
[global]
env_allowed = ["HOME", "USER"]
env_import = [
    "home=HOME",      # OK: HOME は allowlist に含まれている
    "user=USER",      # OK: USER は allowlist に含まれている
    "path=PATH"       # エラー: PATH は allowlist に含まれていない
]
```

### 8.11.3 コマンドパスの要件

展開後のコマンドパスは以下の要件を満たす必要があります:

#### 一般コマンド

`run_as_user` または `run_as_group` が指定されていない通常のコマンドでは、ローカルパス（相対パス）または絶対パスが使用できます:

```toml
# 正しい: 絶対パスに展開される
[[groups.commands]]
name = "valid_absolute"
cmd = "%{tool_dir}/mytool"

[groups.commands.vars]
tool_dir = "/opt/tools"  # 絶対パス

# 正しい: 相対パスに展開される（一般コマンドでは許可）
[[groups.commands]]
name = "valid_relative"
cmd = "%{tool_dir}/mytool"

[groups.commands.vars]
tool_dir = "./tools"  # 相対パス - 一般コマンドではOK
```

#### 特権コマンド

`run_as_user` または `run_as_group` が指定されている特権コマンドでは、セキュリティ上の理由から**絶対パスのみ**が許可されます:

```toml
# 正しい: 絶対パスに展開される
[[groups.commands]]
name = "valid_privileged"
cmd = "%{tool_dir}/mytool"
run_as_user = "appuser"

[groups.commands.vars]
tool_dir = "/opt/tools"  # 絶対パス

# 誤り: 相対パスに展開される（特権コマンドではエラー）
[[groups.commands]]
name = "invalid_privileged"
cmd = "%{tool_dir}/mytool"
run_as_user = "appuser"

[groups.commands.vars]
tool_dir = "./tools"  # 相対パス - 特権コマンドではエラー
```

特権コマンドで絶対パスを要求する理由:
- PATH環境変数を使った攻撃を防止
- 実行するコマンドの正確な位置を明示
- 予期しないコマンド実行のリスクを低減

### 8.11.4 機密情報の扱い

機密情報は内部変数として定義し、必要な場合のみプロセス環境変数として渡します:

```toml
[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
args = [
    "-H", "Authorization: Bearer %{api_token}",
    "%{api_endpoint}/data"
]

[groups.commands.vars]
api_token = "sk-1234567890abcdef"
api_endpoint = "https://api.example.com"
# api_token と api_endpoint は内部変数のみで、子プロセスには渡されない
```

### 8.11.5 変数名の検証

変数名は POSIX 準拠の命名規則に従う必要があり、予約プレフィックス `__runner_` は使用できません:

```toml
# 有効な変数名
[global.vars] # Applies to all 'vars' tables (global, group, and command)
app_dir = "/opt/app"
_private = "value"

# 無効な変数名
[global.vars] # Applies to all 'vars' tables (global, group, and command)
__runner_custom = "value"  # エラー: 予約プレフィックス
123invalid = "value"        # エラー: 数字で開始
my-var = "value"            # エラー: ハイフン使用不可
```

## 8.12 トラブルシューティング

### 未定義変数

内部変数が定義されていない場合、エラーになります:

```toml
[[groups.commands]]
name = "undefined_var"
cmd = "/bin/echo"
args = ["Value: %{undefined}"]
# undefined が vars に定義されていない → エラー
```

**解決方法**: 必要な変数を `vars` または `env_import` で定義する

### 循環参照

内部変数が互いに参照し合う場合、エラーになります:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["%{var1}"]

[groups.commands.vars]
var1 = "%{var2}"
var2 = "%{var1}"  # 循環参照 → エラー
```

**解決方法**: 変数の依存関係を整理する

### allowlist エラー

`env_import` で参照するシステム環境変数が `env_allowed` にない場合、エラーになります:

```toml
[global]
env_allowed = ["HOME"]
env_import = ["path=PATH"]  # エラー: PATH が allowlist にない
```

**解決方法**: `env_allowed` に必要な環境変数を追加する

```toml
[global]
env_allowed = ["HOME", "PATH"]
env_import = ["path=PATH"]  # OK
```

### 展開後のパス検証エラー

展開後のパスが不正な場合、エラーになります:

```toml
[[groups.commands]]
name = "invalid_path"
cmd = "%{tool}"

[groups.commands.vars]
tool = "../tool"  # 相対パス → エラー
```

**解決方法**: 絶対パスを使用する

```toml
[[groups.commands]]
name = "valid_path"
cmd = "%{tool}"

[groups.commands.vars]
tool = "/opt/tools/tool"  # 絶対パス → OK
```

## 実践的な総合例

以下は、変数展開機能を活用した実践的な設定例です:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME", "USER"]
env_import = [
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
    "config_source=%{config_dir}/templates",
    "migration_dir=%{app_root}/migrations"
]

# ステップ1: 設定ファイルの配置
[[groups.commands]]
name = "deploy_config"
description = "環境別設定ファイルの配置"
cmd = "/bin/cp"
args = [
    "%{config_source}/%{env_type}/app.yml",
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
    "db_url=postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"
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
env_vars = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{app_root}/logs/app.log"
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

## 8.13 verify_files での変数展開

### 8.13.1 概要

`verify_files` フィールドでも環境変数展開を使用できます。これにより、ファイル検証パスを動的に構築し、環境に応じた柔軟な検証設定が可能になります。

### 8.13.2 対象フィールド

変数展開は以下の `verify_files` フィールドで使用できます:

- **グローバルレベル**: `[global]` セクションの `verify_files`
- **グループレベル**: `[[groups]]` セクションの `verify_files`

### 8.13.3 基本例

#### グローバルレベルでの展開

```toml
version = "1.0"

[global]
env_allowed = ["HOME"]
env_import = ["home=HOME"]
verify_files = [
    "%{home}/config.toml",
    "%{home}/data.txt"
]

[[groups]]
name = "example"

[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["hello"]
```

展開結果（`HOME=/home/user` の場合）:
- `%{home}/config.toml` → `/home/user/config.toml`
- `%{home}/data.txt` → `/home/user/data.txt`

#### グループレベルでの展開

```toml
version = "1.0"

[global]
env_allowed = ["APP_ROOT"]
env_import = ["app_root=APP_ROOT"]

[[groups]]
name = "app_group"
verify_files = [
    "%{app_root}/config/app.yml",
    "%{app_root}/bin/server"
]

[[groups.commands]]
name = "start"
cmd = "/bin/echo"
args = ["Starting app"]
```

展開結果（`APP_ROOT=/opt/myapp` の場合）:
- `%{app_root}/config/app.yml` → `/opt/myapp/config/app.yml`
- `%{app_root}/bin/server` → `/opt/myapp/bin/server`

### 8.13.4 複雑な例

動的なパス構築を含む例:

```toml
version = "1.0"

[global]
env_allowed = ["ENV", "APP_ROOT"]
env_import = [
    "env_type=ENV",
    "app_root=APP_ROOT"
]
vars = [
    "config_base=%{app_root}/configs",
    "config_path=%{config_base}/%{env_type}"
]
verify_files = [
    "%{config_path}/global.yml",
    "%{config_path}/secrets.enc",
    "%{app_root}/web/nginx.conf",
    "%{app_root}/web/ssl/cert.pem",
    "%{app_root}/web/ssl/key.pem",
    "%{app_root}/db/schema.sql",
    "%{app_root}/db/migrations/%{env_type}/"
]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "/opt/deploy.sh"
```

実行時の環境変数が以下の場合:
- `ENV=production`
- `APP_ROOT=/opt/myapp`

この設定により、以下のファイルが検証されます:
- `/opt/myapp/configs/production/global.yml`
- `/opt/myapp/configs/production/secrets.enc`
- `/opt/myapp/web/nginx.conf`
- `/opt/myapp/web/ssl/cert.pem`
- `/opt/myapp/web/ssl/key.pem`
- `/opt/myapp/db/schema.sql`
- `/opt/myapp/db/migrations/production/`

### 8.13.5 制限事項

1. **絶対パスの要件**: 展開後のパスは絶対パスである必要があります
2. **システム環境変数のみ**: verify_files では Command.Env の変数は使用できません
3. **展開タイミング**: 設定ロード時に1度だけ展開されます（実行時ではありません）

## 8.14 実践的な総合例

以下は、変数展開機能を活用した実践的な設定例です:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME", "USER"]
env_import = [
    "home=HOME",
    "username=USER"
]

[global.vars]
app_root = "/opt/myapp"
config_dir = "%{app_root}/config"
bin_dir = "%{app_root}/bin"

[[groups]]
name = "application_deployment"
description = "アプリケーションのデプロイメント処理"

[groups.vars]
env_type = "production"
log_dir = "%{app_root}/logs"

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
timeout = 600

[groups.commands.vars]
db_user = "appuser"
db_pass = "secret123"
db_host = "localhost"
db_port = "5432"
db_name = "myapp_prod"
db_url = "postgresql://%{db_user}:%{db_pass}@%{db_host}:%{db_port}/%{db_name}"
migration_dir = "%{app_root}/migrations"

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
env_vars = [
    "LOG_LEVEL=info",
    "LOG_PATH=%{log_dir}/app.log"
]

[groups.commands.vars]
app_port = "8080"
worker_count = "4"

# ステップ4: ヘルスチェック
[[groups.commands]]
name = "health_check"
description = "アプリケーションのヘルスチェック"
cmd = "/usr/bin/curl"
args = ["-f", "%{health_url}"]
timeout = 30

[groups.commands.vars]
health_url = "http://localhost:%{app_port}/health"
```

## 8.15 まとめ

### 変数システムの全体像

go-safe-cmd-runnerの変数システムは、以下の3つのコンポーネントで構成されています:

1. **内部変数** (`vars`, `env_import`)
   - TOML設定ファイル内での展開専用
   - `%{var}` 構文で参照
   - 子プロセスには渡されない(デフォルト)

2. **プロセス環境変数** (`env_vars`)
   - 子プロセスに渡される環境変数
   - 内部変数 `%{var}` を値に使用可能

3. **自動変数** (`__runner_datetime`, `__runner_pid`)
   - システムが自動生成
   - 内部変数として利用可能

### ベストプラクティス

1. **内部変数を活用する**: パスやURLなど、TOML展開にのみ必要な値は `vars` で定義
2. **env_import で明示的に取り込む**: システム環境変数は `env_import` で明示的に取り込み、意図を明確に
3. **env_vars は必要最小限に**: 子プロセスに渡す環境変数は必要最小限に抑える
4. **セキュリティを考慮**: 機密情報は慎重に扱い、不要な環境変数は渡さない
5. **命名規則を統一**: 内部変数は小文字とアンダースコア、環境変数は大文字を推奨

### 次のステップ

次章では、これまで学んだ設定を組み合わせた実践的な例を紹介します。実際のユースケースに基づいた設定ファイルの作成方法を学びます。
