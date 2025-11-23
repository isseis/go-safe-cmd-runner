# Dry-Run JSON出力のスキーマ定義

## 概要

本ドキュメントは、`runner` コマンドのドライラン実行時にJSON形式で出力されるデータ構造を定義します。

## 関連ドキュメント

- [runner コマンドユーザーガイド](runner_command.ja.md)

## トップレベル構造

```json
{
  "metadata": {
    "generated_at": "2025-11-23T10:00:00Z",
    "run_id": "abc123",
    "config_path": "/path/to/config.toml",
    "environment_file": "",
    "version": "1.0.0",
    "duration": 1500000000
  },
  "status": "success",
  "phase": "completed",
  "summary": {
    "total_resources": 5,
    "successful": 5,
    "failed": 0,
    "skipped": 0,
    "groups": {
      "total": 2,
      "successful": 2,
      "failed": 0,
      "skipped": 0
    },
    "commands": {
      "total": 3,
      "successful": 3,
      "failed": 0,
      "skipped": 0
    }
  },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": { ... }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "workdir": "/var/backups",
        "timeout": 3600000000000,
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z",
      "debug_info": { ... }
    }
  ],
  "security_analysis": { ... },
  "environment_info": { ... },
  "file_verification": { ... },
  "errors": [],
  "warnings": []
}
```

## DryRunResult (トップレベルオブジェクト)

ドライラン実行の完全な結果を表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `metadata` | ResultMetadata | 実行メタデータ |
| `status` | string | 実行ステータス (`"success"` または `"error"`) |
| `phase` | string | 実行フェーズ (`"completed"`, `"pre_execution"`, `"initialization"`, `"group_execution"`) |
| `error` | ExecutionError? | トップレベルエラー情報 (エラー発生時のみ) |
| `summary` | ExecutionSummary | 実行サマリー統計 |
| `resource_analyses` | ResourceAnalysis[] | リソース分析結果のリスト |
| `security_analysis` | SecurityAnalysis | セキュリティ分析結果 |
| `environment_info` | EnvironmentInfo | 環境変数情報 |
| `file_verification` | FileVerificationSummary? | ファイル検証サマリー (検証実行時のみ) |
| `errors` | DryRunError[] | 発生したエラーのリスト |
| `warnings` | DryRunWarning[] | 発生した警告のリスト |

## ResultMetadata

ドライラン結果のメタデータを含みます。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `generated_at` | string | 結果生成日時 (RFC3339形式) |
| `run_id` | string | 実行ID |
| `config_path` | string | 設定ファイルパス |
| `environment_file` | string | 環境ファイルパス |
| `version` | string | バージョン情報 |
| `duration` | number | 実行時間 (ナノ秒) |

## ExecutionSummary

実行のサマリー統計を提供します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `total_resources` | number | 総リソース数 |
| `successful` | number | 成功したリソース数 |
| `failed` | number | 失敗したリソース数 |
| `skipped` | number | スキップされたリソース数 |
| `groups` | Counts | グループ統計 |
| `commands` | Counts | コマンド統計 |

### Counts

特定のリソースタイプの統計を提供します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `total` | number | 総数 |
| `successful` | number | 成功数 |
| `failed` | number | 失敗数 |
| `skipped` | number | スキップ数 |

## ResourceAnalysis

各リソース（グループまたはコマンド）の分析結果を表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `type` | string | リソースタイプ (`"group"`, `"command"`, `"filesystem"`, `"privilege"`, `"network"`, `"process"`) |
| `operation` | string | 操作タイプ (`"analyze"`, `"execute"`, `"create"`, `"delete"`, `"escalate"`, `"send"`) |
| `target` | string | ターゲット識別子 (例: グループ名、`group.command` 形式) |
| `status` | string | 実行ステータス (`"success"` または `"error"`) |
| `error` | ExecutionError? | エラー情報 (エラー発生時のみ) |
| `skip_reason` | string? | スキップ理由 (スキップ時のみ) |
| `parameters` | object | リソース操作のパラメータ |
| `impact` | ResourceImpact | リソース操作の影響 |
| `timestamp` | string | タイムスタンプ (RFC3339形式) |
| `debug_info` | DebugInfo? | デバッグ情報 (詳細レベルに応じて含まれる) |

### Parameters フィールド

`parameters` フィールドは、リソースタイプと操作によって異なる内容を持ちます：

#### コマンド実行の場合

| フィールド | 型 | 説明 |
|---------|------|------|
| `cmd` | string | 実行するコマンドのパス |
| `args` | string[]? | コマンドライン引数 |
| `workdir` | string? | 作業ディレクトリ |
| `timeout` | number? | タイムアウト (ナノ秒) |
| `risk_level` | string? | リスクレベル (`"low"`, `"medium"`, `"high"`) |

## ResourceImpact

リソース操作の影響を説明します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `reversible` | boolean | 操作が可逆かどうか |
| `persistent` | boolean | 操作が永続的かどうか |
| `security_risk` | string? | セキュリティリスク (`"low"`, `"medium"`, `"high"`) |
| `description` | string | 影響の説明 |

## ExecutionError

実行エラーを表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `type` | string | エラータイプ |
| `message` | string | エラーメッセージ |
| `component` | string | エラー発生コンポーネント |
| `details` | object? | エラー詳細 |

## DebugInfo

デバッグ情報を含むオブジェクトです。詳細レベル (`-dry-run-detail`) に応じて内容が変わります。

### 詳細レベル別の内容

| 詳細レベル | `inheritance_analysis` | `final_environment` |
|----------|----------------------|-------------------|
| `summary` | なし | なし |
| `detailed` | 基本情報のみ | なし |
| `full` | 差分情報を含む完全な情報 | 完全な情報 |

### フィールド

| フィールド | 型 | 説明 | 出現条件 |
|---------|------|------|---------|
| `inheritance_analysis` | InheritanceAnalysis? | 環境変数継承の分析結果 | グループリソースで `detailed` 以上 |
| `final_environment` | FinalEnvironment? | 最終的な環境変数 | コマンドリソースで `full` のみ |

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
| `variables` | map[string]EnvironmentVariable | 環境変数のマップ (キーは変数名) |

## EnvironmentVariable

個々の環境変数を表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `value` | string | 環境変数の値 (センシティブ変数で `show_sensitive=false` の場合は空文字列) |
| `source` | string | 値の出所 |
| `masked` | boolean? | 値がマスクされたかどうか (`show_sensitive=false` でセンシティブデータの場合のみ `true`) |

### Source フィールドの値

| 値 | 説明 |
|----|------|
| `"system"` | システム環境から `env_allowlist` で許可された変数 |
| `"vars"` | グローバルまたはグループレベルの `vars`/`env_import`/`env_vars` セクションで定義された変数 |
| `"command"` | コマンドレベルの `env_vars` セクションで定義された変数 |

**注意**: 現在、`env_import` から取り込まれた変数は `vars` と区別されません。これは設定展開時に `env_import` の変数が `vars` にマージされるためです。両方とも `"vars"` として報告されます。これは現在のアーキテクチャの簡潔性を維持するための既知の制限です。

## センシティブ情報のマスキング

デフォルトでは（`show_sensitive=false` の場合）、以下のパターンに一致する環境変数名の値は：
- `value` フィールドが空文字列になります
- `masked` フィールドが `true` に設定されます

センシティブパターン：
- `*PASSWORD*`
- `*SECRET*`
- `*TOKEN*`
- `*KEY*`
- `*CREDENTIAL*`
- `*AUTH*`

`--show-sensitive` フラグを使用すると、`value` フィールドに実際の値が設定され、`masked` フィールドは含まれません。

## SecurityAnalysis

セキュリティ分析結果を含みます。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `risks` | SecurityRisk[] | セキュリティリスクのリスト |
| `privilege_changes` | PrivilegeChange[] | 権限変更のリスト |
| `environment_access` | EnvironmentAccess[] | 環境変数アクセスのリスト |
| `file_access` | FileAccess[] | ファイルアクセスのリスト |

### SecurityRisk

個々のセキュリティリスクを表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `level` | string | リスクレベル (`"low"`, `"medium"`, `"high"`) |
| `type` | string | リスクタイプ (`"privilege_escalation"`, `"dangerous_command"`, `"data_exposure"`) |
| `description` | string | リスクの説明 |
| `command` | string | 関連するコマンド |
| `group` | string | 関連するグループ |
| `mitigation` | string | リスク軽減策 |

### PrivilegeChange

権限変更を表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `group` | string | グループ名 |
| `command` | string | コマンド名 |
| `from_user` | string | 変更前のユーザー |
| `to_user` | string | 変更後のユーザー |
| `mechanism` | string | 変更メカニズム |

### EnvironmentAccess

環境変数アクセスを表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `variable` | string | 変数名 |
| `access_type` | string | アクセスタイプ (`"read"`, `"write"`) |
| `commands` | string[] | アクセスするコマンドのリスト |
| `groups` | string[] | アクセスするグループのリスト |
| `sensitive` | boolean | センシティブ変数かどうか |

### FileAccess

ファイルアクセスを表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `path` | string | ファイルパス |
| `access_type` | string | アクセスタイプ (`"read"`, `"write"`, `"execute"`) |
| `commands` | string[] | アクセスするコマンドのリスト |
| `groups` | string[] | アクセスするグループのリスト |

## EnvironmentInfo

環境変数に関する情報を含みます。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `total_variables` | number | 環境変数の総数 |
| `allowed_variables` | string[] | 許可された変数名のリスト |
| `filtered_variables` | string[] | フィルタリングされた変数名のリスト |
| `variable_usage` | map[string]string[] | 変数名とそれを使用するコマンドのマップ |

## FileVerificationSummary

ファイル検証の結果サマリーを含みます。検証が実行された場合のみ含まれます。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `total_files` | number | 検証対象ファイルの総数 |
| `verified_files` | number | 検証成功したファイル数 |
| `skipped_files` | number | スキップされたファイル数 |
| `failed_files` | number | 検証失敗したファイル数 |
| `duration` | number | 検証処理時間 (ナノ秒) |
| `hash_dir_status` | HashDirectoryStatus | ハッシュディレクトリの状態 |
| `failures` | FileVerificationFailure[]? | 検証失敗のリスト (失敗がある場合のみ) |

### HashDirectoryStatus

ハッシュディレクトリの状態を表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `path` | string | ハッシュディレクトリのパス |
| `exists` | boolean | ディレクトリが存在するかどうか |
| `validated` | boolean | ディレクトリが検証されたかどうか |

### FileVerificationFailure

個々のファイル検証失敗を表します。

| フィールド | 型 | 説明 |
|---------|------|------|
| `path` | string | ファイルパス |
| `reason` | string | 失敗理由 (`"hash_directory_not_found"`, `"hash_file_not_found"`, `"hash_mismatch"`, `"file_read_error"`, `"permission_denied"`) |
| `level` | string | 重要度レベル |
| `message` | string | エラーメッセージ |
| `context` | string | コンテキスト情報 |

## DryRunError

ドライラン実行中に発生したエラーを表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `type` | string | エラータイプ (`"configuration_error"`, `"verification_error"`, `"variable_error"`, `"security_error"`, `"system_error"`, `"execution_error"`) |
| `code` | string | エラーコード |
| `message` | string | エラーメッセージ |
| `component` | string | エラー発生コンポーネント |
| `group` | string? | 関連するグループ名 (グループレベルのエラーの場合のみ) |
| `command` | string? | 関連するコマンド名 (コマンドレベルのエラーの場合のみ) |
| `details` | object? | エラー詳細 |
| `recoverable` | boolean | 回復可能なエラーかどうか |

## DryRunWarning

ドライラン実行中に発生した警告を表します。

### フィールド

| フィールド | 型 | 説明 |
|---------|------|------|
| `type` | string | 警告タイプ (`"deprecated_feature"`, `"security_concern"`, `"performance_concern"`, `"compatibility"`) |
| `message` | string | 警告メッセージ |
| `component` | string | 警告発生コンポーネント |
| `group` | string? | 関連するグループ名 (グループレベルの警告の場合のみ) |
| `command` | string? | 関連するコマンド名 (コマンドレベルの警告の場合のみ) |

## 使用例

### DetailLevelSummary

```json
{
  "metadata": {
    "generated_at": "2025-11-23T10:00:00Z",
    "run_id": "abc123",
    "config_path": "/path/to/config.toml",
    "environment_file": "",
    "version": "1.0.0",
    "duration": 1500000000
  },
  "status": "success",
  "phase": "completed",
  "summary": {
    "total_resources": 1,
    "successful": 1,
    "failed": 0,
    "skipped": 0,
    "groups": {
      "total": 0,
      "successful": 0,
      "failed": 0,
      "skipped": 0
    },
    "commands": {
      "total": 1,
      "successful": 1,
      "failed": 0,
      "skipped": 0
    }
  },
  "resource_analyses": [
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:00Z"
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
}
```

`debug_info` フィールドは含まれません。

### DetailLevelDetailed

```json
{
  "metadata": { ... },
  "status": "success",
  "phase": "completed",
  "summary": { ... },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["HOME", "PATH"],
          "global_allowlist": ["HOME", "PATH"],
          "group_env_import": ["BACKUP_DIR"],
          "group_allowlist": ["BACKUP_DIR"],
          "inheritance_mode": "inherit"
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z"
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
}
```

基本的なデバッグ情報が含まれます。`detailed` レベルでは、コマンドリソースに `final_environment` は含まれません。

### DetailLevelFull

```json
{
  "metadata": { ... },
  "status": "success",
  "phase": "completed",
  "summary": { ... },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "backup",
      "status": "success",
      "parameters": {},
      "impact": {
        "reversible": true,
        "persistent": false,
        "description": "Configuration analysis only"
      },
      "timestamp": "2025-11-23T10:00:00Z",
      "debug_info": {
        "inheritance_analysis": {
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
      "type": "command",
      "operation": "execute",
      "target": "backup.db_backup",
      "status": "success",
      "parameters": {
        "cmd": "/usr/bin/pg_dump",
        "args": ["-U", "postgres", "mydb"],
        "workdir": "/var/backups",
        "timeout": 3600000000000,
        "risk_level": "medium"
      },
      "impact": {
        "reversible": false,
        "persistent": true,
        "security_risk": "medium",
        "description": "Database backup operation"
      },
      "timestamp": "2025-11-23T10:00:01Z",
      "debug_info": {
        "final_environment": {
          "variables": {
            "BACKUP_DIR": {
              "value": "/var/backups",
              "source": "vars"
            },
            "DB_PASSWORD": {
              "value": "",
              "source": "command",
              "masked": true
            },
            "HOME": {
              "value": "/root",
              "source": "system"
            },
            "PATH": {
              "value": "/usr/local/bin:/usr/bin:/bin",
              "source": "system"
            }
          }
        }
      }
    }
  ],
  "security_analysis": {},
  "environment_info": {},
  "errors": [],
  "warnings": []
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
  jq '.resource_analyses[] | select(.debug_info.inheritance_analysis != null) | .debug_info.inheritance_analysis.inheritance_mode'
```

### 特定のコマンドの最終環境変数を確認

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.target == "backup.db_backup") | .debug_info.final_environment.variables'
```

### 継承された変数のリストを取得

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.inheritance_analysis.inherited_variables != null) | .debug_info.inheritance_analysis.inherited_variables[]'
```

### センシティブな変数を持つコマンドを特定

```bash
runner -config config.toml -dry-run -dry-run-format json -dry-run-detail full | \
  jq '.resource_analyses[] | select(.debug_info.final_environment != null) | select(.debug_info.final_environment.variables | to_entries[] | select(.value.masked == true)) | .target'
```

---

**最終更新**: 2025-11-23
