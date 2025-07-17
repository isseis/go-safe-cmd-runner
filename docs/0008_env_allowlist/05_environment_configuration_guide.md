# 環境別設定ガイド: 環境変数制御の代替ソリューション

## 1. 概要

本ガイドでは、環境変数安全化機能において、テンプレートレベルでの`env_allowlist`設定を使用せずに、環境別の環境変数制御を実現する方法について説明する。

## 2. 推奨される代替ソリューション

### 2.1 環境別設定ファイル方式（推奨）

#### 2.1.1 概要

環境ごとに独立した設定ファイルを作成し、実行時に適切な設定ファイルを選択する方式。

#### 2.1.2 ファイル構成例

```
config/
├── config-dev.toml      # 開発環境用
├── config-staging.toml  # ステージング環境用
├── config-prod.toml     # プロダクション環境用
└── config-local.toml    # ローカル開発用
```

#### 2.1.3 設定ファイル例

**config-dev.toml（開発環境）**
```toml
version = "1.0"

[global]
timeout = 1800
workdir = "/tmp/dev"
log_level = "debug"

# 開発環境用許可環境変数
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "TERM",
    "NODE_ENV",
    "DEBUG",
    "VERBOSE",
    "DEV_DATABASE_URL",
    "DEV_API_KEY"
]

[[groups]]
name = "web_app"
description = "Web application development"

[[groups.commands]]
name = "start_dev_server"
cmd = "npm"
args = ["run", "dev"]
env = [
    "NODE_ENV=development",
    "DEBUG=app:*",
    "DATABASE_URL=${DEV_DATABASE_URL}",
    "API_KEY=${DEV_API_KEY}"
]

[[groups.commands]]
name = "run_tests"
cmd = "npm"
args = ["test"]
env = [
    "NODE_ENV=test",
    "VERBOSE=${VERBOSE}"
]
```

**config-prod.toml（プロダクション環境）**
```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/opt/app"
log_level = "info"

# プロダクション環境用許可環境変数（最小限）
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "NODE_ENV",
    "PROD_DATABASE_URL",
    "PROD_API_KEY",
    "MONITORING_TOKEN"
]

[[groups]]
name = "web_app"
description = "Web application production"

[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["dist/server.js"]
env = [
    "NODE_ENV=production",
    "DATABASE_URL=${PROD_DATABASE_URL}",
    "API_KEY=${PROD_API_KEY}",
    "MONITORING_TOKEN=${MONITORING_TOKEN}"
]

[[groups.commands]]
name = "health_check"
cmd = "curl"
args = ["-f", "http://localhost:8080/health"]
```

#### 2.1.4 実行方法

```bash
# 開発環境で実行
./cmd-runner -config config/config-dev.toml

# プロダクション環境で実行
./cmd-runner -config config/config-prod.toml

# 環境変数による設定ファイル選択
CONFIG_ENV=${CONFIG_ENV:-dev}
./cmd-runner -config config/config-${CONFIG_ENV}.toml
```

#### 2.1.5 メリット

- **セキュリティ境界の明確化**: 環境間で完全に分離された設定
- **設定の単純化**: 各環境で必要な設定のみを記述
- **デプロイの安全性**: 誤った環境設定の混入を防止
- **監査の容易性**: 環境ごとの設定を独立して監査可能

### 2.2 環境別グループ方式

#### 2.2.1 概要

単一の設定ファイル内で、環境別のグループを定義する方式。

#### 2.2.2 設定例

```toml
version = "1.0"

[global]
timeout = 3600
workdir = "/opt/app"
log_level = "info"

# グローバル基本許可環境変数
env_allowlist = ["PATH", "HOME", "USER", "LANG"]

# 開発環境グループ
[[groups]]
name = "web_app_dev"
description = "Web application development environment"

# 開発環境固有の許可環境変数
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "NODE_ENV",
    "DEBUG",
    "VERBOSE",
    "DEV_DATABASE_URL",
    "DEV_API_KEY"
]

[[groups.commands]]
name = "start_dev_server"
cmd = "npm"
args = ["run", "dev"]
env = [
    "NODE_ENV=development",
    "DEBUG=app:*",
    "DATABASE_URL=${DEV_DATABASE_URL}",
    "API_KEY=${DEV_API_KEY}"
]

# プロダクション環境グループ
[[groups]]
name = "web_app_prod"
description = "Web application production environment"

# プロダクション環境固有の許可環境変数（最小限）
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "NODE_ENV",
    "PROD_DATABASE_URL",
    "PROD_API_KEY",
    "MONITORING_TOKEN"
]

[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["dist/server.js"]
env = [
    "NODE_ENV=production",
    "DATABASE_URL=${PROD_DATABASE_URL}",
    "API_KEY=${PROD_API_KEY}",
    "MONITORING_TOKEN=${MONITORING_TOKEN}"
]

# セキュア環境グループ（環境変数なし）
[[groups]]
name = "secure_tasks"
description = "Secure tasks with no environment variables"

env_allowlist = []  # 明示的に環境変数なし

[[groups.commands]]
name = "security_audit"
cmd = "/usr/bin/security-tool"
args = ["--audit"]
# 環境変数なしで実行
```

#### 2.2.3 実行方法

```bash
# 開発環境のグループを実行
./cmd-runner -group web_app_dev

# プロダクション環境のグループを実行
./cmd-runner -group web_app_prod

# セキュア環境のグループを実行
./cmd-runner -group secure_tasks
```

#### 2.2.4 メリット

- **単一ファイル管理**: 1つの設定ファイルで複数環境を管理
- **設定の一元化**: 共通設定の重複排除
- **環境間の比較容易**: 異なる環境の設定を同一ファイル内で比較可能

## 3. 高度な運用パターン

### 3.1 環境変数テンプレート方式

#### 3.1.1 概要

環境固有の値を環境変数として外部化し、設定ファイルでは変数参照のみを記述する方式。

#### 3.1.2 設定例

**config-template.toml**
```toml
version = "1.0"

[global]
timeout = 3600
workdir = "${APP_WORKDIR}"
log_level = "${LOG_LEVEL}"

# 環境別で異なる許可環境変数
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "NODE_ENV",
    "APP_WORKDIR",
    "LOG_LEVEL",
    "DATABASE_URL",
    "API_KEY"
]

[[groups]]
name = "web_app"

[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["server.js"]
env = [
    "NODE_ENV=${NODE_ENV}",
    "DATABASE_URL=${DATABASE_URL}",
    "API_KEY=${API_KEY}"
]
```

**環境別の実行**
```bash
# 開発環境
export NODE_ENV=development
export APP_WORKDIR=/tmp/dev
export LOG_LEVEL=debug
export DATABASE_URL=postgresql://localhost/dev_db
export API_KEY=dev_api_key
./cmd-runner -config config-template.toml

# プロダクション環境
export NODE_ENV=production
export APP_WORKDIR=/opt/app
export LOG_LEVEL=info
export DATABASE_URL=postgresql://prod-db/app_db
export API_KEY=prod_api_key
./cmd-runner -config config-template.toml
```

### 3.2 環境変数ファイル分離方式

#### 3.2.1 概要

`.env`ファイルを環境別に分離し、設定ファイルは共通化する方式。

#### 3.2.2 ファイル構成

```
config/
├── config.toml          # 共通設定
├── .env.dev            # 開発環境変数
├── .env.staging        # ステージング環境変数
└── .env.prod           # プロダクション環境変数
```

**config.toml（共通設定）**
```toml
version = "1.0"

[global]
env_allowlist = [
    "PATH",
    "HOME",
    "USER",
    "LANG",
    "NODE_ENV",
    "DATABASE_URL",
    "API_KEY",
    "LOG_LEVEL"
]

[[groups]]
name = "web_app"

[[groups.commands]]
name = "start_server"
cmd = "node"
args = ["server.js"]
env = [
    "NODE_ENV=${NODE_ENV}",
    "DATABASE_URL=${DATABASE_URL}",
    "API_KEY=${API_KEY}"
]
```

**.env.dev（開発環境変数）**
```env
NODE_ENV=development
DATABASE_URL=postgresql://localhost/dev_db
API_KEY=dev_api_key
LOG_LEVEL=debug
```

**.env.prod（プロダクション環境変数）**
```env
NODE_ENV=production
DATABASE_URL=postgresql://prod-db/app_db
API_KEY=prod_api_key
LOG_LEVEL=info
```

#### 3.2.3 実行方法

```bash
# 開発環境
./cmd-runner -config config.toml -env .env.dev

# プロダクション環境
./cmd-runner -config config.toml -env .env.prod
```

## 4. セキュリティ考慮事項

### 4.1 環境別設定ファイルのセキュリティ

#### 4.1.1 ファイル権限

```bash
# 設定ファイルのセキュアな権限設定
chmod 600 config-prod.toml
chmod 600 .env.prod

# 開発環境は若干緩い権限
chmod 644 config-dev.toml
chmod 644 .env.dev
```

#### 4.1.2 機密情報の管理

```toml
# プロダクション環境では機密情報を.envファイルに分離
[global]
env_allowlist = [
    "PATH",
    "HOME",
    "DATABASE_URL",  # .envファイルで定義
    "API_KEY"        # .envファイルで定義
]
```

### 4.2 環境間の分離

#### 4.2.1 完全分離の確認

```bash
# 開発環境の設定でプロダクション環境変数が使用されていないことを確認
grep -r "PROD_" config-dev.toml || echo "OK: No production variables in dev config"

# プロダクション環境の設定で開発環境変数が使用されていないことを確認
grep -r "DEV_" config-prod.toml || echo "OK: No development variables in prod config"
```

## 5. 運用ガイドライン

### 5.1 設定ファイルの管理

#### 5.1.1 バージョン管理

```bash
# 設定ファイルのバージョン管理
git add config-dev.toml config-staging.toml config-prod.toml

# 機密情報を含む.envファイルは除外
echo "*.env" >> .gitignore
echo ".env.*" >> .gitignore
```

#### 5.1.2 設定の検証

```bash
# 設定ファイルの文法チェック
./cmd-runner -config config-dev.toml -validate
```

### 5.2 デプロイメント

#### 5.2.1 環境別デプロイ

```bash
# CI/CDパイプラインでの環境別デプロイ
case $DEPLOY_ENV in
  dev)
    CONFIG_FILE="config-dev.toml"
    ;;
  staging)
    CONFIG_FILE="config-staging.toml"
    ;;
  prod)
    CONFIG_FILE="config-prod.toml"
    ;;
  *)
    echo "Unknown environment: $DEPLOY_ENV"
    exit 1
    ;;
esac

./cmd-runner -config $CONFIG_FILE
```

## 6. まとめ

環境別の環境変数制御には、以下の方針を推奨する：

1. **基本方針**: 環境別設定ファイル方式を採用
2. **セキュリティ**: 環境間の完全分離を重視
3. **運用性**: 設定の明確化と監査の容易性を優先
4. **保守性**: シンプルな設計による理解しやすさを重視

これらの代替ソリューションにより、テンプレートレベルでの`env_allowlist`設定を使用することなく、安全で効率的な環境別環境変数制御を実現できる。
