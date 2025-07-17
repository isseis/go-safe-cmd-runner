# コマンド実行ラッパー 詳細仕様書

## 1. コマンド定義ファイル仕様

### 1.1 テンプレート定義

```toml
[templates]
  [templates.secure_execution]
  description = "安全なコマンド実行テンプレート"
  verify = []               # 検証ルール名のリスト
  temp_dir = true          # テンポラリディレクトリを自動生成
  cleanup = true           # 自動クリーンアップ
  workdir = "auto"         # auto: テンポラリディレクトリを使用
  env = ["HOME"]          # デフォルトで必要な環境変数
  privileged = false       # 特権実行のデフォルト値
```

### 1.2 ロック設定

#### 1.2.1 基本設定

```toml
[execution]
  # ロックファイルの設定（オプション）
  # 未指定の場合は、コマンド定義ファイルのパスに '.lock' を追加したパスが使用されます
  # 例: /path/to/config.toml → /path/to/config.toml.lock
  # lock_file = "/var/run/backup.lock"  # 明示的に指定する場合

  lock_timeout = 3600  # ロックの有効期限（秒）
  force_unlock = false  # 既存のロックを強制的に解除して実行
```

#### 1.2.2 ロックファイルの自動生成ルール

1. **ロックファイルの場所**:
   - `lock_file` が指定されている場合: 指定されたパスを使用
   - 未指定の場合: コマンド定義ファイルのパスに `.lock` を追加したパスを使用
     - 例: `/etc/cmd-runner/config.toml` → `/etc/cmd-runner/config.toml.lock`

2. **一時ディレクトリの使用**:
   - コマンド定義ファイルと同じディレクトリに書き込み権限がない場合、
     システムの一時ディレクトリ（`/tmp/` など）に自動的に作成
     - 例: `/tmp/cmd_runner/config.toml.<hash>.lock`

3. **パーミッション**:
   - ロックファイルのパーミッションは `0600` に設定
   - 所有者のみが読み書き可能

### 1.3 検証ルール定義

```toml
[verification]
  [verification.rules]
    [verification.rules.foo]
    files = [
      { path = "/usr/local/bin/foo", hash = "sha256:abc123..." },
      { path = "~/.foo.config", hash = "sha256:def456..." }
    ]
```

### 1.4 ファイル形式

#### 1.4.1 コマンド定義ファイル (config.toml)
```toml
version = "1.0.0"

global = {
    timeout = 3600  # 全体のタイムアウト（秒）
    workdir = "/tmp"  # 作業ディレクトリ
    log_level = "info"  # debug, info, warn, error
    env_file = "/path/to/.env"  # 認証情報ファイルのパス（オプション）
}

# コマンドグループ
[groups]
  [groups.backup]
  description = "バックアップ処理"
  priority = 100  # 実行優先度（昇順）
  depends_on = []  # 依存するグループ

  # コマンド定義
  [[groups.backup.commands]]
  name = "backup-db"
  description = "データベースのバックアップ"
  # ユーザー名、パスワードは環境変数 DB_USER, DB_PASSWORD 経由で渡す
  cmd = "mysqldump ${DB_NAME}"
  args = []
  # 環境変数のマッピング
  # 形式: "TARGET_VAR=SOURCE_VAR" または "TARGET_VAR=value"
  # SOURCE_VARが存在する場合はその値が、存在しない場合はそのままの値が使われる
  env = [
    "DB_USER=MYSQL_DB_USER",
    "DB_PASSWORD=MYSQL_DB_PASSWORD",
    "DB_NAME=app1_db",  # 直接値を指定
    "HOSTNAME"  # 既存の環境変数をそのまま渡す
  ]
  dir = "/var/backups"
  user = "mysql"
  privileged = false
  timeout = 300

  [[groups.backup.commands]]
  name = "compress-backup"
  cmd = "gzip"
  args = ["backup.sql"]
  depends_on = ["backup-db"]
  privileged = false

[security]
  allowed_commands = ["mysqldump", "gzip", "tar"]
  allowed_dirs = ["/var/backups", "/tmp"]
  allowed_env_allowlist = ["DB_.*"]  # 許可する環境変数名のパターン
  max_memory = "1G"
  max_cpu = 1.0
  secure_env = true  # セキュアな環境変数処理を有効化
```

#### 1.4.2. 認証情報ファイル (.env)
```
# 認証情報ファイルの例
# ファイルパーミッションは600に設定する必要があります

# データベース接続情報
DB_USER=dbuser
DB_PASSWORD=your_secure_password_here
DB_NAME=mydb

# APIキー
API_KEY=your_api_key_here

# コメント行は#で始めます
# 空行は無視されます
```

### 1.5. セキュリティ要件

#### 1.5.1. 認証情報ファイルの取り扱い
- ファイルパーミッションは600（所有者のみ読み書き可能）に制限
- ファイルの所有者は実行ユーザーと一致すること
- ファイルのシンボリックリンクは追跡しない
- 起動時に認証情報ファイルのパーミッションを検証
- 認証情報はメモリ上で暗号化して保持
- 不要になった認証情報は即座にメモリからクリア
- コアダンプが生成されないように設定

#### 1.5.2. 環境変数の取り扱い
- 認証情報は環境変数経由でのみ渡す
- 環境変数名は英数字とアンダースコアのみ許可
- 環境変数名のホワイトリストを設定
- ログ出力時は環境変数の値をマスク
- コマンドライン引数として認証情報を渡さない
- 子プロセスに不要な環境変数を引き継がない

#### 1.6 実行制御の使用例

```toml
[groups.backup]
description = "データベースバックアップ"
lock = true  # このグループの実行を排他制御
lock_file = "/var/run/db_backup.lock"  # ロックファイルのパス（オプション）

[[groups.backup.commands]]
name = "backup-db"
cmd = "/usr/local/bin/backup"
```

### 1.7 コマンドグループでのテンプレート使用例

```toml
[groups.foo_processing]
description = "Foo コマンドの実行"
template = "secure_execution"  # テンプレートを適用
verify = "foo"                # 検証ルールを指定

[[groups.foo_processing.commands]]
name = "execute-foo"
cmd = "/usr/local/bin/foo"
# temp_dir, cleanup, verify_files はテンプレートから継承
# workdir は自動的にテンポラリディレクトリに設定
```

## 2. 実行制御の詳細仕様

### 2.1 ロックメカニズム

#### 2.1.1 ロックファイルの形式と配置

ロックファイルは以下のいずれかの場所に作成されます（優先順位順）:

1. `lock_file` 設定で明示的に指定されたパス
2. コマンド定義ファイルと同じディレクトリに `<config_filename>.lock`
3. システムの一時ディレクトリ（`/tmp/`）に `cmd_runner/<config_basename>.<hash>.lock`

ロックファイルの内容は以下のJSON形式で保存されます：
- ロックファイルにはJSON形式で以下の情報を保存:
  ```json
  {
    "pid": 12345,
    "started_at": "2025-07-07T02:53:43Z",
    "timeout": 3600
  }
  ```

#### 2.1.2 ロックの取得フロー
1. ロックファイルの存在を確認
2. ロックファイルが存在する場合:
   - ファイルの内容を読み込み、プロセスIDを取得
   - プロセスが実行中か確認（`/proc/<pid>/status` の確認）
   - プロセスが存在しない、またはタイムアウトしている場合は古いロックとみなす
3. 新規ロックを取得:
   - 一時ファイルにロック情報を書き込み
   - アトミックなリネームでロックファイルを作成

#### 2.1.3 エラー処理
- ロック取得失敗時:
  - `--force` オプションが指定されている場合は既存のロックを上書き
  - 指定されていない場合はエラーを表示して終了
- プロセス異常終了時:
  - シグナルハンドラを設定し、強制終了時にロックを解放

### 2.2 セキュリティ考慮事項
- ロックファイルのパーミッションは600に設定
- ロックファイルの所有者を確認し、適切なユーザーのみが操作可能に
- ログに機密情報が含まれないよう注意

## 3. テンプレートデバッグ機能

### 3.1 デバッグオプション

```toml
[debug]
  # テンプレート展開の詳細なログを有効化
  verbose_template = false

  # テンプレート展開前の検証のみ実行（ドライラン）
  validate_only = false

  # テンプレート変数のトレース情報を出力
  trace_variables = false

  # 展開前後の設定の差分を表示
  show_diff = true

  # 環境変数のみを表示して終了
  show_env = false
```

### 3.2 デバッグ出力の例

#### 3.2.1 テンプレート展開の差分表示

```bash
$ cmd-runner --debug-template config.toml
[DEBUG] テンプレート展開前:
  command: "{{.app_path}}/bin/start --port={{.port}}"
  env:
    CONFIG_PATH: "{{.config_dir}}/app.conf"

[DEBUG] テンプレート展開後:
  command: "/usr/local/app/bin/start --port=8080"
  env:
    CONFIG_PATH: "/etc/app/config/app.conf"
```

#### 3.2.2 変数のトレース

```bash
$ cmd-runner --trace-template config.toml
[TRACE] 変数の解決:
  app_path: "/usr/local/app" (from: default.template)
  port: 8080 (from: config.toml [production])
  config_dir: "/etc/app/config" (from: /etc/cmd-runner/global.toml)
```

### 3.3 エラーメッセージの改善

エラーが発生した場合、具体的な問題箇所と解決策を提示します：

```
[ERROR] テンプレートエラー:
  ファイル: /path/to/config.toml
  行: 42
  エラー: 未定義の変数 'database.host' が参照されています

  解決策:
    1. 変数 'database.host' を定義ファイルに追加する
    2. デフォルト値を設定する: {{.database.host | default "localhost"}}
    3. 変数名が正しいか確認する（大文字小文字の区別に注意）
```

## 4. 環境変数のマッピング仕様

環境変数のマッピングは以下の形式で指定します：

1. **直接マッピング**
   ```
   TARGET_VAR=SOURCE_VAR
   ```
   - `SOURCE_VAR` 環境変数の値を `TARGET_VAR` として設定
   - `SOURCE_VAR` が存在しない場合はエラー

2. **デフォルト値付きマッピング**
   ```
   TARGET_VAR=SOURCE_VAR:default_value
   ```
   - `SOURCE_VAR` が存在する場合はその値、存在しない場合は `default_value` が設定

3. **リテラル値**
   ```
   TARGET_VAR=value
   ```
   - 指定された値をそのまま `TARGET_VAR` に設定

4. **既存環境変数の継承**
   ```
   EXISTING_VAR
   ```
   - 既存の環境変数をそのまま引き継ぐ
   - セキュリティ上、明示的に許可された変数のみが引き継がれる

5. **複雑な式（将来対応予定）**
   ```
   # 将来のバージョンでサポート予定
   DB_URL=postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}
   ```

### 1.3. フィールド仕様

#### グローバル設定
- `version`: 定義ファイルのバージョン
- `global.timeout`: 全体のタイムアウト（秒）
- `global.workdir`: デフォルトの作業ディレクトリ
- `global.log_level`: ログレベル

#### グループ設定
- `groups.<name>.description`: グループの説明
- `groups.<name>.priority`: 実行優先度（小さい値から順に実行）
- `groups.<name>.depends_on`: 依存するグループ名のリスト

#### コマンド設定
- `name`: コマンドの識別子（一意）
- `description`: コマンドの説明
- `cmd`: 実行するコマンド
- `args`: コマンドライン引数のリスト
- `env`: 必要な環境変数名のリスト
- `dir`: コマンド実行ディレクトリ
- `user`: 実行ユーザー
- `privileged`: 特権実行フラグ
- `timeout`: コマンドのタイムアウト（秒）
- `ignore_errors`: エラーを無視するか
- `depends_on`: 依存するコマンド名のリスト

## 2. コアコンポーネント仕様

### 2.1. 認証情報管理モジュール

#### 2.1.1. 認証情報の読み込みと検証
```go
type CredentialManager struct {
    envVars   map[string]string
    filePath  string
    isLoaded  bool
    mu        sync.RWMutex
}

// LoadCredentials は認証情報ファイルを読み込む
func (cm *CredentialManager) LoadCredentials(filePath string) error {
    // ファイルの存在確認
    if _, err := os.Stat(filePath); os.IsNotExist(err) {
        return fmt.Errorf("credential file not found: %s", filePath)
    }

    // ファイルパーミッションの確認 (600のみ許可)
    if err := checkFilePermissions(filePath); err != nil {
        return fmt.Errorf("invalid file permissions: %w", err)
    }

    // ファイルの読み込み
    content, err := os.ReadFile(filePath)
    if err != nil {
        return fmt.Errorf("failed to read credential file: %w", err)
    }

    // パースと検証
    envVars, err := parseEnvFile(content)
    if err != nil {
        return fmt.Errorf("failed to parse credential file: %w", err)
    }

    cm.mu.Lock()
    defer cm.mu.Unlock()

    // 既存の認証情報をクリア
    clearCredentials(cm.envVars)

    // 新しい認証情報を設定
    cm.envVars = envVars
    cm.filePath = filePath
    cm.isLoaded = true

    return nil
}

// GetSecureEnv は安全な方法で環境変数を取得
// envSpec の形式: "TARGET_VAR=SOURCE_VAR" または "TARGET_VAR=value" または "VAR_NAME"
func (cm *CredentialManager) GetMappedEnv(envSpec string) (string, string, error) {
    cm.mu.RLock()
    defer cm.mu.RUnlock()

    if !cm.isLoaded {
        return "", "", errors.New("credentials not loaded")
    }

    // 形式: TARGET=SOURCE または VAR_NAME
    parts := strings.SplitN(envSpec, "=", 2)
    var targetVar, sourceSpec string

    if len(parts) == 2 {
        // TARGET=SOURCE 形式
        targetVar = parts[0]
        sourceSpec = parts[1]
    } else {
        // VAR_NAME 形式 - 既存の環境変数をそのまま使用
        targetVar = parts[0]
        sourceSpec = parts[0]
    }

    // ソースが別の環境変数を参照している場合
    if val, exists := os.LookupEnv(sourceSpec); exists {
        return targetVar, val, nil
    }

    // 認証情報から取得を試みる
    if val, exists := cm.envVars[sourceSpec]; exists {
        return targetVar, val, nil
    }

    // デフォルト値のチェック (source:default_value 形式)
    if strings.Contains(sourceSpec, ":") {
        sourceParts := strings.SplitN(sourceSpec, ":", 2)
        if val, exists := cm.envVars[sourceParts[0]]; exists {
            return targetVar, val, nil
        }
        return targetVar, sourceParts[1], nil
    }

    // リテラル値として扱う
    if len(parts) == 2 {
        return targetVar, sourceSpec, nil
    }

    return "", "", fmt.Errorf("environment variable not found: %s", sourceSpec)
}

// Clear はメモリ上の認証情報をクリア
func (cm *CredentialManager) Clear() {
    cm.mu.Lock()
    defer cm.mu.Unlock()

    clearCredentials(cm.envVars)
    cm.envVars = make(map[string]string)
    cm.isLoaded = false
}

// clearCredentials はメモリ上の認証情報を安全にクリア
func clearCredentials(envVars map[string]string) {
    for k := range envVars {
        // ゼロクリア
        for i := 0; i < len(envVars[k]); i++ {
            envVars[k] = "\x00"
        }
        delete(envVars, k)
    }
}
```

### 2.2.1. 環境変数の解決

```go
// resolveEnvVars はコマンドの環境変数を解決する
func (e *Executor) resolveEnvVars(envSpecs []string) ([]string, error) {
    envVars := make(map[string]string)

    // 現在の環境変数をコピー（セーフリストに基づいてフィルタリング）
    for _, env := range os.Environ() {
        if e.isAllowedEnv(env) {
            parts := strings.SplitN(env, "=", 2)
            if len(parts) == 2 {
                envVars[parts[0]] = parts[1]
            }
        }
    }

    // コマンド固有の環境変数を解決
    for _, spec := range envSpecs {
        targetVar, value, err := e.credManager.GetMappedEnv(spec)
        if err != nil {
            return nil, fmt.Errorf("failed to resolve env var %s: %w", spec, err)
        }
        envVars[targetVar] = value
    }

    // 環境変数のリストに変換
    result := make([]string, 0, len(envVars))
    for k, v := range envVars {
        result = append(result, fmt.Sprintf("%s=%s", k, v))
    }

    return result, nil
}
```

### 2.2.2. コマンド実行エンジン

#### 2.2.2.1. コマンド実行フロー
1. コマンド定義の読み込みと検証
2. 依存関係の解決と実行順序の決定
3. コマンドごとの実行コンテキストの作成
4. セキュリティポリシーの適用
5. コマンドの実行と監視
6. 結果の収集と集計

#### 2.1.2. エラーハンドリング
- コマンドが失敗した場合のリトライ処理
- 依存関係のあるコマンドの連鎖的な失敗の伝搬
- クリーンアップ処理の保証

### 2.3. セキュリティモジュール

#### 2.2.1. 権限管理
```go
type PrivilegeManager struct {
    allowedUsers map[string]bool
    sudoPath     string
}

func (p *PrivilegeManager) Execute(cmd *Command) error {
    if cmd.Privileged {
        if !p.allowedUsers[cmd.User] {
            return ErrPrivilegeEscalationNotAllowed
        }
        return p.executeWithSudo(cmd)
    }
    return p.executeAsUser(cmd)
}
```

#### 2.2.2. サンドボックス
- コマンドごとに専用の名前空間を提供
- ファイルシステムの読み取り専用マウント
- ネットワークアクセスの制限
- リソース制限の適用

### 2.4. ロギングモジュール

#### 2.3.1. ログ形式
```json
{
  "timestamp": "2025-07-07T10:00:00Z",
  "level": "info",
  "command": "backup-db",
  "group": "backup",
  "pid": 12345,
  "user": "mysql",
  "exit_code": 0,
  "duration": 12.345,
  "message": "Command completed successfully"
}
```

#### 2.3.2. ログレベル
- `debug`: デバッグ情報（コマンドの入出力を含む）
- `info`: 通常の操作ログ
- `warn`: 警告（処理は継続可能）
- `error`: エラー（処理が失敗）

## 3. API仕様

### 3.1. コマンドラインインターフェース
```
Usage: cmd-runner [options] <command-file>

Options:
  -c, --config string        設定ファイルのパス
  -e, --env-file string      認証情報ファイルのパス (default: ./.env)
  -l, --log-level string     ログレベル (debug, info, warn, error) (default "info")
  -d, --dry-run              ドライラン（実際には実行しない）
  --no-env-check            認証情報ファイルのチェックをスキップ
  -v, --version              バージョン情報を表示
  -h, --help                 ヘルプを表示

セキュリティオプション:
  --allow-env-pattern string   許可する環境変数名の正規表現 (default: "^[A-Z0-9_]+$")
  --deny-env-pattern string    拒否する環境変数名の正規表現
  --max-env-size int           環境変数の最大サイズ (KB) (default: 4096)
```

### 3.2. 終了コード
- `0`: 正常終了
- `1`: コマンド実行エラー
- `2`: 設定エラー
- `3`: セキュリティエラー
- `4`: システムエラー

## 4. 実装詳細

### 4.1. コマンド実行コンテキスト
```go
type CommandContext struct {
    ID          string
    Name        string
    Cmd         string
    Args        []string
    Env         []string
    Dir         string
    User        string
    Privileged  bool
    Timeout     time.Duration
    CreatedAt   time.Time
    StartedAt   time.Time
    CompletedAt time.Time
    ExitCode    int
    Output      []byte
    Error       error
    Cancel      context.CancelFunc
}
```

### 4.2. コマンド実行フロー
1. コマンド定義の読み込みと検証
2. 依存関係の解決と実行順序の決定
3. コマンドごとの実行コンテキストの作成
4. セキュリティポリシーの適用
5. コマンドの実行と監視
6. 結果の収集と集計

## 5. セキュリティ考慮事項

### 5.1. 認証情報の取り扱い

#### 5.1.1. メモリ保護
- 認証情報はメモリ上で暗号化
- 不要になったら即座にメモリからクリア
- スワップ領域への書き込み防止（mlock/mlockall）
- コアダンプの無効化

#### 5.1.2. ファイルベースの保護
- 認証情報ファイルのパーミッション強制（600）
- ファイル所有者の検証
- シンボリックリンクの追跡防止
- ファイルの内容のハッシュ値検証

#### 5.1.3. 環境変数の保護
- 環境変数名の検証（ホワイトリスト/ブラックリスト）
- 環境変数のサイズ制限
- 子プロセスへの環境変数の継承制御
- ログ出力時のマスキング

### 5.2. 入力検証
- すべての入力パラメータの検証
- コマンドインジェクションの防止
- パストラバーサルの防止
- 環境変数名の検証

### 5.3. 認証と認可
- 設定ファイルのパーミッション確認
- 特権昇格の制御
- コマンド実行の監査証跡
- 環境変数へのアクセス制御

### 5.4. セキュアなデフォルト設定
- 最小権限の原則
- デフォルトでのセーフモード
- 明示的な許可が必要な危険な操作
- デフォルトの環境変数制限

### 5.5. 監査とロギング
- 認証情報へのアクセス監査
- セキュリティ関連イベントのログ記録
- 機密情報のログ出力防止
- 監査証跡の完全性保証
