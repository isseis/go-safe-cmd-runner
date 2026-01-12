# サンプル設定ファイル

このディレクトリには、go-safe-cmd-runnerのさまざまな機能を実装例として示す設定ファイルが含まれています。

## テンプレートインクルード機能

### テンプレートファイル

- **`templates_backup_commands.toml`**: 再利用可能なバックアップコマンドテンプレート (restic, tar)
- **`templates_docker_commands.toml`**: 再利用可能なDockerコマンドテンプレート (run, exec, compose)

### インクルード機能を使用するメイン設定

- **`includes_example.toml`**: テンプレートインクルード機能を使用してコマンドを編成する方法を実装例として示します

ワークフロー例：
```bash
# 1. インクルード機能を含む設定を検証します
safe-cmd-runner runner -c sample/includes_example.toml --dry-run

# 2. すべてのファイル（メイン設定＋インクルードされたテンプレート）のハッシュを記録します
safe-cmd-runner record -c sample/includes_example.toml -o /tmp/hashes/

# 3. インクルードされたファイルのテンプレートを使用してコマンドを実行します
safe-cmd-runner runner -c sample/includes_example.toml -g backup_tasks -d /tmp/hashes/ -r backup-run-001
```

## コマンドテンプレート

- **`command_template_example.toml`**: 基本的なコマンドテンプレート使用例
- **`starter.toml`**: シンプルなスターター設定

## 変数展開

- **`variable_expansion_basic.toml`**: 基本的な変数展開の実装例
- **`variable_expansion_advanced.toml`**: 高度な変数展開パターン
- **`variable_expansion_security.toml`**: セキュリティ重視の変数例
- **`vars_env_separation_e2e.toml`**: vars と env_vars の分離を実装例として示します

## 環境変数

- **`auto_env_example.toml`**: 自動環境変数インポート実装例
- **`auto_env_group.toml`**: グループレベルの環境変数実装例
- **`dot.env.sample`**: .envファイル形式のサンプル

## 出力キャプチャ

- **`output_capture_basic.toml`**: 基本的な出力キャプチャ設定
- **`output_capture_advanced.toml`**: 高度な出力キャプチャ（サイズ制限付き）
- **`output_capture_security.toml`**: 出力キャプチャのセキュリティ考慮事項

## リスクベースの制御

- **`risk-based-control.toml`**: リスクレベル設定と制御を実装例として示します

## その他の機能

- **`timeout_examples.toml`**: タイムアウト設定の実装例
- **`workdir_examples.toml`**: 作業ディレクトリ設定の実装例
- **`comprehensive.toml`**: 複数の機能を実装例として示す包括的な例

## テスト用設定

テスト目的で使用されるファイル：
- `auto_env_test.toml`
- `group_cmd_allowed.toml`
- `output_capture_error_test.toml`
- `output_capture_single_error.toml`
- `output_capture_too_large_error.toml`
- `slack-group-notification-test.toml`
- `slack-notify.toml`
- `variable_expansion_test.toml`
