# 要件定義書: 内部変数とプロセス環境変数の分離

## 1. プロジェクト概要

### 1.1 プロジェクト名
内部変数とプロセス環境変数の分離 (Separation of Internal Variables and Process Environment Variables)

### 1.2 プロジェクト目的
TOML設定ファイル展開用の「内部変数」と、子プロセス起動時に設定される「プロセス環境変数」を明確に分離することで、セキュリティを向上させ、意図しない環境変数の漏洩を防ぐ。システム環境変数の使用を厳格に制御し、明示的な取り込みを強制する。

### 1.3 背景と課題

**現在の状況**:
現在のgo-safe-cmd-runnerでは、環境変数が以下の2つの役割を持っている：
1. **TOML展開用**: `cmd`、`args`、`verify_files` などの文字列を展開する際に参照される
2. **プロセス環境変数**: runner が子プロセスを起動する際に、子プロセスの環境変数として設定される

これらは同じ名前空間で管理されており、以下の課題がある：

**課題**:
- **セキュリティリスク**: TOML展開にのみ必要な変数（パス情報など）が子プロセスの環境変数として漏洩する可能性がある
- **意図の不明確さ**: ある環境変数が「TOML展開用」なのか「子プロセス用」なのか、あるいは「両方」なのかが不明確
- **システム環境変数の制御不足**: システム環境変数を直接TOML内で参照できるため、意図しない変数が使用される可能性がある
- **保守性の低下**: 環境変数の用途が混在しているため、設定ファイルの理解が困難

**既存機能**:
- Global/Group/Command レベルでの `env` フィールド（Task 0031で実装済み）
- 環境変数の変数展開機能（`${VAR}` 形式のサポート）
- 環境変数の allowlist 機能（`env_allowlist`）
- verify_files での環境変数展開機能（Task 0030で実装済み）

## 2. 機能要件

### 2.1 内部変数システム

#### F001: `vars` フィールドによる内部変数定義
**概要**: TOML展開専用の内部変数を定義する

**設定形式**:
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
```

**スコープ**:
- **Global.vars**: すべてのグループとコマンドから参照可能
- **Group.vars**: そのグループ内のコマンドから参照可能
- **Command.vars**: そのコマンド内でのみ参照可能
- **内部変数は子プロセスの環境変数にはならない**（デフォルト）

**参照構文**:
- `%{variable_name}` の形式で参照
- `cmd`, `args`, `verify_files`, `env` の値、および他の `vars` 定義内で使用可能

**例**:
```toml
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

#### F002: `from_env` による環境変数の取り込み
**概要**: システム環境変数を内部変数として取り込む

**設定形式**:
```toml
[global]
env_allowlist = ["HOME", "PATH", "DB_PASSWORD"]
from_env = [
    "home=HOME",
    "user_path=PATH",
    "db_pass=DB_PASSWORD"
]

[[groups]]
name = "example"
from_env = [
    "custom=CUSTOM_VAR"  # このグループ専用の取り込み
]
```

**構文**: `内部変数名=システム環境変数名` の形式
- 既存の `env` や `vars` と同じ `[]string` 型、`KEY=VALUE` 形式
- 左辺: 内部変数名（`%{変数名}` で参照する名前）
- 右辺: システム環境変数名

**内部変数名の命名規則**:
- **POSIX準拠**: `[a-zA-Z_][a-zA-Z0-9_]*` の形式
- **予約プレフィックスの禁止**: `__runner_` で始まる名前は使用不可（runner の予約変数）
- **推奨**: 小文字とアンダースコアを使用（例: `home`, `user_path`, `db_host`）
- **大文字小文字の区別**: 変数名は大文字小文字を区別する（`home` と `HOME` は別の変数）
- **バリデーション**: 不正な変数名は設定読み込み時にエラー

**システム環境変数名の制約**:
- **POSIX準拠**: システム環境変数名も POSIX 形式に準拠すること
- **慣例**: システム環境変数は通常大文字（例: `HOME`, `PATH`, `USER`）

**例**:
```toml
[global]
env_allowlist = ["HOME", "PATH"]
from_env = [
    "home=HOME",           # OK: 小文字の内部変数名
    "user_home=HOME",      # OK: アンダースコア使用
    "HOME=HOME",           # OK: 大文字も可能（ただし推奨は小文字）
    "__runner_home=HOME",  # エラー: 予約プレフィックス
    "123invalid=HOME",     # エラー: 数字で始まる
    "invalid-name=HOME"    # エラー: ハイフン使用不可
]
```

**動作**:
- システム環境変数を内部変数名にマッピング
- `env_allowlist` に含まれる変数のみ取り込み可能
- 取り込んだ内部変数は `%{変数名}` で参照可能
- 取り込んだ変数はデフォルトでは子プロセスの環境変数にはならない

**スコープと継承ルール**:
- **Global.from_env**: すべてのグループとコマンドから参照可能（デフォルト）
- **Group.from_env の継承方式**: **マージ（Merge）**
  - グループが `from_env` を定義していない（`nil`）場合: `Global.from_env` を**継承**します。
  - グループが `from_env` を明示的に定義した場合: `Global.from_env` と**マージ**され、同名の変数はグループの定義で優先されます。
    - `from_env = []` と定義しても、Global.from_env が継承されます（Merge方式）。

**重要**: `from_env` は「Merge（マージ）」方式を採用する。グループが独自の `from_env` を定義すると、Global.from_env との統合が行われ、グループで定義された変数が同名の Global 変数を上書きします。

**セキュリティ制約**:
- `from_env` で参照するシステム環境変数は `env_allowlist` に含まれている必要がある
- `env_allowlist` に含まれていない変数を参照しようとするとエラー

**例1: 基本的な使い方**:
```toml
[global]
env_allowlist = ["HOME", "USER"]
from_env = [
    "home=HOME",
    "username=USER"
]
vars = ["config_dir=%{home}/.config"]

[[groups.commands]]
name = "show_config"
cmd = "cat"
args = ["%{config_dir}/app.conf"]
# 子プロセスには HOME, USER は設定されない（env で明示していないため）
```

**例2: Group.from_env の継承（グループが from_env を定義しない場合）**:
```toml
[global]
env_allowlist = ["HOME", "USER"]
from_env = [
    "home=HOME",
    "username=USER"
]

[[groups]]
name = "inherit_group"
# from_env 未定義 → Global.from_env を継承

[[groups.commands]]
name = "show_home"
cmd = "echo"
args = ["%{home}"]       # OK: Global.from_env の home を参照可能
# %{username} も参照可能（Global.from_env から継承）
```

**例3: Group.from_env の上書き（グループが from_env を定義する場合）**:
```toml
[global]
env_allowlist = ["HOME", "USER", "CUSTOM_VAR"]
from_env = [
    "home=HOME",
    "username=USER"
]

[[groups]]
name = "override_group"
from_env = [
    "custom=CUSTOM_VAR"  # 明示的定義 → Global.from_env は無視される
]

[[groups.commands]]
name = "show_custom"
cmd = "echo"
args = ["%{custom}"]     # OK: Group.from_env の custom を参照可能
args = ["%{home}"]       # エラー: Global.from_env の home は継承されない
```

**例4: allowlist チェック（エラーケース）**:
```toml
[global]
env_allowlist = ["HOME"]  # DB_PASSWORD は含まれていない
from_env = [
    "home=HOME",
    "db_pass=DB_PASSWORD"  # エラー: DB_PASSWORD は allowlist に含まれていない
]
```

#### F003: 内部変数間の参照
**概要**: 内部変数定義内で他の内部変数を参照可能

**参照可能な変数**:
- 同レベルの他の内部変数（`vars` 内の他の変数）
- 上位レベルの内部変数（Global.vars → Group.vars → Command.vars）
- `from_env` で取り込んだ変数（継承ルールに従う）

**from_env の参照における継承ルール**:
- グループが `from_env` を定義していない場合: Global.from_env の変数を参照可能
- グループが `from_env` を定義した場合: Global.from_env の変数は参照不可（上書き）

**展開順序**:
1. Global.from_env の処理（システム環境変数の取り込み）
2. Global.vars の展開（Global.from_env で取り込んだ変数を参照可能）
3. Group.from_env の処理（存在する場合、Global.from_env を上書き）
4. Group.vars の展開（Group.from_env または Global.from_env（継承時）+ Global.vars を参照可能）
5. Command.vars の展開（Group と Global の内部変数を参照可能）

**例1: 基本的な参照**:
```toml
[global]
env_allowlist = ["HOME"]
from_env = ["home=HOME"]
vars = [
    "app_name=myapp",
    "app_dir=%{home}/%{app_name}",   # from_env の home と vars の app_name を参照
    "data_dir=%{app_dir}/data"       # vars の app_dir を参照
]

[[groups]]
name = "processing"
# from_env 未定義 → Global.from_env を継承
vars = [
    "input_dir=%{data_dir}/input",   # Global.vars の data_dir を参照
    "output_dir=%{data_dir}/output", # Global.vars の data_dir を参照
    "home_backup=%{home}/backup"     # Global.from_env の home を参照可能（継承）
]

[[groups.commands]]
name = "process_data"
vars = [
    "temp_dir=%{input_dir}/temp",    # Group.vars の input_dir を参照
    "log_file=%{temp_dir}/process.log"  # Command.vars の temp_dir を参照
]
cmd = "process"
args = ["--input", "%{input_dir}", "--log", "%{log_file}"]
```

**例2: Group.from_env による上書き時の参照**:
```toml
[global]
env_allowlist = ["HOME", "CUSTOM_VAR"]
from_env = ["home=HOME"]
vars = ["global_dir=%{home}/global"]

[[groups]]
name = "custom_group"
from_env = ["custom=CUSTOM_VAR"]  # Global.from_env を上書き
vars = [
    "custom_dir=%{custom}/data",      # OK: Group.from_env の custom を参照
    "combined=%{global_dir}/custom"   # OK: Global.vars の global_dir を参照
    # "home_dir=%{home}/data"         # エラー: Global.from_env の home は継承されない
]

[[groups.commands]]
name = "process"
vars = [
    "work_dir=%{custom_dir}/work",    # OK: Group.vars の custom_dir を参照
    "output=%{combined}/output"       # OK: Group.vars の combined を参照
]
```

#### F004: 自動設定内部変数
**概要**: runner が自動的に提供する内部変数

**提供される変数**:
```
%{__runner_datetime}  - 実行開始日時（YYYYMMDD_HHMMSS 形式）
%{__runner_pid}       - 実行プロセスID（10進数文字列）
```

**特性**:
- すべての TOML 展開フィールド（`cmd`, `args`, `verify_files`, `env`, `vars`）で使用可能
- ユーザーが定義する必要がない（runner が自動的に提供）
- 実行時に動的に設定される値
- **内部変数として提供**されるため、デフォルトでは子プロセスの環境変数にはならない

**スコープ**:
- `%{__runner_datetime}`: Global スコープ（実行中は一定・すべてのグループ/コマンドで同じ値）
- `%{__runner_pid}`: Global スコープ（runner プロセスIDのため一定）

**用途例**:
```toml
[[groups]]
name = "backup"

[[groups.commands]]
name = "daily_backup"
cmd = "tar"
args = [
    "-czf",
    "/backup/backup_%{__runner_datetime}_pid%{__runner_pid}.tar.gz",
    "/data"
]
# 実行結果: /backup/backup_20250114_153045_pid12345.tar.gz
```

**子プロセスに環境変数として渡す場合**:
```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup"
env = [
    "BACKUP_TIME=%{__runner_datetime}",  # 内部変数を環境変数として明示的に設定
    "RUNNER_PID=%{__runner_pid}"
]
# 子プロセスの環境変数: BACKUP_TIME=20250114_153045, RUNNER_PID=12345
```

**Task 0028 との関係**:
- **Task 0028**: `${__RUNNER_DATETIME}` などの自動設定**環境変数**を実装
- **本タスク**: これらを自動設定**内部変数**に変更（`%{__runner_datetime}` など）
- **移行理由**:
  - セキュリティの一貫性（すべての環境変数は明示的に定義）
  - 子プロセスへの不要な情報漏洩を防止
  - 命名規則の統一（`%{変数名}` に統一）
  - 必要な場合のみ `env` で明示的に環境変数として設定可能

**命名規則**:
- プレフィックス: `__runner_` （アンダースコア2つ + runner）
- 形式: 小文字、アンダースコア区切り
- ユーザー定義変数との名前衝突を避けるための予約プレフィックス

**予約変数名**:
`__runner_` で始まる変数名はすべて runner の予約変数として扱われる。ユーザーは `vars` や `from_env` でこのプレフィックスを使用してはならない。

### 2.2 プロセス環境変数システム

#### F005: `env` フィールドの役割変更
**概要**: `env` フィールドは子プロセスの環境変数設定専用となる

**重要な変更点**:
- **従来**: `env` で定義した変数は TOML 展開でも参照可能だった
- **新仕様**: `env` で定義した変数は TOML 展開では参照不可（子プロセス環境変数のみ）

**設定形式**:
```toml
[global]
env = ["APP_ENV=production", "LOG_LEVEL=info"]

[[groups]]
name = "database"
env = ["DB_POOL_SIZE=20"]

[[groups.commands]]
name = "migrate"
cmd = "/usr/bin/migrate"
env = ["MIGRATION_TIMEOUT=300"]
```

**スコープと優先順位**:
優先順位（低 → 高）:
1. システム環境変数（`env_allowlist` に含まれるもののみ）
2. Global.env
3. Group.env
4. Command.env

**重要な参照制約**:
- **`env` は他フィールドから参照不可**: `env` で定義した変数を `cmd`, `args`, `verify_files`, `vars` から参照することはできません
- **一方向の参照**: `env` は内部変数（`vars` と `from_env`）を参照できますが、その逆はできません
- **実行時のみ有効**: `env` で定義した変数は子プロセス起動時にのみ設定され、TOML 展開フェーズでは使用不可

**例（エラーケース）**:
```toml
[global]
env = ["BASE_DIR=/opt/myapp"]

[[groups.commands]]
cmd = "%{BASE_DIR}/bin/tool"  # エラー: env の変数は参照不可

# 正しい方法
[global]
vars = ["base_dir=/opt/myapp"]
env = ["BASE_DIR=%{base_dir}"]  # 内部変数を環境変数として設定

[[groups.commands]]
cmd = "%{base_dir}/bin/tool"    # OK: vars の変数を参照
```

**例**:
```toml
[global]
env_allowlist = ["PATH", "HOME"]
env = ["LOG_LEVEL=info"]

[[groups]]
name = "batch"
env = ["LOG_LEVEL=debug"]  # global.env を上書き

[[groups.commands]]
name = "process"
cmd = "/usr/bin/process"
env = ["LOG_LEVEL=trace"]  # group.env を上書き
# 子プロセスの環境変数: LOG_LEVEL=trace (最優先), PATH=<システム>, HOME=<システム>
```

#### F006: `env` での内部変数参照
**概要**: `env` の値として内部変数を展開可能

**動作**:
- `env = ["VAR=%{internal_var}"]` の形式で内部変数を参照可能
- 内部変数の値が展開されて子プロセスの環境変数として設定される

**例**:
```toml
[global]
env_allowlist = ["HOME"]
from_env = ["home=HOME"]
vars = [
    "app_dir=/opt/myapp",
    "log_dir=%{app_dir}/logs"
]
env = [
    "APP_DIR=%{app_dir}",    # 内部変数を環境変数として公開
    "LOG_DIR=%{log_dir}"     # 内部変数を環境変数として公開
]

[[groups.commands]]
name = "run_app"
cmd = "/usr/bin/myapp"
# 子プロセスの環境変数: APP_DIR=/opt/myapp, LOG_DIR=/opt/myapp/logs, HOME=<システム>
```

#### F007: システム環境変数の引き継ぎ
**概要**: `env_allowlist` に含まれるシステム環境変数を子プロセスに引き継ぐ

**動作**:
- `env_allowlist` に列挙された変数のみが子プロセスに渡される
- `env` で明示的に定義された変数が優先される（システム環境変数を上書き）

**allowlist の継承ルール（既存仕様を維持）**:
- グループが `env_allowlist` を定義していない（nil）: globalのallowlistを**継承**
- グループが `env_allowlist` を明示的に定義: そのリストのみを使用（globalは継承しない = **上書き**）
- グループが `env_allowlist = []` を定義: すべての環境変数を**拒否**

**例1: allowlist による引き継ぎ**:
```toml
[global]
env_allowlist = ["PATH", "HOME", "LANG"]

[[groups.commands]]
name = "show_env"
cmd = "/usr/bin/env"
# 子プロセスの環境変数: PATH, HOME, LANG のみ（システム環境変数から）
```

**例2: env による上書き（from_env 経由で PATH を拡張）**:
```toml
[global]
env_allowlist = ["PATH"]
from_env = ["system_path=PATH"]
env = ["PATH=/custom/bin:%{system_path}"]  # from_env で取り込んだ PATH を拡張

[[groups.commands]]
name = "run"
cmd = "/custom/bin/tool"
# 子プロセスの環境変数: PATH=/custom/bin:<システムのPATH>
```

### 2.3 変数展開の構文と処理

#### F008: 変数参照構文の統一
**概要**: すべてのフィールドで `%{変数名}` 構文のみを使用

**構文ルール**:
- **内部変数参照**: `%{variable_name}`
  - `vars` で定義した変数
  - `from_env` で取り込んだ変数
- **使用可能な場所**: `cmd`, `args`, `verify_files`, `env`, `vars`
- **システム環境変数の直接参照は不可**
  - `${SYSTEM_VAR}` は**どこでも使用不可**（従来は可能だった）
  - システム環境変数を使うには必ず `from_env` で取り込む必要がある

**重要な変更点**:
- **従来の仕様**: `${VAR}` で env 変数やシステム環境変数を参照可能だった
- **新仕様**: `${VAR}` 構文は**完全に廃止**
- **すべての変数参照は `%{VAR}` に統一**

**変更の影響**:
- **後方互換性なし**: `${VAR}` 構文を使用している既存設定ファイルはすべてエラーになる
- **移行が必要**: すべての `${VAR}` を `%{VAR}` に書き換える必要がある
- **システム環境変数**: `from_env` で明示的に取り込む必要がある

**例: 変更前と変更後**:
```toml
# 変更前（Task 0031 の仕様）
[global]
env = ["BASE_DIR=/opt"]
[[groups.commands]]
cmd = "${BASE_DIR}/bin/tool"  # env で定義した変数を cmd で参照

# 変更後（本タスクの仕様）
[global]
vars = ["base_dir=/opt"]
[[groups.commands]]
cmd = "%{base_dir}/bin/tool"  # vars で定義した変数を cmd で参照
```

#### F009: `env` での変数参照
**概要**: `env` フィールドでは内部変数（`%{VAR}`）のみ参照可能

**構文**:
- **内部変数の参照**: `%{variable_name}` のみ使用可能
  - `vars` で定義した変数
  - `from_env` で取り込んだ変数
- **`${VAR}` 構文は使用不可**: 従来可能だった `${VAR}` 構文は完全に廃止

**重要な変更点**:
- **従来の仕様**: `env = ["PATH=/custom:${PATH}"]` のように env 変数や環境変数を参照可能だった
- **新仕様**: `env` でも `%{VAR}` のみ使用可能。システム環境変数は `from_env` 経由でのみアクセス可能

**PATH 拡張の実現方法**:
システム環境変数（例: PATH）を拡張する場合は、`from_env` で取り込んでから `env` で参照する：

```toml
[global]
env_allowlist = ["PATH"]
from_env = ["system_path=PATH"]
env = ["PATH=/custom/bin:%{system_path}"]  # from_env 経由で PATH を拡張

[[groups]]
from_env = ["global_path=PATH"]  # Global.from_env を上書き
env = ["PATH=/group/bin:%{global_path}"]

[[groups.commands]]
# Group.from_env を継承
env = ["PATH=/cmd/bin:%{global_path}"]
# 注意: ここで参照する %{global_path} は Group.from_env の global_path
```

**階層的な PATH 拡張の実現**:
各レベルで PATH を段階的に拡張するには、内部変数を経由する：

```toml
[global]
env_allowlist = ["PATH"]
from_env = ["system_path=PATH"]
vars = ["path_level1=%{system_path}"]
env = ["PATH=/custom/bin:%{system_path}"]

[[groups]]
# Global.from_env を継承（system_path が使用可能）
vars = ["path_level2=/custom/bin:%{system_path}"]
env = ["PATH=/group/bin:%{path_level2}"]

[[groups.commands]]
# Group.vars と Global.vars を参照可能
env = ["PATH=/cmd/bin:%{path_level2}"]
# 最終的な PATH: /cmd/bin:/custom/bin:<システムのPATH>
# （Command.env が Group.env を上書きするため、/group/bin は含まれません）
```

**セキュリティ上の利点**:
- すべてのシステム環境変数アクセスが `from_env` + `env_allowlist` で制御される
- `env` で意図しないシステム環境変数を参照するリスクを排除
- 一貫した構文（`%{VAR}` のみ）で混乱を防ぐ

#### F010: 展開処理の順序
**概要**: 設定ファイル読み込み時に以下の順序で展開

**処理順序**:
```
1. Global.from_env の処理
   - システム環境変数を内部変数にマッピング
   - env_allowlist チェック

2. Global.vars の展開
   - from_env で取り込んだ内部変数を参照可能（%{VAR}）
   - 循環参照検出

3. Global.env の展開
   - 内部変数（vars, from_env）を参照可能（%{VAR} のみ）
   - ${VAR} 構文は使用不可
   - env_allowlist チェックは不要（内部変数のみ参照するため）

4. Global.verify_files の展開
   - 内部変数（vars, from_env）を参照可能（%{VAR} のみ）
   - env で定義した変数は参照不可

5. 各 Group について以下を実行:
   a. Group.from_env の処理（存在する場合）
   b. Group.vars の展開（Global の内部変数 + Group の from_env を参照可能）
   c. Group.env の展開（Group と Global の内部変数を参照可能、%{VAR} のみ）
   d. Group.verify_files の展開（Group と Global の内部変数を参照可能、%{VAR} のみ）

6. 各 Command について以下を実行:
   a. Command.vars の展開（Group と Global の内部変数を参照可能）
   b. Command.env の展開（Command, Group, Global の内部変数を参照可能、%{VAR} のみ）
   c. Command.cmd の展開（Command, Group, Global の内部変数を参照可能、%{VAR} のみ）
   d. Command.args の展開（Command, Group, Global の内部変数を参照可能、%{VAR} のみ）
```

**重要な制約**:
- **すべての変数参照は %{VAR} のみ**: ${VAR} 構文は完全に廃止
- **verify_files は env を参照できない**: verify_files の展開時には env で定義した変数は使用不可
- **env は実行時のみ有効**: env で定義した変数は子プロセス起動時にのみ設定される
- **セキュリティ**: システム環境変数は from_env + env_allowlist 経由でのみアクセス可能
- **Command.vars のスコープ**: Command.vars で定義した変数はそのコマンド内でのみ参照可能（他のコマンドからは参照不可）

### 2.4 セキュリティ要件

#### F011: システム環境変数アクセスの制限
**概要**: システム環境変数へのアクセスを厳格に制御

**制約**:
1. **TOML展開でのシステム環境変数参照**: `from_env` 経由でのみ可能
2. **子プロセスへのシステム環境変数引き継ぎ**: `env_allowlist` に含まれるもののみ
3. **allowlist のチェックタイミング**:
   - `from_env` で取り込む時のみ（システム環境変数が allowlist に含まれるか）
   - `env` の展開時はチェック不要（`%{VAR}` のみ使用可能で、システム環境変数を直接参照できないため）

**例: セキュアな設定**:
```toml
[global]
env_allowlist = ["HOME", "PATH"]  # 許可するシステム環境変数を明示

from_env = [
    "home=HOME",      # OK: allowlist に含まれる
    "path=PATH"       # OK: allowlist に含まれる
]

vars = [
    "config=%{home}/.config",   # OK: from_env で取り込んだ変数を参照
    "secret=%{SECRET}"          # エラー: SECRET は from_env で取り込んでいない
]
```

#### F012: 循環参照検出
**概要**: 変数参照の循環を検出してエラーとする

**禁止される循環参照**:
1. **同一レベル内での循環参照**:
   ```toml
   vars = ["A=%{B}", "B=%{A}"]  # エラー: A ↔ B の循環
   ```

2. **同一変数名での完全な自己参照**:
   ```toml
   vars = ["A=%{A}"]  # エラー: 値の追加なしの自己参照
   ```

3. **複数変数を経由する循環参照**:
   ```toml
   vars = ["A=%{B}", "B=%{C}", "C=%{A}"]  # エラー: A → B → C → A の循環
   ```

**循環参照は発生しないケース**:
1. **レベル間の参照**:
   - Global.vars と Group.vars 間での循環参照は構造上発生しない
   - Group.vars は Global.vars を参照できるが、逆は不可（一方向参照）

2. **from_env と vars の組み合わせ**:
   ```toml
   [global]
   env_allowlist = ["PATH"]
   from_env = ["path=PATH"]            # システムPATHを取り込み
   vars = ["path=/custom/bin:%{path}"] # from_env の path を参照（OK）
   ```
   - `from_env` で取り込んだ変数は `vars` で上書き可能です。`from_env` の処理後に `vars` を展開するため、循環は発生しません。

3. **階層的な展開による PATH 拡張**:
   ```toml
   [global]
   env_allowlist = ["PATH"]
   from_env = ["path=PATH"]            # ステップ1: システムPATHを取り込み
   env = ["PATH=/global/bin:%{path}"]  # ステップ2: from_env の path を参照

   [[groups]]
   env = ["PATH=/group/bin:%{path}"]   # ステップ3: Global.from_env の path を参照（継承）
   ```
   - 処理順序: Global.from_env → Global.env → Group.env
   - 各ステップで参照するのは既に確定した値なので循環は発生しない

**循環参照検出の実装**:
- **既存の visited map 方式を使用**: 変数展開時に visited マップで訪問済み変数を追跡
- **無限ループ防止**: 最大15回の反復制限を維持
- **エラーメッセージ**: 循環参照チェーンを明示（例: `A → B → C → A`）

**重要**: 処理順序（from_env → vars → env）と階層構造（Global → Group → Command）により、自己参照のための特別な処理は不要です。

#### F013: エスケープシーケンス
**概要**: 特殊文字をリテラルとして扱うためのエスケープ機能

**サポートするエスケープ**:
- `\%` → `%` （パーセント記号をリテラルとして使用、`%{VAR}` を展開させない場合に使用）
- `\\` → `\` （バックスラッシュをリテラルとして使用）
- その他の `\x` の組み合わせはエラー

**重要な変更点**:
- **`\$` エスケープは廃止**: `${VAR}` 構文が完全に廃止されたため、`$` 記号は特殊文字ではなくなりました
- `$` 記号はエスケープなしでそのまま使用可能（通常の文字として扱われる）

**例**:
```toml
[global]
vars = [
    "price=item costs $100",          # 結果: item costs $100（エスケープ不要）
    "template=use \%{variable}",      # 結果: use %{variable}（展開を防ぐ）
    "path=C:\\Windows\\System32",     # 結果: C:\Windows\System32
    "literal_percent=discount is 20\%"  # 結果: discount is 20%
]
```

**Task 0026 からの変更**:
- Task 0026 では `\$` エスケープがサポートされていた（`${VAR}` 構文のため）
- 本タスクで `${VAR}` 構文が廃止されたため、`\$` エスケープも不要に

### 2.5 後方互換性と移行

#### F014: 非互換性の明示と移行支援
**概要**: 本タスクは後方互換性を**破壊する**

**主な非互換性**:
1. **`${VAR}` 構文の完全廃止**:
   - 従来: `cmd`, `args`, `verify_files`, `env` で使用可能
   - 新仕様: すべてのフィールドで使用不可（エラーになる）

2. **システム環境変数の直接参照不可**:
   - 従来: `cmd = "${HOME}/bin/tool"` が可能
   - 新仕様: `from_env` で取り込む必要がある

3. **env で定義した変数を TOML 展開で参照不可**:
   - 従来: `env = ["DIR=/opt"]` → `cmd = "${DIR}/tool"` が可能
   - 新仕様: `vars = ["dir=/opt"]` → `cmd = "%{dir}/tool"` を使用

**必須の移行ドキュメント**:

以下のドキュメントを作成することが本タスクの要件に含まれる：

1. **構文早見表（チートシート）**
   - どのフィールドでどの構文が使えるかを一覧化
   - Markdown テーブル形式で視覚的にわかりやすく提示

   **内容例**:
   ```
   | フィールド | %{VAR} | ${VAR} | 参照可能な変数 |
   |-----------|--------|--------|--------------|
   | vars      | ✓      | ✗      | from_env, 上位vars |
   | env       | ✓      | ✗      | from_env, vars |
   | cmd       | ✓      | ✗      | from_env, vars |
   | args      | ✓      | ✗      | from_env, vars |
   | verify_files | ✓   | ✗      | from_env, vars |
   ```

2. **実践的な変換例集**

   以下の典型的なユースケースについて「変更前」→「変更後」のコードスニペットを提供：

   a. **基本的なシステム環境変数の使用（PATH拡張の基本形）**
   ```toml
   # 変更前
   [global]
   env = ["PATH=/custom/bin:${PATH}"]

   # 変更後（自己参照拡張を利用）
   [global]
   env_allowlist = ["PATH"]
   from_env = ["path=PATH"]               # システムPATHを内部変数として取り込み
   env = ["PATH=/custom/bin:%{path}"]     # 自己参照による拡張
   ```

   b. **HOME ディレクトリの使用**
   ```toml
   # 変更前
   [[groups.commands]]
   cmd = "${HOME}/bin/tool"

   # 変更後
   [global]
   env_allowlist = ["HOME"]
   from_env = ["home=HOME"]

   [[groups.commands]]
   cmd = "%{home}/bin/tool"
   ```

   c. **秘密情報の受け渡し**
   ```toml
   # 変更前
   [[groups.commands]]
   cmd = "/usr/bin/app"
   args = ["--token", "${API_TOKEN}"]

   # 変更後
   [global]
   env_allowlist = ["API_TOKEN"]
   from_env = ["api_token=API_TOKEN"]

   [[groups.commands]]
   cmd = "/usr/bin/app"
   args = ["--token", "%{api_token}"]
   ```

   d. **env 変数を TOML 展開で使う（vars への移行）**
   ```toml
   # 変更前
   [global]
   env = ["BASE_DIR=/opt/app"]

   [[groups.commands]]
   cmd = "${BASE_DIR}/bin/process"

   # 変更後
   [global]
   vars = ["base_dir=/opt/app"]
   env = ["BASE_DIR=%{base_dir}"]  # 子プロセスにも渡す場合

   [[groups.commands]]
   cmd = "%{base_dir}/bin/process"
   ```

   e. **複雑な階層的設定（内部変数を活用した段階的構築）**
   ```toml
   # 変更前（複数のレベルで環境変数を使用）
   [global]
   env = ["BASE_DIR=/opt", "PATH=/global/bin:${PATH}"]

   [[groups]]
   env = ["WORK_DIR=${BASE_DIR}/work", "PATH=/group/bin:${PATH}"]

   # 変更後（内部変数で段階的に構築）
   [global]
   env_allowlist = ["PATH"]
   from_env = ["path=PATH"]
   vars = [
       "base_dir=/opt",
       "global_bin=/global/bin",
       "extended_path=%{global_bin}:%{path}"
   ]
   env = [
       "BASE_DIR=%{base_dir}",              # 内部変数を環境変数として公開
       "PATH=%{extended_path}"
   ]

   [[groups]]
   vars = [
       "work_dir=%{base_dir}/work",         # Global.vars を参照
       "group_path=/group/bin:%{extended_path}"  # Global.vars を参照して拡張
   ]
   env = [
       "WORK_DIR=%{work_dir}",
       "PATH=%{group_path}"
   ]
   # 最終的なPATH: /group/bin:/global/bin:<システムのPATH>
   ```

   f. **自動設定変数の変更（Task 0028からの移行）**
   ```toml
   # 変更前（Task 0028）
   [[groups.commands]]
   name = "backup"
   cmd = "tar"
   args = [
       "-czf",
       "/backup/backup_${__RUNNER_DATETIME}.tar.gz",  # 自動設定環境変数
       "/data"
   ]
   # 子プロセスの環境変数: __RUNNER_DATETIME=20250114_153045 (自動)

   # 変更後（本タスク）
   [[groups.commands]]
   name = "backup"
   cmd = "tar"
   args = [
       "-czf",
       "/backup/backup_%{__runner_datetime}.tar.gz",  # 自動設定内部変数
       "/data"
   ]
   # 子プロセスの環境変数: なし（明示的に設定しない限り）

   # 子プロセスにも環境変数として渡す必要がある場合
   [[groups.commands]]
   name = "backup_with_env"
   cmd = "/usr/bin/backup"
   env = [
       "BACKUP_TIME=%{__runner_datetime}",  # 明示的に環境変数として設定
       "RUNNER_PID=%{__runner_pid}"
   ]
   # 子プロセスの環境変数: BACKUP_TIME=20250114_153045, RUNNER_PID=12345 (明示的)
   ```

3. **エラーメッセージと対処法（FAQ）**

   新仕様で発生しがちなエラーメッセージと解決方法：

   **Q1: `Error: ${VAR} syntax is no longer supported`**
   - **原因**: `${VAR}` 構文を使用している
   - **対処法**: `%{VAR}` に変更し、必要に応じて `from_env` で変数を取り込む

   **Q2: `Error: Undefined variable '%{HOME}' referenced`**
   - **原因**: システム環境変数を直接参照しようとしている
   - **対処法**: `from_env = ["home=HOME"]` を追加し、`env_allowlist` に `HOME` を含める

   **Q3: `Error: System environment variable 'SECRET' not in allowlist`**
   - **原因**: `from_env` で参照する変数が `env_allowlist` に含まれていない
   - **対処法**: `env_allowlist` に `SECRET` を追加する

   **Q4: `Error: Cannot reference env variable in cmd field`**
   - **原因**: `env` で定義した変数を `cmd` で参照しようとしている
   - **対処法**: `vars` に移動するか、`from_env` で取り込む

   **Q5: `Error: Undefined variable '${__RUNNER_DATETIME}' referenced`**
   - **原因**: Task 0028の自動設定環境変数構文を使用している
   - **対処法**: `%{__runner_datetime}` に変更する（小文字、`%{}`構文）

   **Q6: 自動設定変数が子プロセスの環境変数に設定されない**
   - **原因**: 新仕様では自動設定変数は内部変数として提供される
   - **対処法**: 必要な場合は `env = ["MY_VAR=%{__runner_datetime}"]` で明示的に環境変数として設定

4. **移行チェックリスト**

   既存設定ファイルを新仕様に移行する手順：

   - [ ] すべての `${VAR}` を検索
   - [ ] システム環境変数を使っている箇所を特定
   - [ ] `${__RUNNER_*}` 自動設定変数を使っている箇所を特定
   - [ ] 必要な環境変数を `env_allowlist` に追加
   - [ ] システム環境変数を `from_env` で取り込み
   - [ ] すべての `${VAR}` を `%{VAR}` に変更
   - [ ] `${__RUNNER_*}` を `%{__runner_*}` に変更（大文字→小文字）
   - [ ] `env` で定義していた変数で TOML 展開に使うものを `vars` に移動
   - [ ] 子プロセスに渡す必要がある変数は `env` に残す（`%{VAR}` 形式で）
   - [ ] 自動設定変数を子プロセスに渡す必要がある場合は `env` で明示的に設定
   - [ ] 設定ファイルを読み込んでエラーがないか確認
   - [ ] テスト実行して動作を確認

5. **移行ガイドの配置**

   以下のドキュメントを作成：
   - `docs/migration/0033_vars_env_separation.md`: 移行ガイド本体
   - `docs/migration/0033_cheatsheet.md`: 構文早見表
   - `docs/migration/0033_examples.md`: 変換例集
   - `docs/migration/0033_faq.md`: FAQ
   - README への移行ガイドへのリンク追加

## 3. 非機能要件

### 3.1 性能要件

#### P001: 展開処理性能
**目標**:
- 変数1個あたりの展開時間: 1ms以下
- 設定ファイルの読み込み時間への影響: 既存実装から10%以内の増加
- メモリ使用量の増加: 変数定義の2倍以内

#### P002: スケーラビリティ
**制限**:
- 内部変数の合計数: 最大500個（Global + Group + Command）
- from_env のマッピング数: 最大100個
- 変数展開のネスト深度: 最大10レベル

### 3.2 信頼性要件

#### R001: エラーハンドリング
**要件**:
- 変数定義のエラーは設定ファイル読み込み時に検出
- 循環参照、未定義変数、allowlist違反などで明確なエラーメッセージ
- エラー発生箇所（global/group/command、変数名）を明示

**エラーメッセージ例**:
```
Error: Undefined variable '%{VAR1}' referenced in global.vars
Error: Circular reference detected in group 'database' vars: db_host -> db_port -> db_host
Error: System environment variable 'SECRET' not in allowlist (referenced in global.from_env)
Error: ${VAR} syntax is no longer supported. Use %{VAR} for internal variables (in cmd field)
Error: To use system environment variable 'PATH', add it to from_env first (in global.env)
```

#### R002: デバッグサポート
**要件**:
- 変数展開の過程をログ出力（デバッグレベル）
- 内部変数と環境変数の最終的な値を確認可能
- from_env のマッピング結果を出力

### 3.3 保守性要件

#### M001: コード設計
**原則**:
- 既存の環境変数展開ロジックを可能な限り再利用
- 内部変数と環境変数の展開処理を明確に分離
- 設定読み込み時に一度だけ展開（実行時の再展開なし）

#### M002: テスト要件
**カバレッジ目標**:
- 単体テスト: 95%以上
- 統合テスト: 主要シナリオを100%カバー
- エッジケーステスト: 循環参照、未定義変数、allowlist違反など
- 移行テスト: Task 0031 の設定例を新仕様に変換してテスト



## 4. 実装スコープ

### 4.1 スコープ内 (In Scope)

- [ ] `GlobalConfig`, `CommandGroup` に `FromEnv`, `Vars`, `InternalVars` フィールドを追加
- [ ] `Command` に `Vars`, `InternalVars` フィールドを追加（`FromEnv` は追加しない）
- [ ] `from_env` のパースと処理（システム環境変数の取り込み、Global と Group レベルのみ）
- [ ] `from_env` の継承ルール実装（Override 方式）
- [ ] `vars` のパースと展開（内部変数の定義、Global, Group, Command レベル）
- [ ] Command.vars の実装（コマンド専用の内部変数）
- [ ] `%{variable}` 構文での内部変数参照
- [ ] `env` での内部変数参照（`%{VAR}` 構文のみ）
- [ ] `cmd`, `args`, `verify_files` での内部変数参照（`%{VAR}` のみ）
- [ ] すべてのフィールドで `${VAR}` 構文の完全廃止とエラー処理
- [ ] allowlist チェックの統合（from_env でのみ）
- [ ] 循環参照検出（vars と env で）
- [ ] エラーハンドリングとメッセージ
- [ ] 既存機能との統合テスト
- [ ] 移行ドキュメントの作成（構文早見表、変換例集、FAQ、チェックリスト）
- [ ] ユーザーガイドの更新（新構文、使用例）
- [ ] README への移行ガイドリンク追加

### 4.2 スコープ外 (Out of Scope)

- 既存設定ファイルの自動変換ツール（将来の拡張として検討）
- 条件付き変数定義
- 変数のテンプレート機能
- 実行時の動的な変数変更

## 5. 成功基準

### 5.1 機能的成功基準
- [ ] `vars` と `from_env` による内部変数の定義と参照が正常動作
- [ ] `from_env` の継承ルール（Override 方式）が正常動作
- [ ] 内部変数の階層的参照（Global → Group → Command）が正常動作
- [ ] Command.vars の定義と参照が正常動作
- [ ] すべてのフィールド（`env`, `cmd`, `args`, `verify_files`, `vars`）で `%{VAR}` が正常動作
- [ ] すべてのフィールドで `${VAR}` がエラーとなることを確認
- [ ] allowlist チェックが正常動作（from_env でのみ）
- [ ] 循環参照検出が正常動作（vars と env で）
- [ ] システム環境変数の引き継ぎが正常動作（env_allowlist に基づく）

### 5.2 性能成功基準
- [ ] 変数展開処理時間が1ms/変数以下
- [ ] 設定ファイル読み込み時間の増加が10%以内
- [ ] メモリ使用量の増加が変数定義の2倍以内

### 5.3 品質成功基準
- [ ] 単体テストカバレッジ95%以上
- [ ] 統合テスト全パターンPASS
- [ ] エッジケーステスト（循環参照、未定義変数等）全PASS
- [ ] 移行テスト（Task 0031 の例を変換）全PASS

### 5.4 ドキュメント成功基準
- [ ] 構文早見表（チートシート）の作成（Markdown テーブル形式）
- [ ] 実践的な変換例集の作成（5つ以上の典型的なユースケース）
- [ ] エラーメッセージと対処法（FAQ）の作成（主要なエラー4つ以上）
- [ ] 移行チェックリストの作成（手順の明確化）
- [ ] 移行ガイド本体の完成（上記をまとめた総合ドキュメント）
- [ ] ユーザーガイドの更新（新構文、使用例）
- [ ] セキュリティガイドラインの更新（allowlist の使い方）
- [ ] README への移行ガイドリンク追加
- [ ] すべてのサンプル TOML ファイルの新仕様への更新

## 6. リスク分析

### 6.1 技術リスク

**リスク**: 既存の環境変数展開処理との複雑な相互作用
**影響**: 高
**対策**:
- 既存の expansion.go を慎重に拡張
- 段階的な実装とテスト（vars → from_env → env の順）
- 既存テストの継続実行

**リスク**: 複雑な変数参照での性能劣化
**影響**: 低
**対策**:
- 設定読み込み時に一度だけ展開（実行時の再展開なし）
- ベンチマークテストで性能を監視

### 6.2 セキュリティリスク

**リスク**: allowlist のバイパス
**影響**: 高
**対策**:
- from_env と env の両方で厳格な allowlist チェック
- セキュリティ監査テストの実施
- ドキュメントでのベストプラクティス提示

**リスク**: 循環参照による無限ループ
**影響**: 中
**対策**:
- 既存の反復制限方式（最大15回）を使用
- vars と env の両方で循環参照検出

### 6.3 互換性リスク

**リスク**: 既存設定ファイルが動作しない（破壊的変更）
**影響**: 高
**対策**:
- 詳細な移行ガイドの提供
- 一般的なパターンの変換例を文書化
- エラーメッセージに移行のヒントを含める
- サンプルファイルを新仕様に更新

### 6.4 ユーザビリティリスク

**リスク**: システム環境変数を使うために `from_env` が必須になることで設定が複雑化する
**影響**: 中
**対策**:
- **構文早見表**（チートシート）の提供
  - どのフィールドでどの構文が使えるかを視覚的に一覧化
  - Markdown テーブル形式で印刷・参照しやすく
- **実践的な変換例集**
  - PATH 拡張、HOME ディレクトリ、秘密情報など典型的な5つ以上のパターン
  - 「変更前」→「変更後」を並べて提示
  - コピー&ペーストで使える完全なコードスニペット
- **エラーメッセージと対処法（FAQ）**
  - 新仕様で頻出する4つ以上のエラーとその解決方法
  - 具体的な修正例を含める
- **移行チェックリスト**
  - 段階的な移行手順を明確化
  - チェックボックス形式で進捗を確認可能
- **エラーメッセージの改善**
  - `${VAR}` を検出したら `%{VAR}` への変更を提案
  - `from_env` の使い方を具体的に示す
- **構文の統一による混乱の軽減**
  - `%{VAR}` のみに統一されたことで、「どちらを使うべきか」の判断が不要に

## 7. 関連資料

### 7.1 関連タスク
- Task 0031: Global・Groupレベル環境変数設定機能（本タスクで大幅に変更）
- Task 0026: コマンド・引数内環境変数展開機能（変数展開ロジックの基盤）
- Task 0030: verify_files内環境変数展開（verify_files 展開の基盤）
- Task 0008: env_allowlist機能（allowlist検証の基盤）

### 7.2 設計ドキュメント
- `docs/tasks/0033_vars_env_separation/02_architecture.md`: システムアーキテクチャ設計
- `docs/tasks/0033_vars_env_separation/03_detailed_design.md`: 詳細設計とデータ構造

### 7.3 参考実装
- `internal/runner/config/expansion.go`: 既存の環境変数展開実装
- `internal/runner/environment/processor.go`: Command.Env処理
- `internal/runner/runnertypes/config.go`: 設定データ構造

### 7.4 参考ドキュメント
- `docs/dev/config-inheritance-behavior.ja.md`: allowlist継承動作の仕様
- `docs/tasks/0031_global_group_env/`: 変更対象となる既存機能の仕様
- TOML v1.0.0 仕様

## 8. 用語集

- **内部変数 (Internal Variable)**: `vars` または `from_env` で定義される、TOML展開専用の変数。デフォルトでは子プロセスの環境変数にはならない。`%{変数名}` で参照。
- **プロセス環境変数 (Process Environment Variable)**: `env` で定義される、子プロセス起動時に設定される環境変数。TOML展開では参照不可。
- **システム環境変数 (System Environment Variable)**: runner 実行時のシェル環境に存在する環境変数。`from_env` で取り込むか、`env_allowlist` で引き継ぐ必要がある。
- **from_env**: システム環境変数を内部変数にマッピングする機能。`[]string` 型で `内部変数名=システム環境変数名` の形式（`env` や `vars` と同じ）。グループが `from_env` を定義すると Global.from_env は継承されない（Override 方式）。
- **from_env の継承ルール**: グループが `from_env` を定義していない場合は Global.from_env を継承。定義した場合は Global.from_env は無視される（`env_allowlist` と同じ Override 方式）。
- **env_allowlist**: システム環境変数のホワイトリスト。allowlist に含まれる変数のみが子プロセスに引き継がれ、`from_env` で参照可能。
- **TOML展開 (TOML Expansion)**: 設定ファイル内の `cmd`, `args`, `verify_files` などのフィールドで変数参照を実際の値に置き換える処理。
