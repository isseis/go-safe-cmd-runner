# 付録

## 付録A: パラメータ一覧表

### A.1 ルートレベルパラメータ

| パラメータ | 型 | 必須 | デフォルト値 | 説明 |
|-----------|-----|------|------------|------|
| version | string | ✓ | なし | 設定ファイルのバージョン(現在: "1.0") |

### A.2 グローバルレベルパラメータ ([global])

| パラメータ | 型 | 必須 | デフォルト値 | 説明 |
|-----------|-----|------|------------|------|
| timeout | int | - | システムデフォルト | コマンド実行のタイムアウト(秒) |
| workdir | string | - | 実行ディレクトリ | 作業ディレクトリの絶対パス |
| verify_standard_paths | bool | - | true | 標準パスの検証を有効化 |
| env_allowed | []string | - | [] | 環境変数の許可リスト |
| verify_files | []string | - | [] | 検証対象ファイルのリスト |
| output_size_limit | int64 | - | 10485760 | 出力サイズ上限(バイト) |

### A.3 グループレベルパラメータ ([[groups]])

| パラメータ | 型 | 必須 | デフォルト値 | 説明 |
|-----------|-----|------|------------|------|
| name | string | ✓ | なし | グループ名(一意) |
| description | string | - | "" | グループの説明 |
| priority | int | - | 0 | 実行優先度(小さいほど優先) |
| workdir | string | - | 自動生成 | 作業ディレクトリ(未指定時は一時ディレクトリを自動生成) |
| verify_files | []string | - | [] | 検証対象ファイル(グローバルに追加) |
| env_allowed | []string | - | nil(継承) | 環境変数許可リスト(継承モード参照) |

### A.4 コマンドレベルパラメータ ([[groups.commands]])

| パラメータ | 型 | 必須 | デフォルト値 | 説明 |
|-----------|-----|------|------------|------|
| name | string | ✓ | なし | コマンド名(グループ内で一意) |
| description | string | - | "" | コマンドの説明 |
| cmd | string | ✓ | なし | 実行するコマンド(絶対パスまたはPATH上) |
| args | []string | - | [] | コマンドの引数 |
| env | []string | - | [] | 環境変数("KEY=VALUE"形式) |
| workdir | string | - | グループ設定 | 作業ディレクトリ(グループ設定をオーバーライド) |
| timeout | int | - | グローバル設定 | タイムアウト(グローバルをオーバーライド) |
| run_as_user | string | - | "" | 実行ユーザー |
| run_as_group | string | - | "" | 実行グループ |
| risk_level | string | - | "low" | 最大リスクレベル(low/medium/high) |
| output_file | string | - | "" | 標準出力の保存先ファイルパス |

### A.5 環境変数継承モード

| モード | 条件 | 動作 |
|--------|------|------|
| 継承 (inherit) | env_allowed が未定義(nil) | グローバル設定を継承 |
| 明示 (explicit) | env_allowed に値が設定 | 設定値のみを使用(グローバル無視) |
| 拒否 (reject) | env_allowed = [] (空配列) | 全ての環境変数を拒否 |

### A.6 リスクレベル

| レベル | 値 | 説明 | 例 |
|--------|-----|------|-----|
| 低リスク | "low" | 読み取り専用操作 | cat, ls, grep, echo |
| 中リスク | "medium" | ファイル作成・変更 | tar, cp, mkdir, wget |
| 高リスク | "high" | システム変更・権限昇格 | apt-get, systemctl, rm -rf |

## 付録B: サンプル設定ファイル集

### B.1 最小構成

```toml
version = "1.0"

[[groups]]
name = "minimal"

[[groups.commands]]
name = "hello"
cmd = "/bin/echo"
args = ["Hello, World!"]
```

### B.2 基本的なバックアップ

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/var/backups"
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "daily_backup"

[[groups.commands]]
name = "backup_data"
cmd = "/bin/tar"
args = ["-czf", "data-backup.tar.gz", "/opt/data"]

[[groups.commands]]
name = "backup_config"
cmd = "/bin/tar"
args = ["-czf", "config-backup.tar.gz", "/etc/myapp"]
```

### B.3 セキュアな設定

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/opt/secure"
verify_standard_paths = true
env_allowed = ["PATH"]
verify_files = []  # コマンドは自動検証される

[[groups]]
name = "secure_backup"
verify_files = ["/opt/secure/config/backup.conf"]  # 追加ファイルのみ指定

[[groups.commands]]
name = "backup"
cmd = "/opt/secure/bin/backup-tool"  # 自動的に検証される
args = ["--encrypt", "--output", "backup.enc"]
risk_level = "medium"
```

### B.4 変数展開の活用

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "HOME", "APP_DIR", "ENV_TYPE"]

[[groups]]
name = "deployment"

[[groups.commands]]
name = "deploy"
cmd = "${APP_DIR}/bin/deploy"
args = ["--env", "${ENV_TYPE}", "--config", "${APP_DIR}/config/${ENV_TYPE}.yml"]
env_vars = [
    "APP_DIR=/opt/myapp",
    "ENV_TYPE=production",
]
```

### B.5 権限管理

```toml
version = "1.0"

[global]
timeout = 600
env_allowed = ["PATH"]

[[groups]]
name = "system_maintenance"

[[groups.commands]]
name = "check_status"
cmd = "/usr/bin/systemctl"
args = ["status", "myapp"]
risk_level = "low"

[[groups.commands]]
name = "restart_service"
cmd = "/usr/bin/systemctl"
args = ["restart", "myapp"]
run_as_user = "root"
risk_level = "high"
```

### B.6 出力キャプチャ

```toml
version = "1.0"

[global]
workdir = "/var/reports"
output_size_limit = 10485760

[[groups]]
name = "system_report"

[[groups.commands]]
name = "disk_usage"
cmd = "/bin/df"
args = ["-h"]
output_file = "disk-usage.txt"

[[groups.commands]]
name = "memory_usage"
cmd = "/usr/bin/free"
args = ["-h"]
output_file = "memory-usage.txt"
```

### B.7 複数環境対応

```toml
version = "1.0"

[global]
env_allowed = ["PATH", "APP_BIN", "CONFIG_DIR", "ENV_TYPE", "DB_URL"]

# 開発環境
[[groups]]
name = "dev_deploy"
priority = 1

[[groups.commands]]
name = "run_dev"
cmd = "${APP_BIN}"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml", "--db", "${DB_URL}"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "CONFIG_DIR=/etc/app",
    "ENV_TYPE=development",
    "DB_URL=postgresql://localhost/dev_db",
]

# 本番環境
[[groups]]
name = "prod_deploy"
priority = 2

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = ["--config", "${CONFIG_DIR}/${ENV_TYPE}.yml", "--db", "${DB_URL}"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "CONFIG_DIR=/etc/app",
    "ENV_TYPE=production",
    "DB_URL=postgresql://prod-db/prod_db",
]
run_as_user = "appuser"
risk_level = "high"
```

## 付録C: 用語集

### C.1 一般用語

**TOML (Tom's Obvious, Minimal Language)**
: 人間が読み書きしやすい設定ファイル形式。明確な構文とデータ型を持つ。

**絶対パス (Absolute Path)**
: ルートディレクトリ(`/`)から始まる完全なファイルパス。例: `/usr/bin/tool`

**相対パス (Relative Path)**
: 現在のディレクトリからの相対的なファイルパス。例: `./tool`, `../bin/tool`

**環境変数 (Environment Variable)**
: オペレーティングシステムが提供する動的な値のペア(KEY=VALUE)。

**タイムアウト (Timeout)**
: コマンドの最大実行時間。この時間を超えるとコマンドは強制終了される。

### C.2 設定関連用語

**グローバル設定 (Global Configuration)**
: 全てのグループとコマンドに適用される共通設定。`[global]` セクションで定義。

**グループ (Group)**
: 関連するコマンドをまとめる論理的な単位。`[[groups]]` で定義。

**コマンド (Command)**
: 実際に実行するコマンドの定義。`[[groups.commands]]` で定義。

**優先度 (Priority)**
: グループの実行順序を制御する数値。小さい数値ほど先に実行される。

### C.3 セキュリティ関連用語

**ファイル検証 (File Verification)**
: ファイルのハッシュ値を照合して改ざんを検出する機能。

**環境変数許可リスト (Environment Variable Allowlist)**
: 使用を許可する環境変数のリスト。リストにない変数は除外される。

**最小権限の原則 (Principle of Least Privilege)**
: 必要最小限の権限のみを付与するセキュリティの原則。

**権限昇格 (Privilege Escalation)**
: より高い権限(root など)でコマンドを実行すること。

**リスクレベル (Risk Level)**
: コマンドのセキュリティリスクを表す指標(low/medium/high)。

### C.4 変数展開関連用語

**変数展開 (Variable Expansion)**
: `${VAR}` 形式の変数を実際の値に置き換える処理。

**Command.Env**
: コマンドレベルで定義される環境変数。`env` パラメータで設定。

**継承モード (Inheritance Mode)**
: グループレベルで環境変数許可リストをどのように扱うかを決定するモード。

**ネスト変数 (Nested Variable)**
: 変数の値に別の変数を含める入れ子構造。例: `VAR1=${VAR2}/path`

**エスケープシーケンス (Escape Sequence)**
: 特殊文字をリテラル(文字通り)として扱うための記法。例: `\$`, `\\`

### C.5 実行関連用語

**ドライラン (Dry Run)**
: 実際には実行せず、実行計画のみを表示するモード。

**作業ディレクトリ (Working Directory)**
: コマンドが実行される現在のディレクトリ。`workdir` で設定。

**一時ディレクトリ (Temporary Directory)**
: ランナーが自動的に作成・管理する作業用ディレクトリ。`%{__runner_workdir}` 変数でアクセス可能。

**出力キャプチャ (Output Capture)**
: コマンドの標準出力をファイルに保存する機能。`output` パラメータで設定。

**標準出力 (Standard Output / stdout)**
: コマンドが通常の出力を送信するストリーム。

**標準エラー出力 (Standard Error / stderr)**
: コマンドがエラーメッセージを送信するストリーム。

### C.6 オーバーライドと継承

**オーバーライド (Override)**
: 下位レベルの設定が上位レベルの設定を置き換えること。

**マージ (Merge)**
: 複数の設定を統合すること。例: グローバルとグループの `verify_files` を結合。

**継承 (Inheritance)**
: 下位レベルが上位レベルの設定を引き継ぐこと。

## 付録D: 設定ファイルテンプレート

### D.1 基本テンプレート

```toml
version = "1.0"

[global]
timeout = 300
workdir = "/path/to/workdir"
env_allowed = ["PATH", "HOME"]

[[groups]]
name = "group_name"
description = "グループの説明"

[[groups.commands]]
name = "command_name"
description = "コマンドの説明"
cmd = "/path/to/command"
args = ["arg1", "arg2"]
```

### D.2 セキュア設定テンプレート

```toml
version = "1.0"

[global]
timeout = 600
workdir = "/opt/secure"
verify_standard_paths = true
env_allowed = ["PATH"]
verify_files = [
    # 追加の検証ファイル (コマンドは自動検証される)
]

[[groups]]
name = "secure_group"
description = "セキュアな操作グループ"
verify_files = [
    # グループ固有の検証ファイル (例: 設定ファイル、ライブラリ)
]

[[groups.commands]]
name = "secure_command"
description = "セキュアなコマンド"
cmd = "/path/to/verified/command"
args = []
risk_level = "medium"
```

### D.3 変数展開テンプレート

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",
    "HOME",
    # 追加の許可変数
]

[[groups]]
name = "variable_group"

[[groups.commands]]
name = "command_with_vars"
cmd = "${TOOL_DIR}/tool"
args = [
    "--config", "${CONFIG_FILE}",
    "--output", "${OUTPUT_DIR}/result.txt",
]
env_vars = [
    "TOOL_DIR=/opt/tools",
    "CONFIG_FILE=/etc/app/config.yml",
    "OUTPUT_DIR=/var/output",
]
```

### D.4 多環境対応テンプレート

```toml
version = "1.0"

[global]
env_allowed = [
    "PATH",
    "APP_BIN",
    "ENV_TYPE",
    "CONFIG_DIR",
]

# 開発環境
[[groups]]
name = "dev_environment"
priority = 1

[[groups.commands]]
name = "run_dev"
cmd = "${APP_BIN}"
args = ["--env", "${ENV_TYPE}", "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "ENV_TYPE=development",
    "CONFIG_DIR=/etc/app/configs",
]

# 本番環境
[[groups]]
name = "prod_environment"
priority = 2

[[groups.commands]]
name = "run_prod"
cmd = "${APP_BIN}"
args = ["--env", "${ENV_TYPE}", "--config", "${CONFIG_DIR}/${ENV_TYPE}.yml"]
env_vars = [
    "APP_BIN=/opt/app/bin/server",
    "ENV_TYPE=production",
    "CONFIG_DIR=/etc/app/configs",
]
run_as_user = "appuser"
risk_level = "high"
```

## 付録E: 参考リンク

### E.1 公式リソース

- **プロジェクトリポジトリ**: `github.com/isseis/go-safe-cmd-runner`
- **サンプル設定**: `sample/` ディレクトリ
- **開発者向けドキュメント**: `docs/dev/` ディレクトリ

### E.2 関連技術

- **TOML 仕様**: https://toml.io/
- **Go 言語**: https://golang.org/
- **セキュリティベストプラクティス**: OWASP Secure Coding Practices

### E.3 コミュニティ

- **Issue トラッカー**: GitHub Issues でバグ報告・機能要望
- **プルリクエスト**: 改善提案や貢献を歓迎

## おわりに

本ドキュメントは、go-safe-cmd-runner の TOML 設定ファイルの完全なガイドです。基本的な概念から高度な使い方まで、段階的に学べるように構成されています。

### 推奨される学習順序

1. **第1章〜第3章**: 基本概念と構造を理解
2. **第4章〜第6章**: 各レベルのパラメータを詳細に学習
3. **第7章**: 変数展開機能をマスター
4. **第8章〜第9章**: 実践例とベストプラクティスを習得
5. **第10章**: トラブルシューティング手法を身につける
6. **付録**: リファレンスとして活用

### さらなる学習

- `sample/` ディレクトリの実例を参照
- 実際の環境で小さな設定から始める
- ドライランで動作を確認しながら段階的に複雑化
- コミュニティに質問や改善提案を投稿

安全で効率的なコマンド実行環境の構築に、本ドキュメントが役立つことを願っています。
