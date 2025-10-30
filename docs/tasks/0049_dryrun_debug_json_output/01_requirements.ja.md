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
       _, _ = fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")
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
- [internal/runner/debug/inheritance.go:14-107](../../../internal/runner/debug/inheritance.go#L14-L107) - `PrintFromEnvInheritance()`
- [internal/runner/debug/environment.go](../../../internal/runner/debug/environment.go) - `PrintFinalEnvironment()`

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
- `DetailLevelFull`:
  - `from_env` 継承情報を出力
  - 最終環境変数を出力
- `DetailLevelDetailed` 以下: デバッグ情報を出力しない

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
    // from_env 継承情報（DetailLevelFull のみ）
    InheritanceAnalysis *InheritanceAnalysis `json:"inheritance_analysis,omitempty"`

    // 最終環境変数（DetailLevelFull のみ）
    FinalEnvironment *FinalEnvironment `json:"final_environment,omitempty"`
}

type InheritanceAnalysis struct {
    GlobalEnvImport    []string            `json:"global_env_import"`
    GlobalAllowlist    []string            `json:"global_allowlist"`
    GroupEnvImport     []string            `json:"group_env_import"`
    GroupAllowlist     []string            `json:"group_allowlist"`
    InheritanceMode    string              `json:"inheritance_mode"` // "inherit", "explicit", "reject"
    InheritedVariables []string            `json:"inherited_variables,omitempty"`
    RemovedVariables   []string            `json:"removed_variables,omitempty"`
}

type FinalEnvironment struct {
    Variables map[string]EnvironmentVariable `json:"variables"`
}

type EnvironmentVariable struct {
    Value  string `json:"value,omitempty"`          // ShowSensitive=true の場合のみ
    Source string `json:"source"`                   // "global", "group", "command", "system"
    Masked bool   `json:"masked,omitempty"`         // ShowSensitive=false でマスクされた場合
}
```

### 4.4 JSON出力例

```json
{
  "metadata": {
    "generated_at": "2025-01-30T12:34:56Z",
    "run_id": "01JKXY...",
    "duration": 123456789
  },
  "resource_analyses": [
    {
      "type": "command_execution",
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
          "global_env_import": [],
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

## 5. 制約条件

### 5.1 技術的制約
- Go 1.23.10 を使用
- 既存の `encoding/json` パッケージを使用
- 既存の構造体定義と互換性を保つ

### 5.2 実装上の制約
- `PrintFromEnvInheritance()` と `PrintFinalEnvironment()` の既存の実装ロジックを可能な限り再利用
- テキスト形式出力には変更を加えない

### 5.3 運用上の制約
- 既存のdry-runユーザーに影響を与えない（デフォルト動作は変更なし）
- JSON Schema の後方互換性を維持（`debug_info` は optional）

## 6. 成功基準

### 6.1 機能的成功基準
- [ ] `--dry-run --dry-run-format json --dry-run-detail full` で有効なJSONが出力される
- [ ] `jq` などのJSONパーサーで処理可能
- [ ] すべてのデバッグ情報がJSON構造に含まれる
- [ ] `--dry-run-format text` の出力は変更されない

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
**課題**: `PrintFromEnvInheritance()` はグループレベル、`PrintFinalEnvironment()` はコマンドレベルで呼ばれる。

**解決策**:
- グループの継承情報は最初のコマンドの `DebugInfo` に含める
- 各コマンドの最終環境変数は各コマンドの `DebugInfo` に含める

#### C-2: `DryRunResult` の構築タイミング
**課題**: 現在 `DryRunResult` は `ResourceManager` で構築されるが、`group_executor.go` で収集される情報をどう渡すか。

**解決策**:
- `ResourceManager.ExecuteCommand()` に `DebugInfo` を渡すパラメータを追加
- または `ResourceAnalysis` 作成後に `DebugInfo` を追加するメソッドを提供

## 8. 次のステップ

1. アーキテクチャ設計書の作成（02_architecture.ja.md）
2. 詳細設計書の作成（03_detailed_design.ja.md）
3. 実装計画書の作成（04_implementation_plan.ja.md）
4. 実装とテスト
5. ドキュメント更新
