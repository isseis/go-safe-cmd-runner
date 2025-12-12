# Go Safe Command Runner

特権タスクの委譲と自動バッチ処理のために設計された、包括的なセキュリティ制御を備えたGoによる安全なコマンド実行フレームワークです。

プロジェクトページ: https://github.com/isseis/go-safe-cmd-runner/

## 目次

- [背景](#背景)
- [主要セキュリティ機能](#主要セキュリティ機能)
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

## コア機能

### ファイル整合性と検証
- **SHA-256ハッシュ検証**: 実行前のすべての実行ファイルと重要ファイルの検証
- **事前実行検証**: 使用前の設定ファイルと環境ファイルの検証
- **ハイブリッドハッシュエンコーディング**: 人間が読めるフォールバック機能を備えた空間効率の高いエンコーディング
- **一元化検証**: 自動権限処理を伴う統一された検証管理
- **グループおよびグローバル検証**: 複数レベルでの柔軟なファイル検証

### コマンド実行
- **コマンドテンプレート**: 保守性を高めるパラメータ置換機能を備えた再利用可能なコマンド定義
  - 必須パラメータ: `${param}` - 必ず指定が必要
  - オプショナルパラメータ: `${?param}` - 空の場合は省略
  - 配列パラメータ: `${@param}` - 複数の引数に展開
- **バッチ処理**: 依存関係管理を備えた組織化されたグループでのコマンド実行
- **自動一時ディレクトリ**: グループごとの自動一時ディレクトリ生成とクリーンアップ機能
- **作業ディレクトリ制御**: 固定ディレクトリまたは自動生成一時ディレクトリでの実行
- **`__runner_workdir`変数**: 実行時作業ディレクトリを参照する予約変数
- **変数展開**: コマンド名と引数での`%{var}`形式の展開
- **自動環境変数**: タイムスタンプとプロセス追跡のための自動生成変数
- **出力キャプチャ**: セキュアな権限でのファイルへのコマンド出力保存
- **バックグラウンド実行**: シグナル処理を伴う長時間実行プロセスのサポート
- **拡張ドライラン**: 包括的なセキュリティ分析を伴う現実的なシミュレーション
  - 出力ストリームの分離: 標準出力にドライラン結果、標準エラー出力に実行ログ
  - `--dry-run-format=json`: デバッグ情報を含む機械処理用JSON出力
  - `--dry-run-detail=full`: 最終環境変数とその出所、継承分析を表示
  - `--show-sensitive`: センシティブ情報を平文表示（デバッグ用、使用注意）
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
│   ├── debug/         # デバッグ機能とユーティリティ
│   ├── environment/   # 環境変数処理
│   ├── errors/        # 一元化エラー処理
│   ├── executor/      # コマンド実行ロジック
│   ├── output/        # 出力キャプチャ管理
│   ├── privilege/     # 権限管理
│   ├── resource/      # リソース管理（通常/ドライラン）
│   ├── risk/          # リスクベースコマンド評価
│   ├── runnertypes/   # 型定義とインターフェース
│   ├── security/      # セキュリティ検証フレームワーク
│   └── variable/      # 自動変数の生成と定義
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

詳細な使用方法については、[runner コマンドガイド](docs/user/runner_command.ja.md)を参照してください。

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

TOML形式の設定ファイルでコマンドの実行方法を定義します。設定ファイルは以下の階層構造を持ちます：

- **ルートレベル**: バージョン情報
- **グローバルレベル**: 全グループに適用されるデフォルト設定
- **グループレベル**: 関連するコマンドのグループ化
- **コマンドレベル**: 個別のコマンド設定

### 基本設定例

```toml
version = "1.0"

[global]
timeout = 3600
log_level = "info"
env_allowlist = ["PATH", "HOME", "USER", "LANG"]

[[groups]]
name = "backup"
description = "バックアップ操作"
# workdir未指定 - 自動的に一時ディレクトリが生成される

[[groups.commands]]
name = "database_backup"
cmd = "/usr/bin/mysqldump"
args = ["--all-databases", "--result-file=%{__runner_workdir}/db.sql"]
max_risk_level = "medium"

[[groups]]
name = "maintenance"
description = "システム保守タスク"
workdir = "/tmp/maintenance"  # 固定作業ディレクトリを指定

[[groups.commands]]
name = "system_check"
cmd = "/usr/bin/systemctl"
args = ["status"]
max_risk_level = "medium"
```

### 自動変数

システムは以下の内部変数を自動的に提供します：

- `__runner_datetime`: runner実行開始タイムスタンプ（UTC）を`YYYYMMDDHHmmSS.msec`形式で表現
- `__runner_pid`: runnerのプロセスID
- `__runner_workdir`: グループの作業ディレクトリ（コマンドレベルで利用可能）

これらの変数は`%{変数名}`の形式でコマンドパス、引数、環境変数の値で参照できます：

```toml
[[groups.commands]]
name = "backup_with_timestamp"
cmd = "/usr/bin/tar"
args = ["czf", "/tmp/backup/data-%{__runner_datetime}.tar.gz", "/data"]

[[groups.commands]]
name = "log_execution"
cmd = "/bin/sh"
args = ["-c", "echo 'PID: %{__runner_pid}, Time: %{__runner_datetime}' >> /var/log/executions.log"]
```

**注意**: プレフィックス `__runner_` は予約されており、ユーザー定義の変数では使用できません。

### コマンドテンプレート

コマンドテンプレートを使用すると、パラメータを持つ再利用可能なコマンドパターンを定義でき、設定の重複を減らすことができます：

```toml
# テンプレートの定義
[command_templates.restic_backup]
cmd = "restic"
args = ["${@flags}", "backup", "${path}"]
env = ["RESTIC_REPOSITORY=${repo}"]

# 異なるパラメータでテンプレートを使用
[[groups]]
name = "backup"

[[groups.commands]]
name = "backup_volumes"
template = "restic_backup"

[groups.commands.params]
flags = ["-v", "--exclude-caches"]
path = "/data/volumes"
repo = "/backup/repo"

[[groups.commands]]
name = "backup_database"
template = "restic_backup"

[groups.commands.params]
flags = ["-q"]
path = "/data/database"
repo = "/backup/repo"
```

テンプレートパラメータは3つのタイプをサポートします：
- `${param}`: 必須パラメータ（欠落時はエラー）
- `${?param}`: オプショナルパラメータ（空の場合は省略）
- `${@param}`: 配列パラメータ（複数の引数に展開）

詳細は[コマンドテンプレートガイド](docs/user/command_templates.md)を参照してください。

### グループレベルコマンド許可リスト

グループごとに、ハードコードされたグローバルパターン（`^/bin/.*`, `^/usr/bin/.*`, `^/usr/sbin/.*`, `^/usr/local/bin/.*`）以外の追加コマンドを許可できます：

```toml
[global]
env_import = ["home=HOME"]

[[groups]]
name = "custom_build"
# このグループでのみ許可される追加コマンド
cmd_allowed = [
    "%{home}/bin/custom_tool",
    "/opt/myapp/bin/processor"
]

[[groups.commands]]
name = "run_custom"
cmd = "%{home}/bin/custom_tool"
args = ["--verbose"]
```

**主な特徴**:
- ハードコードされたグローバルパターンまたはグループレベルの `cmd_allowed` リストのいずれかにマッチすればコマンド実行可能
- `cmd_allowed` パスで変数展開（`%{variable}`）をサポート
- 絶対パスのみ許可（相対パスはセキュリティのため拒否）
- 他のセキュリティチェック（パーミッション、リスク評価など）は継続して実行

完全な例は `sample/group_cmd_allowed.toml` を参照してください。

### 詳細な設定方法

設定ファイルの詳細な記述方法については、以下のドキュメントを参照してください：

- [TOML設定ファイル ユーザーガイド](docs/user/toml_config/README.ja.md) - 包括的な設定ガイド
  - [設定ファイルの階層構造](docs/user/toml_config/02_hierarchy.ja.md)
  - [グローバルレベル設定](docs/user/toml_config/04_global_level.ja.md)
  - [グループレベル設定](docs/user/toml_config/05_group_level.ja.md)
  - [コマンドレベル設定](docs/user/toml_config/06_command_level.ja.md)
  - [変数展開機能](docs/user/toml_config/07_variable_expansion.ja.md)
  - [実践的な設定例](docs/user/toml_config/08_practical_examples.ja.md)

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
- **変数展開**: 許可リスト付きのセキュアな`%{var}`展開
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

go-safe-cmd-runnerは3つのコマンドラインツールを提供します：

### runner - メイン実行コマンド

```bash
# 基本実行
./runner -config config.toml

# ドライラン（実行内容の確認）
./runner -config config.toml -dry-run

# 設定検証
./runner -config config.toml -validate
```

詳細は [runner コマンドガイド](docs/user/runner_command.ja.md) を参照してください。

### グループフィルタリング

`--groups` フラグにカンマ区切りでグループ名を渡すことで、必要なグループだけを実行できます。

```bash
# 単一グループ
./runner -config config.toml --groups=build

# 複数グループ
./runner -config config.toml --groups=build,test

# 省略時（全グループ）
./runner -config config.toml
```

選択したグループが `depends_on` を持つ場合は、依存先が自動的に追加され先に実行されます。

```toml
[[groups]]
name = "build"
depends_on = ["preparation"]

[[groups]]
name = "test"
depends_on = ["build"]
```

```bash
./runner -config config.toml --groups=test
# 実行順序: preparation -> build -> test
```

グループ名は環境変数と同じ命名規則に従い、`[A-Za-z_][A-Za-z0-9_]*`（先頭は英字またはアンダースコア、以降は英数字またはアンダースコア）でなければなりません。

### record - ハッシュ記録コマンド

```bash
# ファイルハッシュの記録
./record -file /path/to/executable

# 既存ハッシュの強制上書き
./record -file /path/to/file -force
```

詳細は [record コマンドガイド](docs/user/record_command.ja.md) を参照してください。

### verify - ファイル検証コマンド

```bash
# ファイル整合性の検証
./verify -file /path/to/file
```

詳細は [verify コマンドガイド](docs/user/verify_command.ja.md) を参照してください。

### 包括的なユーザーガイド

詳細な使用方法、設定例、トラブルシューティングについては、[ユーザーガイド](docs/user/README.ja.md) を参照してください。

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
