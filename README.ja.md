# Go Safe Command Runner

特権タスクの委譲と自動バッチ処理のために設計された、包括的なセキュリティ制御を備えたGoによる安全なコマンド実行フレームワークです。

プロジェクトページ: https://github.com/isseis/go-safe-cmd-runner/

## 目次

- [背景](#背景)
- [主要セキュリティ機能](#主要セキュリティ機能)
- [最近のセキュリティ強化](#最近のセキュリティ強化)
- [コア機能](#コア機能)
- [アーキテクチャ](#アーキテクチャ)
- [クイックスタート](#クイックスタート)
- [設定](#設定)
- [セキュリティモデル](#セキュリティモデル)
- [コマンドラインツール](#コマンドラインツール)
- [ビルドとインストール](#ビルドとインストール)
- [開発](#開発)
- [貢献](#貢献)
- [ライセンス](#ライセンス)

## 背景

Go Safe Command Runnerは、以下のような環境における安全なコマンド実行の重要なニーズに対応します：
- 一般ユーザーが特権操作を安全に実行する必要がある場合
- 自動システムがセキュアなバッチ処理機能を必要とする場合
- コマンド実行前にファイルの整合性検証が不可欠な場合
- 環境変数の公開に厳格な制御が必要な場合
- コマンド実行に監査証跡とセキュリティ境界が必要な場合

一般的な用途には、定期バックアップ、システム保守タスク、セキュリティ制御を維持しながら特定の管理操作を非rootユーザーに委譲することなどが含まれます。

## 主要セキュリティ機能

### 多層防御アーキテクチャ
- **事前実行検証**: 設定ファイルと環境ファイルの使用前ハッシュ検証により、悪意のある設定攻撃を防止
- **固定ハッシュディレクトリ**: プロダクションビルドはデフォルトハッシュディレクトリのみを使用し、カスタムハッシュディレクトリ攻撃ベクターを排除
- **セキュア固定PATH**: ハードコードされたセキュアPATH（`/sbin:/usr/sbin:/bin:/usr/bin`）を使用し、PATH操作攻撃を完全に排除
- **リスクベースコマンド制御**: 高リスク操作を自動的にブロックするインテリジェントなセキュリティ評価
- **環境変数分離**: ゼロトラストアプローチによる厳格な許可リストベースのフィルタリング
- **ハイブリッドハッシュエンコーディング**: 自動フォールバック機能を備えた空間効率の高いファイル整合性検証
- **機密データ保護**: パスワード、トークン、APIキーの自動検出と編集

### コマンド実行セキュリティ
- **ユーザー/グループ実行制御**: 包括的な検証を伴う安全なユーザーおよびグループ切り替え
- **権限管理**: 自動権限降下を伴う制御された権限昇格
- **パス検証**: シンボリックリンク攻撃防止を備えたコマンドパス解決
- **出力キャプチャセキュリティ**: 出力ファイルのセキュアなファイル権限（0600）
- **タイムアウト制御**: リソース枯渇攻撃の防止

### 監査とモニタリング
- **ULID実行追跡**: 一意な識別子による時系列順実行追跡
- **マルチハンドラーログ**: 機密データ編集機能を備えたコンソール、ファイル、Slack統合
- **インタラクティブターミナルサポート**: スマートなターミナル検出を伴うカラーコード化された出力
- **包括的監査証跡**: 特権操作とセキュリティイベントの完全なログ記録

## 最近のセキュリティ強化

### ⚠️ 破壊的変更（重要なセキュリティ改善）

最近のバージョンでは、破壊的変更を伴う重要なセキュリティ改善が導入されています：

#### 削除された機能（セキュリティ）
- **`--hash-directory`フラグ**: カスタムハッシュディレクトリ攻撃を防ぐため、runnerから完全に削除
- **カスタムハッシュディレクトリAPI**: 内部APIはプロダクションビルドでカスタムハッシュディレクトリを受け付けない
- **ハッシュディレクトリ設定**: 設定ファイルでのハッシュディレクトリ指定はサポートされない
- **PATH環境変数継承**: 環境変数PATHは親プロセスから継承されない

#### セキュリティ強化機能
1. **事前実行検証**（タスク0021）
   - 読み込み前の設定ファイル検証
   - 使用前の環境ファイル検証
   - 悪意のある設定攻撃の防止
   - 検証失敗時の強制stderr出力

2. **ハッシュディレクトリセキュリティ**（タスク0022）
   - 固定デフォルトハッシュディレクトリ: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - ビルドタグによるプロダクション/テストAPI分離
   - セキュリティ違反の静的解析検出
   - カスタムハッシュディレクトリ攻撃の完全防止

3. **ハイブリッドハッシュエンコーディング**（タスク0023）
   - 空間効率の高い置換+ダブルエスケープエンコーディング
   - 一般的なパスで1.00x膨張率
   - 長いパスに対する自動SHA256フォールバック
   - デバッグ用の人間が読めるハッシュファイル名

4. **出力キャプチャセキュリティ**（タスク0025）
   - 出力ファイルのセキュアなファイル権限（0600）
   - Tee機能（画面+ファイル出力）
   - 権限分離（出力ファイルは実UIDを使用）
   - セキュアな権限でのディレクトリ自動作成

5. **変数展開**（タスク0026）
   - cmdとargsでの`${VAR}`形式の変数展開
   - visited mapによる循環参照検出
   - セキュリティのための許可リスト統合
   - OS環境よりCommand.Envを優先

#### 移行ガイド
- **設定**: TOMLファイルから`hash_directory`設定を削除
- **スクリプト**: スクリプトや自動化から`--hash-directory`フラグを削除
- **開発**: テスト用に`//go:build test`タグでテストAPIを使用
- **PATH依存関係**: 必要な全バイナリが標準システムパス（/sbin、/usr/sbin、/bin、/usr/bin）にあることを確認
- **環境変数**: 環境変数の許可リストを見直して更新

詳細な移行情報については、[検証API文書](docs/verification_api.md)を参照してください。

## コア機能

### ファイル整合性と検証
- **SHA-256ハッシュ検証**: 実行前のすべての実行ファイルと重要ファイルの検証
- **事前実行検証**: 使用前の設定ファイルと環境ファイルの検証
- **ハイブリッドハッシュエンコーディング**: 人間が読めるフォールバック機能を備えた空間効率の高いエンコーディング
- **一元化検証**: 自動権限処理を伴う統一された検証管理
- **グループおよびグローバル検証**: 複数レベルでの柔軟なファイル検証

### コマンド実行
- **バッチ処理**: 依存関係管理を備えた組織化されたグループでのコマンド実行
- **変数展開**: コマンド名と引数での`${VAR}`形式の展開
- **出力キャプチャ**: セキュアな権限でのファイルへのコマンド出力保存
- **バックグラウンド実行**: シグナル処理を伴う長時間実行プロセスのサポート
- **拡張ドライラン**: 包括的なセキュリティ分析を伴う現実的なシミュレーション
- **タイムアウト制御**: コマンド実行の設定可能なタイムアウト
- **ユーザー/グループコンテキスト**: 検証を伴う特定ユーザーでのコマンド実行

### ログとモニタリング
- **マルチハンドラーログ**: 複数の出力先へのログルーティング（コンソール、ファイル、Slack）
- **インタラクティブターミナルサポート**: 視認性を高めたカラーコード化出力
- **スマートなターミナル検出**: ターミナル機能の自動検出
- **カラー制御**: CLICOLOR、NO_COLOR、CLICOLOR_FORCE環境変数のサポート
- **Slack統合**: セキュリティイベントのリアルタイム通知
- **機密データ編集**: 機密情報の自動フィルタリング
- **ULID実行追跡**: 時系列順実行追跡

### ファイル操作
- **セーフファイルI/O**: セキュリティチェック付きのシンボリックリンク対応ファイル操作
- **ハッシュ記録**: 整合性検証用のSHA-256ハッシュ記録
- **検証ツール**: ファイル検証用のスタンドアロンユーティリティ

## アーキテクチャ

システムは関心の明確な分離を伴うモジュラーアーキテクチャに従います：

```
cmd/                    # コマンドラインエントリーポイント
├── runner/            # メインコマンドランナーアプリケーション
├── record/            # ハッシュ記録ユーティリティ
└── verify/            # ファイル検証ユーティリティ

internal/              # コア実装
├── cmdcommon/         # 共有コマンドユーティリティ
├── color/             # ターミナルカラーサポート
├── common/            # 共通ユーティリティとファイルシステム抽象化
├── filevalidator/     # ファイル整合性検証
│   └── encoding/      # ハイブリッドハッシュファイル名エンコーディング
├── groupmembership/   # ユーザー/グループメンバーシップ検証
├── logging/           # Slack統合を備えた高度なログ機能
├── redaction/         # 機密データの自動フィルタリング
├── runner/            # コマンド実行エンジン
│   ├── audit/         # セキュリティ監査ログ
│   ├── bootstrap/     # システム初期化
│   ├── cli/           # コマンドラインインターフェース
│   ├── config/        # 設定管理
│   ├── environment/   # 環境変数処理
│   ├── errors/        # 一元化エラー処理
│   ├── executor/      # コマンド実行ロジック
│   ├── hashdir/       # ハッシュディレクトリセキュリティ
│   ├── output/        # 出力キャプチャ管理
│   ├── privilege/     # 権限管理
│   ├── resource/      # リソース管理（通常/ドライラン）
│   ├── risk/          # リスクベースコマンド評価
│   ├── runnertypes/   # 型定義とインターフェース
│   └── security/      # セキュリティ検証フレームワーク
├── safefileio/        # セキュアファイル操作
├── terminal/          # ターミナル機能検出
└── verification/      # 一元化検証管理
```

## クイックスタート

### 基本的な使用方法

```bash
# 設定ファイルからコマンドを実行
./runner -config config.toml

# ドライランモード（実行なしでプレビュー）
./runner -config config.toml -dry-run

# 設定ファイルの検証
./runner -config config.toml -validate
```

### シンプルな設定例

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER"]

[[groups]]
name = "backup"
description = "システムバックアップ操作"

[[groups.commands]]
name = "database_backup"
description = "データベースのバックアップ"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases"]
output = "backup.sql"  # ファイルに出力を保存
run_as_user = "mysql"
max_risk_level = "medium"
```

## 設定

### 基本設定構造

```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/tmp"
log_level = "info"
skip_standard_paths = true  # システムパスの検証をスキップ
env_allowlist = ["PATH", "HOME", "USER", "LANG"]
verify_files = ["/etc/passwd", "/bin/bash"]

[[groups]]
name = "maintenance"
description = "システム保守タスク"
priority = 1
env_allowlist = ["PATH", "HOME"]  # グローバル許可リストを上書き

[[groups.commands]]
name = "system_check"
cmd = "/usr/bin/systemctl"
args = ["status"]
max_risk_level = "medium"
```

### 変数展開

動的設定に`${VAR}`形式を使用：

```toml
[[groups.commands]]
name = "deploy"
cmd = "${TOOL_DIR}/deploy.sh"
args = ["--config", "${CONFIG_FILE}"]
env = ["TOOL_DIR=/opt/tools", "CONFIG_FILE=/etc/app.conf"]
```

### 出力キャプチャ

コマンド出力をファイルに保存：

```toml
[[groups.commands]]
name = "generate_report"
cmd = "/usr/bin/df"
args = ["-h"]
output = "reports/disk_usage.txt"  # ファイルに出力をTee（0600権限）
```

### リスクベース制御

セキュリティリスク閾値を設定：

```toml
[[groups.commands]]
name = "file_operation"
cmd = "/bin/cp"
args = ["source.txt", "dest.txt"]
max_risk_level = "low"  # 低リスクコマンドのみ許可

[[groups.commands]]
name = "system_admin"
cmd = "/usr/bin/systemctl"
args = ["restart", "nginx"]
max_risk_level = "high"  # 高リスク操作を許可
```

### ユーザーとグループ実行

特定のユーザー/グループコンテキストでコマンドを実行：

```toml
[[groups.commands]]
name = "db_backup"
cmd = "/usr/bin/pg_dump"
args = ["mydb"]
run_as_user = "postgres"
run_as_group = "postgres"
output = "/backups/db.sql"
```

### 環境変数セキュリティ

厳格な許可リストベースの制御：

```toml
[global]
# グローバル許可リスト（全グループのデフォルト）
env_allowlist = ["PATH", "HOME"]

[[groups]]
name = "secure_group"
# 空リストで上書き = 環境変数なし
env_allowlist = []

[[groups]]
name = "web_group"
# カスタムリストで上書き
env_allowlist = ["PATH", "HOME", "WEB_ROOT"]
```

## セキュリティモデル

### ファイル整合性検証

1. **事前実行検証**
   - 読み込み前の設定ファイル検証
   - 使用前の環境ファイル検証
   - 悪意のある設定攻撃の防止

2. **ハッシュディレクトリセキュリティ**
   - 固定デフォルト: `/usr/local/etc/go-safe-cmd-runner/hashes`
   - プロダクション環境ではカスタムディレクトリなし
   - ビルドタグで分離されたテストAPI

3. **ハイブリッドハッシュエンコーディング**
   - 空間効率の高いエンコーディング（1.00x膨張）
   - デバッグ用に人間が読める
   - 自動SHA256フォールバック

4. **検証管理**
   - 一元化検証
   - 自動権限処理
   - グループおよびグローバル検証リスト

### リスクベースセキュリティ制御

- **自動リスク評価**: リスクレベル別のコマンド分類
- **設定可能な閾値**: コマンドごとのリスクレベル制限
- **自動ブロック**: 高リスクコマンドの自動ブロック
- **リスクカテゴリ**:
  - **低**: 基本操作（ls、cat、grep）
  - **中**: ファイル変更（cp、mv）、パッケージ管理
  - **高**: システム管理（systemctl）、破壊的操作
  - **クリティカル**: 権限昇格（sudo、su）- 常にブロック

### 環境分離

- **セキュア固定PATH**: ハードコードされた`/sbin:/usr/sbin:/bin:/usr/bin`
- **PATH継承なし**: PATH操作攻撃の排除
- **許可リストフィルタリング**: 厳格なゼロトラスト環境制御
- **変数展開**: 許可リスト付きのセキュアな`${VAR}`展開
- **Command.Env優先**: 設定がOS環境を上書き

### 権限管理

- **自動降下**: 初期化後の権限降下
- **制御された昇格**: リスク対応権限管理
- **ユーザー/グループ切り替え**: 検証を伴うセキュアなコンテキスト切り替え
- **監査証跡**: 権限変更の完全なログ記録

### 出力キャプチャセキュリティ

- **セキュアな権限**: 0600権限で作成された出力ファイル
- **権限分離**: 出力ファイルは実UIDを使用（run_as_userではない）
- **ディレクトリセキュリティ**: セキュアな権限でのディレクトリ自動作成
- **パス検証**: パストラバーサル攻撃の防止

### ログセキュリティ

- **機密データ編集**: シークレット、トークン、APIキーの自動検出
- **マルチチャンネル通知**: 暗号化されたSlack通信
- **監査証跡保護**: 改ざん耐性のある構造化ログ
- **リアルタイムアラート**: セキュリティ違反の即座の通知

## コマンドラインツール

### メインランナー

```bash
# 基本実行
./runner -config config.toml

# セキュリティ分析付きドライラン
./runner -config config.toml -dry-run

# 設定検証
./runner -config config.toml -validate

# カスタムログ設定
./runner -config config.toml -log-dir /var/log/runner -log-level debug

# カラー制御
CLICOLOR=1 ./runner -config config.toml       # カラー有効
NO_COLOR=1 ./runner -config config.toml       # カラー無効
CLICOLOR_FORCE=1 ./runner -config config.toml # カラー強制

# Slack通知（GSCR_SLACK_WEBHOOK_URLが必要）
GSCR_SLACK_WEBHOOK_URL=https://hooks.slack.com/... ./runner -config config.toml
```

### ハッシュ管理

```bash
# ファイルハッシュの記録（デフォルトハッシュディレクトリ使用）
./record -file /path/to/executable

# 既存ハッシュの強制上書き
./record -file /path/to/file -force

# ファイル整合性の検証（デフォルトハッシュディレクトリ使用）
./verify -file /path/to/file

# 注記: -hash-dirオプションはテスト用のみ利用可能
./record -file /path/to/file -hash-dir /custom/test/hashes
```

## ビルドとインストール

### 前提条件

- Go 1.23以降（slicesパッケージ、range over countに必要）
- golangci-lint（開発用）
- gofumpt（コードフォーマット用）

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

# コードをフォーマット
make fmt

# ビルド成果物をクリーン
make clean

# ベンチマークを実行
make benchmark

# カバレッジレポートを生成
make coverage
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

# デフォルトハッシュディレクトリの作成
sudo mkdir -p /usr/local/etc/go-safe-cmd-runner/hashes
sudo chown root:root /usr/local/etc/go-safe-cmd-runner/hashes
sudo chmod 755 /usr/local/etc/go-safe-cmd-runner/hashes
```

## 開発

### 依存関係

- `github.com/pelletier/go-toml/v2` - TOML設定パーシング
- `github.com/oklog/ulid/v2` - 実行追跡用ULID生成
- `github.com/stretchr/testify` - テストフレームワーク
- `golang.org/x/term` - ターミナル機能検出

### テスト

```bash
# すべてのテストを実行
go test -v ./...

# 特定パッケージのテストを実行
go test -v ./internal/runner

# 統合テストを実行
make integration-test

# Slack通知テストを実行（GSCR_SLACK_WEBHOOK_URLが必要）
make slack-notify-test
make slack-group-notification-test
```

### プロジェクト構造

コードベースはGoのベストプラクティスに従います：
- テスト容易性のための**インターフェース駆動設計**
- カスタムエラー型による**包括的エラー処理**
- 広範囲な検証を伴う**セキュリティファーストアプローチ**
- 明確な境界を持つ**モジュラーアーキテクチャ**
- プロダクション/テストコードの**ビルドタグ分離**

### ULIDによる実行識別

システムはULID（汎用一意字句順ソート可能識別子）を使用：
- **時系列順ソート可能**: 作成時刻順に自然に並べられる
- **URL安全**: 特殊文字なし、ファイル名に適している
- **コンパクト**: 26文字固定長
- **衝突耐性**: 単調エントロピーにより一意性を保証
- **例**: `01K2YK812JA735M4TWZ6BK0JH9`

## スコープ外

このプロジェクトは明示的に以下を提供**しません**：
- コンテナオーケストレーションやDocker統合
- ネットワークセキュリティ機能（ファイアウォール、VPNなど）
- ユーザー認証や認可システム
- WebインターフェースやREST API
- データベース管理機能
- リアルタイム監視やアラートシステム
- クロスプラットフォームGUIアプリケーション
- パッケージ管理やソフトウェアインストール

Unix系環境での包括的なセキュリティ制御を伴うセキュアなコマンド実行に焦点を当てています。

## 貢献

このプロジェクトはセキュリティと信頼性を重視しています。貢献する際は：
- セキュリティファーストの設計原則に従う
- 新機能には包括的なテストを追加
- 設定変更に対してドキュメントを更新
- すべてのセキュリティ検証がテストされていることを確認
- 静的解析ツール（golangci-lint）を使用
- Goのコーディング標準とベストプラクティスに従う

質問や貢献については、プロジェクトのイシュートラッカーを参照してください。

## ライセンス

本プロジェクトはMITライセンスで公開されています。詳細は[LICENSE](./LICENSE)ファイルをご参照ください。
