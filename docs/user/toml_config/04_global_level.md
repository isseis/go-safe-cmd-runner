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
| **型** | 整数 (int) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、コマンド |
| **デフォルト値** | システムデフォルト(通常は制限なし) |
| **有効な値** | 正の整数(秒単位) |
| **オーバーライド** | コマンドレベルでオーバーライド可能 |

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

### 動作の詳細

タイムアウトが発生すると:
1. 実行中のコマンドに終了シグナル(SIGTERM)を送信
2. 一定時間待機後、強制終了シグナル(SIGKILL)を送信
3. エラーとして記録し、次のコマンドに進む

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

#### 2. 0 や負の値は無効

```toml
[global]
timeout = 0   # 無効な設定
timeout = -1  # 無効な設定
```

## 4.2 workdir - 作業ディレクトリ

### 概要

コマンドを実行する作業ディレクトリ(カレントディレクトリ)を指定します。

### 文法

```toml
[global]
workdir = "ディレクトリパス"
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバル、グループ |
| **デフォルト値** | go-safe-cmd-runner の実行ディレクトリ |
| **有効な値** | 絶対パス |
| **オーバーライド** | グループレベルでオーバーライド可能 |

### 役割

- **実行環境の統一**: 全てのコマンドが同じディレクトリで実行されることを保証
- **相対パス参照の基準**: 相対パスを使用するコマンドの基準ディレクトリを設定
- **セキュリティ**: 予期しないディレクトリでのコマンド実行を防止

### 設定例

#### 例1: グローバル作業ディレクトリの設定

```toml
version = "1.0"

[global]
workdir = "/var/app/workspace"

[[groups]]
name = "file_operations"

[[groups.commands]]
name = "create_file"
cmd = "touch"
args = ["test.txt"]
# /var/app/workspace/test.txt が作成される
```

#### 例2: グループレベルでのオーバーライド

```toml
version = "1.0"

[global]
workdir = "/tmp"

[[groups]]
name = "log_processing"
workdir = "/var/log/app"  # グループ専用の作業ディレクトリ

[[groups.commands]]
name = "grep_errors"
cmd = "grep"
args = ["ERROR", "app.log"]
# /var/log/app ディレクトリで実行される
```

### 注意事項

#### 1. 絶対パスを使用

相対パスは使用できません:

```toml
[global]
workdir = "./workspace"  # エラー: 相対パスは使用不可
workdir = "/tmp/workspace"  # 正しい: 絶対パス
```

#### 2. ディレクトリの存在確認

指定されたディレクトリが存在しない場合、エラーになります:

```toml
[global]
workdir = "/nonexistent/directory"  # ディレクトリが存在しない場合エラー
```

#### 3. 権限の確認

指定したディレクトリに対する読み書き権限が必要です。

## 4.3 log_level - ログレベル

### 概要

ログ出力の詳細度を制御します。

### 文法

```toml
[global]
log_level = "ログレベル"
```

### パラメータの詳細

| 項目 | 内容 |
|-----|------|
| **型** | 文字列 (string) |
| **必須/オプション** | オプション |
| **設定可能な階層** | グローバルのみ |
| **デフォルト値** | "info" |
| **有効な値** | "debug", "info", "warn", "error" |
| **オーバーライド** | 不可(グローバルレベルのみ) |

### ログレベルの詳細

| レベル | 用途 | 出力される情報 |
|--------|------|--------------|
| **debug** | 開発・デバッグ | 全ての詳細情報(変数値、内部状態など) |
| **info** | 通常運用 | 実行状況、完了通知など |
| **warn** | 警告の監視 | 警告と重要な情報のみ |
| **error** | エラーのみ | エラーメッセージのみ |

### 設定例

#### 例1: デバッグモード

```toml
version = "1.0"

[global]
log_level = "debug"  # 詳細なデバッグ情報を出力

[[groups]]
name = "troubleshooting"

[[groups.commands]]
name = "test_command"
cmd = "echo"
args = ["test"]
```

出力例:
```
[DEBUG] Configuration loaded: version=1.0
[DEBUG] Global settings: timeout=default, workdir=default
[DEBUG] Processing group: troubleshooting
[DEBUG] Executing command: test_command
[DEBUG] Command path: /usr/bin/echo
[DEBUG] Arguments: [test]
[INFO] Command completed successfully
```

#### 例2: 本番環境(info レベル)

```toml
version = "1.0"

[global]
log_level = "info"  # 標準的な情報のみ出力

[[groups]]
name = "production"

[[groups.commands]]
name = "backup"
cmd = "/usr/bin/backup.sh"
args = []
```

出力例:
```
[INFO] Starting command group: production
[INFO] Executing command: backup
[INFO] Command completed successfully
```

#### 例3: エラーのみ(error レベル)

```toml
version = "1.0"

[global]
log_level = "error"  # エラーのみ出力

[[groups]]
name = "silent_operation"

[[groups.commands]]
name = "routine_check"
cmd = "test"
args = ["-f", "/tmp/check.txt"]
```

正常時は何も出力されず、エラー時のみメッセージが表示されます。

### ベストプラクティス

- **開発時**: `debug` レベルを使用して詳細を確認
- **テスト時**: `info` レベルで実行状況を確認
- **本番環境**: `info` または `warn` レベルを使用
- **静かな運用**: `error` レベルでエラーのみを記録

## 4.4 skip_standard_paths - 標準パス検証のスキップ

### 概要

標準的なシステムパス(`/bin`, `/usr/bin` など)に対するファイル検証をスキップします。

### 文法

```toml
[global]
skip_standard_paths = true/false
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
skip_standard_paths = true  # /bin, /usr/bin などの検証をスキップ

[[groups]]
name = "system_commands"

[[groups.commands]]
name = "list_files"
cmd = "/bin/ls"  # 検証なしで実行可能
args = ["-la"]
```

#### 例2: 全てのコマンドを検証(デフォルト)

```toml
version = "1.0"

[global]
skip_standard_paths = false  # または省略
verify_files = ["/bin/ls", "/usr/bin/grep"]  # 明示的にハッシュ指定が必要

[[groups]]
name = "verified_commands"

[[groups.commands]]
name = "search"
cmd = "/usr/bin/grep"
args = ["pattern", "file.txt"]
```

### セキュリティ上の注意

`skip_standard_paths = true` を設定すると、標準パスのコマンドが改ざんされていても検出できません。セキュリティ要件が高い環境では `false` (デフォルト)のままにすることを推奨します。

## 4.5 env_allowlist - 環境変数許可リスト

### 概要

コマンド実行時に使用を許可する環境変数を指定します。リストにない環境変数は全て除外されます。

### 文法

```toml
[global]
env_allowlist = ["変数1", "変数2", ...]
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
env_allowlist = [
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
env_allowlist = [
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
env = ["APP_CONFIG_DIR=/etc/myapp"]
```

#### 例3: 空のリスト(全て拒否)

```toml
version = "1.0"

[global]
env_allowlist = []  # 全ての環境変数を拒否

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
env_allowlist = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... 多すぎる
]

# 推奨: 必要最小限
[global]
env_allowlist = ["PATH", "HOME", "USER"]
```

## 4.6 verify_files - ファイル検証リスト

### 概要

実行前に整合性を検証するファイルのリストを指定します。指定されたファイルはハッシュ値と照合され、改ざんが検出されると実行が中止されます。

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

#### 例1: 基本的なファイル検証

```toml
version = "1.0"

[global]
verify_files = [
    "/bin/sh",
    "/bin/bash",
    "/usr/bin/python3",
]

[[groups]]
name = "scripts"

[[groups.commands]]
name = "run_script"
cmd = "/usr/bin/python3"
args = ["script.py"]
# 実行前に /usr/bin/python3 のハッシュを検証
```

#### 例2: グループレベルでの追加

```toml
version = "1.0"

[global]
verify_files = ["/bin/sh"]  # 全グループで検証

[[groups]]
name = "database_group"
verify_files = ["/usr/bin/psql", "/usr/bin/pg_dump"]  # グループ固有の検証

[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
# /bin/sh, /usr/bin/psql, /usr/bin/pg_dump が検証される(マージ)
```

### 検証の仕組み

1. **ハッシュファイルの事前作成**: `record` コマンドでファイルのハッシュを記録
2. **実行時の検証**: 設定ファイルに指定されたファイルのハッシュを照合
3. **不一致時の動作**: ハッシュが一致しない場合、実行を中止しエラーを報告

### ハッシュファイルの作成方法

```bash
# record コマンドで検証対象ファイルのハッシュを記録
$ go-safe-cmd-runner record config.toml

# または個別にファイルを指定
$ go-safe-cmd-runner record /bin/sh /usr/bin/python3
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

#### 3. パフォーマンスへの影響

多数のファイルを検証すると起動時間が増加します。必要なファイルのみを指定してください。

## 4.7 max_output_size - 出力サイズ上限

### 概要

コマンドの標準出力をキャプチャする際の最大サイズをバイト単位で指定します。

### 文法

```toml
[global]
max_output_size = バイト数
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
max_output_size = 1048576  # 1MB = 1024 * 1024 バイト

[[groups]]
name = "log_analysis"

[[groups.commands]]
name = "grep_logs"
cmd = "grep"
args = ["ERROR", "/var/log/app.log"]
output = "errors.txt"
# 出力が 1MB を超えるとエラー
```

#### 例2: 大きなファイルの処理

```toml
version = "1.0"

[global]
max_output_size = 104857600  # 100MB = 100 * 1024 * 1024 バイト

[[groups]]
name = "data_export"

[[groups.commands]]
name = "export_database"
cmd = "/usr/bin/pg_dump"
args = ["large_db"]
output = "database_dump.sql"
# 大きなデータベースダンプを許可
```

#### 例3: サイズ制限の目安

```toml
[global]
# 一般的な用途に応じた推奨値
max_output_size = 1048576      # 1MB  - ログ分析、小規模データ
max_output_size = 10485760     # 10MB - デフォルト、中規模データ
max_output_size = 104857600    # 100MB - 大規模データ、データベースダンプ
max_output_size = 1073741824   # 1GB  - 非常に大きなデータ(注意が必要)
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
max_output_size = 1024  # 1KB - ほとんどのコマンドで不足

# 推奨: 適切な制限
[global]
max_output_size = 10485760  # 10MB - 一般的な用途に適切
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
skip_standard_paths = true

# 環境変数許可リスト
env_allowlist = [
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
max_output_size = 10485760  # 10MB

[[groups]]
name = "application_tasks"
# ... グループ設定が続く
```

## 次のステップ

次章では、グループレベルの設定(`[[groups]]`)について詳しく解説します。コマンドをグループ化し、グループ固有の設定を行う方法を学びます。
