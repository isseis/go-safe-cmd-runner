# Go Safe Command Runner

特権タスクの委譲と自動バッチ処理のために設計された、包括的なセキュリティ制御を備えたGoによる安全なコマンド実行フレームワークです。

プロジェクトページ: https://github.com/isseis/go-safe-cmd-runner/

## 背景

Go Safe Command Runnerは、以下のような環境における安全なコマンド実行の重要なニーズに対応します：
- 一般ユーザーが特権操作を安全に実行する必要がある場合
- 自動システムがセキュアなバッチ処理機能を必要とする場合
- コマンド実行前にファイルの整合性検証が不可欠な場合
- 環境変数の公開に厳格な制御が必要な場合
- コマンド実行に監査証跡とセキュリティ境界が必要な場合

一般的な用途には、定期バックアップ、システム保守タスク、セキュリティ制御を維持しながら特定の管理操作を非rootユーザーに委譲することなどが含まれます。

## 機能

### コアセキュリティ機能
- **ファイル整合性検証**: 実行前の実行ファイルと設定ファイルのSHA-256ハッシュ検証
- **環境変数分離**: グローバルレベルとグループレベルでの許可リストベースの環境変数フィルタリング
- **権限管理**: 制御された権限昇格と自動権限降下
- **パス検証**: シンボリックリンク攻撃防止を備えたコマンドパス解決
- **設定検証**: 包括的なTOML設定ファイル検証

### コマンド実行
- **バッチ処理**: 依存関係管理を備えた組織化されたグループでのコマンド実行
- **バックグラウンド実行**: 適切なシグナル処理を備えた長時間実行プロセスのサポート
- **出力キャプチャ**: 構造化されたログ記録と出力管理
- **ドライランモード**: 実際の実行なしでのコマンド実行プレビュー
- **タイムアウト制御**: コマンド実行の設定可能なタイムアウト

### ログとモニタリング
- **マルチハンドラーログ**: 複数の出力先への同時ログ配信（コンソール、ファイル、Slack）
- **Slack統合**: セキュリティイベントと障害のリアルタイム通知
- **監査ログ**: 特権操作とセキュリティイベントの包括的な監査証跡
- **機密データ削除**: ログからの機密情報の自動検出と削除
- **構造化ログ**: 豊富なコンテキスト情報を含むJSON形式のログ
- **ULID実行追跡**: 時系列順に並べ可能な汎用一意識別子による実行追跡

### ファイル操作
- **セーフファイルI/O**: セキュリティチェック付きのシンボリックリンク対応ファイル操作
- **ハッシュ記録**: 後の検証のための重要ファイルのSHA-256ハッシュ記録
- **検証ツール**: ファイル整合性検証のためのスタンドアロンユーティリティ

## アーキテクチャ

システムは関心の明確な分離を伴うモジュラーアーキテクチャに従います：

```
cmd/                    # コマンドライン エントリーポイント
├── runner/            # メインコマンドランナーアプリケーション
├── record/            # ハッシュ記録ユーティリティ
└── verify/            # ファイル検証ユーティリティ

internal/              # コア実装
├── cmdcommon/         # 共有コマンドユーティリティ
├── filevalidator/     # ファイル整合性検証
├── logging/           # 高度なログシステム
│   ├── multihandler/  # マルチ出力先ログハンドラー
│   ├── slack/         # Slack通知統合
│   └── redaction/     # 機密データ削除
├── runner/            # コマンド実行エンジン
│   ├── audit/         # セキュリティ監査ログ
│   ├── config/        # 設定管理
│   ├── executor/      # コマンド実行ロジック
│   └── privilege/     # 権限管理
├── safefileio/        # セキュアファイル操作
└── verification/      # ハッシュ検証システム
```

## コマンドラインツール

### メインランナー
```bash
# 設定ファイルからコマンドを実行
./runner -config config.toml

# ドライランモード（実行なしでプレビュー）
./runner -config config.toml -dry-run

# 設定ファイルの検証
./runner -config config.toml -validate

# カスタム環境ファイルを使用
./runner -config config.toml -env-file .env.production

# カスタムハッシュディレクトリ
./runner -config config.toml -hash-directory /custom/hash/dir

# カスタムログディレクトリとレベル
./runner -config config.toml -log-dir /var/log/go-safe-cmd-runner -log-level debug

# Slack通知付きで実行（環境ファイルにSLACK_WEBHOOK_URLが必要）
./runner -config config.toml -env-file .env
```

### ハッシュ管理
```bash
# ファイルハッシュの記録
./record -file /path/to/executable -hash-dir /etc/hashes

# 既存ハッシュの強制上書き
./record -file /path/to/file -force

# ファイル整合性の検証
./verify -file /path/to/file -hash-dir /etc/hashes
```

## 設定

### 基本設定例
```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
# セキュリティのための環境変数許可リスト
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG"
]
# 実行前に検証するファイル
verify_files = ["/etc/passwd", "/bin/bash"]

[[groups]]
name = "backup"
description = "システムバックアップ操作"
priority = 1
# グループ固有の環境変数（グローバル設定を上書き）
env_allowlist = ["PATH", "HOME", "BACKUP_DIR"]

[[groups.commands]]
name = "database_backup"
description = "データベースのバックアップ"
cmd = "mysqldump"
args = ["--all-databases", "--single-transaction"]
env = ["BACKUP_DIR=/backups"]
privileged = false

[[groups.commands]]
name = "system_backup"
description = "システムファイルのバックアップ"
cmd = "rsync"
args = ["-av", "/etc/", "/backups/etc/"]
privileged = true
```

### 高度な設定機能
```toml
[global]
# 標準システムパスの検証をスキップ
skip_standard_paths = true
# グローバルファイル検証リスト
verify_files = ["/usr/bin/rsync", "/etc/rsync.conf"]

[[groups]]
name = "web_deployment"
description = "Webアプリケーションのデプロイメント"
priority = 2
# 厳格な環境制御（空リスト = 環境変数なし）
env_allowlist = []
# グループ固有のファイル検証
verify_files = ["/usr/local/bin/deploy.sh"]

[[groups.commands]]
name = "deploy_app"
cmd = "/usr/local/bin/deploy.sh"
args = ["production"]
# 空のenv_allowlistのため環境変数は利用不可
```

### 環境変数セキュリティ
システムは環境変数に対して厳格な許可リストベースのアプローチを実装します：

1. **グローバル許可リスト**: すべてのグループで利用可能な基本環境変数を定義
2. **グループ上書き**: グループは独自の許可リストを定義し、グローバル設定を完全に上書き可能
3. **継承**: 明示的な許可リストのないグループはグローバル設定を継承
4. **ゼロトラスト**: 未定義の許可リストは環境変数が渡されないことを意味

### 環境ファイル設定
メインのTOML設定に保存すべきでない機密設定のために`.env`ファイルを作成：

```bash
# 本番環境用の.envファイル
# Slack通知用のwebhook URL
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK

# オプション: デフォルトログ設定の上書き
LOG_LEVEL=info
LOG_DIR=/var/log/go-safe-cmd-runner

# アプリケーション固有の変数
DATABASE_URL=postgresql://localhost:5432/myapp
API_KEY=your-secret-api-key
```

**セキュリティ注意**: `.env`ファイルは厳格なセキュリティ検証を受けます：
- ファイル権限のチェック（所有者のみ読み取り可能であるべき）
- パストラバーサル攻撃の防止
- セーフファイルI/O操作を使用したセキュアなコンテンツ解析

## セキュリティモデル

### ファイル整合性検証
- すべての実行ファイルと重要ファイルは事前記録されたSHA-256ハッシュと照合して検証
- 設定ファイルは実行前に自動的に検証
- グループ固有およびグローバルファイル検証リスト
- 検証が失敗した場合は実行を中止

### 権限管理
- 初期化後の自動権限降下
- 特定コマンドの制御された権限昇格
- 最小権限原則の強制
- 包括的な監査ログ

### 環境分離
- 厳格な許可リストベースの環境変数フィルタリング
- 環境変数インジェクション攻撃からの保護
- グループレベルおよびグローバル環境制御
- セキュアな変数参照解決

### ログセキュリティ
- **機密データ削除**: シークレット、トークン、機密パターンの自動検出と削除
- **セキュア通知**: セキュリティアラート用の暗号化Slack webhook通信
- **監査証跡保護**: 構造化形式による改ざん耐性のあるログ
- **アクセス制御**: ログファイルの権限とセキュアストレージ実践

## スコープ外

このプロジェクトは明示的に以下を提供**しません**：
- **コンテナオーケストレーション**やDocker統合
- **ネットワークセキュリティ**機能（ファイアウォール、VPNなど）
- **ユーザー認証**や認可システム
- **WebインターフェースやREST API**
- **データベース管理**機能
- **リアルタイム監視**やアラートシステム
- **クロスプラットフォームGUI**アプリケーション
- **パッケージ管理**やソフトウェアインストール

Unix系環境でのファイル整合性検証を伴うセキュアなコマンド実行に焦点を当てています。

## ライセンス
本プロジェクトはMITライセンスで公開されています。詳細は[LICENSE](./LICENSE)ファイルをご参照ください。

## ビルドとインストール

### 前提条件
- Go 1.21以降（slicesパッケージサポートに必要）
- golangci-lint（開発用）

### ビルドコマンド
```bash
# すべてのバイナリをビルド
make build

# 特定のバイナリをビルド
make build/runner
make build/record
make build/verify

# テストを実行
make test

# リンターを実行
make lint

# ビルド成果物をクリーン
make clean
```

### インストール
```bash
# ソースからインストール
git clone https://github.com/isseis/go-safe-cmd-runner.git
cd go-safe-cmd-runner
make build

# システムロケーションにバイナリをインストール
sudo install -o root -g root -m 4755 build/runner /usr/local/bin/go-safe-cmd-runner
sudo install -o root -g root -m 0755 build/record /usr/local/bin/go-safe-cmd-record
sudo install -o root -g root -m 0755 build/verify /usr/local/bin/go-safe-cmd-verify
```

## 開発

### 依存関係
- `github.com/pelletier/go-toml/v2` - TOML設定パーシング
- `github.com/joho/godotenv` - 環境ファイル読み込み
- `github.com/oklog/ulid/v2` - 実行追跡と識別のためのULID生成
- `github.com/stretchr/testify` - テストフレームワーク

### テスト
```bash
# すべてのテストを実行
go test -v ./...

# 特定パッケージのテストを実行
go test -v ./internal/runner

# 統合テストを実行
make integration-test
```

### プロジェクト構造
コードベースはGoのベストプラクティスに従います：
- テスト容易性のためのインターフェース駆動設計
- カスタムエラー型による包括的エラー処理
- 広範囲な検証を伴うセキュリティファーストアプローチ
- 明確な境界を持つモジュラーアーキテクチャ

### ULIDによる実行識別
システムは実行追跡にULID（汎用一意字句順ソート可能識別子）を使用：
- **時系列順ソート可能**: ULIDは作成時刻順に自然に並べられる
- **URL安全**: 特殊文字なし、ファイル名やURLに適している
- **コンパクト**: 26文字固定長（UUIDの36文字より短い）
- **衝突耐性**: 単調エントロピーにより同一ミリ秒内でも一意性を保証
- **例**: `01K2YK812JA735M4TWZ6BK0JH9`

## 貢献

このプロジェクトはセキュリティと信頼性を重視しています。貢献する際は：
- セキュリティファーストの設計原則に従う
- 新機能には包括的なテストを追加
- 設定変更に対してはドキュメントを更新
- すべてのセキュリティ検証が適切にテストされていることを確認

質問や貢献については、プロジェクトのイシュートラッカーを参照するか、メンテナにお問い合わせください。
