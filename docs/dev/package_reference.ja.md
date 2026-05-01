# パッケージ構造リファレンス

本ドキュメントは、このコードベースにおけるパッケージ構造の詳細なリファレンスを提供する。

## ディレクトリ構造

- `cmd/`: コマンドライン エントリーポイント
  - `runner/`: メインのコマンドランナーアプリケーション
  - `record/`: ハッシュ記録ユーティリティ
  - `verify/`: ファイル検証ユーティリティ
- `internal/`: コア実装
  - `ansicolor/`: ターミナルカラーサポート（ANSI エスケープコード）
  - `arm64util/`: ARM64 命令デコードの共通ユーティリティ
  - `cmdcommon/`: 共通コマンドユーティリティ
  - `common/`: 共通ユーティリティとファイルシステム抽象化
  - `dynlib/`: 動的ライブラリ依存解析
    - `elfdynlib/`: ELF バイナリの動的ライブラリ依存解析
    - `machodylib/`: Mach-O バイナリの動的ライブラリ依存解析
  - `fileanalysis/`: 統合ファイル解析レコード（ハッシュ、syscall、シンボル、shebang）
  - `filevalidator/`: ファイル整合性検証
    - `pathencoding/`: ハイブリッドハッシュファイル名エンコーディング
  - `groupmembership/`: ユーザー/グループメンバーシップ検証
  - `libccache/`: libc syscall ラッパーシンボルのキャッシュとマッチング
  - `logging/`: Slack 連携を含む高度なロギングシステム
  - `redaction/`: 機密データの自動フィルタリング
  - `runner/`: コマンド実行エンジン
    - `audit/`: セキュリティ監査ログ
    - `bootstrap/`: システム初期化とブートストラップ
    - `cli/`: コマンドラインインターフェース管理
    - `config/`: 設定管理
    - `debuginfo/`: デバッグ機能とユーティリティ
    - `environment/`: 環境変数の処理とフィルタリング
    - `executor/`: コマンド実行ロジック
    - `output/`: 出力パスの検証とセキュリティ
    - `privilege/`: 特権管理
    - `resource/`: 統合リソース管理（通常/ドライラン）
    - `risk/`: リスクベースのコマンド評価
    - `runerrors/`: 集中型エラーハンドリング
    - `runnertypes/`: 型定義とインターフェース
    - `security/`: セキュリティバリデーションフレームワーク
    - `variable/`: 自動変数生成と定義
  - `safefileio/`: シンボリックリンク保護を含む安全なファイル操作
  - `security/`: バイナリセキュリティ解析フレームワーク
    - `binaryanalyzer/`: バイナリ解析用の共通インターフェースと型
    - `elfanalyzer/`: ELF バイナリのネットワーク機能と syscall 検出
    - `machoanalyzer/`: Mach-O バイナリのネットワーク機能検出
  - `shebang/`: shebang 行のパースとインタープリタパス解決
  - `terminal/`: ターミナル機能検出とインタラクティブ UI サポート
  - `verification/`: 集中型ファイル検証管理（実行前検証、パス解決）
- `docs/`: 要件とアーキテクチャ設計書を含むプロジェクトドキュメント

## パッケージの責務

### コマンドラインツール（`cmd/`）

- **`runner/`**: TOML の設定ファイルに基づいてコマンドを実行するメインアプリケーション
- **`record/`**: 整合性検証用のハッシュファイルを生成するユーティリティ
- **`verify/`**: 記録されたハッシュに対してファイルの整合性を検証するユーティリティ

### コアパッケージ（`internal/`）

#### ファイル操作
- **`safefileio/`**: シンボリックリンク攻撃防止を含む安全なファイル I/O 操作
- **`filevalidator/`**: ハッシュ検証によるファイル整合性の確認
- **`filevalidator/pathencoding/`**: 自動フォールバックを含むハイブリッドハッシュファイル名エンコーディング
- **`verification/`**: 実行前ファイル検証の集中管理

#### バイナリ解析
- **`security/`**: バイナリセキュリティ解析フレームワーク
  - **`binaryanalyzer/`**: バイナリアナライザー間で共有される共通インターフェースと型
  - **`elfanalyzer/`**: ELF バイナリ解析 — ネットワーク機能検出（socket/connect シンボル）、危険な syscall パターン（PROT_EXEC を伴う mprotect/pkey_mprotect）、静的 syscall 番号抽出
  - **`machoanalyzer/`**: Mach-O バイナリのネットワーク機能検出
- **`dynlib/`**: 動的ライブラリ依存解析
  - **`elfdynlib/`**: ELF バイナリの動的ライブラリ依存解析（DT_NEEDED、RPATH、RUNPATH）
  - **`machodylib/`**: Mach-O バイナリの動的ライブラリ依存解析
- **`fileanalysis/`**: ハッシュ、syscall、シンボル、shebang の結果を統合したファイル解析レコード
- **`libccache/`**: libc syscall ラッパーシンボルのキャッシュとマッチング
- **`arm64util/`**: `elfanalyzer` および関連パッケージが使用する ARM64 命令デコードの共通ユーティリティ
- **`shebang/`**: shebang 行のパースとインタープリタパス解決

#### コマンド実行
- **`runner/`**: コアのコマンド実行エンジン
  - **`executor/`**: 出力ハンドリングを含むコマンド実行
  - **`config/`**: TOML 設定の読み込みとバリデーション
  - **`runnertypes/`**: 共通の型定義とインターフェース
  - **`environment/`**: 環境変数の処理とフィルタリング
  - **`variable/`**: 自動変数生成

#### セキュリティ
- **`runner/security/`**: セキュリティバリデーションフレームワーク
- **`runner/audit/`**: セキュリティ監査ログ
- **`runner/privilege/`**: 特権管理
- **`runner/risk/`**: リスクベースのコマンド評価
- **`runner/output/`**: 出力パスの検証とセキュリティ
- **`groupmembership/`**: ユーザー/グループメンバーシップ検証

#### ユーザーインターフェース
- **`terminal/`**: ターミナル機能検出
- **`ansicolor/`**: ターミナルカラーサポート（ANSI エスケープコード）
- **`runner/cli/`**: コマンドラインインターフェース管理
- **`logging/`**: Slack 連携を含む高度なロギング

#### ユーティリティ
- **`common/`**: 共通ユーティリティとファイルシステム抽象化
- **`cmdcommon/`**: 共通コマンドユーティリティ
- **`redaction/`**: 機密データの自動フィルタリング
- **`runner/debuginfo/`**: デバッグ機能
- **`runner/runerrors/`**: 集中型エラーハンドリング
- **`runner/resource/`**: 統合リソース管理（通常/ドライランモード）
- **`runner/bootstrap/`**: システム初期化とブートストラップ

## 主要な設計パターン

- **関心の分離**: 各パッケージは単一の責務を持つ
- **インターフェースベースの設計**: テスタビリティのためにインターフェースを多用する（例: `CommandExecutor`、`FileSystem`、`OutputWriter`）
- **セキュリティ優先**: パスの検証、コマンドインジェクション防止、特権分離
- **エラーハンドリング**: 包括的なエラー型とバリデーション
