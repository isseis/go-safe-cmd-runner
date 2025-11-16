# 詳細仕様書: CommandResults 型安全性改善

## 目次

1. [概要](#概要)
2. [型定義の詳細仕様](#型定義の詳細仕様)
3. [LogValue() の実装仕様](#logvalue-の実装仕様)
4. [extractCommandResults の簡略化仕様](#extractcommandresults-の簡略化仕様)
5. [RedactingHandler との統合仕様](#redactinghandler-との統合仕様)
6. [使用箇所の更新仕様](#使用箇所の更新仕様)
7. [エラーハンドリング仕様](#エラーハンドリング仕様)
8. [パフォーマンス要件](#パフォーマンス要件)
9. [テスト仕様](#テスト仕様)

## 概要

本仕様書は、アーキテクチャ設計書（`02_architecture.md`）に基づき、CommandResults 型の実装における技術的詳細を定義する。

### 設計方針の確定事項

以下の設計方針が確定している:

1. **ユーティリティメソッド**: YAGNI 原則に従い、実装しない
   - 調査の結果、`Len()`, `HasFailures()`, `SuccessCount()` は現在のコードベースで使用されていない
   - Option A（純粋な LogValuer 実装）を採用

2. **大量コマンド結果の処理**: 最大100件を想定
   - 通常時: 10件程度
   - 最大時: 100件
   - ログの切り詰め戦略: 100件を上限とする

3. **移行方式**: 新形式への一括移行
   - Phase 2 で extractCommandResults を更新
   - Phase 3 で全ての使用箇所を一括変更
   - Phase 4 は不要（後方互換コードを含めない）

4. **エラーハンドリング**: デバッグログを追加
   - パース失敗時に詳細なデバッグログを出力
   - メトリクス収集は実装しない（YAGNI）

## 型定義の詳細仕様

### CommandResults 型

#### ファイル配置

- **ファイルパス**: `internal/common/logschema.go`
- **パッケージ**: `common`
- **配置理由**: 既存の `CommandResult` 型と同じファイルに配置し、関連する型を集約

#### 型定義

```go
// CommandResults は複数のコマンド実行結果を保持する型
//
// この型はスライス全体で slog.LogValuer インターフェースを実装することで、
// RedactingHandler によるスライス要素の型変換問題を回避する。
//
// 設計上の特徴:
//   - []CommandResult のエイリアス型（基底型は同じ）
//   - LogValuer を実装し、Group 構造でログ出力
//   - ユーティリティメソッドは含めない（YAGNI 原則）
//
// 使用例:
//   results := []common.CommandResult{...}
//   commandResults := common.CommandResults(results)
//   slog.InfoContext(ctx, "Group completed",
//       slog.Any(common.GroupSummaryAttrs.Commands, commandResults))
type CommandResults []CommandResult
```

#### 型の特性

| 項目 | 仕様 |
|------|------|
| **基底型** | `[]CommandResult` |
| **変換方法** | 型変換: `CommandResults(slice)` |
| **ゼロ値** | `nil` または空スライス `CommandResults{}` |
| **nil 安全性** | `LogValue()` は nil スライスを安全に処理 |
| **不変性** | スライスの内容を変更可能（不変型ではない） |

#### インターフェース実装

```go
var _ slog.LogValuer = CommandResults(nil) // コンパイル時チェック
```

`CommandResults` 型は `slog.LogValuer` インターフェースを実装する:

```go
type LogValuer interface {
    LogValue() slog.Value
}
```

## LogValue() の実装仕様

### メソッドシグネチャ

```go
// LogValue implements slog.LogValuer to provide structured logging support.
//
// この実装は、スライス内の各 CommandResult を個別の Group として構造化し、
// 全体を1つの GroupValue として返す。これにより:
//   - slog が LogValuer を認識し、processSlice() を回避
//   - RedactingHandler が Group として redaction を実行
//   - SlackHandler がシンプルな Group 処理で情報を抽出
//
// 出力構造:
//   GroupValue {
//     "total_count": int,
//     "cmd_0": GroupValue { "name": ..., "exit_code": ..., ... },
//     "cmd_1": GroupValue { ... },
//     ...
//   }
//
// 制限事項:
//   - 最大100件のコマンド結果を含める（MaxLoggedCommands）
//   - それを超える場合は切り詰め、truncated フラグを設定
func (cr CommandResults) LogValue() slog.Value
```

### 実装の詳細

#### 定数定義

```go
const (
    // MaxLoggedCommands は1回のログ出力に含めるコマンド結果の上限
    // この制限により、ログサイズとパフォーマンスを管理する
    MaxLoggedCommands = 100
)
```

#### 実装ロジック

```go
func (cr CommandResults) LogValue() slog.Value {
    // nil または空スライスの処理
    if len(cr) == 0 {
        return slog.GroupValue(
            slog.Int("total_count", 0),
        )
    }

    // 切り詰め判定
    commandsToLog := cr
    truncated := false
    if len(cr) > MaxLoggedCommands {
        commandsToLog = cr[:MaxLoggedCommands]
        truncated = true
    }

    // 属性配列を事前確保（最適化）
    // +2: total_count と truncated フラグ
    // +len(commandsToLog): 各コマンド
    attrs := make([]slog.Attr, 0, len(commandsToLog)+2)

    // メタデータを追加
    attrs = append(attrs,
        slog.Int("total_count", len(cr)),
        slog.Bool("truncated", truncated),
    )

    // 各コマンド結果を Group として追加
    for i, cmd := range commandsToLog {
        // キー名は "cmd_0", "cmd_1", ... の形式
        // 数値のみ（"0", "1", ...）にすると JSON での可読性が低下するため、
        // プレフィックス "cmd_" を付与
        attrs = append(attrs, slog.Group(
            fmt.Sprintf("cmd_%d", i),
            slog.String(LogFieldName, cmd.Name),
            slog.Int(LogFieldExitCode, cmd.ExitCode),
            slog.String(LogFieldOutput, cmd.Output),
            slog.String(LogFieldStderr, cmd.Stderr),
        ))
    }

    return slog.GroupValue(attrs...)
}
```

#### データ構造の例

**入力**:
```go
results := CommandResults{
    {CommandResultFields: CommandResultFields{Name: "test1", ExitCode: 0, Output: "ok", Stderr: ""}},
    {CommandResultFields: CommandResultFields{Name: "test2", ExitCode: 1, Output: "", Stderr: "error"}},
}
```

**出力（slog.Value の内部構造）**:
```
slog.Value {
    Kind: KindGroup
    Group: []Attr{
        {Key: "total_count", Value: IntValue(2)},
        {Key: "truncated", Value: BoolValue(false)},
        {Key: "cmd_0", Value: GroupValue{
            {Key: "name", Value: StringValue("test1")},
            {Key: "exit_code", Value: IntValue(0)},
            {Key: "output", Value: StringValue("ok")},
            {Key: "stderr", Value: StringValue("")},
        }},
        {Key: "cmd_1", Value: GroupValue{
            {Key: "name", Value: StringValue("test2")},
            {Key: "exit_code", Value: IntValue(1)},
            {Key: "output", Value: StringValue("")},
            {Key: "stderr", Value: StringValue("error")},
        }},
    }
}
```

**JSON シリアライズ後**:
```json
{
  "total_count": 2,
  "truncated": false,
  "cmd_0": {
    "name": "test1",
    "exit_code": 0,
    "output": "ok",
    "stderr": ""
  },
  "cmd_1": {
    "name": "test2",
    "exit_code": 1,
    "output": "",
    "stderr": "error"
  }
}
```

### エッジケース

#### 1. nil スライス

```go
var cr CommandResults // nil
value := cr.LogValue()
// 結果: GroupValue{ "total_count": 0 }
```

#### 2. 空スライス

```go
cr := CommandResults{}
value := cr.LogValue()
// 結果: GroupValue{ "total_count": 0 }
```

#### 3. 切り詰め発生

```go
// 150件のコマンド結果
cr := make(CommandResults, 150)
value := cr.LogValue()
// 結果: GroupValue{
//   "total_count": 150,
//   "truncated": true,
//   "cmd_0": ...,
//   ...
//   "cmd_99": ..., // 100件まで
// }
```

#### 4. ちょうど100件

```go
cr := make(CommandResults, 100)
value := cr.LogValue()
// 結果: GroupValue{
//   "total_count": 100,
//   "truncated": false,
//   "cmd_0": ...,
//   ...
//   "cmd_99": ...,
// }
```

## extractCommandResults の簡略化仕様

### 関数シグネチャ

```go
// extractCommandResultsFromGroup extracts command result information from a Group value.
//
// この関数は CommandResults.LogValue() が生成した Group 構造から
// コマンド情報を抽出する。
//
// 入力形式:
//   - value.Kind() == slog.KindGroup
//   - Group の各属性が "cmd_N" という名前の Group
//   - 各 "cmd_N" Group が CommandResult のフィールドを含む
//
// 処理:
//   1. Group の属性を取得
//   2. "total_count" と "truncated" をスキップ
//   3. 各 "cmd_N" Group から情報を抽出
//   4. extractFromAttrs() で個別のコマンド情報を生成
//
// エラーハンドリング:
//   - 不正な形式の場合は nil を返す
//   - デバッグログで詳細を記録
//
// 旧実装との違い:
//   - 型チェック: 6回 → 2回
//   - 処理パス: 4パス → 1パス
//   - 型アサーション: あり → なし
func extractCommandResultsFromGroup(groupValue slog.Value) []commandResultInfo
```

### 実装の詳細

```go
func extractCommandResultsFromGroup(groupValue slog.Value) []commandResultInfo {
    // 型チェック 1: Group であることを確認
    if groupValue.Kind() != slog.KindGroup {
        slog.Debug("Command results extraction failed: unexpected value kind",
            "expected", "KindGroup",
            "actual", groupValue.Kind(),
            "function", "extractCommandResultsFromGroup",
        )
        return nil
    }

    // Group の属性を取得
    attrs := groupValue.Group()
    if len(attrs) == 0 {
        slog.Debug("Command results extraction: empty group",
            "function", "extractCommandResultsFromGroup",
        )
        return nil
    }

    // メタデータフィールドを除外し、コマンド数を推定
    // total_count と truncated を除く
    estimatedCmdCount := len(attrs) - 2
    if estimatedCmdCount < 0 {
        estimatedCmdCount = 0
    }

    commands := make([]commandResultInfo, 0, estimatedCmdCount)
    skipped := 0

    for i, attr := range attrs {
        // メタデータフィールドをスキップ
        if attr.Key == "total_count" || attr.Key == "truncated" {
            continue
        }

        // 型チェック 2: 各属性が Group であることを確認
        if attr.Value.Kind() != slog.KindGroup {
            slog.Debug("Skipping non-group attribute in command results",
                "index", i,
                "key", attr.Key,
                "kind", attr.Value.Kind(),
                "function", "extractCommandResultsFromGroup",
            )
            skipped++
            continue
        }

        // コマンド情報を抽出（既存の extractFromAttrs を再利用）
        cmdAttrs := attr.Value.Group()
        cmdInfo := extractFromAttrs(cmdAttrs)

        // 必須フィールドの検証
        if cmdInfo.Name == "" {
            slog.Debug("Skipping command result with missing name",
                "index", i,
                "key", attr.Key,
                "function", "extractCommandResultsFromGroup",
            )
            skipped++
            continue
        }

        commands = append(commands, cmdInfo)
    }

    if skipped > 0 {
        slog.Debug("Command results extraction completed with some skipped items",
            "extracted", len(commands),
            "skipped", skipped,
            "total_attrs", len(attrs),
            "function", "extractCommandResultsFromGroup",
        )
    }

    return commands
}
```

### 既存の extractFromAttrs の再利用

`extractFromAttrs` 関数は既存のまま使用する:

```go
// extractFromAttrs extracts command result information from slog attributes
// This is a helper function used by extractCommandResultsFromGroup
func extractFromAttrs(attrs []slog.Attr) commandResultInfo {
    info := commandResultInfo{}
    for _, attr := range attrs {
        switch attr.Key {
        case common.LogFieldName:
            info.Name = attr.Value.String()
        case common.LogFieldExitCode:
            info.ExitCode = int(attr.Value.Int64())
        case common.LogFieldOutput:
            info.Output = attr.Value.String()
        case common.LogFieldStderr:
            info.Stderr = attr.Value.String()
        }
    }
    return info
}
```

### 処理フローの比較

#### 旧実装（複雑）

```
extractCommandResults(value)
  ↓
value.Kind() == KindAny? → No → return nil
  ↓ Yes
anyVal := value.Any()
  ↓
anyVal.([]CommandResult)? → Yes → Process1
  ↓ No
anyVal.([]any)? → No → return nil
  ↓ Yes
Loop: for elem in anyVal
  ↓
elem.(CommandResult)? → Yes → Process2
  ↓ No
elem.(slog.Value)? → Yes → Process3
  ↓ No
elem.([]slog.Attr)? → Yes → Process4
  ↓ No
continue loop
```

#### 新実装（シンプル）

```
extractCommandResultsFromGroup(value)
  ↓
value.Kind() == KindGroup? → No → return nil
  ↓ Yes
attrs := value.Group()
  ↓
Loop: for attr in attrs
  ↓
attr.Key == metadata? → Yes → skip
  ↓ No
attr.Value.Kind() == KindGroup? → No → skip
  ↓ Yes
extractFromAttrs(attr.Value.Group())
  ↓
Validate name field
  ↓
Append to results
```

### コード行数の削減

| 項目 | 旧実装 | 新実装 | 削減率 |
|------|--------|--------|--------|
| 型チェック | 6回 | 2回 | 67% |
| 処理パス | 4パス | 1パス | 75% |
| コード行数 | 約60行 | 約30行 | 50% |
| 循環複雑度 | 8 | 3 | 63% |

## RedactingHandler との統合仕様

### RedactingHandler の動作

`RedactingHandler` は slog.Handler の wrapper として動作し、ログ属性を redaction（機密情報のマスク）する。

#### 処理フロー

```mermaid
sequenceDiagram
    participant App as Application
    participant RH as RedactingHandler
    participant CR as CommandResults
    participant UH as Underlying Handler

    App->>RH: Handle(record)
    activate RH

    RH->>RH: record.Attrs() でループ

    loop 各属性
        RH->>RH: attr = "commands"
        RH->>RH: value.Kind() を確認

        alt KindLogValuer
            RH->>RH: processLogValuer()
            RH->>CR: LogValue() 呼び出し
            activate CR
            CR-->>RH: GroupValue
            deactivate CR
            RH->>RH: redactLogAttribute() で Group を処理
            Note over RH: ネストした Group の各属性を redaction
        end

        RH->>RH: redacted attr を newRecord に追加
    end

    RH->>UH: Handle(newRecord)
    deactivate RH
```

### Group Redaction の詳細

`RedactingHandler` は Group 構造を再帰的に処理し、各属性を redaction する。

#### 処理の流れ

1. **LogValuer の検出**:
   ```go
   if value.Kind() == slog.KindLogValuer {
       logValuer := value.Any().(slog.LogValuer)
       value = logValuer.LogValue() // CommandResults.LogValue() が呼ばれる
   }
   ```

2. **Group の Redaction**:
   ```go
   if value.Kind() == slog.KindGroup {
       attrs := value.Group()
       redactedAttrs := make([]slog.Attr, len(attrs))
       for i, attr := range attrs {
           redactedAttrs[i] = slog.Attr{
               Key:   attr.Key,
               Value: redactLogAttribute(attr.Value, ctx), // 再帰的に処理
           }
       }
       return slog.GroupValue(redactedAttrs...)
   }
   ```

3. **String 値の Redaction**:
   ```go
   if value.Kind() == slog.KindString {
       str := value.String()
       redacted := redactor.RedactSensitiveData(str)
       return slog.StringValue(redacted)
   }
   ```

### Redaction の例

#### 入力（CommandResults.LogValue() の出力）

```json
{
  "total_count": 2,
  "truncated": false,
  "cmd_0": {
    "name": "setup",
    "exit_code": 0,
    "output": "Database password=secret123 configured",
    "stderr": ""
  },
  "cmd_1": {
    "name": "deploy",
    "exit_code": 0,
    "output": "API key=sk-1234567890abcdef set",
    "stderr": ""
  }
}
```

#### 出力（RedactingHandler 処理後）

```json
{
  "total_count": 2,
  "truncated": false,
  "cmd_0": {
    "name": "setup",
    "exit_code": 0,
    "output": "Database password=[REDACTED] configured",
    "stderr": ""
  },
  "cmd_1": {
    "name": "deploy",
    "exit_code": 0,
    "output": "API key=[REDACTED] set",
    "stderr": ""
  }
}
```

### 重要な特性

1. **型変換の回避**:
   - `CommandResults` は `LogValuer` として認識される
   - `processSlice()` が呼ばれず、`[]any` への変換が発生しない

2. **構造の維持**:
   - Group 構造がそのまま維持される
   - `extractCommandResultsFromGroup` がシンプルな処理で情報を抽出できる

3. **Redaction の完全性**:
   - ネストした Group の各フィールドが個別に redaction される
   - `output` と `stderr` フィールドに含まれる機密情報が適切にマスクされる

## 使用箇所の更新仕様

### 影響範囲

以下のファイルで `[]CommandResult` が使用されている:

1. **internal/runner/group_executor.go** (主要な使用箇所)
2. **internal/runner/runner.go** (通知関数)
3. **internal/logging/slack_handler.go** (extractCommandResults)
4. テストファイル (複数)

### group_executor.go の更新

#### 変更箇所: executeAllCommands メソッド

**Before**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) ([]common.CommandResult, *groupExecutionResult, error) {
    commandResults := make([]common.CommandResult, 0, len(groupSpec.Commands))

    // ... コマンド実行ループ ...

    return commandResults, nil, nil
}
```

**After**:
```go
func (ge *DefaultGroupExecutor) executeAllCommands(
    ctx context.Context,
    groupSpec *runnertypes.GroupSpec,
    runtimeGroup *runnertypes.RuntimeGroup,
    runtimeGlobal *runnertypes.RuntimeGlobal,
) ([]common.CommandResult, *groupExecutionResult, error) {
    commandResults := make([]common.CommandResult, 0, len(groupSpec.Commands))

    // ... コマンド実行ループ ...

    // 戻り値の型は変更しない（[]common.CommandResult のまま）
    // 呼び出し側で CommandResults に変換する
    return commandResults, nil, nil
}
```

**変更なし**: 戻り値の型は `[]common.CommandResult` のまま維持する。

#### 変更箇所: ExecuteGroup メソッド

**Before**:
```go
// 7. Execute commands in the group sequentially
commandResults, errResult, err := ge.executeAllCommands(ctx, groupSpec, runtimeGroup, runtimeGlobal)
if err != nil {
    executionResult = errResult
    return err
}

// Set success result for notification
executionResult = &groupExecutionResult{
    status:   GroupExecutionStatusSuccess,
    commands: commandResults,
    errorMsg: "",
}

slog.Info("Group completed successfully", slog.String("name", groupSpec.Name))
return nil
```

**After**:
```go
// 7. Execute commands in the group sequentially
commandResults, errResult, err := ge.executeAllCommands(ctx, groupSpec, runtimeGroup, runtimeGlobal)
if err != nil {
    executionResult = errResult
    return err
}

// Set success result for notification
executionResult = &groupExecutionResult{
    status:   GroupExecutionStatusSuccess,
    commands: commandResults, // []common.CommandResult のまま（groupExecutionResult の型に合わせる）
    errorMsg: "",
}

slog.Info("Group completed successfully", slog.String("name", groupSpec.Name))
return nil
```

**変更なし**: `groupExecutionResult.commands` フィールドは `[]common.CommandResult` のまま。

### runner.go の更新

#### 変更箇所: logGroupExecutionSummary 関数

**Before**:
```go
func logGroupExecutionSummary(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
    slog.InfoContext(
        context.Background(),
        "Command group execution summary",
        slog.String(common.GroupSummaryAttrs.Status, string(result.status)),
        slog.String(common.GroupSummaryAttrs.Group, group.Name),
        slog.Int64(common.GroupSummaryAttrs.DurationMs, duration.Milliseconds()),
        slog.Any(common.GroupSummaryAttrs.Commands, result.commands),
    )
}
```

**After**:
```go
func logGroupExecutionSummary(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration) {
    // []CommandResult を CommandResults に変換してログ記録
    commandResults := common.CommandResults(result.commands)

    slog.InfoContext(
        context.Background(),
        "Command group execution summary",
        slog.String(common.GroupSummaryAttrs.Status, string(result.status)),
        slog.String(common.GroupSummaryAttrs.Group, group.Name),
        slog.Int64(common.GroupSummaryAttrs.DurationMs, duration.Milliseconds()),
        slog.Any(common.GroupSummaryAttrs.Commands, commandResults),
    )
}
```

**変更内容**: `slog.Any()` に渡す前に `CommandResults` に型変換。

### slack_handler.go の更新

#### 変更箇所: extractCommandResults 関数

**Before**:
```go
// extractCommandResults は複雑な型アサーションを含む（約60行）
func extractCommandResults(value slog.Value) []commandResultInfo {
    // ... 6段階の型チェックと4つの処理パス ...
}
```

**After**:
```go
// extractCommandResults は新形式の Group 処理に置き換え
func extractCommandResults(value slog.Value) []commandResultInfo {
    return extractCommandResultsFromGroup(value)
}

// extractCommandResultsFromGroup は新しい実装（約30行）
func extractCommandResultsFromGroup(groupValue slog.Value) []commandResultInfo {
    // ... シンプルな Group 処理 ...
}
```

**変更内容**: 既存の `extractCommandResults` は `extractCommandResultsFromGroup` の wrapper に変更。

### テストファイルの更新

#### slack_handler_test.go

**テストケースの更新**:

1. **新形式のテストケース追加**:
   ```go
   func TestExtractCommandResults_NewFormat(t *testing.T) {
       // CommandResults 型を使用
       results := common.CommandResults{
           {CommandResultFields: common.CommandResultFields{Name: "test1", ExitCode: 0}},
           {CommandResultFields: common.CommandResultFields{Name: "test2", ExitCode: 1}},
       }
       value := results.LogValue()

       commands := extractCommandResults(value)

       assert.Len(t, commands, 2)
       assert.Equal(t, "test1", commands[0].Name)
       assert.Equal(t, 0, commands[0].ExitCode)
   }
   ```

2. **既存のテストケースは削除または更新**:
   - 旧形式（`[]CommandResult` を直接 `slog.Any()` に渡す）のテストは削除
   - または新形式に更新

## エラーハンドリング仕様

### デバッグログの要件

#### ログレベル

- **レベル**: `slog.Debug`
- **理由**: パース失敗は通常のエラーではなく、開発時のデバッグ情報

#### ログ出力のタイミング

1. **不正な Kind の検出時**:
   ```go
   if groupValue.Kind() != slog.KindGroup {
       slog.Debug("Command results extraction failed: unexpected value kind",
           "expected", "KindGroup",
           "actual", groupValue.Kind(),
           "function", "extractCommandResultsFromGroup",
       )
       return nil
   }
   ```

2. **空の Group の検出時**:
   ```go
   if len(attrs) == 0 {
       slog.Debug("Command results extraction: empty group",
           "function", "extractCommandResultsFromGroup",
       )
       return nil
   }
   ```

3. **非 Group 属性のスキップ時**:
   ```go
   if attr.Value.Kind() != slog.KindGroup {
       slog.Debug("Skipping non-group attribute in command results",
           "index", i,
           "key", attr.Key,
           "kind", attr.Value.Kind(),
           "function", "extractCommandResultsFromGroup",
       )
       skipped++
       continue
   }
   ```

4. **必須フィールド欠損時**:
   ```go
   if cmdInfo.Name == "" {
       slog.Debug("Skipping command result with missing name",
           "index", i,
           "key", attr.Key,
           "function", "extractCommandResultsFromGroup",
       )
       skipped++
       continue
   }
   ```

5. **スキップ発生時のサマリー**:
   ```go
   if skipped > 0 {
       slog.Debug("Command results extraction completed with some skipped items",
           "extracted", len(commands),
           "skipped", skipped,
           "total_attrs", len(attrs),
           "function", "extractCommandResultsFromGroup",
       )
   }
   ```

### ログ属性の標準化

すべてのデバッグログに含める属性:

| 属性名 | 型 | 説明 |
|--------|----|----|
| `function` | string | 関数名（"extractCommandResultsFromGroup"） |
| `expected` | string | 期待される値の種類 |
| `actual` | any | 実際の値または Kind |
| `index` | int | 配列インデックス（該当する場合） |
| `key` | string | 属性のキー名（該当する場合） |
| `kind` | slog.Kind | slog.Value の Kind（該当する場合） |

### エラーからの回復

`extractCommandResultsFromGroup` は以下のエラーから回復し、部分的な結果を返す:

1. **非 Group 属性**: スキップして次の属性を処理
2. **必須フィールド欠損**: そのコマンドをスキップ
3. **不正な形式**: デバッグログを出力し、`nil` を返す

## パフォーマンス要件

### ベンチマーク測定項目

#### 1. CommandResults.LogValue()

```go
func BenchmarkCommandResultsLogValue(b *testing.B) {
    sizes := []int{10, 50, 100}

    for _, size := range sizes {
        b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
            results := createCommandResults(size)
            commandResults := common.CommandResults(results)

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _ = commandResults.LogValue()
            }
        })
    }
}
```

**目標値**:

| コマンド数 | 目標処理時間 | メモリ割り当て目標 |
|-----------|------------|------------------|
| 10件 | 500 ns/op 以下 | 5 KB/op 以下 |
| 50件 | 2.5 μs/op 以下 | 25 KB/op 以下 |
| 100件 | 5 μs/op 以下 | 50 KB/op 以下 |

#### 2. extractCommandResultsFromGroup()

```go
func BenchmarkExtractCommandResultsFromGroup(b *testing.B) {
    sizes := []int{10, 50, 100}

    for _, size := range sizes {
        b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
            results := common.CommandResults(createCommandResults(size))
            value := results.LogValue()

            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                _ = extractCommandResultsFromGroup(value)
            }
        })
    }
}
```

**目標値**:

| コマンド数 | 目標処理時間 | メモリ割り当て目標 |
|-----------|------------|------------------|
| 10件 | 200 ns/op 以下 | 2 KB/op 以下 |
| 50件 | 1 μs/op 以下 | 10 KB/op 以下 |
| 100件 | 2 μs/op 以下 | 20 KB/op 以下 |

### 現状との比較

**現在のベンチマーク結果**（参考値）:

```
BenchmarkExtractCommandResults_Current-4          	 1000000	       453.5 ns/op	    1648 B/op	       9 allocs/op
BenchmarkExtractCommandResults_WithSlogValue-4    	 1000000	       248.9 ns/op	     848 B/op	       4 allocs/op
```

**期待される改善**:

- 処理時間: 20%以上の改善（型アサーション削減）
- メモリ割り当て: 50%以上の削減（中間データ構造の排除）
- アロケーション回数: 67%削減（9回 → 3回以下）

## テスト仕様

### 単体テスト

#### CommandResults.LogValue() のテスト

**ファイル**: `internal/common/logschema_test.go`

```go
func TestCommandResults_LogValue(t *testing.T) {
    tests := []struct {
        name     string
        results  CommandResults
        validate func(t *testing.T, value slog.Value)
    }{
        {
            name:    "empty slice",
            results: CommandResults{},
            validate: func(t *testing.T, value slog.Value) {
                assert.Equal(t, slog.KindGroup, value.Kind())
                attrs := value.Group()
                // total_count のみ
                assert.Len(t, attrs, 1)
                assert.Equal(t, "total_count", attrs[0].Key)
                assert.Equal(t, int64(0), attrs[0].Value.Int64())
            },
        },
        {
            name: "single command",
            results: CommandResults{
                {CommandResultFields: CommandResultFields{
                    Name:     "test1",
                    ExitCode: 0,
                    Output:   "ok",
                    Stderr:   "",
                }},
            },
            validate: func(t *testing.T, value slog.Value) {
                assert.Equal(t, slog.KindGroup, value.Kind())
                attrs := value.Group()
                // total_count, truncated, cmd_0
                assert.Len(t, attrs, 3)

                // total_count
                assert.Equal(t, "total_count", attrs[0].Key)
                assert.Equal(t, int64(1), attrs[0].Value.Int64())

                // truncated
                assert.Equal(t, "truncated", attrs[1].Key)
                assert.False(t, attrs[1].Value.Bool())

                // cmd_0
                assert.Equal(t, "cmd_0", attrs[2].Key)
                assert.Equal(t, slog.KindGroup, attrs[2].Value.Kind())
            },
        },
        {
            name: "multiple commands",
            results: CommandResults{
                {CommandResultFields: CommandResultFields{Name: "test1", ExitCode: 0}},
                {CommandResultFields: CommandResultFields{Name: "test2", ExitCode: 1}},
                {CommandResultFields: CommandResultFields{Name: "test3", ExitCode: 0}},
            },
            validate: func(t *testing.T, value slog.Value) {
                attrs := value.Group()
                // total_count, truncated, cmd_0, cmd_1, cmd_2
                assert.Len(t, attrs, 5)
                assert.Equal(t, int64(3), attrs[0].Value.Int64())
            },
        },
        {
            name:    "truncation at 100 commands",
            results: createLargeCommandResults(150),
            validate: func(t *testing.T, value slog.Value) {
                attrs := value.Group()
                // total_count, truncated, cmd_0..cmd_99 = 102
                assert.Len(t, attrs, 102)

                // total_count = 150
                assert.Equal(t, int64(150), attrs[0].Value.Int64())

                // truncated = true
                assert.True(t, attrs[1].Value.Bool())
            },
        },
        {
            name:    "exactly 100 commands - no truncation",
            results: createLargeCommandResults(100),
            validate: func(t *testing.T, value slog.Value) {
                attrs := value.Group()
                // total_count, truncated, cmd_0..cmd_99 = 102
                assert.Len(t, attrs, 102)

                // total_count = 100
                assert.Equal(t, int64(100), attrs[0].Value.Int64())

                // truncated = false
                assert.False(t, attrs[1].Value.Bool())
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            value := tt.results.LogValue()
            tt.validate(t, value)
        })
    }
}
```

#### extractCommandResultsFromGroup() のテスト

**ファイル**: `internal/logging/slack_handler_test.go`

```go
func TestExtractCommandResultsFromGroup(t *testing.T) {
    tests := []struct {
        name     string
        input    slog.Value
        expected []commandResultInfo
    }{
        {
            name:  "valid group structure",
            input: createValidGroupValue(),
            expected: []commandResultInfo{
                {Name: "test1", ExitCode: 0, Output: "ok", Stderr: ""},
                {Name: "test2", ExitCode: 1, Output: "", Stderr: "error"},
            },
        },
        {
            name:     "empty group",
            input:    slog.GroupValue(),
            expected: nil,
        },
        {
            name:     "non-group value",
            input:    slog.StringValue("invalid"),
            expected: nil,
        },
        {
            name:     "group with metadata only",
            input:    slog.GroupValue(slog.Int("total_count", 0)),
            expected: []commandResultInfo{}, // empty slice
        },
        {
            name:  "group with non-group attributes - skip them",
            input: createGroupWithNonGroupAttrs(),
            expected: []commandResultInfo{
                {Name: "valid", ExitCode: 0},
            },
        },
        {
            name:  "command with missing name - skip it",
            input: createGroupWithMissingName(),
            expected: []commandResultInfo{
                {Name: "valid", ExitCode: 0},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := extractCommandResultsFromGroup(tt.input)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

### 統合テスト

#### RedactingHandler + CommandResults

**ファイル**: `internal/redaction/redactor_test.go`

```go
func TestRedactingHandler_CommandResults(t *testing.T) {
    tests := []struct {
        name     string
        results  common.CommandResults
        validate func(t *testing.T, redacted slog.Value)
    }{
        {
            name: "redact password in output",
            results: common.CommandResults{
                {CommandResultFields: common.CommandResultFields{
                    Name:     "setup",
                    ExitCode: 0,
                    Output:   "Database password=secret123 configured",
                    Stderr:   "",
                }},
            },
            validate: func(t *testing.T, redacted slog.Value) {
                // Group 構造が維持されていることを確認
                assert.Equal(t, slog.KindGroup, redacted.Kind())

                // cmd_0 の output が redaction されていることを確認
                attrs := redacted.Group()
                cmd0 := findAttr(attrs, "cmd_0")
                cmdAttrs := cmd0.Value.Group()
                output := findAttr(cmdAttrs, "output")

                assert.Contains(t, output.Value.String(), "[REDACTED]")
                assert.NotContains(t, output.Value.String(), "secret123")
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // RedactingHandler のセットアップ
            var buf bytes.Buffer
            handler := slog.NewJSONHandler(&buf, nil)
            redactingHandler := redaction.NewRedactingHandler(handler)

            // CommandResults をログ記録
            record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
            record.AddAttrs(slog.Any("commands", tt.results))

            // Handle を実行
            err := redactingHandler.Handle(context.Background(), record)
            require.NoError(t, err)

            // 結果をパース
            var logged map[string]any
            err = json.Unmarshal(buf.Bytes(), &logged)
            require.NoError(t, err)

            // 検証
            tt.validate(t, logged["commands"])
        })
    }
}
```

### E2Eテスト

#### 実際のワークフロー

**ファイル**: `internal/runner/integration_test.go`

```go
func TestGroupExecutor_CommandResults_E2E(t *testing.T) {
    // Slack handler のモックをセットアップ
    var capturedMessages []SlackMessage
    mockSlackHandler := &MockSlackHandler{
        onHandle: func(msg SlackMessage) {
            capturedMessages = append(capturedMessages, msg)
        },
    }

    // Logger のセットアップ（RedactingHandler + SlackHandler）
    handler := setupHandlerChain(mockSlackHandler)
    logger := slog.New(handler)
    slog.SetDefault(logger)

    // GroupExecutor でコマンド実行
    executor := setupGroupExecutor(t)
    err := executor.ExecuteGroup(context.Background(), testGroupSpec, testRuntimeGlobal)
    require.NoError(t, err)

    // Slack メッセージが送信されたことを確認
    assert.Len(t, capturedMessages, 1)
    msg := capturedMessages[0]

    // メッセージ内容の検証
    assert.Contains(t, msg.Text, "Command group execution summary")
    assert.Len(t, msg.Attachments, 1)

    // コマンド結果のフィールドが含まれることを確認
    fields := msg.Attachments[0].Fields
    assert.Contains(t, fieldNames(fields), "Commands")
}
```

### テストカバレッジ目標

| コンポーネント | 目標カバレッジ | 優先度 |
|--------------|--------------|--------|
| **CommandResults.LogValue()** | 100% | 高 |
| **extractCommandResultsFromGroup()** | 100% | 高 |
| **RedactingHandler 統合** | 90% | 高 |
| **SlackHandler 統合** | 85% | 中 |
| **E2Eワークフロー** | 80% | 中 |

## 参照

- [要件定義書](./01_requirements.md)
- [アーキテクチャ設計書](./02_architecture.md)
- [実装計画書](./04_implementation_plan.md)
