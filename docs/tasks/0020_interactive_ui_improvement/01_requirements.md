# 対話的UI改善 要件定義書

## 1. プロジェクト概要

### 1.1 目的
runner コマンドの対話的利用時におけるユーザビリティを改善し、エラーメッセージや状況表示をより理解しやすく、実用的なものにする。

### 1.2 背景
現在のrunner コマンドは、エラー発生時に以下のような技術的で冗長なメッセージを表示している：
```
Error: required_argument_missing - Config file path is required (run_id: 01K3M7QQKAK1XGD5SAM87SJ0S6)
```

これらのメッセージは：
- 機械的で理解しにくい
- 情報量が多すぎて重要な部分が埋もれる
- 対話的利用時の体験を損なう

### 1.3 対象範囲
- 対話的利用時のエラーメッセージ改善
- 非対話的利用（CI/CD環境等）への影響の最小化
- 既存のログシステムとの統合

## 2. 機能要件

### 2.1 対話モード検知
- システムがターミナル環境で対話的に実行されているかを自動判定する
- CI環境など非対話的環境では従来通りの詳細メッセージを維持する

### 2.2 簡潔なエラーメッセージ
対話的利用時には、以下の原則に従ったメッセージを表示する：

#### 2.2.1 簡潔性
- 技術的な詳細（run_id、component等）は表示しない
- ユーザーが理解しやすい言葉で表現する
- 1-2行以内の簡潔なメッセージとする

#### 2.2.2 実用性
- 問題の内容を明確に説明する
- 可能な場合は具体的な解決方法を含む
- 使用例やサンプルコマンドを提示する
- エラーの具体的詳細を含む：
  - 構文エラーの場合：行番号、カラム番号、具体的なエラー内容
  - ファイルアクセスエラーの場合：ファイル名、アクセス理由（設定ファイル、チェックサム、実行ファイル等）
  - シンボリックリンクが関わる場合：元のファイル名と解決後のファイル名の両方

#### 2.2.3 メッセージ例
```
現在: Error: required_argument_missing - Config file path is required (run_id: 01K3M7...)
改善: Config file path is required. Usage: runner -config config.toml

現在: Error: config_parsing_failed - TOML syntax error at line 15 (component: config_loader, run_id: ...)
改善: Config file parsing failed at line 15: expected key but found ']'. Check TOML syntax

現在: Error: file_access_failed - Permission denied: /var/log/runner.log (component: logger, run_id: ...)
改善: Cannot access log file '/var/log/runner.log': permission denied. Check file permissions or specify different log path

現在: Error: file_access_failed - Permission denied: /opt/app/real_config.toml (component: config_loader, run_id: ...)
改善: Cannot access config file '/home/user/config.toml' -> '/opt/app/real_config.toml': permission denied. Check file permissions

現在: Error: config_parsing_failed - Invalid key 'invalid_key' at line 8, column 12 (component: config_loader, run_id: ...)
改善: Config file parsing failed at line 8, column 12: invalid key 'invalid_key'. Check TOML syntax

現在: Error: file_access_failed - No such file or directory: /tmp/checksum.sha256 (component: file_validator, run_id: ...)
改善: Cannot access checksum file '/tmp/checksum.sha256': file not found. Specify valid checksum file path
```

### 2.3 詳細情報の管理
- 技術的な詳細情報（run_id、component、完全なエラーメッセージ等）は常にログファイルに記録する
- 対話的利用時でも、必要に応じてログファイルを参照できるように案内する

### 2.4 メッセージ出力制御
既存の `--log-level` オプションを流用して、対話的利用時のメッセージ出力を制御する。

#### 2.4.1 メッセージ出力制御
- メッセージの詳細度は一定に保つ（例：Config file parsing failed at line 15: expected key but found ']'. Check TOML syntax）
- `--log-level` で指定されたレベル以上のメッセージのみを出力する
- メッセージの内容は変更せず、出力するかどうかのみを制御する

#### 2.4.2 出力制御例
```
--log-level=error:
（エラーレベルのメッセージのみ出力）

--log-level=warn:
（警告レベル以上のメッセージを出力）

--log-level=info: (デフォルト)
（情報レベル以上のメッセージを出力）

--log-level=debug:
（デバッグレベル以上の全メッセージを出力）
```

#### 2.4.3 設計方針
- 非対話的環境では既存の動作を維持（常に全メッセージを出力）
- 既存のデフォルト `--log-level=info` を維持
- ログファイル出力とUI表示の両方で同一のログレベル設定を適用
- 既存のログレベル概念と一貫性を保つ

### 2.5 カラー表示（オプション機能）
- ANSIエスケープシーケンスを使用したシンプルなカラー表示を検討
- エラーメッセージを赤色で表示するなどの基本的な視覚的改善
- カラー表示の無効化オプションを提供

## 3. 非機能要件

### 3.1 互換性
- 既存の非対話的利用に影響を与えない
- ログフォーマットの変更は最小限に留める
- 既存のエラーハンドリングロジックを維持

### 3.2 パフォーマンス
- 対話モード検知のオーバーヘッドを最小限に抑える
- メッセージ表示の遅延を避ける

### 3.3 メンテナンス性
- エラータイプと表示メッセージのマッピングを管理しやすい構造にする
- 新しいエラータイプの追加が容易である

### 3.4 多言語対応の考慮
- 今回のスコープでは英語メッセージのみを対象とする
- 将来的な多言語対応を妨げない設計とする

## 4. 制約条件

### 4.1 UI の制約
- 単純なテキスト出力に留める
- プログレスバーや画面書き換えを伴う高度なテキストUIは対象外
- インタラクティブな修正提案機能は不要

### 4.2 技術的制約
- 既存のログシステム（`internal/logging`）を活用する
- Go 1.23.10の標準ライブラリを基本とする
- 外部依存関係の追加は最小限に抑える

### 4.3 スコープ外
- 多言語対応
- 設定ファイルによるメッセージのカスタマイズ
- 複雑なインタラクティブ機能

## 5. 成功基準

### 5.1 ユーザビリティ
- エラーメッセージの理解しやすさが向上している
- 問題解決に必要な情報が適切に提供されている
- メッセージの長さが適切である（1-2行程度）

### 5.2 技術的品質
- 既存のテストが全て通過する
- 新機能に対する適切なテストが追加されている
- 非対話的環境での動作に影響がない

### 5.3 保守性
- コードの複雑度が適切に管理されている
- 新しいエラータイプの追加が容易である
- ドキュメントが適切に更新されている

## 6. 依存関係

### 6.1 既存システム
- `internal/logging` パッケージ
- 既存のエラーハンドリングシステム
- 設定ファイル読み込み機能

### 6.2 外部要素
- ターミナル環境の検知ライブラリ（検討）
- CI/CD環境変数
- ログファイルシステム
