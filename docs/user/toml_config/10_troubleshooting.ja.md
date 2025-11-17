# 第10章: トラブルシューティング

本章では、設定ファイル作成時によくある問題とその解決方法を紹介します。エラーメッセージの読み方や、デバッグのテクニックを学びましょう。

## 10.1 よくあるエラーと対処法

### 10.1.1 設定ファイルの読み込みエラー

#### エラー例

```
Error: failed to load configuration: toml: line 15: expected key, but got '=' instead
```

#### 原因

TOML ファイルの文法エラー。キーが指定されていない、または不正な形式。

#### 解決方法

```toml
# 誤り: キーがない
= "value"

# 正しい
key = "value"

# 誤り: クォートが閉じていない
name = "unclosed string

# 正しい
name = "closed string"
```

### 10.1.2 バージョン指定エラー

#### エラー例

```
Error: unsupported configuration version: 2.0
```

#### 原因

サポートされていないバージョンが指定されている。

#### 解決方法

```toml
# 誤り: サポートされていないバージョン
version = "2.0"

# 正しい: サポートされているバージョンを使用
version = "1.0"
```

### 10.1.3 必須フィールドの欠落

#### エラー例

```
Error: group 'backup_tasks' is missing required field 'name'
Error: command is missing required field 'cmd'
```

#### 原因

必須フィールド(`name`, `cmd` など)が設定されていない。

#### 解決方法

```toml
# 誤り: name がない
[[groups]]
description = "Backup tasks"

# 正しい: name を追加
[[groups]]
name = "backup_tasks"
description = "Backup tasks"

# 誤り: cmd がない
[[groups.commands]]
name = "backup"
args = ["/data"]

# 正しい: cmd を追加
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/tar"
args = ["-czf", "backup.tar.gz", "/data"]
```

### 10.1.4 環境変数の許可エラー

#### エラー例

```
Error: environment variable 'CUSTOM_VAR' is not allowed by env_allowed
```

#### 原因

使用している環境変数が `env_allowed` に含まれていない。

#### 解決方法

**方法1**: グローバルまたはグループの `env_allowed` に追加

```toml
[global]
env_allowed = ["PATH", "HOME", "CUSTOM_VAR"]  # CUSTOM_VAR を追加
```

**方法2**: Command.Env で定義(推奨)

```toml
# env_allowed に追加不要
[[groups.commands]]
name = "custom_command"
cmd = "${CUSTOM_TOOL}"
args = []
env_vars = ["CUSTOM_TOOL=/opt/tools/mytool"]  # Command.Env で定義
```

### 10.1.5 変数展開エラー

#### エラー例

```
Error: undefined variable: UNDEFINED_VAR
Error: circular variable reference detected: VAR1 -> VAR2 -> VAR1
```

#### 原因

- 変数が定義されていない
- 変数の循環参照

#### 解決方法

**未定義変数の場合**:

```toml
# 誤り: TOOL_DIR が定義されていない
[[groups.commands]]
name = "run_tool"
cmd = "${TOOL_DIR}/mytool"
args = []

# 正しい: env で定義
[[groups.commands]]
name = "run_tool"
cmd = "${TOOL_DIR}/mytool"
args = []
env_vars = ["TOOL_DIR=/opt/tools"]
```

**循環参照の場合**:

```toml
# 誤り: 循環参照
env_vars = [
    "VAR1=${VAR2}",
    "VAR2=${VAR1}",
]

# 正しい: 循環を解消
env_vars = [
    "VAR1=/path/to/dir",
    "VAR2=${VAR1}/subdir",
]
```

### 10.1.6 ファイル検証エラー

#### エラー例

```
Error: file verification failed: /usr/bin/tool: hash mismatch
Error: file verification failed: /opt/app/script.sh: hash file not found
```

#### 原因

- ファイルが改ざんされている
- ハッシュファイルが作成されていない

#### 解決方法

**ハッシュファイルの作成**:

```bash
# record コマンドでハッシュを記録
record -file /usr/bin/tool
record -file /opt/app/script.sh
```

**ファイルが正当に変更された場合**:

```bash
# ハッシュを再記録
record -file /usr/bin/tool
```

### 10.1.7 コマンドパスのエラー

#### エラー例

```
Error: command path must be absolute: ./tool
Error: command not found: mytool
```

#### 原因

- 相対パスが使用されている
- コマンドが存在しない

#### 解決方法

```toml
# 誤り: 相対パス
[[groups.commands]]
name = "run"
cmd = "./mytool"

# 正しい: 絶対パス
[[groups.commands]]
name = "run"
cmd = "/opt/tools/mytool"

# 誤り: PATH 依存だが存在しない
[[groups.commands]]
name = "run"
cmd = "nonexistent-command"

# 正しい: 存在するコマンドの絶対パス
[[groups.commands]]
name = "run"
cmd = "/usr/bin/existing-command"
```

### 10.1.8 タイムアウトエラー

#### エラー例

```
Error: command timeout: exceeded 60 seconds
```

#### 原因

コマンドの実行時間がタイムアウト値を超えた。

#### 解決方法

```toml
# 方法1: グローバルタイムアウトを延長
[global]
timeout = 600  # 60秒 → 600秒

# 方法2: 特定のコマンドのみ延長
[[groups.commands]]
name = "long_running"
cmd = "/usr/bin/long-process"
args = []
timeout = 3600  # このコマンドのみ 1時間
```

### 10.1.9 権限エラー

#### エラー例

```
Error: permission denied: /var/secure/data
Error: failed to change user: operation not permitted
```

#### 原因

- ファイルやディレクトリへのアクセス権限がない
- ユーザー変更が許可されていない

#### 解決方法

**ファイル権限の場合**:

```bash
# ファイル権限を確認
ls -la /var/secure/data

# 適切な権限を設定
sudo chmod 644 /var/secure/data
sudo chown user:group /var/secure/data
```

**ユーザー変更の場合**:

```toml
# run_as_user を使用するには root 権限が必要
# go-safe-cmd-runner を root または適切な権限で実行
```

```bash
# root 権限で実行
sudo go-safe-cmd-runner -file config.toml
```

### 10.1.10 リスクレベル超過エラー

#### エラー例

```
Error: command risk level exceeds maximum: command risk=medium, risk_level=low
```

#### 原因

コマンドのリスクレベルが `risk_level` を超えている。

#### 解決方法

```toml
# 方法1: risk_level を引き上げ
[[groups.commands]]
name = "risky_command"
cmd = "/bin/rm"
args = ["-rf", "/tmp/data"]
risk_level = "medium"  # low → medium に変更

# 方法2: より安全なコマンドに変更
[[groups.commands]]
name = "safer_command"
cmd = "/bin/rm"
args = ["/tmp/data/specific-file.txt"]  # -rf を削除
risk_level = "low"
```

## 10.2 設定検証方法

### 10.2.1 文法チェック

設定ファイルの文法を検証:

```bash
# 設定ファイルの読み込みテスト
go-safe-cmd-runner --validate config.toml

# ドライランで実行前検証
go-safe-cmd-runner --dry-run --file config.toml
```

### 10.2.2 段階的な検証

複雑な設定は段階的に検証:

```toml
# ステップ1: 最小構成
version = "1.0"

[[groups]]
name = "test"

[[groups.commands]]
name = "simple"
cmd = "/bin/echo"
args = ["test"]
```

```bash
# 実行して基本動作を確認
go-safe-cmd-runner -file minimal.toml
```

```toml
# ステップ2: 変数展開を追加
[[groups.commands]]
name = "with_variables"
cmd = "/bin/echo"
args = ["Value: ${VAR}"]
env_vars = ["VAR=hello"]
```

```bash
# 変数展開の動作を確認
go-safe-cmd-runner -file with-vars.toml
```

### 10.2.3 ログレベルの活用

デバッグ時は詳細なログを有効化: `-log-level debug`

出力例:
```
[DEBUG] Loading configuration from: config.toml
[DEBUG] Parsed version: 1.0
[DEBUG] Global timeout: 300
[DEBUG] Processing group: backup_tasks
[DEBUG] Expanding variables in command: backup_database
[DEBUG] Variable BACKUP_DIR expanded to: /var/backups
[DEBUG] Executing: /usr/bin/pg_dump --all-databases
[INFO] Command completed successfully
```

## 10.3 デバッグ手法

### 10.3.1 エコーコマンドでの変数確認

変数が正しく展開されているか確認:

```toml
# デバッグ用コマンド
[[groups.commands]]
name = "debug_variables"
cmd = "/bin/echo"
args = [
    "TOOL_DIR=${TOOL_DIR}",
    "CONFIG=${CONFIG}",
    "ENV=${ENV_TYPE}",
]
env_vars = [
    "TOOL_DIR=/opt/tools",
    "CONFIG=/etc/app/config.yml",
    "ENV_TYPE=production",
]
output_file = "debug-vars.txt"
```

実行後、`debug-vars.txt` を確認:
```
TOOL_DIR=/opt/tools CONFIG=/etc/app/config.yml ENV=production
```

### 10.3.2 出力キャプチャでの診断

コマンド出力を保存して詳細を確認:

```toml
[[groups.commands]]
name = "diagnose"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp.service"]
output_file = "service-status.txt"
```

実行後、出力ファイルを確認:
```bash
cat service-status.txt
```

### 10.3.3 個別コマンドのテスト

問題のあるコマンドを個別にテスト:

```toml
# 問題のあるコマンドのみを含むテスト設定
version = "1.0"

[[groups]]
name = "test_single_command"

[[groups.commands]]
name = "problematic_command"
cmd = "/usr/bin/tool"
args = ["--option", "value"]
env_vars = ["CUSTOM_VAR=test"]
```

### 10.3.4 ドライランの活用

実際に実行せずに動作を確認:

```bash
# ドライランで実行計画を表示
go-safe-cmd-runner --dry-run --file config.toml
```

出力例:
```
[DRY RUN] Would execute: /usr/bin/pg_dump --all-databases
[DRY RUN] Working directory: /var/backups
[DRY RUN] Timeout: 600 seconds
[DRY RUN] Environment variables: PATH=/usr/bin, DB_USER=postgres
```

### 10.3.5 権限の確認

権限関連の問題を診断:

```toml
# 権限確認用コマンド
[[groups.commands]]
name = "check_permissions"
cmd = "/usr/bin/id"
args = []
output_file = "current-user.txt"

[[groups.commands]]
name = "check_file_access"
cmd = "/bin/ls"
args = ["-la", "/path/to/file"]
output_file = "file-permissions.txt"
```

### 10.3.6 環境変数の確認

環境変数の状態を診断:

```toml
[[groups.commands]]
name = "dump_env"
cmd = "/usr/bin/env"
args = []
output_file = "environment.txt"
```

## 10.4 パフォーマンス問題

### 10.4.1 起動が遅い

#### 原因

- 大量のファイル検証
- 重い初期化処理

#### 解決方法

```toml
# 標準パスの検証をスキップ
[global]
verify_standard_paths = false

# 必要最小限のファイルのみ検証
verify_files = [
    "/opt/app/bin/critical-tool",
]
```

### 10.4.2 実行が遅い

#### 原因

- タイムアウトが長すぎる
- 不要な出力キャプチャ

#### 解決方法

```toml
# 適切なタイムアウトを設定
[[groups.commands]]
name = "quick_command"
cmd = "/bin/echo"
args = ["test"]
timeout = 10  # 短いコマンドには短いタイムアウト

# 不要な出力キャプチャを削除
[[groups.commands]]
name = "simple_command"
cmd = "/bin/echo"
args = ["Processing..."]
# output を指定しない
```

## 10.5 よくある質問 (FAQ)

### Q1: 環境変数が展開されない

**Q**: `${HOME}` が展開されず、そのまま文字列として扱われる。

**A**: 環境変数は `env_allowed` に含めるか、`Command.Env` で定義してください。

```toml
# 方法1: env_allowed に追加
[global]
env_allowed = ["PATH", "HOME"]

# 方法2: Command.Env で定義(推奨)
[[groups.commands]]
name = "test"
cmd = "/bin/echo"
args = ["${MY_HOME}"]
env_vars = ["MY_HOME=/home/user"]
```

### Q2: コマンドが見つからない

**Q**: `command not found` エラーが発生する。

**A**: 絶対パスを使用するか、PATH が正しく設定されているか確認してください。

```toml
# 推奨: 絶対パス
cmd = "/usr/bin/tool"

# または: PATH を確認
[global]
env_allowed = ["PATH"]
```

### Q3: ファイル検証が失敗する

**Q**: ハッシュ検証でエラーが発生する。

**A**: ハッシュファイルを作成または更新してください。

```bash
# ハッシュファイルの作成
record -file config.toml
record -file /usr/bin/tool
```

### Q4: タイムアウトエラーが頻発する

**Q**: 多くのコマンドでタイムアウトが発生する。

**A**: タイムアウト値を延長するか、コマンドを見直してください。

```toml
# グローバルタイムアウトを延長
[global]
timeout = 1800  # 30分

# または特定のコマンドのみ延長
[[groups.commands]]
name = "long_process"
cmd = "/usr/bin/process"
timeout = 3600  # 1時間
```

### Q5: 権限エラーが発生する

**Q**: `permission denied` エラーが発生する。

**A**: 適切な権限で go-safe-cmd-runner を実行してください。

```bash
# root 権限が必要な場合
sudo go-safe-cmd-runner -file config.toml

# または設定で run_as_user を使用
```

```toml
[[groups.commands]]
name = "privileged_op"
cmd = "/usr/bin/privileged-tool"
args = []
run_as_user = "root"
```

## 10.6 サポートとヘルプ

### コミュニティリソース

- **ドキュメント**: 公式ドキュメントを参照
- **Issue トラッカー**: GitHub Issues でバグ報告や質問
- **サンプル設定**: `sample/` ディレクトリの設定例を参照

### デバッグ情報の収集

問題報告時には以下の情報を含めてください:

```bash
# バージョン情報
go-safe-cmd-runner --version

# 設定ファイル(機密情報を除く)
cat config.toml

# エラーログ(デバッグレベル)
go-safe-cmd-runner --log-level=debug --file config.toml 2>&1 | tee debug.log
```

## まとめ

本章では以下のトラブルシューティング手法を学びました:

1. **よくあるエラー**: 設定ファイル、環境変数、変数展開、ファイル検証などのエラーと対処法
2. **設定検証**: 文法チェック、段階的検証、ログレベル活用
3. **デバッグ手法**: エコーコマンド、出力キャプチャ、ドライラン、権限確認
4. **パフォーマンス**: 起動・実行速度の改善
5. **FAQ**: よくある質問と回答

これらの知識を活用して、問題を迅速に診断・解決できるようになります。

## 次のステップ

付録では、パラメータ一覧表、サンプル設定ファイル集、用語集を提供します。リファレンスとして活用してください。
