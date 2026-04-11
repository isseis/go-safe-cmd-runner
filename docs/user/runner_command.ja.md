# runner コマンド ユーザーガイド

go-safe-cmd-runner のメイン実行コマンド `runner` の使用方法を解説します。

## 目次

- [1. 概要](#1-概要)
- [2. クイックスタート](#2-クイックスタート)
- [3. コマンドラインフラグ詳解](#3-コマンドラインフラグ詳解)
- [4. 環境変数](#4-環境変数)
- [5. 実践例](#5-実践例)
- [6. トラブルシューティング](#6-トラブルシューティング)
- [7. 関連ドキュメント](#7-関連ドキュメント)

## 1. 概要

### 1.1 runner コマンドとは

`runner` は go-safe-cmd-runner のメインコマンドで、TOML設定ファイルに基づいてコマンドを安全に実行します。

### 1.2 主な用途

- **セキュアなバッチ処理**: 複数のコマンドをグループ化して順次実行
- **権限委譲**: 一般ユーザーに特定の管理タスクを安全に委譲
- **自動化タスク**: バックアップ、デプロイ、システムメンテナンスの自動化
- **監査とロギング**: 実行履歴の記録と追跡

### 1.3 基本的な使用フロー

```
1. TOML設定ファイルを作成
   ↓
2. ハッシュ値を記録（record コマンド）
   - TOML設定ファイル自体のハッシュ（必須）
   - 実行バイナリのハッシュ
   - verify_files で指定したファイルのハッシュ
   ↓
3. ドライランで動作確認（-dry-run フラグ）
   ↓
4. 本番実行（runner コマンド）
```

## 2. クイックスタート

### 2.1 最小構成での実行

```bash
# 1. 設定ファイルを作成（config.toml）
cat > config.toml << 'EOF'
version = "1.0"
verify_standard_paths = false

[[groups]]
name = "hello"

[[groups.commands]]
name = "greet"
cmd = "/bin/echo"
args = ["Hello, World!"]
EOF

# 2. 実行
runner -config config.toml
```

### 2.2 事前準備：ハッシュファイルの作成

**重要**: runner コマンドは、TOML設定ファイルと実行バイナリの両方についてハッシュ検証を行います。これにより、設定ファイルや実行ファイルの改ざんを防ぎ、TOCTOU攻撃（Time-of-check to time-of-use）から保護します。

実行前に、以下のファイルのハッシュ値を記録する必要があります：

1. **TOML設定ファイル自体** （必須）
2. 設定ファイル内で指定された実行バイナリ
3. `verify_files` で指定されたファイル

```bash
# 1. TOML設定ファイルのハッシュを記録（最も重要）
# 2. 実行バイナリのハッシュを記録
# 3. verify_files で指定したファイルのハッシュを記録（環境設定ファイルなど）
record -d /usr/local/etc/go-safe-cmd-runner/hashes \
    config.toml \
    /usr/local/bin/backup.sh \
    /etc/myapp/database.conf
```

詳細は [record コマンドガイド](record_command.ja.md) を参照してください。

### 2.3 設定ファイルについて

TOML設定ファイルの詳細な記述方法については、以下のドキュメントを参照してください：

- [TOML設定ファイル ユーザーガイド](toml_config/README.ja.md)

## 3. コマンドラインフラグ詳解

### 3.1 必須フラグ

#### `-config <path>` / `-c <path>`

**概要**

TOML形式の設定ファイルのパスを指定します。

**文法**

```bash
runner -config <path>
runner -c <path>
```

**パラメータ**

- `<path>`: 設定ファイルへの絶対パスまたは相対パス（必須）

**使用例**

```bash
# 相対パスで指定
runner -config config.toml
runner -c config.toml

# 絶対パスで指定
runner -config /etc/go-safe-cmd-runner/production.toml

# ホームディレクトリからの指定
runner -config ~/configs/backup.toml
```

**注意事項**

- **設定ファイルは事前にハッシュ値を記録しておく必要があります（TOML設定ファイル自体も検証対象です）**
- ファイルが存在しない場合はエラーになります
- 設定ファイルの検証に失敗した場合、実行は中断されます
- TOML設定ファイルの読み取りと検証はアトミックに実行され、TOCTOU攻撃を防ぎます

### 3.2 実行モード制御

#### `-dry-run` / `-n`

**概要**

コマンドを実際には実行せず、実行内容をシミュレーションして表示します。

**文法**

```bash
runner -config <path> -dry-run
runner -c <path> -n
```

**使用例**

```bash
# 基本的なドライラン
runner -config config.toml -dry-run
runner -c config.toml -n

# 詳細レベルとフォーマットを指定
runner -config config.toml -dry-run -dry-run-detail full -dry-run-format json
```

**ユースケース**

- **設定変更後の確認**: 設定ファイルを変更した後、意図通りに動作するか確認
- **影響範囲の把握**: どのコマンドが実行されるか事前に確認
- **セキュリティチェック**: リスク評価結果を確認
- **デバッグ**: 変数展開や環境変数の状態を確認

**ドライランの特徴**

- ファイル検証は実行されます（ハッシュ値のチェック）
- 実際のコマンドは実行されません
- 環境変数の展開結果を確認できます
- リスク評価結果が表示されます

#### `-dry-run-format <format>`

**概要**

ドライラン実行時の出力フォーマットを指定します。

**文法**

```bash
runner -config <path> -dry-run -dry-run-format <format>
```

**選択肢**

- `text`: 人間が読みやすいテキスト形式（デフォルト）
- `json`: 機械処理しやすいJSON形式

**使用例**

**テキスト形式（デフォルト）**

```bash
runner -config config.toml -dry-run -dry-run-format text
```

出力例：
```
=== Dry Run Analysis ===

Group: backup (Priority: 1)
  Description: Database backup operations

  Command: db_backup
    Description: Backup PostgreSQL database
    Command Path: /usr/bin/pg_dump
    Arguments: ["-U", "postgres", "mydb"]
    Working Directory: /var/backups
    Timeout: 3600s
    Risk Level: medium
    Environment Variables:
      PATH=/sbin:/usr/sbin:/bin:/usr/bin
      HOME=/root
```

**JSON形式**

```bash
runner -config config.toml -dry-run -dry-run-format json
```

出力例：
```json
{
  "groups": [
    {
      "name": "backup",
      "description": "Database backup operations",
      "commands": [
        {
          "name": "db_backup",
          "description": "Backup PostgreSQL database",
          "cmd": "/usr/bin/pg_dump",
          "args": ["-U", "postgres", "mydb"],
          "workdir": "/var/backups",
          "timeout": 3600,
          "risk_level": "medium",
          "env": {
            "PATH": "/sbin:/usr/sbin:/bin:/usr/bin",
            "HOME": "/root"
          }
        }
      ]
    }
  ]
}
```

**JSON形式の活用**

```bash
# jqでフィルタリング
runner -config config.toml -dry-run -dry-run-format json | jq '.groups[0].commands[0].cmd'

# ファイルに保存して解析
runner -config config.toml -dry-run -dry-run-format json > dryrun.json
```

#### `-dry-run-detail <level>`

**概要**

ドライラン実行時の出力の詳細レベルを指定します。

**文法**

```bash
runner -config <path> -dry-run -dry-run-detail <level>
```

**選択肢**

- `summary`: サマリー情報のみ表示
- `detailed`: 詳細情報を表示（デフォルト）
- `full`: 全情報を表示（環境変数、検証ファイルなど全て）

**使用例と出力例**

**summary レベル**

```bash
runner -config config.toml -dry-run -dry-run-detail summary
```

出力例：
```
=== Dry Run Summary ===
Total Groups: 2
Total Commands: 5
Estimated Duration: ~180s
```

**detailed レベル（デフォルト）**

```bash
runner -config config.toml -dry-run -dry-run-detail detailed
```

出力例：
```
=== Dry Run Analysis ===

Group: backup (Priority: 1)
  Commands: 2

  Command: db_backup
    Path: /usr/bin/pg_dump
    Args: ["-U", "postgres", "mydb"]
    Risk: medium
```

**full レベル**

```bash
runner -config config.toml -dry-run -dry-run-detail full
```

出力例：
```
=== Dry Run Analysis (Full Detail) ===

Group: backup (Priority: 1)
  Description: Database backup operations
  Working Directory: /var/backups
  Temp Directory: /tmp/runner-backup
  Environment Variables:
    PATH=/sbin:/usr/sbin:/bin:/usr/bin
    HOME=/root
  Verified Files:
    /usr/bin/pg_dump (SHA256: abc123...)

  Command: db_backup
    Description: Backup PostgreSQL database
    Command Path: /usr/bin/pg_dump
    Arguments: ["-U", "postgres", "mydb"]
    Working Directory: /var/backups
    Timeout: 3600s
    Risk Level: medium
    Risk Factors:
      - Database operation
      - Requires elevated privileges
    Run As User: postgres
    Run As Group: postgres
    Environment Variables:
      PATH=/sbin:/usr/sbin:/bin:/usr/bin
      HOME=/root
      PGPASSWORD=[REDACTED]

===== Final Process Environment =====

Environment variables (5):
  BACKUP_DIR=/var/backups
    (from Global)
  HOME=/root
    (from System (filtered by allowlist))
  PATH=/sbin:/usr/sbin:/bin:/usr/bin
    (from System (filtered by allowlist))
  PGPASSWORD=[REDACTED]
    (from Command[db_backup])
  TEMP_DIR=/tmp/runner-backup
    (from Group[backup])
```

**センシティブ情報の表示**

デフォルトでは、パスワードやトークンなどのセンシティブ情報は `[REDACTED]` でマスクされます。デバッグ時に平文で表示する必要がある場合は、`--show-sensitive` フラグを使用します。

```bash
runner -config config.toml -dry-run -dry-run-detail full --show-sensitive
```

出力例（センシティブ情報を表示）：
```
===== Final Process Environment =====

Environment variables (5):
  BACKUP_DIR=/var/backups
    (from Global)
  HOME=/root
    (from System (filtered by allowlist))
  PATH=/sbin:/usr/sbin:/bin:/usr/bin
    (from System (filtered by allowlist))
  PGPASSWORD=super_secret_password_123
    (from Command[db_backup])
  TEMP_DIR=/tmp/runner-backup
    (from Group[backup])
```

**注意事項**:
- `--show-sensitive` はデバッグ用途のみで使用してください
- 本番環境やログファイルに出力する場合は使用しないでください
- CI/CD環境では機密情報の漏洩リスクがあるため、デフォルトのマスク動作を推奨します

**詳細レベルの使い分け**

- `summary`: CI/CDでの概要確認、大量の設定の一覧表示
- `detailed`: 通常の確認作業、設定変更後のチェック
- `full`: デバッグ、トラブルシューティング、環境変数の確認

#### JSON形式でのデバッグ情報出力

JSON形式(`-dry-run-format json`)を指定した場合、詳細レベルに応じてデバッグ情報が `debug_info` フィールドに含まれます。

**DetailLevelSummary**

`debug_info` フィールドは含まれません。

**DetailLevelDetailed**

グループレベルとコマンドレベルのデバッグ情報が含まれます：

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR"],
          "inheritance_mode": "inherit"
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

**DetailLevelFull**

すべてのデバッグ情報が含まれます。`from_env_inheritance` には差分情報（継承された変数、削除された変数、利用不可能な変数）が追加されます：

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH", "USER"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR", "TEMP_DIR"],
          "inheritance_mode": "inherit",
          "inherited_variables": ["HOME", "PATH"],
          "removed_allowlist_variables": ["USER"],
          "unavailable_env_import_variables": []
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "workdir": "/var/backups",
      "timeout": 3600,
      "risk_level": "medium",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "DB_PASSWORD",
              "value": "[REDACTED]",
              "source": "Command[db_backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            },
            {
              "name": "PATH",
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

**センシティブ情報のマスキング**

デフォルトでは、パスワードやトークンなどのセンシティブ情報は `[REDACTED]` でマスクされます。デバッグ時に平文で表示する必要がある場合は、`--show-sensitive` フラグを使用します：

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full --show-sensitive
```

**JSON出力の活用例**

```bash
# デバッグ情報のみを抽出
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'

# 環境変数の継承モードを確認
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail detailed | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance != null) | .debug_info.from_env_inheritance.inheritance_mode'

# 最終的な環境変数を確認
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | .debug_info.final_environment.variables'
```

#### `-show-sensitive`

**概要**

ドライラン実行時に、センシティブな環境変数の値をマスクせずに平文で表示します。デフォルトでは、パスワードやトークンなどのセンシティブ情報は `[REDACTED]` として表示されます。

**セキュリティ警告**: このフラグは、デバッグやトラブルシューティング時にのみ使用してください。本番環境や共有環境では使用しないでください。ログファイルやCI/CD環境への機密情報漏洩のリスクがあります。

**文法**

```bash
runner -config <path> -dry-run -dry-run-detail full -show-sensitive
```

**使用例**

**デフォルト動作（センシティブ情報はマスク）**

```bash
runner -config config.toml -dry-run -dry-run-detail full
```

出力例：
```
===== Final Process Environment =====

Environment variables (5):
  DB_HOST=localhost
    (from Global)
  DB_USER=appuser
    (from Global)
  DB_PASSWORD=[REDACTED]
    (from Global)
  API_TOKEN=[REDACTED]
    (from Command[deploy])
  LOG_LEVEL=info
    (from Command[deploy])
```

**センシティブ情報を表示（`-show-sensitive` 使用時）**

```bash
runner -config config.toml -dry-run -dry-run-detail full -show-sensitive
```

出力例：
```
===== Final Process Environment =====

Environment variables (5):
  DB_HOST=localhost
    (from Global)
  DB_USER=appuser
    (from Global)
  DB_PASSWORD=MySecretPassword123
    (from Global)
  API_TOKEN=sk-1234567890abcdef
    (from Command[deploy])
  LOG_LEVEL=info
    (from Command[deploy])
```

**センシティブ環境変数の判定基準**

以下のパターンに一致する環境変数名は、センシティブ情報として扱われます：

- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`

例：`DB_PASSWORD`, `API_SECRET_KEY`, `GITHUB_TOKEN`, `AWS_SECRET_ACCESS_KEY`, `OAUTH_CREDENTIAL`, `AUTH_TOKEN`

**ユースケース**

- **ローカル開発環境でのデバッグ**: 環境変数の展開が正しく行われているか確認
- **トラブルシューティング**: 環境変数の値が期待通りに設定されているか確認
- **設定ファイルの初期検証**: 新しい設定ファイルを作成した際の動作確認

**使用上の注意事項**

1. **本番環境では使用しないでください**: 機密情報がログに記録される可能性があります
2. **CI/CD環境では使用しないでください**: ビルドログに機密情報が残る可能性があります
3. **Slack通知が有効な場合は特に注意**: センシティブ情報が通知メッセージに含まれる可能性があります
4. **ログファイルの取り扱いに注意**: `-show-sensitive` を使用した実行ログは、適切に保護してください

**推奨される使用方法**

```bash
# ローカル環境での一時的なデバッグ
runner -config config.toml -dry-run -dry-run-detail full -show-sensitive

# 実行後、ターミナルの履歴をクリア（bash）
history -c

# または、出力をファイルに保存し、確認後に削除
runner -config config.toml -dry-run -dry-run-detail full -show-sensitive > debug.txt
# 確認後
shred -u debug.txt  # secure deletion
```

### 3.3 ログ設定

#### `-log-level <level>` / `-l <level>`

**概要**

ログ出力のレベルを指定します。指定したレベル以上のログが出力されます。

**文法**

```bash
runner -config <path> -log-level <level>
runner -c <path> -l <level>
```

**選択肢**

- `debug`: デバッグ情報を含む全てのログ
- `info`: 通常の情報ログ以上（デフォルト）
- `warn`: 警告以上のログのみ
- `error`: エラーログのみ

**使用例**

```bash
# デバッグモードで実行
runner -config config.toml -log-level debug
runner -c config.toml -l debug

# 警告とエラーのみ表示
runner -config config.toml -log-level warn

# エラーのみ表示
runner -config config.toml -log-level error
```

**各レベルで出力される情報**

**debug レベル**
```
2025-10-02T10:30:00Z DEBUG Loading configuration file path=/etc/runner/config.toml
2025-10-02T10:30:00Z DEBUG Verifying file hash file=/usr/bin/backup.sh hash=abc123...
2025-10-02T10:30:00Z DEBUG Environment variable filtered out var=SHELL reason=not_in_allowlist
2025-10-02T10:30:00Z INFO  Starting command group=backup command=db_backup
2025-10-02T10:30:05Z INFO  Command completed successfully group=backup command=db_backup duration=5.2s
```

**info レベル（デフォルト）**
```
2025-10-02T10:30:00Z INFO  Starting command group=backup command=db_backup
2025-10-02T10:30:05Z INFO  Command completed successfully group=backup command=db_backup duration=5.2s
```

**warn レベル**
```
2025-10-02T10:30:10Z WARN  Command execution slow group=backup command=full_backup duration=125s timeout=120s
```

**error レベル**
```
2025-10-02T10:30:15Z ERROR Command failed group=backup command=db_backup error="exit status 1"
```

**ログレベルの使い分け**

- `debug`: 開発時、トラブルシューティング時
- `info`: 通常運用時（デフォルト）
- `warn`: 本番環境で問題の兆候のみ記録
- `error`: 監視システムと連携してエラーのみ記録

**注意事項**

- センシティブな情報は自動的にマスクされます（パスワード、トークンなど）

#### `-log-dir <directory>`

**概要**

実行ログを保存するディレクトリを指定します。各実行ごとにULID付きのJSONログファイルが作成されます。

**文法**

```bash
runner -config <path> -log-dir <directory>
```

**パラメータ**

- `<directory>`: ログファイルを保存するディレクトリパス（絶対パスまたは相対パス）

**使用例**

```bash
# ログディレクトリを指定して実行
runner -config config.toml -log-dir /var/log/go-safe-cmd-runner

# 相対パスで指定
runner -config config.toml -log-dir ./logs
```

**ログファイルの命名規則**

```
<log-dir>/runner-<run-id>.json
```

例：
```
/var/log/go-safe-cmd-runner/runner-01K2YK812JA735M4TWZ6BK0JH9.json
```

**ログファイルの内容（JSON形式）**

```json
{
  "timestamp": "2025-10-02T10:30:00Z",
  "level": "INFO",
  "message": "Command completed successfully",
  "run_id": "01K2YK812JA735M4TWZ6BK0JH9",
  "group": "backup",
  "command": "db_backup",
  "duration_ms": 5200,
  "exit_code": 0
}
```

**ユースケース**

- **監査ログの保存**: 全実行履歴を記録
- **トラブルシューティング**: 過去の実行ログを解析
- **統計分析**: 実行時間、エラー率などの分析
- **コンプライアンス**: 実行証跡の保存

**ログローテーション**

ログファイルは自動的にローテーションされません。定期的なクリーンアップが必要です。

```bash
# 30日以上前のログを削除
find /var/log/go-safe-cmd-runner -name "runner-*.json" -mtime +30 -delete
```

**注意事項**

- コマンドラインフラグは TOML設定や環境変数より優先されます
- ディレクトリが存在しない場合は自動的に作成されます
- ログファイルは 0600 権限で作成されます（所有者のみ読み書き可能）

#### `-run-id <id>`

**概要**

実行を識別するための一意なIDを明示的に指定します。指定しない場合はULIDが自動生成されます。

**文法**

```bash
runner -config <path> -run-id <id>
```

**パラメータ**

- `<id>`: 実行を識別する一意な文字列（推奨：ULID形式）

**使用例**

```bash
# カスタムRun IDを指定
runner -config config.toml -run-id my-custom-run-001

# ULID形式で指定
runner -config config.toml -run-id 01K2YK812JA735M4TWZ6BK0JH9

# 自動生成（デフォルト）
runner -config config.toml
```

**ULID形式について**

ULID (Universally Unique Lexicographically Sortable Identifier) は以下の特徴を持ちます：

- **時系列順**: 生成時刻順にソート可能
- **一意性**: 衝突の可能性が極めて低い
- **URL安全**: 特殊文字を含まない
- **固定長**: 26文字
- **例**: `01K2YK812JA735M4TWZ6BK0JH9`

**ユースケース**

- **外部システムとの連携**: CI/CDのビルドIDと紐付け
- **分散実行の追跡**: 複数サーバーでの実行を統一IDで管理
- **デバッグ**: 特定の実行を再現

**外部システム連携の例**

```bash
# GitHub ActionsのRun IDを使用
runner -config config.toml -run-id "gh-${GITHUB_RUN_ID}"

# Jenkinsのビルド番号を使用
runner -config config.toml -run-id "jenkins-${BUILD_NUMBER}"

# タイムスタンプベースのID
runner -config config.toml -run-id "backup-$(date +%Y%m%d-%H%M%S)"
```

**注意事項**

- Run IDはログファイル名やログエントリに含まれます
- 同じRun IDを複数回使用すると、ログファイルが上書きされる可能性があります
- ULID以外の形式も使用可能ですが、時系列順ソートができない場合があります

### 3.4 出力制御

#### `-interactive`

**概要**

インタラクティブモードを強制的に有効化します。カラー出力と進捗表示が有効になります。

**文法**

```bash
runner -config <path> -interactive
```

**使用例**

```bash
# インタラクティブモードで実行
runner -config config.toml -interactive

# パイプ経由でもカラー出力を有効化
runner -config config.toml -interactive | tee output.log
```

**インタラクティブモードの特徴**

- **カラー出力**: エラーは赤、警告は黄、成功は緑で表示
- **進捗表示**: コマンド実行中の状態を視覚的に表示
- **対話的な体験**: 人間が読みやすい形式で情報を表示

**出力例**

```
✓ Configuration loaded successfully
✓ File verification completed (5 files)

→ Starting group: backup [Priority: 1]
  ✓ db_backup completed (5.2s)
  ✓ file_backup completed (12.8s)

→ Starting group: cleanup [Priority: 2]
  ✓ old_logs_cleanup completed (2.1s)

✓ All commands completed successfully
  Total duration: 20.1s
```

**ユースケース**

- **対話的な実行**: コマンドラインから手動実行する場合
- **デバッグ**: 問題を視覚的に確認したい場合
- **デモ**: 実行状況をプレゼンテーションする場合
- **パイプ経由での確認**: `less -R` などでカラー出力を保持

**環境変数との関係**

`-interactive` フラグは環境変数より優先されます：

```bash
# NO_COLORが設定されていてもカラー出力される
NO_COLOR=1 runner -config config.toml -interactive
```

**注意事項**

- CI/CD環境では通常使用しません（`-quiet` を推奨）
- ログファイルにはANSIエスケープシーケンスが含まれません
- `-quiet` フラグと同時に指定した場合は `-quiet` が優先されます

#### `-quiet` / `-q`

**概要**

非インタラクティブモードを強制します。カラー出力と進捗表示が無効になります。

**文法**

```bash
runner -config <path> -quiet
runner -c <path> -q
```

**使用例**

```bash
# 非インタラクティブモードで実行
runner -config config.toml -quiet
runner -c config.toml -q

# ログファイルへのリダイレクト
runner -config config.toml -quiet > output.log 2>&1
```

**非インタラクティブモードの特徴**

- **プレーンテキスト**: カラーコードなし
- **簡潔な出力**: 必要最小限の情報のみ
- **機械処理向け**: スクリプトやパイプラインで処理しやすい

**出力例**

```
2025-10-02T10:30:00Z INFO Configuration loaded
2025-10-02T10:30:00Z INFO File verification completed files=5
2025-10-02T10:30:00Z INFO Starting group name=backup
2025-10-02T10:30:05Z INFO Command completed group=backup command=db_backup duration=5.2s exit_code=0
2025-10-02T10:30:18Z INFO Command completed group=backup command=file_backup duration=12.8s exit_code=0
2025-10-02T10:30:20Z INFO All commands completed duration=20.1s
```

**ユースケース**

- **CI/CD環境**: 自動化されたビルド・デプロイパイプライン
- **cronジョブ**: 定期実行スクリプト
- **ログ解析**: ログを後から解析する場合
- **スクリプト統合**: 他のスクリプトから呼び出す場合

**CI/CDでの使用例**

```yaml
# .github/workflows/deploy.yml
name: Deploy

on: [push]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Run deployment
        run: |
          runner -config deploy.toml -quiet -log-dir ./logs
```

**cronでの使用例**

```bash
# crontab
0 2 * * * /usr/local/bin/runner -config /etc/runner/backup.toml -quiet -log-dir /var/log/runner
```

**注意事項**

- `-interactive` と `-quiet` を同時に指定した場合は `-quiet` が優先されます
- エラーメッセージは stderr に出力されます
- ログレベルの設定は引き続き有効です

#### `--groups <names>` / `-g <names>`

**概要**

実行するグループをカンマ区切りで指定します。指定しない場合は、設定ファイル内のすべてのグループが実行されます。

**文法**

```bash
runner -config <path> --groups <names>
runner -c <path> -g <names>
```

**パラメータ**

- `<names>`: 実行するグループ名をカンマ区切りで指定（例: `build,test,deploy`）

**グループ名の命名規則**

グループ名は環境変数と同じ命名規則に従います:

- **パターン**: `[A-Za-z_][A-Za-z0-9_]*`
- **先頭文字**: 英字（大文字・小文字）またはアンダースコア
- **以降の文字**: 英数字またはアンダースコア

**使用例**

```bash
# 単一グループを実行
runner -config config.toml --groups build
runner -c config.toml -g build

# 複数グループを実行
runner -config config.toml --groups build,test
runner -c config.toml -g build,test

# 空白を含む指定（空白は自動的にトリミングされる）
runner -config config.toml --groups "build, test, deploy"

# すべてのグループを実行（--groups 省略時）
runner -config config.toml
```

**エラーハンドリング**

**無効なグループ名**

```bash
runner -config config.toml --groups 123test
```

エラー出力:
```
Error: invalid group name: "123test" must match pattern [A-Za-z_][A-Za-z0-9_]*
```

**存在しないグループ**

```bash
runner -config config.toml --groups xyz
```

エラー出力:
```
Error: group not found: group(s) [xyz] specified in --groups do not exist in configuration
Available groups: [build, test, deploy]
```

**ユースケース**

- **段階的なデプロイ**: 特定の環境（開発、ステージング、本番）のグループのみを実行
- **部分的なテスト**: 変更に関連するテストグループのみを実行
- **デバッグ**: 問題のあるグループだけを個別に実行
- **CI/CDパイプライン**: ブランチやイベントに応じて異なるグループを実行

**CI/CDでの活用例**

```yaml
# .github/workflows/ci.yml
name: CI

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Build
        run: runner -config config.toml --groups build

  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Test
        run: runner -config config.toml --groups test

  deploy:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Deploy
        run: runner -config config.toml --groups deploy
```

**注意事項**

- グループ名は大文字・小文字を区別します
- 存在しないグループを指定するとエラーになります
- 実行順序は設定ファイル内の記述順序に従います

#### `--keep-temp-dirs`

**概要**

一時ディレクトリを実行終了後も削除せずに保持します。デバッグ目的で使用します。

**文法**

```bash
runner -config <path> --keep-temp-dirs
```

**使用例**

```bash
# 一時ディレクトリを保持して実行
runner -config config.toml --keep-temp-dirs

# ドライランと組み合わせ（一時ディレクトリのパスを確認）
runner -config config.toml --keep-temp-dirs -dry-run
```

**動作の詳細**

通常、グループが `workdir` を指定しない場合、自動的に一時ディレクトリが生成され、グループ実行終了後に削除されます。このフラグを指定すると：

- 一時ディレクトリが削除されずに残ります
- 一時ディレクトリのパスがログに記録されます
- デバッグや結果確認に使用できます

**一時ディレクトリの場所**

```
/tmp/scr-<グループ名>-<ランダム文字列>
```

例：
```
/tmp/scr-backup-20250102123456789
/tmp/scr-analysis-20250102123500123
```

**ユースケース**

- **デバッグ**: コマンド実行結果のファイルを確認
- **トラブルシューティング**: 中間ファイルや一時ファイルを調査
- **開発・テスト**: 設定変更の影響を確認
- **監査**: 実行結果の証跡を保存

**使用例（実際のワークフロー）**

```bash
# 1. 一時ディレクトリを保持して実行
runner -config backup.toml --keep-temp-dirs

# 2. ログから一時ディレクトリのパスを確認
# 出力例: "Created temporary directory for group 'backup': /tmp/scr-backup-20250102123456"

# 3. 一時ディレクトリの内容を確認
ls -la /tmp/scr-backup-20250102123456

# 4. 必要に応じて手動でクリーンアップ
rm -rf /tmp/scr-backup-20250102123456
```

**dry-runモードとの組み合わせ**

```bash
# 一時ディレクトリのパスを確認（実際には作成されない）
runner -config config.toml --keep-temp-dirs -dry-run
```

dry-runモードでは実際には一時ディレクトリは作成されませんが、どのパスが使用されるかを確認できます。

**注意事項**

- 一時ディレクトリは手動でクリーンアップする必要があります
- 固定の `workdir` が指定されているグループには影響しません
- 複数回実行すると、複数の一時ディレクトリが作成されます
- ディスク容量に注意してください

## 4. 環境変数

### 4.1 カラー出力制御

runner コマンドは標準的なカラー制御環境変数をサポートしています。

#### `CLICOLOR`

カラー出力の有効/無効を制御します。

**値**

- `0`: カラー出力を無効化
- `1` または設定済み: カラー出力を有効化（ターミナルがサポートしている場合）

**使用例**

```bash
# カラー出力を有効化
CLICOLOR=1 runner -config config.toml

# カラー出力を無効化
CLICOLOR=0 runner -config config.toml
```

#### `NO_COLOR`

カラー出力を無効化します（[NO_COLOR標準仕様](https://no-color.org/)に準拠）。

**値**

- 設定済み（任意の値）: カラー出力を無効化
- 未設定: デフォルトの動作

**使用例**

```bash
# カラー出力を無効化
NO_COLOR=1 runner -config config.toml

# 環境変数として設定
export NO_COLOR=1
runner -config config.toml
```

#### `CLICOLOR_FORCE`

ターミナルの自動検出を無視してカラー出力を強制します。

**値**

- `0` または `false`: 強制しない
- その他の値: カラー出力を強制

**使用例**

```bash
# パイプ経由でもカラー出力
CLICOLOR_FORCE=1 runner -config config.toml | less -R

# リダイレクトしてもカラー出力（ANSIエスケープシーケンスがファイルに保存される）
CLICOLOR_FORCE=1 runner -config config.toml > output-with-colors.log
```

#### 優先順位

カラー出力の判定は以下の優先順位で行われます：

```
1. コマンドラインフラグ（-interactive, -quiet）
   ↓
2. CLICOLOR_FORCE 環境変数
   ↓
3. NO_COLOR 環境変数
   ↓
4. CLICOLOR 環境変数
   ↓
5. ターミナルの自動検出
```

**優先順位の例**

```bash
# -quiet が最優先（カラー出力されない）
CLICOLOR_FORCE=1 runner -config config.toml -quiet

# CLICOLOR_FORCE がターミナル検出より優先（カラー出力される）
CLICOLOR_FORCE=1 runner -config config.toml > output.log

# NO_COLOR が CLICOLOR より優先（カラー出力されない）
CLICOLOR=1 NO_COLOR=1 runner -config config.toml
```

### 4.2 通知設定

Slack通知は環境変数で設定できます。成功通知とエラー通知を別々のWebhookに送信できます。

#### `GSCR_SLACK_WEBHOOK_URL_SUCCESS`

成功通知用のWebhook URLを指定します。設定すると、コマンドグループの正常完了がこのWebhookに通知されます（INFOレベル）。

#### `GSCR_SLACK_WEBHOOK_URL_ERROR`

エラー通知用のWebhook URLを指定します。設定すると、警告とエラーがこのWebhookに通知されます（WARNおよびERRORレベル）。

**設定パターン**

| SUCCESS URL | ERROR URL | 動作 |
|-------------|-----------|------|
| 未設定 | 未設定 | Slack通知無効 |
| 未設定 | 設定済み | エラー通知のみ |
| 設定済み | 設定済み | 成功とエラー両方の通知 |
| 設定済み | 未設定 | **無効な設定**（エラー） |

**使用例**

```bash
# エラー通知のみ（本番環境推奨）
export GSCR_SLACK_WEBHOOK_URL_ERROR="https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX"
runner -config config.toml

# 成功とエラー両方の通知
export GSCR_SLACK_WEBHOOK_URL_SUCCESS="https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX"
export GSCR_SLACK_WEBHOOK_URL_ERROR="https://hooks.slack.com/services/T00000000/B00000000/YYYYYYYYYYYY"
runner -config config.toml

# 同じWebhookを両方に使用（すべての通知を1つのチャンネルへ）
export GSCR_SLACK_WEBHOOK_URL_SUCCESS="https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX"
export GSCR_SLACK_WEBHOOK_URL_ERROR="https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXX"
runner -config config.toml
```

**各Webhookに送信されるイベント**

| イベント種別 | SUCCESS に送信 | ERROR に送信 |
|-------------|---------------|-------------|
| コマンドグループ成功 | ✓ | - |
| コマンドグループ失敗 | - | ✓ |
| 警告 | - | ✓ |
| エラー | - | ✓ |

**通知例（成功）**

```
🤖 go-safe-cmd-runner

✅ Command completed successfully
Group: backup
Command: db_backup
Duration: 5.2s
Run ID: 01K2YK812JA735M4TWZ6BK0JH9
```

**通知例（エラー）**

```
🤖 go-safe-cmd-runner

❌ Command failed
Group: backup
Command: db_backup
Error: exit status 1
Run ID: 01K2YK812JA735M4TWZ6BK0JH9
```

**セキュリティ上の注意**

- Webhook URLは機密情報として扱ってください
- 環境変数やシークレット管理ツールで管理することを推奨します
- ログやエラーメッセージには含まれません
- dry-runモードでは、どちらのWebhookにも通知は送信されません

### 4.3 CI環境の自動検出

以下の環境変数が設定されている場合、自動的にCI環境として認識され、非インタラクティブモードで動作します。

**検出される環境変数**

| 環境変数 | CI/CDシステム |
|---------|-------------|
| `CI` | 汎用CI環境 |
| `CONTINUOUS_INTEGRATION` | 汎用CI環境 |
| `GITHUB_ACTIONS` | GitHub Actions |
| `TRAVIS` | Travis CI |
| `CIRCLECI` | CircleCI |
| `JENKINS_URL` | Jenkins |
| `GITLAB_CI` | GitLab CI |
| `APPVEYOR` | AppVeyor |
| `BUILDKITE` | Buildkite |
| `DRONE` | Drone CI |
| `TF_BUILD` | Azure Pipelines |

**CI環境での動作**

- カラー出力が自動的に無効化されます
- 進捗表示が簡潔になります
- タイムスタンプ付きのログ形式になります

**CI環境でカラー出力を有効にする**

```bash
# GitHub Actionsでカラー出力
runner -config config.toml -interactive

# または環境変数で強制
CLICOLOR_FORCE=1 runner -config config.toml
```

## 5. 実践例

### 5.1 基本的な実行

**シンプルな実行**

```bash
runner -config config.toml
```

**ログレベルを指定して実行**

```bash
runner -config config.toml -log-level debug
```

**ログファイルを保存して実行**

```bash
runner -config config.toml -log-dir /var/log/runner -log-level info
```

### 5.2 ドライランの活用

**設定変更前の確認**

```bash
# 設定ファイルを編集
vim config.toml

# ドライランで確認
runner -config config.toml -dry-run

# 問題なければ実行
runner -config config.toml
```

**詳細レベルの使い分け**

```bash
# サマリーのみ表示（全体像の把握）
runner -config config.toml -dry-run -dry-run-detail summary

# 詳細表示（通常の確認）
runner -config config.toml -dry-run -dry-run-detail detailed

# 完全な情報表示（デバッグ）
runner -config config.toml -dry-run -dry-run-detail full
```

**Dry-Run出力とログの分離**

dry-runモードでは、runnerコマンドは出力先を分離して使いやすくしています:

- **標準出力 (stdout)**: Dry-run解析結果（テキストまたはJSON形式）
- **標準エラー出力 (stderr)**: 実行ログ（slogメッセージ）

この分離により、以下のような使い方ができます:

```bash
# dry-run出力のみをファイルに保存
runner -config config.toml -dry-run > dryrun-report.txt

# ログのみをファイルに保存
runner -config config.toml -dry-run 2> execution.log

# 両方を別々のファイルに保存
runner -config config.toml -dry-run > dryrun-report.txt 2> execution.log

# dry-run出力のみを表示（ログを抑制）
runner -config config.toml -dry-run 2>/dev/null

# ログのみを表示（dry-run出力を抑制）
runner -config config.toml -dry-run 2>&1 1>/dev/null
```

**JSON出力での解析**

```bash
# JSON形式で出力してjqで解析
runner -config config.toml -dry-run -dry-run-format json | jq '.'

# 特定のコマンドのリスクレベルを確認
runner -config config.toml -dry-run -dry-run-format json | \
  jq '.resource_analyses[] | select(.risk_level == "high")'

# 実行時間の長いコマンドを確認
runner -config config.toml -dry-run -dry-run-format json | \
  jq '.resource_analyses[] | select(.timeout > 3600)'

# デバッグ情報を含めて出力
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | jq '.'

# 環境変数の継承モードを確認
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail detailed | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance != null) | .debug_info.from_env_inheritance.inheritance_mode'

# 最終的な環境変数を確認
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | .debug_info.final_environment'
```

### 5.3 ログ管理

**ログをファイルに保存**

```bash
# ログディレクトリを指定
runner -config config.toml -log-dir /var/log/runner

# デバッグログを保存
runner -config config.toml -log-dir /var/log/runner -log-level debug
```

**ログローテーション**

```bash
# 古いログを削除（30日以上前）
find /var/log/runner -name "runner-*.json" -mtime +30 -delete

# ログをアーカイブ（7日以上前）
find /var/log/runner -name "runner-*.json" -mtime +7 -exec gzip {} \;
```

**ログ解析**

```bash
# 最新のログを表示
ls -t /var/log/runner/runner-*.json | head -1 | xargs cat | jq '.'

# エラーログのみ抽出
cat /var/log/runner/runner-*.json | jq 'select(.level == "ERROR")'

# 特定のRun IDのログを表示
cat /var/log/runner/runner-01K2YK812JA735M4TWZ6BK0JH9.json | jq '.'
```

### 5.4 CI/CD環境での使用

**非インタラクティブモードでの実行**

```bash
# CI環境では明示的に-quietを指定
runner -config config.toml -quiet -log-dir ./logs
```

**GitHub Actionsでの実行例**

```yaml
name: Deployment

on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup runner
        run: |
          make build
          sudo install -o root -g root -m 4755 build/runner /usr/local/bin/runner

      - name: Record hashes
        run: |
          sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
          # TOML設定ファイル自体のハッシュを記録（最重要）
          sudo ./build/record config.toml -d /usr/local/etc/go-safe-cmd-runner/hashes
          # 実行バイナリのハッシュを記録
          sudo ./build/record /usr/local/bin/backup.sh -d /usr/local/etc/go-safe-cmd-runner/hashes

      - name: Dry run
        run: |
          runner -c config.toml -n -dry-run-format json > dryrun.json
          cat dryrun.json | jq '.'

      - name: Deploy
        run: |
          runner -c config.toml -q -log-dir ./logs
        env:
          GSCR_SLACK_WEBHOOK_URL_ERROR: ${{ secrets.SLACK_WEBHOOK_URL_ERROR }}

      - name: Upload logs
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: runner-logs
          path: logs/
```

**Jenkins Pipelineでの実行例**

```groovy
pipeline {
    agent any

    stages {
        stage('Dry Run') {
            steps {
                sh 'runner -config config.toml -dry-run'
            }
        }

        stage('Deploy') {
            steps {
                withCredentials([string(credentialsId: 'slack-webhook-error', variable: 'SLACK_WEBHOOK_ERROR')]) {
                    sh '''
                        export GSCR_SLACK_WEBHOOK_URL_ERROR="${SLACK_WEBHOOK_ERROR}"
                        runner -config config.toml -quiet -log-dir ./logs -run-id "jenkins-${BUILD_NUMBER}"
                    '''
                }
            }
        }
    }

    post {
        always {
            archiveArtifacts artifacts: 'logs/*.json', allowEmptyArchive: true
        }
    }
}
```

### 5.5 カラー出力の制御

**環境に応じた出力調整**

```bash
# 対話的な実行（カラー出力あり）
runner -config config.toml

# ログファイルへのリダイレクト（カラー出力なし）
runner -config config.toml -quiet > output.log

# パイプ経由でカラー出力を保持
runner -config config.toml -interactive | less -R
```

**強制カラー出力（パイプ経由での確認時）**

```bash
# パイプ経由でもカラー表示
CLICOLOR_FORCE=1 runner -config config.toml | less -R

# tmuxセッション内でカラー表示
CLICOLOR_FORCE=1 runner -config config.toml
```

**カラー出力を完全に無効化**

```bash
# 環境変数で無効化
NO_COLOR=1 runner -config config.toml

# フラグで無効化
runner -config config.toml -quiet
```

## 6. トラブルシューティング

### 6.1 設定ファイル関連

#### 設定ファイルが見つからない

**エラーメッセージ**
```
Error: Configuration file not found: config.toml
```

**対処法**

```bash
# ファイルの存在確認
ls -l config.toml

# 絶対パスで指定
runner -config /path/to/config.toml

# カレントディレクトリの確認
pwd
```

#### 設定検証エラー

**エラーメッセージ**
```
Configuration validation failed:
  - Group 'backup': command 'db_backup' has invalid timeout: -1
```

**対処法**

```bash
# ドライランで設定ファイルを検証
runner -config config.toml -dry-run

# 詳細なエラーメッセージを確認
runner -config config.toml -dry-run -log-level debug
```

詳細な設定方法は [TOML設定ファイルガイド](toml_config/README.ja.md) を参照してください。

### 6.2 実行時エラー

#### 権限エラー

**エラーメッセージ**
```
Error: Permission denied: /usr/local/etc/go-safe-cmd-runner/hashes
```

**対処法**

```bash
# ディレクトリの権限確認
ls -ld /usr/local/etc/go-safe-cmd-runner/hashes

# 権限の修正（管理者権限が必要）
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes

# runner実行ファイルの権限確認（setuid bitが必要）
ls -l /usr/local/bin/runner
# -rwsr-xr-x (4755) であることを確認
```

#### ファイル検証エラー

**エラーメッセージ**
```
Error: File verification failed: /usr/bin/backup.sh
Hash mismatch: expected abc123..., got def456...
```

**対処法**

```bash
# ファイルが変更されていないか確認
ls -l /usr/bin/backup.sh

# ハッシュを再記録
record /usr/bin/backup.sh -d /usr/local/etc/go-safe-cmd-runner/hashes -force

# 個別に検証
verify /usr/bin/backup.sh -d /usr/local/etc/go-safe-cmd-runner/hashes
```

詳細は [verify コマンドガイド](verify_command.ja.md) を参照してください。

#### タイムアウトエラー

**エラーメッセージ**
```
Error: Command timed out after 3600s
Group: backup
Command: full_backup
```

**対処法**

```bash
# タイムアウト値を確認
runner -config config.toml -dry-run | grep -A 5 "full_backup"

# 設定ファイルでタイムアウトを延長
# config.toml
[[groups.commands]]
name = "full_backup"
timeout = 7200  # 2時間に延長
```

### 6.3 ログ・出力関連

#### ログが出力されない

**症状**

ログファイルが作成されない、またはログが空

**対処法**

```bash
# ログディレクトリの確認
ls -ld /var/log/runner

# ディレクトリが存在しない場合は作成
sudo mkdir -p /var/log/runner
sudo chmod 755 /var/log/runner

# ログレベルを上げて詳細確認
runner -config config.toml -log-dir /var/log/runner -log-level debug

# 権限エラーの確認
runner -config config.toml -log-dir ./logs  # カレントディレクトリで試す
```

#### カラー出力が表示されない

**症状**

カラー出力が期待通りに表示されない

**対処法**

```bash
# ターミナルのカラーサポート確認
echo $TERM
# xterm-256color, screen-256color などであることを確認

# TERM環境変数が正しく設定されていない場合
export TERM=xterm-256color

# カラー出力を強制
runner -config config.toml -interactive

# または環境変数で強制
CLICOLOR_FORCE=1 runner -config config.toml

# NO_COLORが設定されていないか確認
env | grep NO_COLOR
unset NO_COLOR  # 設定されている場合は解除
```

## 7. 関連ドキュメント

### コマンドラインツール

- [record コマンドガイド](record_command.ja.md) - ハッシュファイルの作成（管理者向け）
- [verify コマンドガイド](verify_command.ja.md) - ファイル整合性の検証（デバッグ用）

### 設定ファイル

- [TOML設定ファイル ユーザーガイド](toml_config/README.ja.md) - 設定ファイルの詳細な記述方法
  - [はじめに](toml_config/01_introduction.ja.md)
  - [設定ファイルの階層構造](toml_config/02_hierarchy.ja.md)
  - [ルートレベル設定](toml_config/03_root_level.ja.md)
  - [グローバルレベル設定](toml_config/04_global_level.ja.md)
  - [グループレベル設定](toml_config/05_group_level.ja.md)
  - [コマンドレベル設定](toml_config/06_command_level.ja.md)
  - [コマンドテンプレート設定](toml_config/07_command_templates.ja.md)
  - [変数展開機能](toml_config/08_variable_expansion.ja.md)
  - [実践的な設定例](toml_config/09_practical_examples.ja.md)
  - [ベストプラクティス](toml_config/10_best_practices.ja.md)
  - [トラブルシューティング](toml_config/11_troubleshooting.ja.md)

### セキュリティ

- [セキュリティリスク評価](security-risk-assessment.ja.md) - リスクレベルの詳細

### プロジェクト情報

- [README.ja.md](../../README.ja.md) - プロジェクト概要
- [開発者向けドキュメント](../dev/) - アーキテクチャとセキュリティ設計

---

**最終更新**: 2025-10-02
