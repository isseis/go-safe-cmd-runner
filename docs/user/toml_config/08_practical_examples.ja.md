# 第8章: 実践的な設定例

本章では、実際のユースケースに基づいた実践的な設定例を紹介します。これらの例を参考に、自分の環境に合わせた設定ファイルを作成してください。

## 8.1 基本的な設定例

### シンプルなバックアップタスク

日次でファイルをバックアップする基本的な設定:

**実行前の準備:**

```bash
# TOML設定ファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes backup-config.toml

# 実行バイナリのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes /bin/tar /bin/ls
```

**設定ファイル (backup-config.toml):**

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/tmp"
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "daily_backup"
description = "日次ファイルバックアップ"
workdir = "/var/backups"

[[groups.commands]]
name = "backup_configs"
description = "設定ファイルのバックアップ"
cmd = "/bin/tar"
args = [
    "-czf",
    "config-backup.tar.gz",
    "/etc/myapp",
]
risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "backup_logs"
description = "ログファイルのバックアップ"
cmd = "/bin/tar"
args = [
    "-czf",
    "logs-backup.tar.gz",
    "/var/log/myapp",
]
risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "list_backups"
description = "バックアップファイルの一覧表示"
cmd = "/bin/ls"
args = ["-lh", "*.tar.gz"]
output_file = "backup-list.txt"
```

## 8.2 セキュリティを重視した設定例

### ファイル検証とアクセス制御

セキュリティ要件が高い環境向けの設定:

**実行前の準備:**

```bash
# TOML設定ファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes secure-backup.toml

# Global verify_files で指定したファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes /bin/sh /bin/tar /usr/bin/gpg

# Group verify_files で指定したファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes /opt/secure/bin/backup-tool
```

**設定ファイル (secure-backup.toml):**

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/opt/secure"
verify_standard_paths = true  # 全てのファイルを検証
env_allowed = ["PATH"]      # 最小限の環境変数
verify_files = [
    "/bin/sh",
    "/bin/tar",
    "/usr/bin/gpg",
]

[[groups]]
name = "secure_backup"
description = "セキュアなバックアップ処理"
workdir = "/var/secure/backups"
env_allowed = ["PATH", "GPG_KEY_ID"]
verify_files = [
    "/opt/secure/bin/backup-tool",
]

[[groups.commands]]
name = "create_backup"
description = "バックアップアーカイブの作成"
cmd = "/bin/tar"
args = [
    "-czf",
    "data-backup.tar.gz",
    "/opt/secure/data",
]
risk_level = "medium"
timeout = 1800

[[groups.commands]]
name = "encrypt_backup"
description = "バックアップの暗号化"
cmd = "/usr/bin/gpg"
vars = ["gpg_key_id=admin@example.com"]
args = [
    "--encrypt",
    "--recipient", "%{gpg_key_id}",
    "data-backup.tar.gz",
]
risk_level = "medium"

[[groups.commands]]
name = "verify_encrypted"
description = "暗号化ファイルの検証"
cmd = "/usr/bin/gpg"
args = [
    "--verify",
    "data-backup.tar.gz.gpg",
]
output_file = "verification-result.txt"
```

## 8.3 リソース管理を含む設定例

### 一時ディレクトリと自動クリーンアップ

一時的な作業スペースを使用し、処理後に自動削除:

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "temp_processing"
description = "一時ディレクトリでのデータ処理"
# 作業ディレクトリは自動的に作成される一時ディレクトリを使用

[[groups.commands]]
name = "download_data"
description = "データのダウンロード"
cmd = "/usr/bin/curl"
args = [
    "-o", "data.csv",
    "https://example.com/data/export.csv",
]
risk_level = "medium"
timeout = 600

[[groups.commands]]
name = "process_data"
description = "データの加工"
cmd = "/opt/tools/process"
args = [
    "--input", "data.csv",
    "--output", "processed.csv",
]
risk_level = "medium"
timeout = 900

[[groups.commands]]
name = "upload_result"
description = "処理結果のアップロード"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-F", "file=@processed.csv",
    "https://example.com/api/upload",
]
risk_level = "medium"
timeout = 600
output_file = "upload-response.txt"

# 一時ディレクトリは自動的に削除される
```

## 8.4 権限昇格を伴う設定例

### システム管理タスク

root 権限が必要なシステムメンテナンス:

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/tmp"
env_allowed = ["PATH", "HOME"]
verify_files = [
    "/usr/bin/apt-get",
    "/usr/bin/systemctl",
]

[[groups]]
name = "system_maintenance"
description = "システムメンテナンスタスク"

# 非特権タスク: システム状態の確認
[[groups.commands]]
name = "check_disk_space"
description = "ディスク使用量の確認"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"

# 特権タスク: パッケージの更新
[[groups.commands]]
name = "update_packages"
description = "パッケージリストの更新"
cmd = "/usr/bin/apt-get"
args = ["update"]
run_as_user = "root"
risk_level = "high"
timeout = 900

# 特権タスク: サービスの再起動
[[groups.commands]]
name = "restart_service"
description = "アプリケーションサービスの再起動"
cmd = "/usr/bin/systemctl"
args = ["restart", "myapp.service"]
run_as_user = "root"
risk_level = "high"

# 非特権タスク: サービス状態の確認
[[groups.commands]]
name = "check_service_status"
description = "サービス状態の確認"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
output_file = "service-status.txt"
```

## 8.5 出力キャプチャを使用した設定例

### ログ収集とレポート生成

複数のコマンド出力を収集してレポートを作成:

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/var/reports"
env_allowed = ["PATH", "HOME"]
output_size_limit = 10485760  # 10MB

[[groups]]
name = "system_report"
description = "システム状態レポートの生成"

[[groups.commands]]
name = "disk_usage_report"
description = "ディスク使用量レポート"
cmd = "/bin/df"
args = ["-h"]
output_file = "reports/disk-usage.txt"

[[groups.commands]]
name = "memory_report"
description = "メモリ使用状況レポート"
cmd = "/usr/bin/free"
args = ["-h"]
output_file = "reports/memory-usage.txt"

[[groups.commands]]
name = "process_report"
description = "プロセス一覧レポート"
cmd = "/bin/ps"
args = ["aux"]
output_file = "reports/processes.txt"

[[groups.commands]]
name = "network_report"
description = "ネットワーク接続状況レポート"
cmd = "/bin/netstat"
args = ["-tuln"]
output_file = "reports/network-connections.txt"

[[groups.commands]]
name = "service_report"
description = "サービス状態レポート"
cmd = "/usr/bin/systemctl"
args = ["list-units", "--type=service", "--state=running"]
output_file = "reports/services.txt"

# レポートファイルのアーカイブ
[[groups.commands]]
name = "archive_reports"
description = "レポートの圧縮"
cmd = "/bin/tar"
vars = ["date=2025-10-02"]
args = [
    "-czf",
    "system-report-%{date}.tar.gz",
    "reports/",
]
risk_level = "medium"
```

## 8.6 変数展開を活用した設定例

### 環境別デプロイメント

開発・ステージング・本番環境で異なる設定を使用:

```toml
version = "1.0"

[global]
timeout = 600
env_allowed = ["PATH", "HOME"]

# 開発環境
[[groups]]
name = "deploy_development"
description = "開発環境へのデプロイ"

[[groups.commands]]
name = "deploy_dev_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=development",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_dev_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=debug",
    "api_port=8080",
    "db_url=postgresql://localhost/dev_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
env_vars = ["DB_URL=%{db_url}"]
risk_level = "high"

# ステージング環境
[[groups]]
name = "deploy_staging"
description = "ステージング環境へのデプロイ"

[[groups.commands]]
name = "deploy_staging_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=staging",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_staging_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=info",
    "api_port=8081",
    "db_url=postgresql://staging-db/staging_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
env_vars = ["DB_URL=%{db_url}"]
risk_level = "high"

# 本番環境
[[groups]]
name = "deploy_production"
description = "本番環境へのデプロイ"

[[groups.commands]]
name = "deploy_prod_config"
cmd = "/bin/cp"
vars = [
    "config_dir=/opt/configs",
    "env_type=production",
]
args = [
    "%{config_dir}/%{env_type}/app.yml",
    "/etc/myapp/app.yml",
]
risk_level = "medium"

[[groups.commands]]
name = "start_prod_server"
vars = [
    "app_bin=/opt/myapp/bin/server",
    "log_level=warn",
    "api_port=8082",
    "db_url=postgresql://prod-db/prod_db",
]
cmd = "%{app_bin}"
args = [
    "--config", "/etc/myapp/app.yml",
    "--log-level", "%{log_level}",
    "--port", "%{api_port}",
    "--database", "%{db_url}",
]
env_vars = ["DB_URL=%{db_url}"]
run_as_user = "appuser"
risk_level = "high"
```

## 8.7 複合的な設定例

### フルスタックアプリケーションのデプロイ

データベース、アプリケーション、Webサーバーの統合デプロイ:

**実行前の準備:**

```bash
# TOML設定ファイルのハッシュを記録
record deploy-fullstack.toml -d /usr/local/etc/go-safe-cmd-runner/hashes

# Global verify_files で指定したファイルのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes \
    /usr/bin/psql \
    /usr/bin/pg_dump

# 実行バイナリのハッシュを記録
record -d /usr/local/etc/go-safe-cmd-runner/hashes \
    /bin/tar \
    /usr/bin/dpkg \
    /opt/myapp/bin/migrate \
    /usr/bin/systemctl \
    /usr/bin/pip3 \
    /bin/cp \
    /usr/bin/nginx \
    /usr/bin/curl \
    /opt/tools/generate-report \
    /bin/rm
```

**設定ファイル (deploy-fullstack.toml):**

```toml
version = "1.0"

[global]
timeout = 900
workdir = "/opt/deploy"
verify_standard_paths = false
env_allowed = [
    "PATH",
    "HOME",
    "DB_USER",
    "DB_NAME",
    "APP_DIR",
    "WEB_ROOT",
    "BACKUP_DIR",
]
output_size_limit = 52428800  # 50MB

# フェーズ1: 事前準備
[[groups]]
name = "preparation"
description = "デプロイ前の準備作業"
workdir = "/opt/deploy/prep"

[[groups.commands]]
name = "backup_current_version"
description = "現在のバージョンをバックアップ"
cmd = "/bin/tar"
vars = [
    "backup_dir=/var/backups/app",
    "app_dir=/opt/myapp",
    "timestamp=2025-10-02-120000",
]
args = [
    "-czf",
    "%{backup_dir}/app-backup-%{timestamp}.tar.gz",
    "%{app_dir}",
]
risk_level = "medium"
timeout = 1800

[[groups.commands]]
name = "check_dependencies"
description = "依存関係の確認"
cmd = "/usr/bin/dpkg"
args = ["-l"]
output_file = "installed-packages.txt"

# フェーズ2: データベース更新
[[groups]]
name = "database_migration"
description = "データベーススキーマの更新"
env_allowed = ["PATH", "DB_USER", "DB_NAME", "PGPASSWORD"]
verify_files = ["/usr/bin/psql", "/usr/bin/pg_dump"]

[[groups.commands]]
name = "backup_database"
description = "データベースのバックアップ"
cmd = "/usr/bin/pg_dump"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
    "timestamp=2025-10-02-120000",
]
env_vars = ["PGPASSWORD=secret123"]
args = [
    "-U", "%{db_user}",
    "-d", "%{db_name}",
    "-F", "c",
    "-f", "/var/backups/db/backup-%{timestamp}.dump",
]
risk_level = "medium"
timeout = 1800
output_file = "db-backup-log.txt"

[[groups.commands]]
name = "run_migrations"
description = "データベースマイグレーションの実行"
cmd = "/opt/myapp/bin/migrate"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
]
args = [
    "--database", "postgresql://%{db_user}@localhost/%{db_name}",
    "--migrations", "/opt/myapp/migrations",
]
risk_level = "high"
timeout = 600

# フェーズ3: アプリケーションデプロイ
[[groups]]
name = "application_deployment"
description = "アプリケーションのデプロイ"
workdir = "/opt/myapp"

[[groups.commands]]
name = "stop_application"
description = "アプリケーションの停止"
cmd = "/usr/bin/systemctl"
args = ["stop", "myapp.service"]
run_as_user = "root"
risk_level = "high"

[[groups.commands]]
name = "deploy_new_version"
description = "新バージョンのデプロイ"
cmd = "/bin/tar"
args = [
    "-xzf",
    "/opt/deploy/releases/myapp-v2.0.0.tar.gz",
    "-C", "/opt/myapp",
]
risk_level = "medium"

[[groups.commands]]
name = "install_dependencies"
description = "依存パッケージのインストール"
cmd = "/usr/bin/pip3"
args = [
    "install",
    "-r", "/opt/myapp/requirements.txt",
]
risk_level = "high"
timeout = 600

[[groups.commands]]
name = "start_application"
description = "アプリケーションの起動"
cmd = "/usr/bin/systemctl"
args = ["start", "myapp.service"]
run_as_user = "root"
risk_level = "high"

# フェーズ4: Webサーバー設定更新
[[groups]]
name = "web_server_update"
description = "Webサーバーの設定更新"

[[groups.commands]]
name = "update_nginx_config"
description = "Nginx設定の更新"
cmd = "/bin/cp"
args = [
    "/opt/deploy/configs/nginx/myapp.conf",
    "/etc/nginx/sites-available/myapp.conf",
]
run_as_user = "root"
risk_level = "high"

[[groups.commands]]
name = "test_nginx_config"
description = "Nginx設定の検証"
cmd = "/usr/bin/nginx"
args = ["-t"]
run_as_user = "root"
risk_level = "medium"
output_file = "nginx-config-test.txt"

[[groups.commands]]
name = "reload_nginx"
description = "Nginxの再読み込み"
cmd = "/usr/bin/systemctl"
args = ["reload", "nginx"]
run_as_user = "root"
risk_level = "high"

# フェーズ5: デプロイ検証
[[groups]]
name = "deployment_verification"
description = "デプロイの検証"

[[groups.commands]]
name = "health_check"
description = "アプリケーションのヘルスチェック"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "-s",
    "http://localhost:8080/health",
]
timeout = 30
output_file = "health-check-result.txt"

[[groups.commands]]
name = "smoke_test"
description = "基本機能の動作確認"
cmd = "/usr/bin/curl"
args = [
    "-f",
    "-s",
    "http://localhost:8080/api/status",
]
output_file = "smoke-test-result.txt"

[[groups.commands]]
name = "verify_database_connection"
description = "データベース接続の確認"
cmd = "/usr/bin/psql"
vars = [
    "db_user=appuser",
    "db_name=myapp_db",
]
args = [
    "-U", "%{db_user}",
    "-d", "%{db_name}",
    "-c", "SELECT version();",
]
output_file = "db-connection-test.txt"

# フェーズ6: 後処理とレポート
[[groups]]
name = "post_deployment"
description = "デプロイ後の処理"
workdir = "/var/reports/deployment"

[[groups.commands]]
name = "generate_deployment_report"
description = "デプロイレポートの生成"
cmd = "/opt/tools/generate-report"
vars = ["timestamp=2025-10-02-120000"]
args = [
    "--deployment-log", "/var/log/deploy.log",
    "--output", "deployment-report-%{timestamp}.html",
]

[[groups.commands]]
name = "cleanup_temp_files"
description = "一時ファイルの削除"
cmd = "/bin/rm"
args = ["-rf", "/opt/deploy/temp"]
risk_level = "medium"

[[groups.commands]]
name = "send_notification"
description = "デプロイ完了通知"
cmd = "/usr/bin/curl"
args = [
    "-X", "POST",
    "-H", "Content-Type: application/json",
    "-d", '{"message":"Deployment completed successfully"}',
    "https://slack.example.com/webhook",
]
```

## 8.8 リスクベースの制御例

### リスクレベルに応じたコマンド実行

```toml
version = "1.0"

[global]
timeout = 300
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "risk_controlled_operations"
description = "リスクレベルに基づく操作制御"

# 低リスク: 読み取り専用操作
[[groups.commands]]
name = "read_config"
description = "設定ファイルの読み取り"
cmd = "/bin/cat"
args = ["/etc/myapp/config.yml"]
output_file = "config-content.txt"

# 中リスク: ファイル作成・変更
[[groups.commands]]
name = "update_cache"
description = "キャッシュファイルの更新"
cmd = "/opt/myapp/update-cache"
args = ["--refresh"]
risk_level = "medium"

# 高リスク: システム変更
[[groups.commands]]
name = "system_update"
description = "システムパッケージの更新"
cmd = "/usr/bin/apt-get"
args = ["upgrade", "-y"]
run_as_user = "root"
risk_level = "high"
timeout = 1800

# リスクレベル超過で実行拒否される例
[[groups.commands]]
name = "dangerous_deletion"
description = "大量削除(デフォルトリスクレベルでは実行不可)"
cmd = "/bin/rm"
args = ["-rf", "/tmp/old-data"]
# risk_level のデフォルトは "low"
# rm -rf は中リスク以上が必要 → 実行拒否される
```

## まとめ

本章では、以下の実践的な設定例を紹介しました:

1. **基本的な設定**: シンプルなバックアップタスク
2. **セキュリティ重視**: ファイル検証とアクセス制御
3. **リソース管理**: 一時ディレクトリと自動クリーンアップ
4. **権限昇格**: システム管理タスク
5. **出力キャプチャ**: ログ収集とレポート生成
6. **変数展開**: 環境別デプロイメント
7. **複合設定**: フルスタックアプリケーションのデプロイ
8. **リスクベース制御**: リスクレベルに応じた実行制御

これらの例を参考に、自分の環境やユースケースに合わせた設定ファイルを作成してください。

## 次のステップ

次章では、設定ファイル作成時のベストプラクティスを学びます。セキュリティ、保守性、パフォーマンスの観点から、より良い設定ファイルを作成するための指針を提供します。
