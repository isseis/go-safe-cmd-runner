# Dry-Run JSON出力のスキーマ定義

## 概要

本ドキュメントは、`runner` コマンドのドライラン実行時にJSON形式で出力されるデータ構造を定義します。

## 関連ドキュメント

- [runner コマンドユーザーガイド](runner_command.ja.md)

## トップレベル構造

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": { ... }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "workdir": "/var/backups",
      "timeout": 3600,
      "risk_level": "medium",
      "debug_info": { ... }
    }
  ]
}
```

## ResourceAnalysis

各リソース（グループまたはコマンド）の分析結果を表します。

### 共通フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `resource_type` | string | リソースの種類 (`"group"` または `"command"`) |
| `operation` | string | 操作の種類 (`"analyze"` または `"execute"`) |
| `group_name` | string | グループ名 |
| `debug_info` | object? | デバッグ情報 (詳細レベルに応じて含まれる) |

### グループリソース固有フィールド

グループリソース (`resource_type: "group"`) の場合は共通フィールドのみです。

### コマンドリソース固有フィールド

コマンドリソース (`resource_type: "command"`) の場合、以下のフィールドが追加されます：

| フィールド | 型 | 説明 |
|---------|------|------|
| `command_name` | string | コマンド名 |
| `cmd` | string | 実行するコマンドのパス |
| `args` | string[]? | コマンドライン引数 |
| `workdir` | string? | 作業ディレクトリ |
| `timeout` | number? | タイムアウト (秒) |
| `risk_level` | string? | リスクレベル (`"low"`, `"medium"`, `"high"`) |

## DebugInfo

デバッグ情報を含むオブジェクトです。詳細レベル (`-dry-run-detail`) に応じて内容が変わります。

### 詳細レベル別の内容

| 詳細レベル | `from_env_inheritance` | `final_environment` |
|----------|----------------------|-------------------|
| `summary` | なし | なし |
| `detailed` | 基本情報のみ | 基本情報のみ |
| `full` | 差分情報を含む完全な情報 | 完全な情報 |

### フィールド

| フィールド | 型 | 説明 | 出現条件 |
|---------|------|------|---------|
| `from_env_inheritance` | InheritanceAnalysis? | 環境変数継承の分析結果 | グループリソースで `detailed` 以上 |
| `final_environment` | FinalEnvironment? | 最終的な環境変数 | コマンドリソースで `detailed` 以上 |

## InheritanceAnalysis

環境変数の継承に関する分析情報です。

### フィールド

| フィールド | 型 | 説明 | 出現条件 |
|---------|------|------|---------|
| `global_env_import` | string[]? | グローバルレベルの `env_import` 設定 | 常に |
| `global_allowlist` | string[]? | グローバルレベルの `allowlist` 設定 | 常に |
| `group_env_import` | string[]? | グループレベルの `env_import` 設定 | 常に |
| `group_allowlist` | string[]? | グループレベルの `allowlist` 設定 | 常に |
| `inheritance_mode` | string | 継承モード (`"inherit"`, `"explicit"`, `"reject"`) | 常に |
| `inherited_variables` | string[]? | 実際に継承された変数名のリスト | `full` のみ |
| `removed_allowlist_variables` | string[]? | allowlistから削除された変数名のリスト | `full` のみ |
| `unavailable_env_import_variables` | string[]? | env_importで指定されたが利用不可能だった変数名のリスト | `full` のみ |

### 継承モード (inheritance_mode)

| 値 | 説明 |
|----|------|
| `"inherit"` | グローバルレベルの環境変数設定を継承 |
| `"explicit"` | グループレベルで明示的に設定された変数のみを使用 |
| `"reject"` | グローバルレベルの環境変数を拒否 |

### 差分情報の説明

**inherited_variables**

グローバルレベルからグループレベルに実際に継承された環境変数名のリストです。以下の条件を満たす変数が含まれます：

- `global_env_import` に指定されている
- システム環境に存在する
- `global_allowlist` でフィルタリングされた後も残っている

**removed_allowlist_variables**

`global_allowlist` から削除された変数名のリストです。以下の条件を満たす変数が含まれます：

- `global_allowlist` に指定されている
- `group_allowlist` に指定されていない（グループレベルで削除された）

**unavailable_env_import_variables**

`env_import` で指定されたが、システム環境に存在しなかった変数名のリストです。

## FinalEnvironment

コマンド実行時の最終的な環境変数の状態です。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `variables` | EnvironmentVariable[] | 環境変数のリスト |

## EnvironmentVariable

個々の環境変数を表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `name` | string | 環境変数名 |
| `value` | string | 環境変数の値 (センシティブ情報はマスクされる) |
| `source` | string | 値の出所 |

### Source フィールドの値

| 値 | 説明 |
|----|------|
| `"Global"` | グローバルレベルで定義された変数 |
| `"Group[<name>]"` | 特定のグループで定義された変数 (`<name>` はグループ名) |
| `"Command[<name>]"` | 特定のコマンドで定義された変数 (`<name>` はコマンド名) |
| `"System (filtered by allowlist)"` | システム環境から allowlist でフィルタリングされた変数 |
| `"Internal"` | システムが自動的に設定した内部変数 |

## センシティブ情報のマスキング

デフォルトでは、以下のパターンに一致する環境変数名の値は `[REDACTED]` でマスクされます：

- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`

`--show-sensitive` フラグを使用すると、マスクされずに平文で表示されます。

## 使用例

### DetailLevelSummary

```json
{
  "resource_analyses": [
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "risk_level": "medium"
    }
  ]
}
```

`debug_info` フィールドは含まれません。

### DetailLevelDetailed

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR"],
          "inheritance_mode": "inherit"
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "risk_level": "medium",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            },
            {
              "name": "PATH",
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

基本的なデバッグ情報が含まれます。

### DetailLevelFull

```json
{
  "resource_analyses": [
    {
      "resource_type": "group",
      "operation": "analyze",
      "group_name": "backup",
      "debug_info": {
        "from_env_inheritance": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH", "USER"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR", "TEMP_DIR"],
          "inheritance_mode": "inherit",
          "inherited_variables": ["HOME", "PATH"],
          "removed_allowlist_variables": ["USER"],
          "unavailable_env_import_variables": []
        }
      }
    },
    {
      "resource_type": "command",
      "operation": "execute",
      "group_name": "backup",
      "command_name": "db_backup",
      "cmd": "/usr/bin/pg_dump",
      "args": ["-U", "postgres", "mydb"],
      "workdir": "/var/backups",
      "timeout": 3600,
      "risk_level": "medium",
      "debug_info": {
        "final_environment": {
          "variables": [
            {
              "name": "BACKUP_DIR",
              "value": "/var/backups",
              "source": "Group[backup]"
            },
            {
              "name": "DB_PASSWORD",
              "value": "[REDACTED]",
              "source": "Command[db_backup]"
            },
            {
              "name": "HOME",
              "value": "/root",
              "source": "System (filtered by allowlist)"
            },
            {
              "name": "PATH",
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "System (filtered by allowlist)"
            }
          ]
        }
      }
    }
  ]
}
```

差分情報を含む完全なデバッグ情報が含まれます。

## jqを使った解析例

### デバッグ情報のみを抽出

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info != null) | .debug_info'
```

### 環境変数の継承モードを確認

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail detailed | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance != null) | .debug_info.from_env_inheritance.inheritance_mode'
```

### 特定のコマンドの最終環境変数を確認

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.command_name == "db_backup") | .debug_info.final_environment.variables'
```

### 継承された変数のリストを取得

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.from_env_inheritance.inherited_variables != null) | .debug_info.from_env_inheritance.inherited_variables[]'
```

### センシティブな変数を持つコマンドを特定

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | select(.debug_info.final_environment.variables[] | select(.value == "[REDACTED]")) | .command_name'
```

---

**最終更新**: 2025-11-03
