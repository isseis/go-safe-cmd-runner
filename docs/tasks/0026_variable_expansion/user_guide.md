# ユーザーガイド: コマンド・引数内環境変数展開機能

## 1. 機能概要

### 1.1 この機能でできること

go-safe-cmd-runnerの環境変数展開機能を使用すると、TOML設定ファイル内のコマンド名（`cmd`）やコマンド引数（`args`）に環境変数を埋め込み、実行時に動的に値を展開することができます。

**主な利点**:
- **動的な設定管理**: 環境ごとに異なるコマンドパスやオプションを柔軟に指定可能
- **設定の簡素化**: 同じ値を複数箇所で使用する場合、環境変数として定義することで管理が容易に
- **環境依存の吸収**: 開発環境、テスト環境、本番環境で異なるパスやパラメータを同一の設定ファイルで管理可能

### 1.2 サポートされる変数形式

この機能では、**`${VAR_NAME}`形式**の変数参照をサポートしています。

```toml
# サポートされる形式
cmd = "${DOCKER_CMD}"                      # 変数のみ
cmd = "${BIN_DIR}/my-tool"                 # パス結合
args = ["--input", "${INPUT_FILE}"]        # 引数内での使用
args = ["${USER}@${HOST}:${PORT}"]         # 複数変数の組み合わせ
```

**注意**: `$VAR`形式（ブレースなし）はサポートされていません。必ず`${VAR}`形式を使用してください。

### 1.3 セキュリティ機能

この機能は、以下のセキュリティメカニズムにより安全性を確保しています：

- **環境変数allowlist**: 使用可能な環境変数をallowlistで明示的に制限
- **Command.Env優先**: コマンド固有の環境変数定義（`env`）が最優先
- **循環参照検出**: 変数の循環参照を自動的に検出してエラー化
- **コマンドパス検証**: 展開後のコマンドパスに対して絶対パス検証を実施

## 2. 使用方法

### 2.1 基本的な使い方

#### 2.1.1 コマンド名での環境変数展開

コマンド名に環境変数を使用する基本的な例：

```toml
[[groups]]
name = "docker_commands"

[[groups.commands]]
name = "run_docker"
cmd = "${DOCKER_CMD}"
args = ["run", "-it", "ubuntu"]
env = ["DOCKER_CMD=/usr/bin/docker"]
```

**実行結果**:
```
cmd: "/usr/bin/docker"
args: ["run", "-it", "ubuntu"]
```

#### 2.1.2 引数での環境変数展開

引数に環境変数を使用する例：

```toml
[[groups.commands]]
name = "copy_file"
cmd = "/bin/cp"
args = ["${HOME}/source.txt", "${BACKUP_DIR}/backup.txt"]
env = ["HOME=/home/user", "BACKUP_DIR=/opt/backups"]
```

**実行結果**:
```
cmd: "/bin/cp"
args: ["/home/user/source.txt", "/opt/backups/backup.txt"]
```

#### 2.1.3 コマンドと引数の両方で展開

コマンド名と引数の両方で環境変数を使用する例：

```toml
[[groups.commands]]
name = "custom_tool"
cmd = "${TOOL_DIR}/my-script"
args = ["--input", "${INPUT_FILE}", "--output", "${OUTPUT_FILE}"]
env = [
    "TOOL_DIR=/opt/tools",
    "INPUT_FILE=/data/input.txt",
    "OUTPUT_FILE=/data/output.txt"
]
```

### 2.2 高度な使い方

#### 2.2.1 複数変数の組み合わせ

1つの文字列内で複数の環境変数を組み合わせる例：

```toml
[[groups.commands]]
name = "ssh_connection"
cmd = "/usr/bin/ssh"
args = ["${USER}@${HOSTNAME}:${PORT}"]
env = ["USER=admin", "HOSTNAME=server01", "PORT=22"]
```

**実行結果**:
```
args: ["admin@server01:22"]
```

#### 2.2.2 ネスト変数参照

環境変数の値に別の環境変数を参照させる例：

```toml
[[groups.commands]]
name = "nested_vars"
cmd = "/bin/echo"
args = ["Message: ${FULL_MSG}"]
env = ["FULL_MSG=Hello, ${USER}!", "USER=Alice"]
```

**実行結果**:
```
args: ["Message: Hello, Alice!"]
```

**注意**: ネスト変数は最大10レベルまでサポートされています。

#### 2.2.3 エスケープシーケンス

リテラルの`$`記号や`\`記号を使用する場合、バックスラッシュでエスケープします：

```toml
[[groups.commands]]
name = "escape_example"
cmd = "/bin/echo"
args = [
    "Literal \\$HOME keeps dollar sign",  # 結果: "Literal $HOME keeps dollar sign"
    "Path: \\\\${HOME}"                   # 結果: "Path: \<HOME値>"
]
env = ["HOME=/home/user"]
```

**エスケープルール**:
- `\$` → リテラルの`$`記号（変数展開しない）
- `\\` → リテラルの`\`記号
- `\${VAR}` → リテラルの`${VAR}`文字列（変数展開しない）

**不正なエスケープ**:
以下のエスケープシーケンスはエラーになります：
- `\U`, `\1`, `\a` など（`\$`と`\\`以外のエスケープ）
- 末尾の`\`（エスケープ対象がない）

### 2.3 allowlistとの連携

#### 2.3.1 グローバルallowlistの設定

使用可能な環境変数をグローバルに定義：

```toml
[global]
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "TOOL_DIR",
    "INPUT_FILE",
]
```

#### 2.3.2 Command.Env（ローカル定義）

コマンド固有の`env`で定義された変数は、allowlistに含まれていなくても使用可能です：

```toml
[[groups.commands]]
name = "custom_cmd"
cmd = "${MY_TOOL}"
args = ["--config", "${MY_CONFIG}"]
env = [
    "MY_TOOL=/opt/myapp/tool",      # allowlist不要
    "MY_CONFIG=/etc/myapp/conf"     # allowlist不要
]
```

**重要**: Command.Envで定義された変数は、同名のシステム環境変数より優先されます。

#### 2.3.3 環境変数の優先順位

環境変数は以下の優先順位で解決されます：

1. **Command.Env**: コマンド固有の環境変数定義（最優先）
2. **グローバル環境変数**: allowlistで許可されたシステム環境変数

```toml
# システム環境変数: HOME=/home/system-user

[[groups.commands]]
cmd = "/bin/echo"
args = ["Home: ${HOME}"]
env = ["HOME=/home/custom-user"]  # こちらが優先される

# 実行結果: "Home: /home/custom-user"
```

## 3. 設定例

### 3.1 開発環境と本番環境の切り替え

環境変数を使って、開発環境と本番環境で異なるコマンドを実行：

```toml
[global]
env_allowlist = ["ENV_TYPE", "APP_DIR"]

[[groups]]
name = "deploy"

[[groups.commands]]
name = "run_deploy_script"
cmd = "${APP_DIR}/deploy.sh"
args = ["--env", "${ENV_TYPE}"]
env = [
    "APP_DIR=/opt/myapp",
    "ENV_TYPE=production"    # 開発環境では"development"に変更
]
```

### 3.2 ユーザー固有のパス設定

ユーザーのホームディレクトリを動的に使用：

```toml
[global]
env_allowlist = ["USER", "HOME"]

[[groups.commands]]
name = "backup_config"
cmd = "/bin/cp"
args = [
    "/etc/myapp/config.yml",
    "${HOME}/.myapp/config.backup.yml"
]
```

### 3.3 Docker コマンドの動的実行

Dockerコマンドのパスとオプションを環境変数で管理：

```toml
[global]
env_allowlist = ["DOCKER_BIN", "IMAGE_TAG"]

[[groups.commands]]
name = "run_container"
cmd = "${DOCKER_BIN}"
args = [
    "run",
    "-d",
    "--name", "myapp",
    "myimage:${IMAGE_TAG}"
]
env = [
    "DOCKER_BIN=/usr/bin/docker",
    "IMAGE_TAG=latest"
]
```

### 3.4 複数のツールチェーン管理

異なるバージョンのツールを環境変数で切り替え：

```toml
[global]
env_allowlist = ["TOOLCHAIN_DIR", "VERSION"]

[[groups.commands]]
name = "build_with_gcc"
cmd = "${TOOLCHAIN_DIR}/gcc-${VERSION}/bin/gcc"
args = ["-o", "output", "main.c"]
env = [
    "TOOLCHAIN_DIR=/opt/toolchains",
    "VERSION=11.2.0"
]
```

## 4. トラブルシューティング

### 4.1 よくあるエラーと対処法

#### エラー: "variable not allowed"

**原因**: 参照している環境変数がallowlistに含まれておらず、Command.Envでも定義されていない

**対処法**:
1. グローバルallowlistに変数を追加する
```toml
[global]
env_allowlist = ["MY_VAR"]
```

2. またはCommand.Envで変数を定義する
```toml
[[groups.commands]]
env = ["MY_VAR=some_value"]
```

#### エラー: "circular reference detected"

**原因**: 環境変数の循環参照が発生している

```toml
# 問題のある設定
env = ["A=${B}", "B=${A}"]
```

**対処法**: 変数定義を見直し、循環参照を解消する
```toml
# 修正後
env = ["A=value_a", "B=${A}"]
```

#### エラー: "variable not found"

**原因**: 参照している環境変数が定義されていない

**対処法**: Command.Envまたはシステム環境変数で変数を定義する
```toml
[[groups.commands]]
env = ["MISSING_VAR=defined_value"]
```

#### エラー: "invalid escape sequence"

**原因**: サポートされていないエスケープシーケンスを使用している

```toml
# 問題のある設定
args = ["\\n", "\\t"]  # \n や \t はサポートされていない
```

**対処法**: `\$`と`\\`のみを使用する
```toml
# 修正後
args = ["\\$VAR", "\\\\path"]
```

#### エラー: "command path must be absolute"

**原因**: 展開後のコマンドパスが相対パスになっている

```toml
# 問題のある設定
cmd = "${TOOL}"
env = ["TOOL=my-tool"]  # 相対パス
```

**対処法**: 絶対パスを使用する
```toml
# 修正後
cmd = "${TOOL}"
env = ["TOOL=/usr/local/bin/my-tool"]
```

### 4.2 デバッグ方法

#### ログレベルの設定

詳細なログを出力してデバッグ：

```toml
[global]
log_level = "debug"
```

デバッグモードでは、以下の情報が出力されます：
- 変数参照の検出結果
- allowlist検証の詳細
- 展開前後の値の比較
- セキュリティ検証の結果

#### ドライランモード

実際にコマンドを実行せずに設定を検証：

```bash
# ドライランモードで実行（実装されている場合）
go-safe-cmd-runner --dry-run config.toml
```

### 4.3 パフォーマンス関連

#### 変数展開が遅い場合

**確認ポイント**:
1. 引数の数は適切か（推奨: 1000個以下）
2. 1つの要素内の変数数は適切か（推奨: 50個以下）
3. ネスト深度は適切か（推奨: 10レベル以下）

**最適化のヒント**:
- 不要な変数参照を削減
- 深いネストを避け、中間変数を減らす
- 複雑な文字列結合を単純化

#### メモリ使用量が多い場合

**確認ポイント**:
- 大量の環境変数を定義していないか
- 長大な文字列を変数に格納していないか

**対処法**:
- 環境変数の数を削減
- 長い文字列は外部ファイルで管理

## 5. ベストプラクティス

### 5.1 変数命名規則

**推奨**:
- 大文字とアンダースコアを使用（例: `MY_VAR`, `APP_DIR`）
- 意味が明確な名前を使用
- プレフィックスでグルーピング（例: `DB_HOST`, `DB_PORT`, `DB_USER`）

**非推奨**:
- 小文字のみの変数名（システム変数と混同しやすい）
- 1文字の変数名（意味が不明瞭）

### 5.2 セキュリティのベストプラクティス

1. **最小権限の原則**: allowlistには必要最小限の環境変数のみを追加
2. **Command.Envの活用**: 機密情報はCommand.Envで管理し、システム環境変数に依存しない
3. **絶対パスの使用**: コマンドパスは必ず絶対パスで指定
4. **ログのマスキング**: 機密情報を含む変数は適切にマスキング

### 5.3 保守性のベストプラクティス

1. **変数の再利用**: 同じ値を複数箇所で使用する場合は環境変数化
2. **コメントの追加**: 複雑な変数展開にはコメントで説明を追加
```toml
# データベース接続文字列を構築
# 形式: <user>@<host>:<port>
args = ["${DB_USER}@${DB_HOST}:${DB_PORT}"]
```
3. **環境別の設定分離**: 環境ごとに異なる値を持つ変数は明示的に管理

### 5.4 テスト戦略

1. **単体テスト**: 各環境変数展開パターンを個別にテスト
2. **統合テスト**: 実際のコマンド実行でエンドツーエンドの動作を確認
3. **エラーケーステスト**: 異常系（変数未定義、循環参照等）を網羅的にテスト

### 5.5 設定ファイルの構造化

**推奨構造**:
```toml
# グローバル設定（共通項目）
[global]
env_allowlist = [...]

# 環境共通の変数定義を先頭にグループ化
[[groups]]
name = "common_setup"
[[groups.commands]]
env = [
    "APP_DIR=/opt/myapp",
    "LOG_DIR=/var/log/myapp",
]

# 機能別にグループを分割
[[groups]]
name = "database_tasks"
# ...

[[groups]]
name = "backup_tasks"
# ...
```

## 6. 関連ドキュメント

- [要件定義書](01_requirements.md) - 機能の詳細要件
- [アーキテクチャ設計書](02_architecture.md) - システム設計の詳細
- [実装計画書](04_implementation_plan.md) - 開発の進捗状況
- サンプル設定ファイル:
  - [sample/variable_expansion_test.toml](../../../sample/variable_expansion_test.toml) - 基本的な使用例
