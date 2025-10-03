# 第5章: グループレベル設定 [[groups]]

## 概要

`[[groups]]` セクションは、関連するコマンドをまとめる論理的な単位です。各グループには名前、説明、および共通の設定を持たせることができます。設定ファイルには1つ以上のグループが必要です。

## 5.1 グループの基本設定

### 5.1.1 name - グループ名

#### 概要

グループを識別するための一意な名前を指定します。

#### 文法

```toml
[[groups]]
name = "グループ名"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | 必須 |
| **設定可能な階層** | グループのみ |
| **有効な値** | 英数字、アンダースコア、ハイフン |
| **一意性** | 設定ファイル内で一意である必要がある |

#### 役割

- **識別**: グループを一意に識別
- **ログ出力**: 実行ログでどのグループが実行されているかを表示
- **エラー報告**: エラー発生時にどのグループで問題が起きたかを特定

#### 設定例

```toml
version = "1.0"

[[groups]]
name = "database_backup"
# ...

[[groups]]
name = "log_rotation"
# ...

[[groups]]
name = "system_maintenance"
# ...
```

#### 命名のベストプラクティス

```toml
# 推奨: 明確で説明的な名前
[[groups]]
name = "daily_database_backup"

[[groups]]
name = "weekly_log_cleanup"

# 推奨しない: 不明瞭な名前
[[groups]]
name = "group1"

[[groups]]
name = "temp"
```

### 5.1.2 description - 説明

#### 概要

グループの目的や役割を説明する人間が読むためのテキストです。

#### 文法

```toml
[[groups]]
name = "example"
description = "グループの説明"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション(推奨) |
| **設定可能な階層** | グループのみ |
| **有効な値** | 任意の文字列 |

#### 役割

- **ドキュメント化**: グループの目的を明確化
- **保守性向上**: 他の開発者が設定を理解しやすくする
- **ログ出力**: 実行時に表示され、何が実行されているかを理解しやすくする

#### 設定例

```toml
version = "1.0"

[[groups]]
name = "database_maintenance"
description = "データベースのバックアップと最適化を実行"

[[groups.commands]]
name = "backup"
description = "PostgreSQL データベースの完全バックアップ"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups.commands]]
name = "vacuum"
description = "データベースの最適化(VACUUM ANALYZE)"
cmd = "/usr/bin/psql"
args = ["-c", "VACUUM ANALYZE"]
```

### 5.1.3 priority - 優先度

#### 概要

グループの実行優先度を指定します。小さい数字ほど優先度が高く、先に実行されます。

#### 文法

```toml
[[groups]]
name = "example"
priority = 数値
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 整数 (int) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グループのみ |
| **デフォルト値** | 0 |
| **有効な値** | 整数(負の値も可) |

#### 役割

- **実行順序の制御**: 依存関係のあるグループを適切な順序で実行
- **重要度の表現**: 重要なタスクを先に実行

#### 設定例

#### 例1: 優先度による実行順序の制御

```toml
version = "1.0"

[[groups]]
name = "preparation"
description = "事前準備タスク"
priority = 1  # 最初に実行

[[groups.commands]]
name = "create_directory"
cmd = "mkdir"
args = ["-p", "/tmp/workspace"]

[[groups]]
name = "main_tasks"
description = "メインタスク"
priority = 2  # 2番目に実行

[[groups.commands]]
name = "process_data"
cmd = "/opt/app/process"
args = []

[[groups]]
name = "cleanup"
description = "後処理"
priority = 3  # 最後に実行

[[groups.commands]]
name = "remove_temp"
cmd = "rm"
args = ["-rf", "/tmp/workspace"]
```

実行順序: `preparation` → `main_tasks` → `cleanup`

#### 例2: 重要度に応じた優先度設定

```toml
version = "1.0"

[[groups]]
name = "critical_backup"
description = "重要データのバックアップ"
priority = 10  # 高優先度

[[groups.commands]]
name = "backup_database"
cmd = "/usr/bin/backup_db.sh"
args = []

[[groups]]
name = "routine_maintenance"
description = "日常的なメンテナンス"
priority = 50  # 中優先度

[[groups.commands]]
name = "clean_logs"
cmd = "/usr/bin/clean_old_logs.sh"
args = []

[[groups]]
name = "optional_optimization"
description = "オプションの最適化タスク"
priority = 100  # 低優先度

[[groups.commands]]
name = "optimize"
cmd = "/usr/bin/optimize.sh"
args = []
```

#### 注意事項

1. **同じ優先度**: 同じ優先度のグループは定義された順序で実行されます
2. **負の優先度**: 負の値も使用可能(より高い優先度を表現)
3. **省略時**: priority を指定しない場合は 0 として扱われます

## 5.2 リソース管理設定

### 5.2.1 temp_dir - 一時ディレクトリ

#### 概要

グループ実行時に一時ディレクトリを自動作成します。作成されたディレクトリはグループ内の全コマンドの作業ディレクトリになります。

#### 文法

```toml
[[groups]]
name = "example"
temp_dir = true/false
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 真偽値 (boolean) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グループのみ |
| **デフォルト値** | false |
| **有効な値** | true, false |

#### 役割

- **隔離された作業環境**: グループごとに独立した作業スペースを提供
- **衝突回避**: 複数のグループが同時実行されても競合しない
- **自動クリーンアップ**: cleanup オプションと組み合わせて自動削除

#### 設定例

#### 例1: 一時ディレクトリの使用

```toml
version = "1.0"

[[groups]]
name = "data_processing"
temp_dir = true  # 一時ディレクトリを自動作成

[[groups.commands]]
name = "download_data"
cmd = "wget"
args = ["https://example.com/data.csv", "-O", "data.csv"]
# 一時ディレクトリに data.csv がダウンロードされる

[[groups.commands]]
name = "process_data"
cmd = "/opt/app/process"
args = ["data.csv", "output.txt"]
# 同じ一時ディレクトリで処理

[[groups.commands]]
name = "list_results"
cmd = "ls"
args = ["-la"]
# 一時ディレクトリの内容を表示
```

#### 例2: cleanup との組み合わせ

```toml
version = "1.0"

[[groups]]
name = "temporary_work"
temp_dir = true   # 一時ディレクトリを作成
cleanup = true    # グループ終了後に自動削除

[[groups.commands]]
name = "create_temp_files"
cmd = "touch"
args = ["temp1.txt", "temp2.txt"]

[[groups.commands]]
name = "process_files"
cmd = "cat"
args = ["temp1.txt", "temp2.txt"]
# グループ終了後、一時ディレクトリごと削除される
```

#### 一時ディレクトリの場所

一時ディレクトリは以下の場所に作成されます:
- システムの一時ディレクトリ(`$TMPDIR` または `/tmp`)配下
- ディレクトリ名: `go-safe-cmd-runner-<ランダム文字列>`

### 5.2.2 workdir - 作業ディレクトリ

#### 概要

グループ内の全コマンドが実行される作業ディレクトリを指定します。グローバルレベルの `workdir` をオーバーライドします。

#### 文法

```toml
[[groups]]
name = "example"
workdir = "ディレクトリパス"
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | グローバルの workdir、または実行ディレクトリ |
| **有効な値** | 絶対パス |
| **オーバーライド** | グローバル設定をオーバーライド |

#### 設定例

#### 例1: グループ固有の作業ディレクトリ

```toml
version = "1.0"

[global]
workdir = "/tmp"

[[groups]]
name = "log_analysis"
workdir = "/var/log"  # このグループは /var/log で実行

[[groups.commands]]
name = "grep_errors"
cmd = "grep"
args = ["ERROR", "app.log"]
# /var/log/app.log から検索

[[groups]]
name = "backup"
workdir = "/var/backups"  # このグループは /var/backups で実行

[[groups.commands]]
name = "create_backup"
cmd = "tar"
args = ["-czf", "backup.tar.gz", "/etc"]
# /var/backups/backup.tar.gz を作成
```

#### 例2: temp_dir との関係

`temp_dir = true` が指定されている場合、`workdir` は無視され、自動生成された一時ディレクトリが使用されます。

```toml
[[groups]]
name = "temp_work"
workdir = "/var/data"  # この設定は無視される
temp_dir = true        # 一時ディレクトリが優先
```

## 5.3 セキュリティ設定

### 5.3.1 verify_files - ファイル検証(グループレベル)

#### 概要

グループ固有のファイル検証リストを指定します。グローバルレベルの `verify_files` に追加されます(マージ)。

#### 文法

```toml
[[groups]]
name = "example"
verify_files = ["ファイルパス1", "ファイルパス2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | [] |
| **有効な値** | 絶対パスのリスト |
| **マージ動作** | グローバル設定とマージされる |

#### 設定例

```toml
version = "1.0"

[global]
verify_files = ["/bin/sh"]  # 全グループで検証

[[groups]]
name = "database_tasks"
verify_files = [
    "/usr/bin/psql",
    "/usr/bin/pg_dump",
]  # このグループでは /bin/sh, /usr/bin/psql, /usr/bin/pg_dump を検証

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]

[[groups]]
name = "web_tasks"
verify_files = [
    "/usr/bin/curl",
    "/usr/bin/wget",
]  # このグループでは /bin/sh, /usr/bin/curl, /usr/bin/wget を検証

[[groups.commands]]
name = "fetch_data"
cmd = "/usr/bin/curl"
args = ["https://example.com/data"]
```

### 5.3.2 env_allowlist - 環境変数許可リスト(グループレベル)

#### 概要

グループレベルで環境変数の許可リストを指定します。3つの継承モードがあります。

#### 文法

```toml
[[groups]]
name = "example"
env_allowlist = ["変数1", "変数2", ...]
```

#### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列配列 (array of strings) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | なし(継承モード) |
| **有効な値** | 環境変数名のリスト、または空配列 |
| **継承動作** | 3つのモード(後述) |

## 5.4 環境変数継承モード

環境変数の許可リスト(`env_allowlist`)には、3つの継承モードがあります。これは go-safe-cmd-runner の重要な機能の一つです。

### 5.4.1 継承モード (inherit)

#### 動作

グループレベルで `env_allowlist` を**指定しない**場合、グローバルの設定を継承します。

#### 使用シーン

- グローバル設定で十分な場合
- 複数のグループで同じ環境変数を使用する場合

#### 設定例

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "inherit_group"
# env_allowlist を指定しない → グローバルを継承

[[groups.commands]]
name = "show_env"
cmd = "printenv"
args = []
# PATH, HOME, USER が利用可能
```

### 5.4.2 明示モード (explicit)

#### 動作

グループレベルで `env_allowlist` に**具体的な値**を指定した場合、グローバル設定を無視し、指定された値のみを使用します。

#### 使用シーン

- グループ固有の環境変数セットが必要な場合
- グローバル設定とは異なる制限を設けたい場合

#### 設定例

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "explicit_group"
env_allowlist = ["PATH", "DATABASE_URL", "API_KEY"]  # グローバルを無視

[[groups.commands]]
name = "run_app"
cmd = "/opt/app/bin/app"
args = []
env = [
    "DATABASE_URL=postgresql://localhost/mydb",
    "API_KEY=secret123",
]
# PATH, DATABASE_URL, API_KEY のみが利用可能
# HOME, USER は利用不可
```

### 5.4.3 拒否モード (reject)

#### 動作

グループレベルで `env_allowlist = []` と**空の配列**を明示的に指定した場合、全ての環境変数を拒否します。

#### 使用シーン

- 完全に隔離された環境でコマンドを実行したい場合
- セキュリティ要件が非常に高い場合

#### 設定例

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "reject_group"
env_allowlist = []  # 全ての環境変数を拒否

[[groups.commands]]
name = "isolated_command"
cmd = "/bin/echo"
args = ["完全に隔離された実行"]
# 環境変数なしで実行される
```

### 5.4.4 継承モードの判定ルール

モードの判定は以下のロジックで行われます:

```mermaid
flowchart TD
    A["env_allowlist の確認"] --> B{"グループレベルで<br/>env_allowlist が<br/>定義されているか?"}
    B -->|No| C["継承モード<br/>inherit"]
    B -->|Yes| D{"値は空配列<br/>[] か?"}
    D -->|Yes| E["拒否モード<br/>reject"]
    D -->|No| F["明示モード<br/>explicit"]

    C --> G["グローバルの<br/>env_allowlist を使用"]
    E --> H["全ての環境変数を拒否"]
    F --> I["グループの<br/>env_allowlist を使用"]

    style C fill:#e8f5e9
    style E fill:#ffebee
    style F fill:#e3f2fd
```

#### 例: 3つのモードの比較

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "USER"]

# モード1: 継承モード
[[groups]]
name = "group_inherit"
# env_allowlist 未指定
# 結果: PATH, HOME, USER が利用可能

[[groups.commands]]
name = "test1"
cmd = "printenv"
args = ["HOME"]  # HOME が出力される

# モード2: 明示モード
[[groups]]
name = "group_explicit"
env_allowlist = ["PATH", "CUSTOM_VAR"]
# 結果: PATH, CUSTOM_VAR のみが利用可能(HOME, USER は不可)

[[groups.commands]]
name = "test2"
cmd = "printenv"
args = ["HOME"]  # エラー: HOME は許可されていない

[[groups.commands]]
name = "test3"
cmd = "printenv"
args = ["CUSTOM_VAR"]
env = ["CUSTOM_VAR=value"]  # CUSTOM_VAR が出力される

# モード3: 拒否モード
[[groups]]
name = "group_reject"
env_allowlist = []
# 結果: 全ての環境変数が拒否される

[[groups.commands]]
name = "test4"
cmd = "printenv"
args = ["PATH"]  # エラー: PATH も許可されていない
```

### 実践的な使用例

#### 例1: セキュリティレベルに応じた設定

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME", "USER"]

# 通常のタスク: グローバルを継承
[[groups]]
name = "normal_tasks"
# env_allowlist 未指定 → 継承モード

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []

# 機密データ処理: 最小限の環境変数
[[groups]]
name = "sensitive_data"
env_allowlist = ["PATH"]  # PATH のみ許可 → 明示モード

[[groups.commands]]
name = "process_sensitive"
cmd = "/opt/secure/process"
args = []

# 完全隔離タスク: 環境変数なし
[[groups]]
name = "isolated_tasks"
env_allowlist = []  # 全て拒否 → 拒否モード

[[groups.commands]]
name = "isolated_check"
cmd = "/bin/echo"
args = ["完全隔離"]
```

#### 例2: 環境ごとの設定

```toml
version = "1.0"

[global]
env_allowlist = ["PATH", "HOME"]

# 開発環境グループ
[[groups]]
name = "development"
env_allowlist = [
    "PATH",
    "HOME",
    "DEBUG_MODE",
    "DEV_DATABASE_URL",
]  # 明示モード: 開発用変数を追加

[[groups.commands]]
name = "dev_server"
cmd = "/opt/app/server"
args = []
env = ["DEBUG_MODE=true", "DEV_DATABASE_URL=postgresql://localhost/dev"]

# 本番環境グループ
[[groups]]
name = "production"
env_allowlist = [
    "PATH",
    "PROD_DATABASE_URL",
]  # 明示モード: 本番用変数のみ

[[groups.commands]]
name = "prod_server"
cmd = "/opt/app/server"
args = []
env = ["PROD_DATABASE_URL=postgresql://prod-server/prod"]
```

## グループ設定の全体例

以下は、グループレベルの設定を組み合わせた実践的な例です:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/tmp"
env_allowlist = ["PATH", "HOME", "USER"]
verify_files = ["/bin/sh"]

# グループ1: データベースバックアップ
[[groups]]
name = "database_backup"
description = "PostgreSQL データベースの日次バックアップ"
priority = 10
workdir = "/var/backups/db"
verify_files = ["/usr/bin/pg_dump", "/usr/bin/psql"]
env_allowlist = ["PATH", "PGDATA", "PGHOST"]

[[groups.commands]]
name = "backup_main_db"
description = "メインデータベースのバックアップ"
cmd = "/usr/bin/pg_dump"
args = ["-U", "postgres", "maindb"]
output = "maindb_backup.sql"
timeout = 600

# グループ2: ログローテーション
[[groups]]
name = "log_rotation"
description = "古いログファイルの圧縮と削除"
priority = 20
workdir = "/var/log/app"
env_allowlist = ["PATH"]  # 明示モード: PATH のみ

[[groups.commands]]
name = "compress_old_logs"
cmd = "gzip"
args = ["app.log.1"]

[[groups.commands]]
name = "delete_ancient_logs"
cmd = "find"
args = [".", "-name", "*.log.gz", "-mtime", "+30", "-delete"]

# グループ3: 一時ファイル処理
[[groups]]
name = "temp_processing"
description = "一時ディレクトリでのデータ処理"
priority = 30
temp_dir = true   # 一時ディレクトリを自動作成
env_allowlist = []  # 拒否モード: 環境変数なし

[[groups.commands]]
name = "create_temp_data"
cmd = "echo"
args = ["Temporary data"]
output = "temp_data.txt"

[[groups.commands]]
name = "process_temp_data"
cmd = "cat"
args = ["temp_data.txt"]
```

## 次のステップ

次章では、コマンドレベルの設定(`[[groups.commands]]`)について詳しく解説します。実際に実行するコマンドの詳細な設定方法を学びます。
