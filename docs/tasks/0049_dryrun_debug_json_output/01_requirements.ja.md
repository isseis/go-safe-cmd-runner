# 要件定義書: Dry-Run Debug情報のJSON出力対応

## 1. プロジェクト概要

### 1.1 目的
Dry-runモードでJSON形式(`--dry-run-format json`)を指定した際に、デバッグ情報（変数継承分析、最終環境変数）を構造化されたJSONとして出力できるようにする。これにより、自動化ツールやスクリプトからdry-run結果を解析可能にする。

### 1.2 背景
現在、dry-runモードでJSON出力を指定しても、デバッグ情報はテキスト形式で出力され、JSON出力と混在してしまう。その結果、出力全体がJSONとして無効となり、パーサーで処理できない。

### 1.3 スコープ
- **対象範囲**:
  - `PrintFromEnvInheritance()` のJSON形式出力対応
  - `PrintFinalEnvironment()` のJSON形式出力対応
  - グループごとのデバッグ情報のJSON構造への統合
- **対象外**:
  - テキスト形式出力の変更（現状維持）
  - 新しいデバッグ情報の追加

## 2. 現状分析

### 2.1 現在の出力動作

#### 2.1.1 出力フローの概要
Dry-runモードでの出力は2つの独立した箇所から行われる:

1. **グループ実行中のデバッグ出力** ([internal/runner/group_executor.go:131-136](../../../internal/runner/group_executor.go#L131-L136))
   ```go
   if ge.isDryRun {
       fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")
       debug.PrintFromEnvInheritance(os.Stdout, &ge.config.Global, groupSpec, runtimeGroup)
   }
   ```

2. **コマンド実行時の環境変数出力** ([internal/runner/group_executor.go:286-288](../../../internal/runner/group_executor.go#L286-L288))
   ```go
   if ge.isDryRun && ge.dryRunDetailLevel == resource.DetailLevelFull {
       debug.PrintFinalEnvironment(os.Stdout, envMap, ge.dryRunShowSensitive)
   }
   ```

3. **最終結果の出力** ([cmd/runner/main.go:269-291](../../../cmd/runner/main.go#L269-L291))
   ```go
   if *dryRun {
       result := runner.GetDryRunResults()
       // formatter (TextFormatter or JSONFormatter) を使用して出力
       output, err := formatter.FormatResult(result, ...)
       fmt.Print(output)
   }
   ```

#### 2.1.2 JSON出力時の問題

`--dry-run --dry-run-format json` を指定した場合の実際の出力:

```
===== Variable Expansion Debug Information =====

===== from_env Inheritance Analysis =====

[Global Level]
  env_import: not defined

[Group: example]
  env_import: Inheriting from Global
  ...

[Allowlist Inheritance]
  Inheriting Global env_allowlist
  ...

{
  "metadata": {
    "generated_at": "2025-01-30T...",
    "run_id": "01JKXY..."
  },
  "resource_analyses": [...]
}
```

**問題点:**
- テキストとJSONが混在し、全体として無効なJSON
- `jq` などのJSONパーサーで処理不可能
- 自動化ツールから利用できない

### 2.2 detail level との関係

現在の動作:
- `PrintFromEnvInheritance()`: dry-runモード時に**常に出力**（detail level 無視）
- `PrintFinalEnvironment()`: `DetailLevelFull` の時**のみ出力**

### 2.3 関連コードの所在

#### デバッグ出力関数
- internal/runner/debug/inheritance.go:14-107 - `PrintFromEnvInheritance()`
- internal/runner/debug/environment.go - `PrintFinalEnvironment()`

#### フォーマッター
- [internal/runner/resource/formatter.go:32-43](../../../internal/runner/resource/formatter.go#L32-L43) - `JSONFormatter`, `TextFormatter`
- [internal/runner/resource/types.go](../../../internal/runner/resource/types.go) - `DryRunResult` 構造体

#### 呼び出し箇所
- [internal/runner/group_executor.go:131-136](../../../internal/runner/group_executor.go#L131-L136) - `PrintFromEnvInheritance()` 呼び出し
- [internal/runner/group_executor.go:286-288](../../../internal/runner/group_executor.go#L286-L288) - `PrintFinalEnvironment()` 呼び出し

## 3. 要件定義

### 3.1 機能要件

#### FR-1: JSON形式でのデバッグ情報出力
`--dry-run-format json` 指定時、すべての出力をJSON形式で統合する。

**優先度**: 必須

#### FR-2: グループごとのデバッグ情報管理
各グループの実行時に収集されたデバッグ情報を、そのグループに関連付けてJSON構造に含める。

**優先度**: 必須

#### FR-3: Detail Level に基づく出力制御
各detail levelで出力するデバッグ情報を制御する：

- **`DetailLevelFull`** (最も詳細):
  - グループレベル: `InheritanceAnalysis` の**すべてのフィールド**を出力
    - 設定値: `GlobalEnvImport`, `GlobalAllowlist`, `GroupEnvImport`, `GroupAllowlist`
    - 計算値: `InheritanceMode`, `InheritedVariables`
    - 差分情報: `RemovedAllowlistVariables`, `UnavailableEnvImportVariables`
  - コマンドレベル: `FinalEnvironment` を出力

- **`DetailLevelDetailed`** (中程度の詳細):
  - グループレベル: `InheritanceAnalysis` の**基本フィールドのみ**を出力
    - 設定値: `GlobalEnvImport`, `GlobalAllowlist`, `GroupEnvImport`, `GroupAllowlist`
    - 計算値: `InheritanceMode`
    - 差分情報は**出力しない**: `RemovedAllowlistVariables`, `UnavailableEnvImportVariables`, `InheritedVariables`
  - コマンドレベル: `FinalEnvironment` は**出力しない**

- **`DetailLevelSummary`** (要約):
  - デバッグ情報を**出力しない**（`DebugInfo` フィールド自体がnull）

**優先度**: 必須

#### FR-4: テキスト形式出力の維持
`--dry-run-format text` の出力は現状のまま維持する（変更不要）。

**優先度**: 必須

### 3.2 非機能要件

#### NFR-1: JSON Schema の整合性
既存の `DryRunResult` 構造体を拡張し、後方互換性を維持する。

**優先度**: 必須

#### NFR-2: パフォーマンス
デバッグ情報の収集と格納が、実行時間に大きな影響を与えないこと（10%以内のオーバーヘッド）。

**優先度**: 推奨

#### NFR-3: テスト容易性
新しいJSON構造に対するユニットテストと統合テストを実装可能であること。

**優先度**: 必須

## 4. 提案するJSON構造

### 4.1 トップレベル構造の変更なし
既存の `DryRunResult` 構造体にフィールドを追加しない。

### 4.2 グループごとのデバッグ情報配置
`ResourceAnalysis` 構造体に `DebugInfo` フィールドを追加:

```go
type ResourceAnalysis struct {
    Type       ResourceType          `json:"type"`
    Operation  string                `json:"operation"`
    Target     string                `json:"target"`
    Impact     ResourceImpact        `json:"impact"`
    Timestamp  time.Time             `json:"timestamp"`
    Parameters map[string]any        `json:"parameters,omitempty"`
    DebugInfo  *DebugInfo            `json:"debug_info,omitempty"`  // 新規追加
}
```

### 4.3 DebugInfo 構造体

```go
type DebugInfo struct {
    // from_env 継承情報（DetailLevelDetailed以上で出力、内容はdetail levelによる）
    InheritanceAnalysis *InheritanceAnalysis `json:"inheritance_analysis,omitempty"`

    // 最終環境変数（DetailLevelFullのみで出力）
    FinalEnvironment *FinalEnvironment `json:"final_environment,omitempty"`
}

type InheritanceAnalysis struct {
    // 設定値フィールド（DetailLevelDetailed以上で出力）
    GlobalEnvImport    []string                   `json:"global_env_import"`
    GlobalAllowlist    []string                   `json:"global_allowlist"`
    GroupEnvImport     []string                   `json:"group_env_import"`
    GroupAllowlist     []string                   `json:"group_allowlist"`

    // InheritanceMode: 環境変数allowlistの継承モード（DetailLevelDetailed以上で出力）
    // runnertypes.InheritanceMode型を使用（型安全）
    // JSON出力時は String() メソッドで文字列化:
    //   InheritanceModeInherit  -> "inherit"  - グループがグローバルallowlistを継承（env_allowlist未定義）
    //   InheritanceModeExplicit -> "explicit" - グループが独自のallowlistを使用（env_allowlistに値がある）
    //   InheritanceModeReject   -> "reject"   - すべての環境変数を拒否（env_allowlist = []と明示的に空）
    InheritanceMode    runnertypes.InheritanceMode `json:"inheritance_mode"`

    // 差分情報フィールド（DetailLevelFullのみで出力）
    // 継承された変数（グループがグローバル設定を継承する場合）
    InheritedVariables []string                   `json:"inherited_variables,omitempty"`
    // env_allowlistでグローバルから削除された環境変数
    // グループがenv_allowlistをオーバーライドした際、グローバルallowlistから除外された変数
    RemovedAllowlistVariables []string            `json:"removed_allowlist_variables,omitempty"`
    // env_importオーバーライド時に利用不可になる内部変数
    // グループがenv_importをオーバーライドした際、グローバルのenv_importで定義されていた
    // 内部変数のうち、グループでは定義されていないため利用できなくなる変数
    UnavailableEnvImportVariables []string        `json:"unavailable_env_import_variables,omitempty"`
}

type FinalEnvironment struct {
    Variables map[string]EnvironmentVariable `json:"variables"`
}

type EnvironmentVariable struct {
    Value  string `json:"value,omitempty"`          // ShowSensitive=true の場合のみ
    // Source: 変数の出所
    //   "system"     - env_allowlistで許可されたシステム環境変数
    //   "env_import" - env_importでマッピングされた内部変数（元はシステム環境変数）
    //   "vars"       - varsで定義された変数
    //   "command"    - コマンドレベルで定義された変数
    Source string `json:"source"`
    Masked bool   `json:"masked,omitempty"`         // ShowSensitive=false でマスクされた場合
}
```

### 4.4 データ収集層の関数（Single Source of Truth）

テキスト形式とJSON形式の出力内容を一貫させるため、データ収集を行う専用関数を用意する：

```go
// CollectInheritanceAnalysis は継承分析情報を収集する（single source of truth）
// detailLevel に基づいて、どのフィールドを含めるかを制御する
func CollectInheritanceAnalysis(
    global *runnertypes.GlobalSpec,
    group *runnertypes.GroupSpec,
    detailLevel resource.DryRunDetailLevel,
) *InheritanceAnalysis

// CollectFinalEnvironment は最終環境変数情報を収集する
// DetailLevelFull の場合のみ非nilを返す
func CollectFinalEnvironment(
    envMap map[string]string,
    detailLevel resource.DryRunDetailLevel,
    showSensitive bool,
) *FinalEnvironment

// FormatInheritanceAnalysisText は InheritanceAnalysis をテキスト形式で出力する
func FormatInheritanceAnalysisText(
    w io.Writer,
    analysis *InheritanceAnalysis,
    groupName string,
)

// FormatFinalEnvironmentText は FinalEnvironment をテキスト形式で出力する
func FormatFinalEnvironmentText(
    w io.Writer,
    finalEnv *FinalEnvironment,
)
```

**設計ポイント**:
- `CollectInheritanceAnalysis()` 内でdetail levelに基づき、差分情報フィールド（`InheritedVariables`, `RemovedAllowlistVariables`, `UnavailableEnvImportVariables`）をnilにするか設定するかを制御
- これらのフィールドは、JSON構造体定義において `omitempty` タグを付与すること。これにより、nilの場合はJSON出力からフィールドが省略され、クリーンな出力となる（例: 212-219行目の構造体定義参照）。
- `CollectFinalEnvironment()` は`DetailLevelFull`以外ではnilを返す
- フォーマット関数は構造化データを受け取るだけなので、detail level制御は不要

### 4.5 JSON出力例

#### 4.5.1 継承モード（inherit）の例

この例では：
- グローバル設定: `env_import = ["db_host=DB_HOST", "api_key=API_KEY"]`
- グローバル設定: `env_allowlist = ["PATH", "HOME"]`
- グループはこれらを継承（オーバーライドなし）
- 実行時のシステム環境変数: `DB_HOST=localhost`, `API_KEY=secret123`
- 最終環境では、`env_allowlist`で許可された`PATH`, `HOME`と、`env_import`でマッピングされた`db_host`, `api_key`が利用可能

```json
{
  "metadata": {
    "generated_at": "2025-01-30T12:34:56Z",
    "run_id": "01JKXY...",
    "duration": 123456789
  },
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "example-group",
      "impact": {
        "description": "Group configuration analysis",
        "reversible": true,
        "persistent": false
      },
      "timestamp": "2025-01-30T12:34:56Z",
      "parameters": {
        "group_name": "example-group"
      },
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["db_host=DB_HOST", "api_key=API_KEY"],
          "global_allowlist": ["PATH", "HOME"],
          "group_env_import": [],
          "group_allowlist": [],
          "inheritance_mode": "inherit",
          "inherited_variables": ["PATH", "HOME"]
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "/usr/bin/echo",
      "impact": {
        "description": "Execute system command",
        "reversible": true,
        "persistent": false
      },
      "timestamp": "2025-01-30T12:34:56Z",
      "parameters": {
        "command": "echo",
        "args": ["Hello"],
        "working_dir": "/tmp/runner-..."
      },
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["db_host=DB_HOST", "api_key=API_KEY"],
          "global_allowlist": ["PATH", "HOME"],
          "group_env_import": [],
          "group_allowlist": [],
          "inheritance_mode": "inherit",
          "inherited_variables": ["PATH", "HOME"]
        },
        "final_environment": {
          "variables": {
            "PATH": {
              "value": "/usr/bin:/bin",
              "source": "system"
            },
            "HOME": {
              "value": "/home/user",
              "source": "system"
            },
            "db_host": {
              "value": "localhost",
              "source": "env_import"
            },
            "api_key": {
              "value": "secret123",
              "source": "env_import"
            }
          }
        }
      }
    }
  ],
  "security_analysis": {...},
  "errors": [],
  "warnings": []
}
```

#### 4.5.2 オーバーライドモード（explicit）の例

この例では：
- グローバル設定: `env_import = ["db_host=DB_HOST", "api_key=API_KEY"]`
- グローバル設定: `env_allowlist = ["PATH", "HOME", "USER"]`
- グループが`env_import`をオーバーライド: `["db_host=DB_HOST"]` → `api_key`が利用不可に
- グループが`env_allowlist`をオーバーライド: `["PATH"]` → `HOME`, `USER`が削除される
- 実行時のシステム環境変数: `DB_HOST=localhost`
- 最終環境では、`env_allowlist`で許可された`PATH`のみと、`env_import`でマッピングされた`db_host`が利用可能

```json
{
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "api-group",
      "impact": {
        "description": "Group configuration analysis",
        "reversible": true,
        "persistent": false
      },
      "timestamp": "2025-01-30T12:34:57Z",
      "parameters": {
        "group_name": "api-group"
      },
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["db_host=DB_HOST", "api_key=API_KEY"],
          "global_allowlist": ["PATH", "HOME", "USER"],
          "group_env_import": ["db_host=DB_HOST"],
          "group_allowlist": ["PATH"],
          "inheritance_mode": "explicit",
          "removed_allowlist_variables": ["HOME", "USER"],
          "unavailable_env_import_variables": ["api_key"]
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "/usr/bin/curl",
      "debug_info": {
        "final_environment": {
          "variables": {
            "PATH": {
              "value": "/usr/bin:/bin",
              "source": "system"
            },
            "db_host": {
              "value": "localhost",
              "source": "env_import"
            }
          }
        }
      }
    }
  ]
}
```

#### 4.5.3 拒否モード（reject）の例

グループが`env_allowlist = []`と明示的に設定した場合：

```json
{
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "isolated-group",
      "impact": {
        "description": "Group configuration analysis",
        "reversible": true,
        "persistent": false
      },
      "timestamp": "2025-01-30T12:34:58Z",
      "parameters": {
        "group_name": "isolated-group"
      },
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": [],
          "global_allowlist": ["PATH", "HOME"],
          "group_env_import": [],
          "group_allowlist": [],
          "inheritance_mode": "reject",
          "removed_allowlist_variables": ["PATH", "HOME"]
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "/usr/bin/python",
      "debug_info": {
        "final_environment": {
          "variables": {}
        }
      }
    }
  ]
}
```

#### 4.5.4 コマンドを含まないグループの例（DetailLevelFull）

グループレベルの`ResourceAnalysis`を使用することで、コマンドを含まないグループでも継承分析情報が出力される：

```json
{
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "empty-group",
      "impact": {
        "description": "Group configuration analysis",
        "reversible": true,
        "persistent": false
      },
      "timestamp": "2025-01-30T12:35:00Z",
      "parameters": {
        "group_name": "empty-group"
      },
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["db_host=DB_HOST"],
          "global_allowlist": ["PATH"],
          "group_env_import": [],
          "group_allowlist": [],
          "inheritance_mode": "inherit",
          "inherited_variables": ["PATH"]
        }
      }
    }
  ]
}
```

#### 4.5.5 DetailLevelDetailed での出力例

`--dry-run-detail detailed`では、差分情報（`inherited_variables`, `removed_allowlist_variables`, `unavailable_env_import_variables`）と最終環境変数（`final_environment`）は出力されない：

```json
{
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "api-group",
      "debug_info": {
        "inheritance_analysis": {
          "global_env_import": ["db_host=DB_HOST", "api_key=API_KEY"],
          "global_allowlist": ["PATH", "HOME", "USER"],
          "group_env_import": ["db_host=DB_HOST"],
          "group_allowlist": ["PATH"],
          "inheritance_mode": "explicit"
        }
      }
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "/usr/bin/curl"
    }
  ]
}
```

注: `DetailLevelDetailed`では：
- 設定値（`global_env_import`, `global_allowlist`, `group_env_import`, `group_allowlist`）は出力される
- 計算値（`inheritance_mode`）は出力される
- 差分情報（`inherited_variables`, `removed_allowlist_variables`, `unavailable_env_import_variables`）は**出力されない**
- コマンドの`debug_info`は存在しない（`final_environment`が出力されないため）

#### 4.5.6 DetailLevelSummary での出力例

`--dry-run-detail summary`では、`debug_info`フィールド自体が存在しない：

```json
{
  "resource_analyses": [
    {
      "type": "group",
      "operation": "analyze",
      "target": "api-group"
    },
    {
      "type": "command",
      "operation": "execute",
      "target": "/usr/bin/curl"
    }
  ]
}
```

## 5. 制約条件

### 5.1 技術的制約
- Go 1.23.10 を使用
- 既存の `encoding/json` パッケージを使用
- 既存の構造体定義と互換性を保つ
- `runnertypes.InheritanceMode` 型に `MarshalJSON` メソッドを実装し、JSON出力時に文字列表現（"inherit", "explicit", "reject"）を出力する
- `resource.ResourceType` に新しい型 `ResourceTypeGroup` ("group") を追加し、グループレベルの分析情報を表現する
- `resource.ResourceOperation` に新しい操作 `OperationAnalyze` ("analyze") を追加し、グループ分析操作を表現する

### 5.2 実装上の制約
- `PrintFromEnvInheritance()` と `PrintFinalEnvironment()` の既存の実装ロジックを可能な限り再利用
- テキスト形式とJSON形式で表示する情報は同一とする（single source of truth）
- Detail levelによる出力制御はテキスト形式とJSON形式の両方に適用する

### 5.3 運用上の制約
- 既存のdry-runユーザーに影響を与えない（デフォルト動作は変更なし）
- JSON Schema の後方互換性を維持（`debug_info` は optional）

## 6. 成功基準

### 6.1 機能的成功基準

#### Detail Level別の出力検証
- [ ] **`DetailLevelFull`**: すべてのデバッグ情報がJSON構造に含まれる
  - [ ] グループレベルの継承分析情報（`InheritanceAnalysis`の全フィールド）
    - [ ] 設定値: `GlobalEnvImport`, `GlobalAllowlist`, `GroupEnvImport`, `GroupAllowlist`
    - [ ] 計算値: `InheritanceMode`, `InheritedVariables`
    - [ ] 差分情報: `RemovedAllowlistVariables`, `UnavailableEnvImportVariables`
  - [ ] コマンドレベルの最終環境変数（`FinalEnvironment`）

- [ ] **`DetailLevelDetailed`**: 基本情報のみがJSON構造に含まれる
  - [ ] グループレベルの継承分析情報（基本フィールドのみ）
    - [ ] 設定値: `GlobalEnvImport`, `GlobalAllowlist`, `GroupEnvImport`, `GroupAllowlist`
    - [ ] 計算値: `InheritanceMode`
    - [ ] 差分情報は**含まれない**: `InheritedVariables`, `RemovedAllowlistVariables`, `UnavailableEnvImportVariables`
  - [ ] コマンドレベルの`debug_info`は**含まれない**

- [ ] **`DetailLevelSummary`**: デバッグ情報が含まれない
  - [ ] `debug_info`フィールド自体がnullまたは存在しない

#### その他の機能検証
- [ ] `--dry-run --dry-run-format json` で有効なJSONが出力される
- [ ] `jq` などのJSONパーサーで処理可能
- [ ] コマンドを含まないグループの継承分析情報もJSON出力に含まれる
- [ ] `--dry-run-format text` の出力は変更されない

#### テキスト形式とJSON形式の一貫性検証（Single Source of Truth）
- [ ] 同じdetail levelで、テキスト形式とJSON形式の出力に含まれる情報が一致する
  - [ ] `DetailLevelFull`: 両形式ですべてのデバッグ情報が出力される
  - [ ] `DetailLevelDetailed`: 両形式で差分情報が出力されない
  - [ ] `DetailLevelSummary`: 両形式でデバッグ情報が出力されない
- [ ] データ収集関数（`CollectInheritanceAnalysis`, `CollectFinalEnvironment`）のユニットテストが実装される
- [ ] フォーマット関数（`FormatInheritanceAnalysisText`, `FormatFinalEnvironmentText`）のユニットテストが実装される

### 6.2 品質基準
- [ ] 新しいJSON構造に対するユニットテストが実装される
- [ ] 統合テストでJSON出力の妥当性が検証される
- [ ] 既存テストがすべて成功する（回帰なし）

### 6.3 ドキュメント基準
- [ ] JSON Schema がドキュメント化される
- [ ] ユーザーマニュアルが更新される

## 7. リスクと課題

### 7.1 技術的リスク

#### R-1: デバッグ情報の収集タイミング
**リスク**: グループ実行中に段階的に収集される情報を、どのタイミングでJSON構造に統合するか。

**対策**:
- `ResourceAnalysis` 作成時に対応する `DebugInfo` を同時に作成
- グループ実行の各フェーズで `DebugInfo` を段階的に構築

#### R-2: 既存コードへの影響範囲
**リスク**: `group_executor.go` での stdout 直接出力を削除すると、他の部分への影響が大きい可能性。

**対策**:
- 段階的なリファクタリング
- 既存のテキスト出力パスは維持（条件分岐で制御）

### 7.2 設計上の課題

#### C-1: グループとコマンドのデバッグ情報の関連付け
**課題**: `PrintFromEnvInheritance()` はグループレベル、`PrintFinalEnvironment()` はコマンドレベルで呼ばれる。コマンドを含まないグループの場合、継承分析情報がJSON出力から失われてしまう。

**解決策**:
- 新しい `ResourceType` として `ResourceTypeGroup` ("group") を追加
- 各グループの処理開始時に、グループレベルの `ResourceAnalysis` を生成
- グループの継承分析情報は、このグループレベルの `ResourceAnalysis` の `DebugInfo` に含める
- 各コマンドの最終環境変数は、各コマンドの `ResourceAnalysis` の `DebugInfo` に含める
- この設計により、コマンドを含まないグループでも継承分析情報がJSON出力に含まれる

#### C-2: `DryRunResult` の構築タイミング
**課題**: 現在 `DryRunResult` は `ResourceManager` で構築されるが、`group_executor.go` で収集される情報をどう渡すか。

**解決策**:
- `ResourceManager.ExecuteCommand()` に `DebugInfo` を渡すパラメータを追加
- または `ResourceAnalysis` 作成後に `DebugInfo` を追加するメソッドを提供

#### C-3: `InheritanceMode` 型のJSON変換
**課題**: `runnertypes.InheritanceMode` は `int` ベースの列挙型であり、デフォルトのJSON marshalingでは数値（0, 1, 2）として出力される。ユーザーフレンドリーなJSON出力には文字列表現（"inherit", "explicit", "reject"）が必要。

**解決策**:
- `runnertypes.InheritanceMode` 型に `MarshalJSON() ([]byte, error)` メソッドを実装
- 既存の `String()` メソッドを活用して文字列表現を返す
- `UnmarshalJSON([]byte) error` メソッドも実装し、JSON読み込み時の対称性を確保（将来の拡張性のため）

#### C-4: テキスト形式とJSON形式の出力内容の一貫性（Single Source of Truth）
**課題**: 現在、テキスト形式の出力（`PrintFromEnvInheritance()`, `PrintFinalEnvironment()`）とJSON形式の出力は独立した実装になっており、表示する情報に不整合が発生するリスクがある。また、detail levelによる出力制御もそれぞれで実装する必要がある。

**解決策アプローチ1: データ収集とフォーマット分離** (推奨)
1. **データ収集層**を新設:
   - デバッグ情報を収集して構造化データ（`DebugInfo`, `InheritanceAnalysis`, `FinalEnvironment`）を生成する関数を作成
   - Detail levelに基づいてどのフィールドを含めるかを制御
   - 例: `CollectInheritanceAnalysis(global, group, detailLevel) *InheritanceAnalysis`

2. **フォーマット層**を分離:
   - テキストフォーマッター: 構造化データを受け取り、テキスト形式で出力
   - JSONフォーマッター: 構造化データを受け取り、JSON形式で出力（既存の`encoding/json`を使用）

3. **メリット**:
   - データ収集ロジックが1箇所に集約（single source of truth）
   - Detail level制御が1箇所で実装可能
   - テストが容易（データ収集とフォーマットを独立してテスト可能）
   - 将来的に他の出力形式（YAML, XMLなど）を追加しやすい

**解決策アプローチ2: 既存関数の段階的リファクタリング**
1. 既存の`PrintFromEnvInheritance()`を2つの関数に分割:
   - `CollectInheritanceAnalysis()`: データ収集のみ
   - `FormatInheritanceAnalysis()`: テキスト形式での出力

2. JSON出力では`CollectInheritanceAnalysis()`の結果を直接使用

3. **メリット**:
   - 既存コードへの影響を最小化
   - 段階的な移行が可能

**推奨**: アプローチ1を採用。理由:
- より明確な責任分離
- 長期的な保守性が高い
- Detail level制御の実装が簡潔

## 8. 将来の拡張（Phase 6+）

以下の機能はPhase 5.5実装後の将来課題として記録する。

### 8.1 Phase 6: 詳細なskip_reason値の定義

**現状**: Phase 5.5では`skip_reason`フィールドが追加されたが、値は未定義の文字列。

**提案**:
- 定義済みのskip理由を列挙:
  - `"parent_group_failed"` - 親グループが失敗したためスキップ
  - `"dependency_not_met"` - 依存関係が満たされていない
  - `"conditional_skip"` - 条件付きスキップ（将来の機能）
  - `"user_requested"` - ユーザーが明示的にスキップを要求

**優先度**: 中

### 8.2 Phase 6: エラーdetailsの構造化

**現状**: `ExecutionError.Details`は`map[string]any`で非構造化。

**提案**:
- エラータイプごとに標準化されたdetails構造を定義:

```go
// 設定検証エラーの場合
{
  "type": "config_validation_error",
  "details": {
    "system_var_name": "SECRET_KEY",
    "internal_var_name": "my_secret",
    "level": "group",
    "suggestion": "Add 'SECRET_KEY' to global.env_allowed"
  }
}

// ファイル検証エラーの場合
{
  "type": "verification_error",
  "details": {
    "total_files": 10,
    "verified_files": 8,
    "failed_files": 2,
    "failed_file_paths": ["/path/to/file1", "/path/to/file2"]
  }
}
```

**優先度**: 中

### 8.3 Phase 7: タイムスタンプフィールドの追加

**現状**: エラー発生時刻が記録されない。

**提案**:
```go
type ExecutionError struct {
    Type      string         `json:"type"`
    Message   string         `json:"message"`
    Component string         `json:"component"`
    Details   map[string]any `json:"details,omitempty"`
    OccurredAt time.Time     `json:"occurred_at"` // 新規追加
}

type ResourceAnalysis struct {
    // ... 既存フィールド ...
    StartedAt   *time.Time    `json:"started_at,omitempty"`   // 新規追加
    CompletedAt *time.Time    `json:"completed_at,omitempty"` // 新規追加
}
```

**優先度**: 低

### 8.4 Phase 8: エラーリカバリー提案

**現状**: エラーメッセージのみで、ユーザーが対処方法を判断する必要がある。

**提案**:
```go
type ExecutionError struct {
    // ... 既存フィールド ...
    Suggestion string `json:"suggestion,omitempty"` // 新規追加
    RecoverySteps []string `json:"recovery_steps,omitempty"` // 新規追加
}
```

**例**:
```json
{
  "error": {
    "type": "config_validation_error",
    "message": "system environment variable 'SECRET_KEY' not in allowlist",
    "suggestion": "Add 'SECRET_KEY' to global.env_allowed in your configuration",
    "recovery_steps": [
      "Open your configuration file",
      "Add 'SECRET_KEY = \"internal_name\"' to [global.env_allowed] section",
      "Re-run the command"
    ]
  }
}
```

**優先度**: 低

### 8.5 Phase 9: エラー重要度レベル

**現状**: すべてのエラーが同等に扱われる。

**提案**:
```go
type ErrorSeverity string

const (
    SeverityFatal   ErrorSeverity = "fatal"   // 実行を継続できない
    SeverityError   ErrorSeverity = "error"   // 重大なエラーだが部分実行可能
    SeverityWarning ErrorSeverity = "warning" // 警告レベル
)

type ExecutionError struct {
    // ... 既存フィールド ...
    Severity ErrorSeverity `json:"severity"` // 新規追加
}
```

**優先度**: 低

### 8.6 将来課題の実装時の注意事項

- **後方互換性**: 新しいフィールドは常に`omitempty`タグを付与し、既存のJSONパーサーへの影響を最小化
- **段階的導入**: 各フェーズは独立して実装・テスト可能であること
- **ドキュメント更新**: 各フェーズの実装時に、JSON Schemaとユーザードキュメントを更新

## 9. 次のステップ

1. アーキテクチャ設計書の作成（02_architecture.ja.md）
2. 詳細設計書の作成（03_detailed_design.ja.md）
3. 実装計画書の作成（04_implementation_plan.ja.md）
4. 実装とテスト
5. ドキュメント更新
