# Redactionにおけるスライス型変換の動作

## 概要

`internal/redaction` パッケージの `RedactingHandler.processSlice()` メソッドは、すべての型付きスライス（`[]string`, `[]int`, `[]MyStruct` など）を `[]any` に変換します。本ドキュメントでは、この動作の理由、影響、および設計の妥当性について説明します。

## 現在の動作

### 型変換の詳細

```go
// 入力: 型付きスライス
stringSlice := []string{"alice", "bob", "charlie"}
attr := slog.Any("users", stringSlice)

// processSlice 処理後
// 出力: []any に変換される
// attr.Value.Any().([]string) → 失敗
// attr.Value.Any().([]any)    → 成功
```

### 実装の要点

[redactor.go:481-508](../../internal/redaction/redactor.go#L481-L508) 参照

1. **すべてのスライスが処理対象**: `processKindAny()` ([redactor.go:407-409](../../internal/redaction/redactor.go#L407-L409)) により、LogValuer要素の有無に関わらず、すべてのスライスが `processSlice()` を経由します

2. **新しい[]anyスライスの作成**: 処理済み要素を格納するため、`[]any` 型の新しいスライスを作成します

3. **要素の処理**:
   - LogValuer要素: `LogValue()` を呼び出して解決し、再帰的にredaction適用
   - 非LogValuer要素: そのまま保持

4. **戻り値**: `slog.AnyValue(processedElements)` として `[]any` で返却

## 影響

### 影響を受けるケース

1. **型アサーションの失敗**:
   ```go
   // 処理前
   value.Any().([]string) // 成功

   // 処理後
   value.Any().([]string) // 失敗
   value.Any().([]any)    // 成功
   ```

2. **型情報の喪失**:
   - 元の型（`[]string`, `[]int` など）の情報が失われる
   - スライス要素の具体的な型は `[]any` に統一される

### 影響を受けないケース

1. **ログ出力**: JSONハンドラやテキストハンドラは、スライス要素の型に関係なくシリアライズを行うため、出力結果に影響なし

2. **意味的な内容**: スライスの実際の値はすべて保持される

3. **非スライス値**: 他の型（string, int, boolなど）は元の型を保持

## 代替案の検討

### オプション1: Reflectionを使用した型保持

**概要**: `reflect.MakeSlice()` を使用して元の型のスライスを作成

**メリット**:
- 元のスライス型を保持
- 型アサーションが機能する

**デメリット**:
- 実装が複雑
- Reflectionのパフォーマンスオーバーヘッド
- 型不一致のエッジケース処理が必要
- メンテナンスコストが高い

### オプション2: LogValuer要素がある場合のみ処理

**概要**: LogValuer要素がない場合は元のスライスをそのまま返す

**メリット**:
- 一部のケースで型を保持
- パフォーマンス向上

**デメリット**:
- 動作が一貫しない（LogValuerの有無で挙動が変わる）
- テストが複雑化
- 予測が難しい

### オプション3: 現在の実装を維持（推奨）

**理由**:
1. **ログシステムとしての特性**: 本システムはログ出力が目的であり、型情報の保持は重要ではない
2. **シンプルさ**: 実装がシンプルで理解しやすい
3. **一貫性**: すべてのスライスが同じ方法で処理される
4. **パフォーマンス**: Reflectionを使用しないため、オーバーヘッドが最小
5. **実用性**: 実際のユースケースで型アサーションは稀

## 設計の妥当性

### なぜこの設計が適切か

1. **用途**: これはログシステムであり、型保持よりも意味的な内容の保持が重要

2. **ハンドラの実装**: 標準的なslogハンドラ（JSON、テキスト）は型情報を必要としない

3. **セキュリティ**: redactionの主目的は機密情報の保護であり、型保持は二次的

4. **保守性**: シンプルな実装は長期的なメンテナンスコストを低減

### 推奨される使用方法

```go
// ✓ 良い例: ログ出力での使用
logger.Info("Users list", "users", slog.AnyValue(stringSlice))

// ✗ 悪い例: 型アサーションへの依存
sliceValue := attr.Value.Any().([]string) // redaction後は失敗する

// ✓ 良い例: 汎用的な処理
sliceValue := attr.Value.Any().([]any)
for _, elem := range sliceValue {
    // 各要素を処理
}
```

## テスト

型変換の動作は [redactor_test.go:1364-1475](../../internal/redaction/redactor_test.go#L1364-L1475) の `TestRedactingHandler_SliceTypeConversion` で検証されています。

テストケース:
1. LogValuer要素なしの型付きスライス → `[]any` に変換
2. LogValuer要素ありのスライス → `[]any` に変換
3. 混合型スライス → `[]any` に変換、意味的内容は保持

## 結論

現在の `[]any` 変換は、ログシステムとしての用途において適切な設計判断です。型情報は失われますが、意味的な内容はすべて保持され、実装はシンプルで保守しやすくなっています。

型保持を実装する技術的な方法は存在しますが、複雑さとオーバーヘッドが、ログシステムにおける利益を上回ります。

## 参照

- 実装: [internal/redaction/redactor.go](../../internal/redaction/redactor.go)
- テスト: [internal/redaction/redactor_test.go](../../internal/redaction/redactor_test.go)
- processSlice関数: [redactor.go:481-608](../../internal/redaction/redactor.go#L481-L608)
