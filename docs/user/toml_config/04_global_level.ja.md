# 第4章: グローバルレベル設定 [global]

## 概要

`[global]` セクションには、全てのグループとコマンドに適用される共通設定を定義します。このセクションはオプションですが、デフォルト値を一元管理するために使用することを推奨します。

## 4.1 timeout - タイムアウト設定

### 概要

コマンド実行の最大待機時間を秒単位で指定します。

### 文法

```toml
[global]
timeout = 秒数
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 整数 (int) またはnull |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、コマンド |
| **デフォルト値** | 60秒（未指定の場合） |
| **有効な値** | 0（無制限）、正の整数（秒単位） |
| **オーバーライド** | コマンドレベルでオーバーライド可能 |

### ⚠️ 重要な注意事項

`timeout = 0` は**無制限実行**（タイムアウトなし）を意味します。

### 役割

- **無限ループ防止**: コマンドがハングした場合に自動的に終了
- **リソース管理**: システムリソースの過度な占有を防止
- **予測可能な実行時間**: バッチ処理の完了時間を予測可能に

### 設定例

#### 例1: グローバルタイムアウトの設定

```toml
version = "1.0"

[global]
timeout = 60  # 全てのコマンドのデフォルトタイムアウトを60秒に設定

[[groups]]
name = "quick_tasks"

[[groups.commands]]
name = "fast_command"
cmd = "echo"
args = ["完了"]
# timeout 未指定 → グローバルの 60秒 を使用
```

#### 例2: コマンドレベルでのオーバーライド

```toml
version = "1.0"

[global]
timeout = 60  # デフォルト: 60秒

[[groups]]
name = "mixed_tasks"

[[groups.commands]]
name = "quick_check"
cmd = "ping"
args = ["-c", "3", "localhost"]
# timeout 未指定 → グローバルの 60秒 を使用

[[groups.commands]]
name = "long_backup"
cmd = "/usr/bin/backup.sh"
args = []
timeout = 300  # このコマンドのみ 300秒 に設定
```

#### 例3: 無制限実行

```toml
version = "1.0"

[global]
timeout = 60  # デフォルト: ほとんどのコマンドに60秒

[[groups]]
name = "data_processing"

[[groups.commands]]
name = "interactive_setup"
cmd = "/usr/bin/interactive-setup"
args = []
timeout = 0  # ✅ 無制限実行 - タイムアウトなし

[[groups.commands]]
name = "large_data_import"
cmd = "/usr/bin/import-data"
args = ["--large-dataset"]
timeout = 0  # ✅ 長時間実行タスクのための無制限実行
```

### タイムアウト解決階層

タイムアウト値は以下の優先順位で解決されます:
1. **コマンドレベル**: `[[groups.commands]]` のtimeout設定
2. **グローバルレベル**: `[global]` のtimeout設定
3. **システムデフォルト**: 60秒（グローバル、グループレベルでtimeoutが指定されていない場合）

#### 階層解決の例

```toml
version = "1.0"

[global]
timeout = 120  # グローバルデフォルト: 120秒

[[groups]]
name = "tasks"

[[groups.commands]]
name = "command1"
cmd = "/bin/sleep"
args = ["30"]
# グローバルタイムアウトを使用: 120秒

[[groups.commands]]
name = "command2"
cmd = "/bin/sleep"
args = ["30"]
timeout = 0  # コマンド固有: 無制限（グローバルをオーバーライド）

[[groups.commands]]
name = "command3"
cmd = "/bin/sleep"
args = ["30"]
timeout = 60  # コマンド固有: 60秒（グローバルをオーバーライド）
```

### 動作の詳細

#### 制限付きタイムアウト（正の整数）の場合
タイムアウトが発生すると:
1. 実行中のコマンドに終了シグナル(SIGTERM)を送信
2. 一定時間待機後、強制終了シグナル(SIGKILL)を送信
3. エラーとして記録し、次のコマンドに進む

#### 無制限タイムアウト（`timeout = 0`）の場合
無制限タイムアウトが設定されている場合:
1. **自動終了なし**: コマンドは完了まで実行される
2. **リソース監視**: システムは長時間実行プロセスを監視
3. **セキュリティログ**: セキュリティ監査のため無制限実行をログ出力
4. **手動中断**: ユーザー操作（Ctrl+C）により停止可能

### 注意事項

#### 1. タイムアウト値の選定

コマンドの実行時間を考慮して適切な値を設定してください:

```toml
[global]
timeout = 10  # 短すぎる: 通常のコマンドも失敗する可能性

[[groups.commands]]
name = "database_dump"
cmd = "/usr/bin/pg_dump"
args = ["large_database"]
# 10秒では完了しない可能性が高い → タイムアウトエラー
```

#### 2. `timeout = 0` による無制限実行

```toml
[global]
timeout = 0  # ✅ 無制限実行（タイムアウトなし）
```

**無制限タイムアウトの用途:**
- 長時間のデータ処理タスク
- ユーザー入力を必要とする対話型コマンド
- 実行時間が予測できないシステムメンテナンススクリプト

⚠️ **セキュリティ上の注意**: リソース枯渇を避けるため、無制限タイムアウトは慎重に使用してください。

#### 3. 無効な値

```toml
[global]
timeout = -1  # ❌ 無効: 負の値は許可されません
```

## 4.2 skip_standard_paths - 標準パス検証のスキップ

### 概要

標準的なシステムパス(`/bin`, `/usr/bin` など)に対するファイル検証をスキップします。

### 文法

```toml
[global]
verify_standard_paths = false/false
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 真偽値 (boolean) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバルのみ |
| **デフォルト値** | false |
| **有効な値** | true, false |

### 役割

- **パフォーマンス向上**: 標準コマンドの検証をスキップして起動時間を短縮
- **利便性**: 標準的なシステムコマンドのハッシュファイル作成を不要に

### 設定例

#### 例1: 標準パスの検証をスキップ

```toml
version = "1.0"

[global]
verify_standard_paths = false  # /bin, /usr/bin などの検証をスキップ

[[groups]]
name = "system_commands"

[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"  # 検証なしで実行可能
args = ["-la"]
```

#### 例2: 標準パスも検証(デフォルト)

```toml
version = "1.0"

[global]
verify_standard_paths = true  # または省略
verify_files = ["/etc/app/config.ini"]  # 追加の設定ファイルを検証

[[groups]]
name = "verified_commands"

[[groups.commands]]
name = "search"
cmd = "/usr/bin/grep"  # 標準パスだが検証される
args = ["pattern", "file.txt"]
# /usr/bin/grep と /etc/app/config.ini の両方が検証される
```

### セキュリティ上の注意

`verify_standard_paths = false` を設定すると、標準パスのコマンドが改ざんされていても検出できません。セキュリティ要件が高い環境では `false` (デフォルト)のままにすることを推奨します。

## 4.3 vars - グローバル内部変数

### 概要

TOML設定ファイル内での展開専用の内部変数を定義します。ここで定義した内部変数は、全てのグループとコマンドで参照可能です。内部変数はデフォルトでは子プロセスの環境変数にはなりません。

### 文法

```toml
[global]
vars = ["var1=value1", "var2=value2", ...]
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ、コマンド |
| **デフォルト値** | [] (変数なし) |
| **書式** | `"変数名=値"` 形式 |
| **参照構文** | `%{変数名}` |
| **スコープ** | グローバル vars はすべてのグループ・コマンドから参照可能 |

### 役割

- **TOML展開専用**: `cmd`, `args`, `env`, `verify_files` の値を展開
- **セキュリティ向上**: 子プロセスに渡す環境変数と分離
- **設定の再利用**: 共通の値を一元管理
- **パス構築**: ディレクトリパスなどを動的に構築

### 設定例

#### 例1: 基本的な内部変数の定義

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "config_file=%{app_dir}/config.yml"
]

[[groups]]
name = "app_group"

[[groups.commands]]
name = "show_config"
cmd = "/bin/cat"
args = ["%{config_file}"]
# 実際の実行: /bin/cat /opt/myapp/config.yml
```

#### 例2: ネストした変数参照

```toml
version = "1.0"

[global]
vars = [
    "base=/opt",
    "app_root=%{base}/myapp",
    "bin_dir=%{app_root}/bin",
    "data_dir=%{app_root}/data",
    "log_dir=%{app_root}/logs"
]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "start_app"
cmd = "%{bin_dir}/server"
args = ["--data", "%{data_dir}", "--log", "%{log_dir}"]
# 実際の実行: /opt/myapp/bin/server --data /opt/myapp/data --log /opt/myapp/logs
```

#### 例3: 内部変数とプロセス環境変数の組み合わせ

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/myapp",
    "config_path=%{app_dir}/config.yml"
]
env_vars = [
    "APP_HOME=%{app_dir}",           # 内部変数を使ってプロセス環境変数を定義
    "CONFIG_FILE=%{config_path}"     # 設定ファイルのパスを環境変数として渡す
]

[[groups.commands]]
name = "run_app"
cmd = "%{app_dir}/bin/app"
args = ["--config", "%{config_path}"]
# 子プロセスは CONFIG_FILE 環境変数を受け取り、コマンドライン引数として --config /opt/myapp/config.yml を受け取る
# app_dir, config_path 内部変数は環境変数としては受け取らない
```

### 変数名のルール

内部変数名は以下のルールに従う必要があります:

- **POSIX準拠**: `[a-zA-Z_][a-zA-Z0-9_]*` の形式
- **推奨**: 小文字とアンダースコアを使用(例: `app_dir`, `config_file`)
- **大文字も可**: 大文字も使用可能だが、小文字推奨
- **予約プレフィックス禁止**: `__runner_` で始まる名前は使用不可

```toml
[global]
vars = [
    "app_dir=/opt/app",        # 正しい: 小文字とアンダースコア
    "logLevel=info",           # 正しい: キャメルケース
    "APP_ROOT=/opt",           # 正しい: 大文字も可
    "_private=/tmp",           # 正しい: アンダースコアで開始
    "var123=value",            # 正しい: 数字を含む
    "__runner_var=value",      # エラー: 予約プレフィックス
    "123invalid=value",        # エラー: 数字で開始
    "my-var=value"             # エラー: ハイフン使用不可
]
```

### 注意事項

#### 1. 内部変数は子プロセスに渡されない

`vars` で定義した変数は、デフォルトでは子プロセスの環境変数になりません:

```toml
[global]
vars = ["secret_key=abc123"]

[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["secret_key"]
# 出力: 空文字列（secret_key は環境変数として渡されていない）
```

子プロセスに渡したい場合は、`env` フィールドで明示的に定義します:

```toml
[global]
vars = ["secret_key=abc123"]
env_vars = ["SECRET_KEY=%{secret_key}"]  # 内部変数を使ってプロセス環境変数を定義

[[groups.commands]]
name = "test"
cmd = "printenv"
args = ["SECRET_KEY"]
# 出力: abc123
```

#### 2. 循環参照の禁止

変数間で循環参照を作成するとエラーになります:

```toml
[global]
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # エラー: 循環参照
]
```

#### 3. 未定義変数の参照

定義されていない変数を参照するとエラーになります:

```toml
[global]
vars = ["app_dir=/opt/app"]

[[groups.commands]]
name = "test"
cmd = "%{undefined_var}/tool"  # エラー: undefined_var は定義されていない
```

### ベストプラクティス

1. **パスの一元管理**: アプリケーションのルートパスなどを vars で定義
2. **小文字推奨**: 内部変数名は小文字とアンダースコアを推奨
3. **階層構造**: ネストした変数参照で階層的なパスを構築
4. **セキュリティ**: 機密情報は vars で管理し、必要な場合のみ env で公開

## 4.4 from_env - システム環境変数の取り込み

### 概要

システム環境変数を内部変数として明示的に取り込みます。取り込んだ変数は内部変数として `%{変数名}` で参照できます。

### 文法

```toml
[global]
env_import = ["内部変数名=システム環境変数名", ...]
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | [] (取り込みなし) |
| **書式** | `"内部変数名=システム環境変数名"` 形式 |
| **セキュリティ制約** | `env_allowed` に含まれる変数のみ取り込み可能 |

### 役割

- **明示的な取り込み**: システム環境変数を意図的に取り込む
- **名前のマッピング**: システム環境変数を別名で参照可能
- **セキュリティ向上**: allowlist と組み合わせて制御

### 設定例

#### 例1: 基本的なシステム環境変数の取り込み

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER"]
from_env_vars = [
    "home=HOME",
    "username=USER"
]
vars = [
    "config_file=%{home}/.myapp/config.yml"
]

[[groups.commands]]
name = "show_config"
cmd = "/bin/cat"
args = ["%{config_file}"]
# HOME=/home/alice の場合: /bin/cat /home/alice/.myapp/config.yml
```

#### 例2: パスの拡張

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME"]
from_env_vars = [
    "user_path=PATH",
    "home=HOME"
]
vars = [
    "custom_bin=%{home}/bin",
    "extended_path=%{custom_bin}:%{user_path}"
]

[[groups.commands]]
name = "run_tool"
cmd = "/bin/sh"
args = ["-c", "echo Path: %{extended_path}"]
env_vars = ["PATH=%{extended_path}"]
```

#### 例3: 環境別の設定

```toml
version = "1.0"

[global]
env_allowed = ["APP_ENV"]
from_env_vars = ["environment=APP_ENV"]
vars = [
    "config_dir=/etc/myapp/%{environment}",
    "log_level=%{environment}"  # 環境に応じたログレベル
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "%{config_dir}/app.yml", "--log-level", "%{log_level}"]
# APP_ENV=production の場合: --config /etc/myapp/production/app.yml --log-level production
```

### セキュリティ制約

`from_env` で参照するシステム環境変数は、必ず `env_allowed` に含まれている必要があります:

```toml
[global]
env_allowed = ["HOME"]
from_env_vars = [
    "home=HOME",    # OK: HOME は allowlist に含まれている
    "path=PATH"     # エラー: PATH は allowlist に含まれていない
]
```

エラーメッセージ例:
```
system environment variable 'PATH' (mapped to 'path' in global.from_env) is not in env_allowed: [HOME]
```

### 変数名のマッピング

左辺(内部変数名)と右辺(システム環境変数名)で異なる名前を使用できます:

```toml
[global]
env_allowed = ["HOME", "USER", "HOSTNAME"]
from_env_vars = [
    "user_home=HOME",       # HOME を user_home として参照
    "current_user=USER",    # USER を current_user として参照
    "host=HOSTNAME"         # HOSTNAME を host として参照
]

[[groups.commands]]
name = "info"
cmd = "/bin/echo"
args = ["User: %{current_user}, Home: %{user_home}, Host: %{host}"]
```

### 注意事項

#### 1. 環境変数が存在しない場合

システム環境変数が存在しない場合、警告が表示され、空文字列が設定されます:

```toml
[global]
env_allowed = ["NONEXISTENT_VAR"]
from_env_vars = ["var=NONEXISTENT_VAR"]
# 警告: System environment variable 'NONEXISTENT_VAR' is not set
# var には空文字列が設定される
```

#### 2. 変数名の命名規則

内部変数名(左辺)は POSIX 準拠の命名規則に従う必要があります:

```toml
[global]
env_allowed = ["HOME"]
from_env_vars = [
    "home=HOME",            # 正しい
    "user_home=HOME",       # 正しい
    "HOME=HOME",            # 正しい(大文字も可)
    "__runner_home=HOME",   # エラー: 予約プレフィックス
    "123home=HOME",         # エラー: 数字で開始
    "my-home=HOME"          # エラー: ハイフン使用不可
]
```

### ベストプラクティス

1. **小文字推奨**: 内部変数名は小文字とアンダースコアを推奨(例: `home`, `user_path`)
2. **明示的な取り込み**: 必要な環境変数のみを明示的に取り込む
3. **allowlist と併用**: env_allowed で許可した変数のみ取り込む
4. **わかりやすい名前**: システム環境変数名と内部変数名を区別しやすい名前に

## 4.5 env - グローバルプロセス環境変数

### 概要

全てのグループとコマンドで共通して使用するプロセス環境変数を定義します。ここで定義した環境変数は、全てのコマンドの子プロセスに渡されます。内部変数 `%{VAR}` を値に使用できます。

### 文法

```toml
[global]
env_vars = ["KEY1=value1", "KEY2=value2", ...]
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ、コマンド |
| **デフォルト値** | [] (環境変数なし) |
| **書式** | `"KEY=VALUE"` 形式 |
| **値での変数展開** | 内部変数 `%{VAR}` を使用可能 |
| **オーバーライド** | グループ・コマンドレベルで同名変数を上書き可能 |

### 役割

- **子プロセスへの環境変数設定**: コマンド実行時に子プロセスに渡される
- **内部変数の活用**: `%{VAR}` 形式で内部変数を参照可能
- **設定の一元化**: 共通の環境変数を1箇所で管理
- **保守性の向上**: 変更時の修正箇所を削減

### 設定例

#### 例1: 基本的なプロセス環境変数

```toml
version = "1.0"

[global]
vars = [
    "app_dir=/opt/app",
    "log_level=info"
]
env_vars = [
    "APP_HOME=%{app_dir}",
    "LOG_LEVEL=%{log_level}",
    "CONFIG_FILE=%{app_dir}/config.yaml"
]

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/bin/app"
args = []
# 子プロセスは APP_HOME, LOG_LEVEL, CONFIG_FILE 環境変数を受け取る
```

#### 例2: 内部変数を使ったパス構築

```toml
version = "1.0"

[global]
vars = [
    "base=/opt",
    "app_root=%{base}/myapp",
    "data_dir=%{app_root}/data"
]
env_vars = [
    "APP_ROOT=%{app_root}",
    "DATA_PATH=%{data_dir}",
    "BIN_PATH=%{app_root}/bin"
]

[[groups.commands]]
name = "start_app"
cmd = "%{app_root}/bin/server"
args = []
# 子プロセスは APP_ROOT, DATA_PATH, BIN_PATH を受け取る
```

#### 例3: システム環境変数との組み合わせ

```toml
version = "1.0"

[global]
env_allowed = ["HOME", "USER"]
from_env_vars = [
    "home=HOME",
    "username=USER"
]
vars = [
    "log_dir=%{home}/logs"
]
env_vars = [
    "USER_NAME=%{username}",
    "LOG_DIRECTORY=%{log_dir}"
]

[[groups.commands]]
name = "log_info"
cmd = "/bin/sh"
args = ["-c", "echo USER_NAME=$USER_NAME, LOG_DIRECTORY=$LOG_DIRECTORY"]
# 子プロセスは USER_NAME と LOG_DIRECTORY 環境変数を受け取る
```

### 優先順位とマージ

環境変数は以下の順序でマージされます:

1. Global.env（グローバル環境変数）
2. Group.env（グループ環境変数、第5章参照）と結合
3. Command.env（コマンド環境変数、第6章参照）と結合

同じ名前の環境変数が複数レベルで定義された場合、より具体的なレベル(Command > Group > Global)が優先されます。

```toml
[global]
vars = ["base=global_value"]
env_vars = [
    "COMMON_VAR=%{base}",
    "GLOBAL_ONLY=from_global"
]

[[groups]]
name = "example"
vars = ["base=group_value"]
env_vars = ["COMMON_VAR=%{base}"]    # Global.env を上書き

[[groups.commands]]
name = "cmd1"
vars = ["base=command_value"]
env_vars = ["COMMON_VAR=%{base}"]    # Group.env を上書き

# 実行時の環境変数:
# COMMON_VAR=command_value (コマンドレベルが優先)
# GLOBAL_ONLY=from_global (グローバルのみ)
```

### 内部変数との関係

- **env の値**: 内部変数 `%{VAR}` を使用可能
- **子プロセスへの伝播**: env で定義された変数は子プロセスに渡される
- **内部変数は伝播しない**: varsやfrom_envで定義した内部変数はデフォルトでは子プロセスに渡されない

```toml
[global]
vars = ["internal_value=secret"]     # 内部変数のみ
env_vars = ["PUBLIC_VAR=%{internal_value}"]  # 内部変数を使って環境変数を定義

[[groups.commands]]
name = "test"
cmd = "/bin/sh"
args = ["-c", "echo internal_value=$internal_value, PUBLIC_VAR=$PUBLIC_VAR"]
# 出力: internal_value=, PUBLIC_VAR=secret
# (internal_value は環境変数として渡されない)
```

### 注意事項

#### 1. KEY名の制約

環境変数名（KEY部分）は以下のルールに従う必要があります：

```toml
[global]
env_vars = [
    "VALID_NAME=value",      # 正しい: 英大文字、数字、アンダースコア
    "MY_VAR_123=value",      # 正しい
    "123INVALID=value",      # エラー: 数字で始まる
    "MY-VAR=value",          # エラー: ハイフンは使用不可
    "__RUNNER_VAR=value",    # エラー: 予約プレフィックス
]
```

#### 2. 重複定義

同じKEYを複数回定義するとエラーになります：

```toml
[global]
env_vars = [
    "VAR=value1",
    "VAR=value2",  # エラー: 重複定義
]
```

#### 3. 内部変数の参照

env の値には内部変数を参照できますが、未定義の変数を参照するとエラーになります:

```toml
[global]
vars = ["defined=value"]
env_vars = [
    "VALID=%{defined}",      # OK: defined は定義されている
    "INVALID=%{undefined}"   # エラー: undefined は定義されていない
]
```

#### 4. 循環参照の禁止

env 内の変数間で循環参照を作成するとエラーになります:

```toml
[global]
vars = [
    "var1=%{var2}",
    "var2=%{var1}"  # エラー: 循環参照
]
```

### ベストプラクティス

1. **内部変数を活用**: パスなどは vars で定義し、env で必要なもののみ公開
2. **明確な命名**: プロセス環境変数名は大文字とアンダースコアで明確に
3. **最小限の公開**: 子プロセスに必要な環境変数のみを env で定義
4. **セキュリティ考慮**: 機密情報は慎重に扱い、必要最小限の環境変数のみ公開
3. **階層的な定義**: ベースパスをまず定義し、派生パスはそれを参照
4. **allowlist の適切な設定**: システム環境変数を参照する場合は必ず allowlist に追加

```toml
# 推奨される構成
[global]
env_vars = [
    # ベース設定
    "APP_ROOT=/opt/myapp",
    "ENV_TYPE=production",

    # 派生設定（ベースを参照）
    "BIN_DIR=${APP_ROOT}/bin",
    "DATA_DIR=${APP_ROOT}/data",
    "LOG_DIR=${APP_ROOT}/logs",
    "CONFIG_FILE=${APP_ROOT}/etc/${ENV_TYPE}.yaml",
]
env_allowed = ["HOME", "PATH"]
```

### 次のステップ

- **Group.env**: グループレベルの環境変数については第5章を参照
- **Command.env**: コマンドレベルの環境変数については第6章を参照
- **変数展開の詳細**: 変数展開の仕組みについては第7章を参照

## 4.6 env_allowed - 環境変数許可リスト

### 概要

`from_env` でシステム環境変数を取り込む際に許可する環境変数を指定します。リストにない環境変数は取り込めません。

### 文法

```toml
[global]
env_allowed = ["変数1", "変数2", ...]
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | [] (全ての環境変数を拒否) |
| **有効な値** | 環境変数名のリスト |
| **オーバーライド** | グループレベルで継承/オーバーライド可能(詳細は第5章) |

### 役割

- **セキュリティ**: 不要な環境変数の漏洩を防止
- **環境の統一**: コマンド実行環境を予測可能にする
- **最小権限の原則**: 必要な環境変数のみを許可

### 設定例

#### 例1: 基本的な環境変数の許可

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",    # コマンド検索パス
    "HOME",    # ホームディレクトリ
    "USER",    # ユーザー名
    "LANG",    # 言語設定
]

[[groups]]
name = "basic_commands"

[[groups.commands]]
name = "show_env"
cmd = "printenv"
args = []
# PATH, HOME, USER, LANG のみが利用可能
```

#### 例2: アプリケーション固有の環境変数

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",
    "HOME",
    "APP_CONFIG_DIR",   # アプリ設定ディレクトリ
    "APP_LOG_LEVEL",    # ログレベル
    "DATABASE_URL",     # データベース接続文字列
]

[[groups]]
name = "app_tasks"

[[groups.commands]]
name = "run_app"
cmd = "/opt/myapp/bin/app"
args = ["--config", "${APP_CONFIG_DIR}/config.yaml"]
env_vars = ["APP_CONFIG_DIR=/etc/myapp"]
```

#### 例3: 空のリスト(全て拒否)

```toml
version = "1.0"

[global]
env_allowed = []  # 全ての環境変数を拒否

[[groups]]
name = "isolated_tasks"

[[groups.commands]]
name = "pure_command"
cmd = "/bin/echo"
args = ["Hello"]
# 環境変数なしで実行される
```

### よく使用される環境変数

| 変数名 | 用途 | 推奨度 |
|--------|------|--------|
| PATH | コマンド検索パス | 高(ほぼ必須) |
| HOME | ホームディレクトリ | 高 |
| USER | ユーザー名 | 中 |
| LANG, LC_ALL | 言語・ロケール設定 | 中 |
| TZ | タイムゾーン | 低 |
| TERM | 端末タイプ | 低 |

### セキュリティのベストプラクティス

1. **最小限の許可**: 必要な変数のみを許可
2. **機密情報の除外**: パスワードやトークンを含む変数は許可しない
3. **定期的な見直し**: 不要になった変数を削除

```toml
# 推奨しない: 過度に寛容
[global]
env_allowed = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... 多すぎる
]

# 推奨: 必要最小限
[global]
env_allowed = ["PATH", "HOME", "USER"]
```

## 4.7 verify_files - ファイル検証リスト

### 概要

実行前に整合性を検証する追加ファイルのリストを指定します。指定されたファイルはハッシュ値と照合され、改ざんが検出されると実行が中止されます。内部変数 `%{VAR}` を使用してパスを動的に構築できます。

**重要**: コマンド (`cmd`) で指定した実行可能ファイルは自動的にハッシュ検証の対象となります。`verify_files` はコマンド以外の追加ファイル(設定ファイル、スクリプトファイルなど)を検証対象に追加するために使用します。

### 文法

```toml
[global]
verify_files = ["ファイルパス1", "ファイルパス2", ...]
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | [] (検証なし) |
| **有効な値** | 絶対パスのリスト |
| **マージ動作** | グループレベルの設定とマージされる |

### 役割

- **改ざん検出**: ファイルが変更されていないことを確認
- **セキュリティ**: 悪意のあるコードの実行を防止
- **整合性保証**: 意図したバージョンのファイルが使用されることを保証

### 設定例

#### 例1: 追加ファイルの検証

```toml
version = "1.0"

[global]
verify_files = [
    "/opt/app/config/app.conf",
    "/opt/app/scripts/deploy.sh",
]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "/opt/app/scripts/deploy.sh"  # このファイルは自動的に検証される
args = []
# 実行前に /opt/app/scripts/deploy.sh と /opt/app/config/app.conf が検証される
```

#### 例2: グループレベルでの追加

```toml
version = "1.0"

[global]
verify_files = ["/etc/app/global.conf"]  # 全グループで検証される設定ファイル

[[groups]]
name = "database_group"
verify_files = ["/etc/app/db.conf"]  # グループ固有の設定ファイル

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"  # このコマンドは自動的に検証される
args = ["mydb"]
# /usr/bin/pg_dump (自動), /etc/app/global.conf, /etc/app/db.conf が検証される(マージ)
```

### 検証の仕組み

1. **自動検証対象の収集**: 各コマンドの `cmd` フィールドで指定された実行可能ファイルを自動的に検証対象に追加
2. **追加ファイルの指定**: `verify_files` に記載されたファイルも検証対象に追加
3. **ハッシュファイルの事前作成**: `record` コマンドでファイルのハッシュを記録
4. **実行時の検証**: 収集されたすべてのファイルのハッシュを照合
5. **不一致時の動作**: ハッシュが一致しない場合、実行を中止しエラーを報告

### ハッシュファイルの作成方法

```bash
# 個別にファイルを指定
$ record /opt/app/config/app.conf /opt/app/scripts/deploy.sh
```

### 注意事項

#### 1. 絶対パスが必要

```toml
[global]
verify_files = ["./script.sh"]  # エラー: 相対パスは使用不可
verify_files = ["/opt/app/script.sh"]  # 正しい
```

#### 2. ハッシュファイルの管理

指定したファイルのハッシュが事前に記録されていない場合、検証エラーになります。

#### 3. セキュリティのベストプラクティス

- **コマンドは自動検証**: すべてのコマンド (`cmd`) は自動的に検証されるため、`verify_files` に追加する必要はありません
- **追加ファイルの検証**: コマンドが参照する設定ファイル、スクリプトファイル、ライブラリなどを `verify_files` に追加してください
- **パフォーマンス**: ファイルハッシュ検証は高速に動作するため、パフォーマンスへの影響は限定的です
- **改ざん検出**: 検証対象を増やすことでシステム侵害をより確実に防ぐことができます

## 4.8 グローバルコマンド許可リスト (ハードコード)

### 概要

**重要**: グローバルコマンド許可リストは、セキュリティ上の理由からプログラムに**ハードコード**されており、TOMLファイルで**設定できません**。

以下の正規表現パターンが、グローバルに実行を許可するコマンドを決定するために使用されます:

```
^/bin/.*
^/usr/bin/.*
^/usr/sbin/.*
^/usr/local/bin/.*
```

### セキュリティ根拠

グローバルコマンド許可リストは、セキュリティ設定ミスを防ぐためにハードコードされています。ユーザーが設定を通じてこれらの制限を緩和できるようにすると、以下のリスクが生じる可能性があります:

- 信頼できない場所からのコマンド実行を可能にする
- セキュリティコントロールをバイパスする
- 攻撃対象領域を拡大する

### 許可コマンドの拡張

ハードコードされたパターン外のコマンドについては、**グループレベルの `cmd_allowed`** を使用してください (第5.3.6章を参照):

```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_tools"
# このグループのみで追加のコマンドを許可
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor",
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"  # グループレベルの cmd_allowed で許可
args = ["--verbose"]

[[groups.commands]]
name = "run_sh"
cmd = "/bin/sh"  # ハードコードされたグローバルパターンで許可
args = ["-c", "echo hello"]
```

### コマンド検証ロジック

コマンドは以下の**いずれか**に一致する場合に許可されます:

1. **ハードコードされたグローバルパターン** (すべてのグループに適用)、**または**
2. **グループレベルの `cmd_allowed`** (特定のグループのみに適用)

グループレベルの `cmd_allowed` の詳細については、第5.3.6章を参照してください。

### 注意事項

#### 1. パターンマッチング

ハードコードされたパターンは、シンボリックリンク解決後のコマンドの完全な絶対パスに対してマッチングされます。

#### 2. セキュリティのベストプラクティス

- **アンカーされたパターンを使用**: パターンの先頭には常に `^` を使用してアンカーする
- **具体的に記述**: 広範なパターンよりも具体的なパターンを優先する
- **cmd_allowed と組み合わせる**: グローバルパターンを広げるのではなく、例外的なケースにはグループレベルの `cmd_allowed` を使用する

## 4.9 output_size_limit - 出力サイズ上限

### 概要

コマンドの標準出力をキャプチャする際の最大サイズをバイト単位で指定します。

### 文法

```toml
[global]
output_size_limit = バイト数
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 整数 (int64) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバルのみ |
| **デフォルト値** | 10485760 (10MB) |
| **有効な値** | 正の整数(バイト単位) |
| **オーバーライド** | 不可(グローバルレベルのみ) |

### 役割

- **リソース保護**: 過大な出力によるディスク使用量の増加を防止
- **メモリ管理**: メモリ不足を防ぐ
- **予測可能な動作**: 出力サイズの上限を明確化

### 設定例

#### 例1: 1MB の制限

```toml
version = "1.0"

[global]
output_size_limit = 1048576  # 1MB = 1024 * 1024 バイト

[[groups]]
name = "log_analysis"

[[groups.commands]]
name = "grep_logs"
cmd = "grep"
args = ["ERROR", "/var/log/app.log"]
output_file = "errors.txt"
# 出力が 1MB を超えるとエラー
```

#### 例2: 大きなファイルの処理

```toml
version = "1.0"

[global]
output_size_limit = 104857600  # 100MB = 100 * 1024 * 1024 バイト

[[groups]]
name = "data_export"

[[groups.commands]]
name = "export_database"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output_file = "database_dump.sql"
# 大きなデータベースダンプを許可
```

#### 例3: サイズ制限の目安

```toml
[global]
# 一般的な用途に応じた推奨値
output_size_limit = 1048576      # 1MB  - ログ分析、小規模データ
output_size_limit = 10485760     # 10MB - デフォルト、中規模データ
output_size_limit = 104857600    # 100MB - 大規模データ、データベースダンプ
output_size_limit = 1073741824   # 1GB  - 非常に大きなデータ(注意が必要)
```

### 制限超過時の動作

出力サイズが制限を超えた場合:
1. コマンドの実行を継続(出力のみ制限)
2. 超過を警告するエラーメッセージを記録
3. それまでの出力は保存される

### ベストプラクティス

1. **用途に応じた設定**: 処理するデータサイズを考慮
2. **余裕を持った設定**: 想定サイズの1.5〜2倍程度を設定
3. **監視**: 制限に達したケースを定期的に確認

```toml
# 推奨しない: 小さすぎる制限
[global]
output_size_limit = 1024  # 1KB - ほとんどのコマンドで不足

# 推奨: 適切な制限
[global]
output_size_limit = 10485760  # 10MB - 一般的な用途に適切
```

## 全体的な設定例

以下は、グローバルレベルの設定を組み合わせた実践的な例です:

```toml
version = "1.0"

[global]
# タイムアウト設定
timeout = 300  # デフォルト5分

# 作業ディレクトリ
workdir = "/var/app/workspace"

# ログレベル
log_level = "info"

# 標準パスの検証スキップ
verify_standard_paths = false

# 環境変数許可リスト
env_allowed = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "APP_CONFIG_DIR",
    "DATABASE_URL",
]

# ファイル検証リスト
verify_files = [
    "/opt/app/bin/main",
    "/opt/app/scripts/backup.sh",
]

# 出力サイズ制限
output_size_limit = 10485760  # 10MB

[[groups]]
name = "application_tasks"
# ... グループ設定が続く
```

## 次のステップ

次章では、グループレベルの設定(`[[groups]]`)について詳しく解説します。コマンドをグループ化し、グループ固有の設定を行う方法を学びます。
