# 第9章: ベストプラクティス

本章では、go-safe-cmd-runner の設定ファイルを作成する際のベストプラクティスを紹介します。セキュリティ、保守性、パフォーマンスの観点から、より良い設定ファイルを作成するための指針を提供します。

## 9.1 セキュリティのベストプラクティス

### 9.1.1 最小権限の原則

必要最小限の権限でコマンドを実行してください。

#### 推奨される実装

```toml
# 良い例: 必要な権限のみを使用
[[groups.commands]]
name = "read_log"
cmd = "/bin/cat"
args = ["/var/log/app/app.log"]
run_as_group = "loggroup"  # ログ読み取りに必要な権限のみ
risk_level = "low"

# 避けるべき例: 過剰な権限
[[groups.commands]]
name = "read_log"
cmd = "/bin/cat"
args = ["/var/log/app/app.log"]
run_as_user = "root"  # 不必要に root 権限を使用
```

### 9.1.2 環境変数の厳格な管理

環境変数の許可リストは必要最小限に設定してください。

#### 推奨される実装

```toml
# 良い例: 必要な変数のみを許可
[global]
env_allowed = [
    "PATH",           # コマンド検索に必須
    "HOME",           # 設定ファイル検索に使用
    "APP_CONFIG_DIR", # アプリ固有の設定
]

# 避けるべき例: 過度に寛容な設定
[global]
env_allowed = [
    "PATH", "HOME", "USER", "SHELL", "EDITOR", "PAGER",
    "MAIL", "LOGNAME", "HOSTNAME", "DISPLAY", "XAUTHORITY",
    # ... 多すぎる
]
```

### 9.1.3 ファイル検証の活用

重要な設定ファイルやライブラリは必ず検証してください。コマンドの実行可能ファイルは自動的に検証されます。

#### 推奨される実装

```toml
# 良い例: 設定ファイルやスクリプトファイルを検証
[global]
verify_standard_paths = true  # 標準パスのコマンドも検証
verify_files = [
    "/etc/app/global.conf",  # グローバル設定ファイル
]

[[groups]]
name = "critical_operations"
verify_files = [
    "/opt/app/config/critical.conf",  # 重要な設定ファイル
    "/opt/app/lib/helper.sh",         # 補助スクリプト
]
# 注: コマンド自体は自動的に検証されるため verify_files に追加不要
```

### 9.1.4 絶対パスの使用

コマンドは絶対パスで指定してください。

#### 推奨される実装

```toml
# 良い例: 絶対パス
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]

# 避けるべき例: PATH 依存
[[groups.commands]]
name = "backup"
cmd = "tar"  # どの tar が実行されるか不明確
args = ["-czf", "backup.tar.gz", "/data"]
```

### 9.1.5 機密情報の取り扱い

機密情報は Command.Env で管理し、システム環境から隔離してください。

#### 推奨される実装

```toml
# 良い例: vars と env を適切に使い分け
[global]
env_allowed = ["PATH", "HOME"]  # 機密情報は含めない

[[groups.commands]]
name = "api_call"
cmd = "/usr/bin/curl"
vars = [
    "api_token=sk-secret123",
    "api_endpoint=https://api.example.com",
]
args = [
    "-H", "Authorization: Bearer %{api_token}",
    "%{api_endpoint}",
]
env_vars = ["API_TOKEN=%{api_token}"]  # 必要に応じて環境変数として設定

# 避けるべき例: グローバルに機密情報を許可
[global]
env_allowed = ["PATH", "HOME", "API_TOKEN"]  # システム環境変数に依存
```

### 9.1.6 リスクレベルの適切な設定

コマンドの性質に応じて適切なリスクレベルを設定してください。

#### 推奨される実装

```toml
# 読み取り専用: low
[[groups.commands]]
name = "read_config"
cmd = "/bin/cat"
args = ["/etc/app/config.yml"]
risk_level = "low"

# ファイル作成・変更: medium
[[groups.commands]]
name = "create_backup"
cmd = "/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
risk_level = "medium"

# システム変更・権限昇格: high
[[groups.commands]]
name = "install_package"
cmd = "/usr/bin/apt-get"
args = ["install", "-y", "package"]
run_as_user = "root"
risk_level = "high"
```

## 9.2 環境変数管理のベストプラクティス

### 9.2.1 継承モードの適切な使用

環境変数の継承モードを目的に応じて使い分けてください。

#### 使い分けの指針

```toml
[global]
env_allowed = ["PATH", "HOME", "USER"]

# パターン1: 継承モード - グローバル設定で十分な場合
[[groups]]
name = "standard_group"
# env_allowed 未指定 → グローバルを継承

# パターン2: 明示モード - グループ固有の変数が必要な場合
[[groups]]
name = "database_group"
env_allowed = ["PATH", "DB_HOST", "DB_USER"]  # グローバルとは異なる設定

# パターン3: 拒否モード - 完全隔離が必要な場合
[[groups]]
name = "isolated_group"
env_allowed = []  # 全ての環境変数を拒否
```

### 9.2.2 変数の命名規則

一貫した命名規則を使用してください。

#### 推奨される命名規則

```toml
# 良い例: 一貫した命名規則
env_vars = [
    "APP_DIR=/opt/myapp",           # アプリ関連は APP_ プレフィックス
    "APP_CONFIG=/etc/myapp/config.yml",
    "APP_LOG_DIR=/var/log/myapp",
    "DB_HOST=localhost",            # データベース関連は DB_ プレフィックス
    "DB_PORT=5432",
    "DB_NAME=myapp_db",
    "BACKUP_DIR=/var/backups",      # バックアップ関連は BACKUP_ プレフィックス
    "BACKUP_RETENTION_DAYS=30",
]

# 避けるべき例: 不統一な命名
env_vars = [
    "app_directory=/opt/myapp",     # 小文字とアンダースコア
    "APPCONFIG=/etc/myapp/config.yml",  # プレフィックスなし
    "log-dir=/var/log/myapp",       # ハイフン使用
    "DatabaseHost=localhost",       # キャメルケース
]
```

### 9.2.3 変数の再利用

共通の値は変数として定義し、再利用してください。

#### 推奨される実装

```toml
# 良い例: vars での変数の再利用
[global]
vars = ["config_dest=/etc/myapp"]

[[groups.commands]]
name = "deploy_config"
cmd = "/bin/cp"
vars = ["config_source=/opt/configs/prod"]
args = ["%{config_source}/app.yml", "%{config_dest}/app.yml"]

[[groups.commands]]
name = "backup_config"
cmd = "/bin/cp"
vars = ["backup_dir=/var/backups"]
args = ["%{config_dest}/app.yml", "%{backup_dir}/app.yml"]
# config_dest はグローバルから継承されるため再定義不要
```

## 9.3 グループ構成のベストプラクティス

### 9.3.1 論理的なグループ化

関連するコマンドを論理的にグループ化してください。

#### 推奨される構成

```toml
# 良い例: 論理的なグループ分け
[[groups]]
name = "database_operations"
description = "データベース関連の全操作"
# ... データベース関連のコマンド

[[groups]]
name = "file_operations"
description = "ファイル操作関連の全タスク"
# ... ファイル操作関連のコマンド

[[groups]]
name = "network_operations"
description = "ネットワーク通信関連の全タスク"
# ... ネットワーク関連のコマンド

# 避けるべき例: 無秩序なグループ分け
[[groups]]
name = "group1"
# データベース、ファイル、ネットワークが混在
```

### 9.3.2 説明の充実

各グループとコマンドに明確な説明を記述してください。

#### 推奨される実装

```toml
# 良い例: 詳細な説明
[[groups]]
name = "database_backup"
description = "PostgreSQL データベースの日次バックアップ処理。全データベースをダンプし、圧縮・暗号化して保存。"

[[groups.commands]]
name = "full_backup"
description = "全データベースの完全バックアップ(pg_dump --all-databases)"
cmd = "/usr/bin/pg_dump"
args = ["--all-databases"]

# 避けるべき例: 不十分な説明
[[groups]]
name = "db_backup"
description = "Backup"  # 何をバックアップするか不明

[[groups.commands]]
name = "backup"
description = "Run backup"  # 具体性がない
```

## 9.4 エラーハンドリングのベストプラクティス

### 9.4.1 適切なタイムアウト設定

コマンドの性質に応じて適切なタイムアウトを設定してください。

#### 推奨される実装

```toml
[global]
timeout = 300  # デフォルト: 5分

[[groups.commands]]
name = "quick_check"
cmd = "/bin/ping"
args = ["-c", "3", "localhost"]
timeout = 10  # 短いコマンドには短いタイムアウト

[[groups.commands]]
name = "database_dump"
cmd = "/usr/bin/pg_dump"
args = ["large_database"]
timeout = 3600  # 大きなデータベースには長めのタイムアウト

[[groups.commands]]
name = "file_sync"
cmd = "/usr/bin/rsync"
args = ["-av", "/source", "/dest"]
timeout = 7200  # ネットワーク転送には十分な時間を確保
```

### 9.4.2 出力サイズの適切な制限

処理するデータ量に応じて出力サイズ制限を設定してください。

#### 推奨される実装

```toml
# ログファイル解析など、出力が多い場合
[global]
output_size_limit = 104857600  # 100MB

# 小規模な出力のみの場合
[global]
output_size_limit = 1048576  # 1MB
```

## 9.5 保守性のベストプラクティス

### 9.5.1 コメントの活用

設定の意図や注意点をコメントで記述してください。

#### 推奨される実装

```toml
# 本番環境デプロイ設定
# 最終更新: 2025-10-02
# 担当者: DevOps Team
version = "1.0"

[global]
# タイムアウトは最長実行時間の1.5倍に設定
timeout = 900

# セキュリティ要件により、システムパスも検証
verify_standard_paths = true

[[groups]]
name = "production_deployment"
# 注意: このグループは本番環境でのみ実行すること
# ステージング環境では deploy_staging グループを使用
```

### 9.5.2 設定の構造化

設定を論理的に構造化し、可読性を向上させてください。

#### 推奨される実装

```toml
version = "1.0"

# ========================================
# グローバル設定
# ========================================
[global]
timeout = 600
workdir = "/opt/deploy"
env_allowed = ["PATH", "HOME"]

# ========================================
# フェーズ1: 事前準備
# ========================================
[[groups]]
name = "preparation"
# ... コマンド定義

# ========================================
# フェーズ2: デプロイ
# ========================================
[[groups]]
name = "deployment"
# ... コマンド定義

# ========================================
# フェーズ3: 検証
# ========================================
[[groups]]
name = "verification"
# ... コマンド定義
```

### 9.5.3 設定ファイルの分割

大規模な設定は複数のファイルに分割することを検討してください。

#### 推奨される構成

```
configs/
├── base.toml              # 共通設定
├── development.toml       # 開発環境固有の設定
├── staging.toml           # ステージング環境固有の設定
└── production.toml        # 本番環境固有の設定
```

各環境で適切な設定ファイルを使用:
```bash
# 開発環境
go-safe-cmd-runner -file configs/development.toml

# 本番環境
go-safe-cmd-runner -file configs/production.toml
```

## 9.6 パフォーマンスのベストプラクティス

### 9.6.1 並列実行の検討

独立したグループは並列実行できるように設計してください。

#### 推奨される実装

```toml
# 良い例: 独立したグループ(並列実行可能)
[[groups]]
name = "backup_database"
# データベースバックアップ

[[groups]]
name = "backup_files"
# ファイルバックアップ
```

### 9.6.2 ファイル検証の最適化

検証が必要なファイルのみを指定してください。

#### 推奨される実装

```toml
# 良い例: 必要なファイルのみ検証
[global]
verify_standard_paths = false  # 標準パスはスキップ
verify_files = [
    "/opt/app/bin/critical-tool",  # アプリ固有のツールのみ検証
]

# 避けるべき例: 過度な検証
[global]
verify_standard_paths = true
verify_files = [
    "/bin/ls", "/bin/cat", "/bin/grep", "/bin/sed",
    # ... 多数の標準コマンド(パフォーマンス低下)
]
```

### 9.6.3 出力キャプチャの適切な使用

必要な場合のみ出力をキャプチャしてください。

#### 推奨される実装

```toml
# 良い例: 必要な出力のみキャプチャ
[[groups.commands]]
name = "system_info"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"  # レポート生成に必要

[[groups.commands]]
name = "simple_echo"
cmd = "/bin/echo"
args = ["Processing..."]
# output 未指定 → キャプチャしない(標準出力に表示)

# 避けるべき例: 不要な出力キャプチャ
[[groups.commands]]
name = "simple_echo"
cmd = "/bin/echo"
args = ["Processing..."]
output_file = "echo-output.txt"  # 不要なキャプチャ(リソースの無駄)
```

## 9.7 テストとバリデーション

### 9.7.1 段階的なテスト

設定ファイルは段階的にテストしてください。

#### 推奨される手順

1. **基本的なコマンドから開始**
```toml
# ステップ1: 最小構成でテスト
[[groups.commands]]
name = "test_basic"
cmd = "/bin/echo"
args = ["test"]
```

2. **徐々に複雑化**
```toml
# ステップ2: 変数展開を追加
[[groups.commands]]
name = "test_variables"
cmd = "/bin/echo"
vars = ["test_var=hello"]
args = ["Value: %{test_var}"]
```

3. **本番相当の設定**
```toml
# ステップ3: 完全な設定でテスト
[[groups.commands]]
name = "production_command"
cmd = "/opt/app/bin/tool"
vars = ["config=/etc/app/config.yml"]
args = ["--config", "%{config}"]
run_as_user = "appuser"
risk_level = "high"
```

### 9.7.2 ドライラン機能の活用

本番実行前にドライランで動作を確認してください。

```bash
# ドライランで設定を検証
go-safe-cmd-runner --dry-run --file config.toml

# 問題なければ本番実行
go-safe-cmd-runner -file config.toml
```

## 9.8 ドキュメント化

### 9.8.1 設定ファイルのドキュメント化

設定ファイルと合わせて README を作成してください。

#### README.md の例

```markdown
# アプリケーションデプロイ設定

## 概要
本番環境へのアプリケーションデプロイを自動化する設定ファイル。

## 前提条件
- PostgreSQL がインストールされていること
- /opt/app ディレクトリが存在すること
- appuser ユーザーが存在すること

## 実行方法
```bash
go-safe-cmd-runner -file production-deploy.toml
```

## 環境変数
以下の環境変数を設定してください:
- `DB_PASSWORD`: データベースパスワード
- `API_KEY`: 外部APIキー

## トラブルシューティング
### データベース接続エラー
- PostgreSQL サービスが起動しているか確認
- 認証情報が正しいか確認
```

### 9.8.2 変更履歴の記録

設定ファイルの変更履歴をコメントで記録してください。

```toml
# 変更履歴:
# 2025-10-02: タイムアウトを 300秒 → 600秒 に延長 (大規模DBに対応)
# 2025-09-15: 暗号化処理を追加
# 2025-09-01: 初版作成

version = "1.0"
# ...
```

## まとめ

本章で紹介したベストプラクティス:

1. **セキュリティ**: 最小権限、環境変数の厳格管理、ファイル検証、絶対パス使用
2. **環境変数管理**: 適切な継承モード、一貫した命名規則、変数の再利用
3. **グループ構成**: 論理的なグループ化、優先度の効果的な使用、充実した説明
4. **エラーハンドリング**: 適切なタイムアウト、出力サイズ制限
5. **保守性**: コメントの活用、構造化、設定の分割
6. **パフォーマンス**: 並列実行、検証の最適化、適切な出力キャプチャ
7. **テスト**: 段階的なテスト、ドライラン活用
8. **ドキュメント化**: README 作成、変更履歴の記録

これらのプラクティスに従うことで、安全で保守性の高い設定ファイルを作成できます。

## 次のステップ

次章では、設定ファイル作成時によくある問題とその解決方法を学びます。トラブルシューティングのテクニックを習得しましょう。
