# Redactionにおけるスライス型変換の動作

## 概要

`internal/redaction` パッケージの `RedactingHandler.processSlice()` メソッドは、すべての型付きスライス（`[]string`, `[]int`, `[]MyStruct` など）を `[]any` に変換します。本ドキュメントでは、この動作の理由、影響、およびスライス型変換を回避する推奨パターンについて説明します。

## スライス型変換の動作

### 基本的な型変換

RedactingHandler は、すべてのスライスを `[]any` に変換します：

```go
// 入力: 型付きスライス
stringSlice := []string{"alice", "bob", "charlie"}
attr := slog.Any("users", stringSlice)

// processSlice 処理後
// 出力: []any に変換される
attr.Value.Any().([]string) // 失敗
attr.Value.Any().([]any)    // 成功
```

### 処理の流れ

1. **すべてのスライスが処理対象**: LogValuer要素の有無に関わらず、すべてのスライスが `processSlice()` を経由
2. **新しい[]anyスライスの作成**: 処理済み要素を格納するため、`[]any` 型の新しいスライスを作成
3. **要素の処理**:
   - LogValuer要素: `LogValue()` を呼び出して解決し、再帰的にredaction適用
   - 非LogValuer要素: そのまま保持
4. **戻り値**: `slog.AnyValue(processedElements)` として `[]any` で返却

### 影響範囲

**影響を受ける**:
- 型アサーション: `value.Any().([]string)` は失敗し、`value.Any().([]any)` が必要
- 型情報: 元の型（`[]string`, `[]int` など）の情報が失われる

**影響を受けない**:
- ログ出力: JSONハンドラやテキストハンドラの出力結果
- 意味的な内容: スライスの実際の値はすべて保持される
- 非スライス値: string, int, bool などは元の型を保持

## 設計判断の根拠

この設計は、以下の理由により適切と判断しています：

1. **用途**: ログシステムでは、型保持よりも意味的な内容の保持が重要
2. **ハンドラの実装**: 標準的なslogハンドラ（JSON、テキスト）は型情報を必要としない
3. **シンプルさ**: 実装がシンプルで理解しやすい
4. **一貫性**: すべてのスライスが同じ方法で処理される
5. **パフォーマンス**: Reflectionを使用しないため、オーバーヘッドが最小

型保持の実装（Reflectionを使用）も技術的には可能ですが、複雑さとオーバーヘッドが利益を上回ります。

## 型安全な処理が必要な場合の推奨パターン

スライス型変換を回避し、型安全な処理を実現するには、**LogValuerを実装したラッパー型でGroup構造を使用する**パターンを推奨します。

### CommandResults の実装例

`[]CommandResult` スライスの処理では、以下のアプローチを採用しています：

```go
// 型定義
type CommandResults []CommandResult

// LogValuer実装: Group構造を使用してスライス全体を構造化
func (c CommandResults) LogValue() slog.Value {
    attrs := make([]slog.Attr, len(c))
    for i, result := range c {
        attrs[i] = slog.Any(strconv.Itoa(i), result)
    }
    return slog.GroupValue(attrs...)
}
```

### パターンの利点

1. **型安全性**: RedactingHandler を経由しても Group 構造が維持される
2. **型アサーション不要**: SlackHandler 側は Group 値として直接処理可能
3. **パフォーマンス**: 複雑な型アサーションやリフレクション不要
4. **一貫性**: redaction 前後で構造が変わらない

### 使用例

```go
// ログ記録側（Runner）
results := runnertypes.CommandResults{
    {Command: "echo test", ExitCode: 0},
    {Command: "false", ExitCode: 1},
}
logger.Info("Execution summary", "results", results)

// ログ処理側（SlackHandler）
// Group として直接処理可能、型アサーション不要
func (h *SlackHandler) Handle(ctx context.Context, record slog.Record) error {
    record.Attrs(func(attr slog.Attr) bool {
        if attr.Key == "results" && attr.Value.Kind() == slog.KindGroup {
            // Group 構造として直接処理
            for _, a := range attr.Value.Group() {
                // 各 CommandResult を処理
            }
        }
        return true
    })
}
```

## テスト

型変換の動作検証:
- [redactor_test.go の TestRedactingHandler_SliceTypeConversion](../../internal/redaction/redactor_test.go)

CommandResults パターンの統合テスト:
- RedactingHandler 経由での Group 構造維持
- end-to-end での機密情報 redaction

詳細は Task 0056 の成果物を参照:
- [アーキテクチャ設計書](../tasks/0056_command_results_type_safety/02_architecture.md)
- [実装計画](../tasks/0056_command_results_type_safety/04_implementation_plan.md)

## 参照

- 実装: [internal/redaction/redactor.go](../../internal/redaction/redactor.go)
  - `processSlice` 関数: スライス処理の実装
  - `processKindAny` 関数: slog.KindAny値の処理
  - `processLogValuer` 関数: LogValuer要素の処理
- テスト: [internal/redaction/redactor_test.go](../../internal/redaction/redactor_test.go)
  - `TestRedactingHandler_SliceTypeConversion`: 型変換動作の検証
