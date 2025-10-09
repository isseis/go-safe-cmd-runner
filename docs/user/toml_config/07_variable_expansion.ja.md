# 第7章: 変数展開機能

## 7.1 変数展開の概要

変数展開機能は、コマンドやその引数に変数を埋め込み、実行時に実際の値に置き換える機能です。これにより、動的なコマンド構築や、環境に応じた設定の切り替えが可能になります。

### 主な利点

1. **動的なコマンド構築**: 実行時に値を決定できる
2. **設定の再利用**: 同じ変数を複数の場所で使用
3. **環境の切り替え**: 開発/本番環境などの切り替えが容易
4. **保守性の向上**: 変更箇所を一箇所に集約

### 使用可能な場所

変数展開は以下の場所で使用できます:

- **cmd**: 実行するコマンドのパス
- **args**: コマンドの引数
- **env**: 環境変数の値(VALUE 部分)

## 7.2 変数展開の文法

### 基本文法

変数は `${変数名}` の形式で記述します:

```toml
cmd = "${VARIABLE_NAME}"
args = ["${ARG1}", "${ARG2}"]
env = ["VAR=${VALUE}"]
```

### 変数名のルール

- 英大文字、数字、アンダースコア(`_`)が使用可能
- 慣例として大文字を使用(例: `MY_VARIABLE`)
- 先頭は英字またはアンダースコアで開始

```toml
# 有効な変数名
"${PATH}"
"${MY_TOOL}"
"${_PRIVATE_VAR}"
"${VAR123}"

# 無効な変数名
"${123VAR}"      # 数字で開始
"${my-var}"      # ハイフンは使用不可
"${my.var}"      # ドットは使用不可
```

## 7.3 使用可能な場所

### 7.3.1 cmd での変数展開

コマンドパスを変数で指定できます。

#### 例1: 基本的なコマンドパス展開

```toml
[[groups.commands]]
name = "docker_version"
cmd = "${DOCKER_CMD}"
args = ["version"]
env = ["DOCKER_CMD=/usr/bin/docker"]
```

実行時:
- `${DOCKER_CMD}` → `/usr/bin/docker` に展開
- 実際の実行: `/usr/bin/docker version`

#### 例2: バージョン管理されたツール

```toml
[[groups.commands]]
name = "gcc_compile"
cmd = "${TOOLCHAIN_DIR}/gcc-${VERSION}/bin/gcc"
args = ["-o", "output", "main.c"]
env = [
    "TOOLCHAIN_DIR=/opt/toolchains",
    "VERSION=11.2.0",
]
```

実行時:
- `${TOOLCHAIN_DIR}` → `/opt/toolchains` に展開
- `${VERSION}` → `11.2.0` に展開
- 実際の実行: `/opt/toolchains/gcc-11.2.0/bin/gcc -o output main.c`

### 7.3.2 args での変数展開

コマンド引数に変数を使用できます。

#### 例1: ファイルパスの構築

```toml
[[groups.commands]]
name = "backup_copy"
cmd = "/bin/cp"
args = ["${SOURCE_FILE}", "${DEST_FILE}"]
env = [
    "SOURCE_FILE=/data/original.txt",
    "DEST_FILE=/backups/backup.txt",
]
```

#### 例2: 複数の変数を1つの引数に含める

```toml
[[groups.commands]]
name = "ssh_connect"
cmd = "/usr/bin/ssh"
args = ["${USER}@${HOST}:${PORT}"]
env = [
    "USER=admin",
    "HOST=server01.example.com",
    "PORT=22",
]
```

実行時:
- `${USER}@${HOST}:${PORT}` → `admin@server01.example.com:22` に展開

#### 例3: 設定ファイルの切り替え

```toml
[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env = [
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=production",
]
```

実行時:
- `${CONFIG_DIR}/${ENV_TYPE}.yml` → `/etc/myapp/configs/production.yml` に展開

### 7.3.3 複数変数の組み合わせ

複数の変数を組み合わせて、複雑なパスや文字列を構築できます。

#### 例1: タイムスタンプ付きバックアップパス

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/bin/mkdir"
args = ["-p", "${BACKUP_ROOT}/${DATE}/${USER}/data"]
env = [
    "BACKUP_ROOT=/var/backups",
    "DATE=2025-10-02",
    "USER=admin",
]
```

実行時:
- `${BACKUP_ROOT}/${DATE}/${USER}/data` → `/var/backups/2025-10-02/admin/data` に展開

#### 例2: データベース接続文字列

```toml
[[groups.commands]]
name = "db_connect"
cmd = "/usr/bin/psql"
args = ["postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}"]
env = [
    "DB_USER=appuser",
    "DB_PASS=secret123",
    "DB_HOST=localhost",
    "DB_PORT=5432",
    "DB_NAME=myapp_db",
]
```

実行時:
- 接続文字列が完全に展開される
- `postgresql://appuser:secret123@localhost:5432/myapp_db`

## 7.4 実践例

### 7.4.1 コマンドパスの動的構築

環境に応じてコマンドパスを切り替える例:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "PYTHON_ROOT", "PY_VERSION"]

[[groups]]
name = "python_tasks"

# Python 3.10 を使用
[[groups.commands]]
name = "run_with_py310"
cmd = "${PYTHON_ROOT}/python${PY_VERSION}/bin/python"
args = ["-V"]
env = [
    "PYTHON_ROOT=/usr/local",
    "PY_VERSION=3.10",
]

# Python 3.11 を使用
[[groups.commands]]
name = "run_with_py311"
cmd = "${PYTHON_ROOT}/python${PY_VERSION}/bin/python"
args = ["-V"]
env = [
    "PYTHON_ROOT=/usr/local",
    "PY_VERSION=3.11",
]
```

### 7.4.2 引数の動的生成

Docker コンテナの起動パラメータを動的に構築:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "DOCKER_BIN"]

[[groups]]
name = "docker_deployment"

[[groups.commands]]
name = "start_container"
cmd = "${DOCKER_BIN}"
args = [
    "run",
    "-d",
    "--name", "${CONTAINER_NAME}",
    "-v", "${HOST_PATH}:${CONTAINER_PATH}",
    "-e", "APP_ENV=${APP_ENV}",
    "-p", "${HOST_PORT}:${CONTAINER_PORT}",
    "${IMAGE_NAME}:${IMAGE_TAG}",
]
env = [
    "DOCKER_BIN=/usr/bin/docker",
    "CONTAINER_NAME=myapp-prod",
    "HOST_PATH=/opt/myapp/data",
    "CONTAINER_PATH=/app/data",
    "APP_ENV=production",
    "HOST_PORT=8080",
    "CONTAINER_PORT=80",
    "IMAGE_NAME=myapp",
    "IMAGE_TAG=v1.2.3",
]
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

### 7.4.3 環境別設定の切り替え

開発環境と本番環境で異なる設定を使用:

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "LOG_LEVEL", "DB_URL"]

# 開発環境グループ
[[groups]]
name = "development"

[[groups.commands]]
name = "run_dev"
cmd = "${APP_BIN}"
args = [
    "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml",
    "--log-level", "${LOG_LEVEL}",
    "--db", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/myapp",
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=development",
    "LOG_LEVEL=debug",
    "DB_URL=postgresql://localhost/dev_db",
]

# 本番環境グループ
[[groups]]
name = "production"

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = [
    "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml",
    "--log-level", "${LOG_LEVEL}",
    "--db", "${DB_URL}",
]
env = [
    "APP_BIN=/opt/myapp/bin/myapp",
    "CONFIG_DIR=/etc/myapp/configs",
    "ENV_TYPE=production",
    "LOG_LEVEL=info",
    "DB_URL=postgresql://prod-server/prod_db",
]
```

## 7.5 ネスト(入れ子)変数

変数の値に別の変数を含めることができます。

### 基本例

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: ${FULL_MSG}"]
env = [
    "FULL_MSG=Hello, ${USER}!",
    "USER=Alice",
]
```

展開順序:
1. `${USER}` → `Alice` に展開
2. `${FULL_MSG}` → `Hello, Alice!` に展開
3. 最終的な引数: `Message: Hello, Alice!`

### 複雑なパス構築

```toml
[[groups.commands]]
name = "complex_path"
cmd = "/bin/echo"
args = ["Config path: ${CONFIG_PATH}"]
env = [
    "CONFIG_PATH=${BASE_DIR}/${ENV_TYPE}/config.yml",
    "BASE_DIR=/opt/myapp",
    "ENV_TYPE=production",
]
```

展開順序:
1. `${BASE_DIR}` → `/opt/myapp` に展開
2. `${ENV_TYPE}` → `production` に展開
3. `${CONFIG_PATH}` → `/opt/myapp/production/config.yml` に展開

## 7.6 変数の自己参照

変数の自己参照は、環境変数を拡張する際によく使用される重要な機能です。特に `PATH` 環境変数のように、既存の値に新しい値を追加する場合に有用です。

### 自己参照の仕組み

`PATH=/custom/bin:${PATH}` のような記述では、`${PATH}` は **システム環境変数の元の値** を参照します。これは循環参照ではなく、意図的にサポートされた機能です。

### 基本例: PATH の拡張

```toml
[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["PATH is: ${PATH}"]
env = ["PATH=/opt/mytools/bin:${PATH}"]
```

展開過程:
1. システム環境変数 `PATH` の値を取得（例: `/usr/bin:/bin`）
2. `${PATH}` → `/usr/bin:/bin` に展開
3. 最終的な値: `/opt/mytools/bin:/usr/bin:/bin`

### 実用例: カスタムツールディレクトリの追加

```toml
[[groups.commands]]
name = "use_custom_tools"
cmd = "${CUSTOM_TOOL}"
args = ["--version"]
env = [
    "PATH=${TOOL_DIR}/bin:${PATH}",
    "TOOL_DIR=/opt/custom-tools",
    "CUSTOM_TOOL=mytool",
]
```

この設定では:
- `CUSTOM_TOOL` がコマンド名のみで指定されていても、拡張された `PATH` から見つけられる
- システムの既存 `PATH` も保持される

### 他の環境変数での自己参照

`PATH` 以外の環境変数でも同様の自己参照が可能です:

```toml
[[groups.commands]]
name = "extend_lib_path"
cmd = "/opt/myapp/bin/app"
args = []
env = [
    "LD_LIBRARY_PATH=/opt/myapp/lib:${LD_LIBRARY_PATH}",
    "PYTHONPATH=/opt/myapp/python:${PYTHONPATH}",
]
```

### 自己参照と循環参照の違い

**自己参照（正常）**: Command.Env で定義された変数が **システム環境変数** の同名変数を参照
```toml
env = ["PATH=/custom/bin:${PATH}"]  # ${PATH} はシステム環境変数を参照
```

**循環参照（エラー）**: Command.Env 内の変数同士が互いに参照し合う
```toml
env = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",  # エラー: Command.Env 内での循環参照
]
```

### 注意点

1. **システム環境変数が存在しない場合**: `${PATH}` 参照時にシステムに `PATH` が存在しない場合、エラーになります
2. **allowlist との関係**: システム環境変数を参照する場合、その変数が `env_allowlist` に含まれている必要があります

```toml
[global]
env_allowlist = ["PATH", "HOME"]  # PATH の自己参照を許可

[[groups.commands]]
name = "extend_path"
cmd = "/bin/echo"
args = ["${PATH}"]
env = ["PATH=/custom:${PATH}"]  # OK: PATH は allowlist に含まれている
```

## 7.7 エスケープシーケンス

リテラル(文字通りの)`$`や`\`を使用したい場合、エスケープが必要です。

### ドル記号のエスケープ

`\$` でリテラルのドル記号を表現:

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
args = ["Path: C:\\\\Users\\\\${USER}"]
env = ["USER=JohnDoe"]
```

出力: `Path: C:\Users\JohnDoe`

### 混在した例

```toml
[[groups.commands]]
name = "mixed_escape"
cmd = "/bin/echo"
args = ["Literal \\$HOME is different from ${HOME}"]
env = ["HOME=/home/user"]
```

出力: `Literal $HOME is different from /home/user`

## 7.8 自動環境変数

### 7.8.1 概要

システムは各コマンド実行時に以下の環境変数を自動的に設定します:

- **`__RUNNER_DATETIME`**: 実行時刻（UTC）をYYYYMMDDHHmmSS.msec形式で表現
- **`__RUNNER_PID`**: runnerプロセスのプロセスID

これらの変数は、コマンドパス、引数、環境変数の値で通常の変数と同様に使用できます。

### 7.8.2 使用例

#### タイムスタンプ付きバックアップ

```toml
[[groups.commands]]
name = "backup_with_timestamp"
description = "タイムスタンプ付きバックアップの作成"
cmd = "/usr/bin/tar"
args = [
    "czf",
    "/tmp/backup/data-${__RUNNER_DATETIME}.tar.gz",
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
    "echo ${__RUNNER_PID} > /var/run/myapp-${__RUNNER_PID}.lock"
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
    "echo 'Executed at ${__RUNNER_DATETIME} by PID ${__RUNNER_PID}' >> /var/log/executions.log"
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
    "--output", "/reports/${__RUNNER_DATETIME}-${__RUNNER_PID}.html",
    "--title", "Report ${__RUNNER_DATETIME}",
]
```

実行例:
- 出力ファイル: `/reports/20251005143022.123-12345.html`
- レポートタイトル: `Report 20251005143022.123`

### 7.8.3 日時フォーマット

`__RUNNER_DATETIME` のフォーマット仕様:

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

プレフィックス `__RUNNER_` は自動環境変数用に予約されており、ユーザー定義の環境変数では使用できません。

#### エラーになる例

```toml
[[groups.commands]]
name = "invalid_env"
cmd = "/bin/echo"
args = ["${__RUNNER_CUSTOM}"]
env = ["__RUNNER_CUSTOM=value"]  # エラー: 予約プレフィックスの使用
```

エラーメッセージ:
```
environment variable "__RUNNER_CUSTOM" uses reserved prefix "__RUNNER_";
this prefix is reserved for automatically generated variables
```

#### 正しい例

```toml
[[groups.commands]]
name = "valid_env"
cmd = "/bin/echo"
args = ["${MY_CUSTOM_VAR}"]
env = ["MY_CUSTOM_VAR=value"]  # OK: 予約プレフィックスを使用していない
```

### 7.8.5 変数生成のタイミング

自動環境変数（`__RUNNER_DATETIME`と`__RUNNER_PID`）は、設定ファイルのロード時に一度だけ生成され、各コマンドの実行時には生成されません。すべてのグループのすべてのコマンドは、runner実行全体を通じて完全に同じ値を共有します。

```toml
[[groups]]
name = "backup_group"

[[groups.commands]]
name = "backup_db"
cmd = "/usr/bin/pg_dump"
args = ["-f", "/tmp/backup/db-${__RUNNER_DATETIME}.sql", "mydb"]

[[groups.commands]]
name = "backup_files"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/files-${__RUNNER_DATETIME}.tar.gz", "/data"]
```

**重要なポイント**: 両コマンドは完全に同じタイムスタンプを使用します。これは`__RUNNER_DATETIME`が実行時ではなく、設定ロード時にサンプリングされるためです:
- `/tmp/backup/db-20251005143022.123.sql`
- `/tmp/backup/files-20251005143022.123.tar.gz`

これにより、コマンドが異なる時刻に実行される場合や、異なるグループに属している場合でも、単一のrunner実行内のすべてのコマンド間で一貫性が保証されます。

## 7.9 セキュリティ考慮事項

### 7.9.1 Command.Env の優先度

`Command.Env` で定義された変数は、システム環境変数よりも優先されます:

```toml
[global]
env_allowlist = ["PATH", "HOME"]

[[groups.commands]]
name = "override_home"
cmd = "/bin/echo"
args = ["Home: ${HOME}"]
env = ["HOME=/opt/custom-home"]
# システムの $HOME ではなく、Command.Env の HOME が使用される
```

### 7.9.2 env_allowlist との関係

**重要**: `Command.Env` で定義された変数は `env_allowlist` のチェックを受けません。

```toml
[global]
env_allowlist = ["PATH", "HOME"]
# CUSTOM_VAR は allowlist にない

[[groups.commands]]
name = "custom_var"
cmd = "${CUSTOM_TOOL}"
args = []
env = ["CUSTOM_TOOL=/opt/tools/mytool"]
# CUSTOM_TOOL は allowlist にないが、Command.Env で定義されているので使用可能
```

### 7.9.3 絶対パスの要件

展開後のコマンドパスは絶対パスである必要があります:

```toml
# 正しい: 絶対パスに展開される
[[groups.commands]]
name = "valid"
cmd = "${TOOL_DIR}/mytool"
env = ["TOOL_DIR=/opt/tools"]  # 絶対パス

# 誤り: 相対パスに展開される
[[groups.commands]]
name = "invalid"
cmd = "${TOOL_DIR}/mytool"
env = ["TOOL_DIR=./tools"]  # 相対パス - エラー
```

### 7.9.4 機密情報の扱い

機密情報(APIキー、パスワードなど)は `Command.Env` で定義し、システム環境変数から隔離:

```toml
[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
args = [
    "-H", "Authorization: Bearer ${API_TOKEN}",
    "${API_ENDPOINT}/data",
]
# 機密情報は Command.Env に記述し、システム環境から隔離
env = [
    "API_TOKEN=sk-1234567890abcdef",
    "API_ENDPOINT=https://api.example.com",
]
```

### 7.9.5 コマンド間の隔離

各コマンドの `env` は独立しており、他のコマンドに影響を与えません:

```toml
[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"
args = ["DB: ${DB_HOST}"]
env = ["DB_HOST=db1.example.com"]

[[groups.commands]]
name = "cmd2"
cmd = "/bin/echo"
args = ["DB: ${DB_HOST}"]
env = ["DB_HOST=db2.example.com"]
# cmd1 の DB_HOST とは独立
```

## 7.10 トラブルシューティング

### 未定義変数

変数が定義されていない場合、エラーになります:

```toml
[[groups.commands]]
name = "undefined_var"
cmd = "/bin/echo"
args = ["Value: ${UNDEFINED}"]
# UNDEFINED が env に定義されていない → エラー
```

**解決方法**: 必要な変数を全て `env` で定義する

### 循環参照

変数が互いに参照し合う場合、エラーになります:

```toml
[[groups.commands]]
name = "circular"
cmd = "/bin/echo"
args = ["${VAR1}"]
env = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",  # 循環参照 → エラー
]
```

**解決方法**: 変数の依存関係を整理する

**注意**: `PATH=/custom:${PATH}` のような自己参照は循環参照ではありません。詳細は「7.6 変数の自己参照」を参照してください。

### 展開後のパス検証エラー

展開後のパスが不正な場合、エラーになります:

```toml
[[groups.commands]]
name = "invalid_path"
cmd = "${TOOL}"
args = []
env = ["TOOL=../tool"]  # 相対パス → エラー
```

**解決方法**: 絶対パスを使用する

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

## 次のステップ

次章では、これまで学んだ設定を組み合わせた実践的な例を紹介します。実際のユースケースに基づいた設定ファイルの作成方法を学びます。
