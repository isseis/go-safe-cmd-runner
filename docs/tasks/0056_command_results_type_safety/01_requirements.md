# 要件定義書: CommandResults 型安全性改善プロジェクト

## プロジェクト概要

### 目的

コマンド実行結果のログ処理における型安全性を向上させ、`extractCommandResults` 関数の複雑な型アサーション処理を簡素化する。

### 背景

現在の実装では、`[]common.CommandResult` をログに記録する際、以下の問題が発生している:

1. **slog の設計制約**: スライス内の `LogValuer` インターフェースを自動的に解決しない
2. **RedactingHandler による型変換**: `[]common.CommandResult` が `[]any` に変換され、さらに各要素が `CommandResult`、`slog.Value`、`[]slog.Attr` のいずれかになる
3. **複雑な型アサーション**: `extractCommandResults` 関数で複数段階の型チェックが必要
4. **変更に対する脆弱性**: RedactingHandler の実装変更により、新しいケースが追加される可能性

この問題は技術的負債となり、保守性、可読性、拡張性を低下させている。

### スコープ

**対象範囲:**
- `internal/common/logschema.go`: `CommandResults` 型の新規追加
- `internal/logging/slack_handler.go`: `extractCommandResults` 関数の簡略化
- `internal/runner/group_executor.go`: `CommandResults` 型の使用
- 関連するテストコード全般

**対象外:**
- 他のログ属性の処理方法（本プロジェクトでは CommandResults のみを対象とする）
- RedactingHandler 自体の変更（既存の動作を維持）

## 選択した解決策

### LogValuer 再実装アプローチ

**概要:**

`[]common.CommandResult` の代わりに、専用の `CommandResults` 型を導入し、スライス全体で `LogValuer` インターフェースを実装する。これにより、RedactingHandler のスライス型変換問題を根本から回避する。

**主要コンポーネント:**

1. **CommandResults 型**
   - `[]CommandResult` のエイリアス型
   - `LogValuer` インターフェースを実装
   - ユーティリティメソッド（`Len()`, `HasFailures()`, `SuccessCount()`）を提供

2. **ログ出力構造**
   - 各コマンドを `cmd_0`, `cmd_1`, ... というキーを持つ Group として構造化
   - スライス要素ではなく、ネストした Group 構造として表現

3. **extractCommandResults の簡略化**
   - 複雑な型アサーションを排除
   - Group 構造から直接属性を抽出

**技術的メリット:**

- **型安全性**: コンパイル時に型チェック可能
- **パフォーマンス**: reflection や複雑な型アサーションが不要
- **保守性**: シンプルで理解しやすいコード
- **拡張性**: 将来的な機能追加が容易

**リリース前に実施する理由:**

1. **互換性の制約がない**: ログ形式を自由に変更可能
2. **技術的負債の早期解決**: リリース後に対処するコストが大幅に増加
3. **設計の一貫性**: プロジェクト全体の設計品質向上
4. **工数対効果**: 3日の投資で長期的な保守コストを削減

## 先行タスク: RedactingHandler との相性検証

### 検証目的

`CommandResults.LogValue()` が返す `slog.GroupValue` が RedactingHandler で正しく処理されることを確認する。

### 検証項目

1. **Group 構造の redaction**
   - ネストした Group が正しく redaction されるか
   - 機密情報を含む output/stderr が適切に redaction されるか

2. **型変換の回避**
   - `CommandResults` が `[]any` に変換されないことを確認
   - Group 構造が維持されることを確認

3. **パフォーマンス**
   - redaction 処理のオーバーヘッドが許容範囲内か
   - 既存の実装と比較して劣化していないか

### 検証方法

- 単体テスト: `internal/redaction/redactor_test.go` に検証ケースを追加
- E2Eテスト: 実際の GroupExecutor からのログ出力を確認
- パフォーマンステスト: ベンチマークテストで測定

### 成功基準

- [ ] ネストした Group 構造が正しく redaction される
- [ ] 型変換が発生せず、Group 構造が維持される
- [ ] パフォーマンスが既存実装と同等以上
- [ ] すべてのテストケースが通過する

検証結果は、アーキテクチャ設計書に反映する。

## 他の検討案との比較

以下の代替案を検討したが、最終的に LogValuer 再実装を選択した。

### 比較表

| 観点 | LogValuer再実装 | 正規化レイヤー | インターフェース | スライス処理関数 |
|------|----------------|--------------|----------------|----------------|
| **実装工数** | 3日 | 1-2日 | 3-4日 | 1-2日 |
| **型安全性** | ◎ コンパイル時 | △ 実行時 | △ 実行時 | △ 実行時 |
| **パフォーマンス** | ◎ 直接アクセス | △ reflection | △ ループ | △ reflection |
| **保守性** | ◎ シンプル | ○ 関数単位 | ○ SRP準拠 | ○ 関数単位 |
| **拡張性** | △ 特定型に依存 | ○ 汎用的 | ◎◎ 最高 | ◎ 汎用的 |
| **根本解決** | ◎ 根本から解決 | × 対症療法 | × 対症療法 | × 対症療法 |
| **後方互換性** | × 変更必要 | ◎ 完全維持 | ◎ 完全維持 | ◎ 完全維持 |
| **YAGNI準拠** | ○ 必要最小限 | ○ 適度 | × 過剰設計 | ○ 適度 |

### 選択理由

1. **プロジェクトのライフサイクル**
   - 正式リリース前は大規模変更の最適なタイミング
   - リリース後は互換性維持のコストが大幅に増加

2. **長期的な価値**
   - 技術的負債を根本から解決
   - 型安全性とパフォーマンスの両立
   - 保守性の大幅な向上

3. **リスクとコストのバランス**
   - 3日の実装工数は許容範囲内
   - リスクは低〜中程度で管理可能
   - リリース後に対処するコストを考慮すると投資効果が高い

4. **設計品質**
   - プロジェクト全体の設計の一貫性向上
   - 他の部分でも同様のパターンを適用可能

## 非機能要件

### パフォーマンス

- **ログ処理のオーバーヘッド**: 既存実装と同等以上
- **メモリ使用量**: 大幅な増加がないこと（10%以内）
- **処理時間**: グループ実行完了時のログ処理が 10ms 以内

### 信頼性

- **後方互換性**: テスト環境での十分な検証
- **エラーハンドリング**: 適切なエラーメッセージとログ出力
- **ロールバック**: 問題発生時の迅速な切り戻しが可能

### 保守性

- **コードの可読性**: 複雑な型アサーションを排除
- **テスト容易性**: 単体テストで完全にカバー
- **ドキュメント**: 設計意図とトレードオフを文書化

### セキュリティ

- **機密情報の保護**: RedactingHandler との互換性を維持
- **ログの完全性**: 監査ログとしての要件を満たす

## 成功基準

### 必須要件

- [ ] `CommandResults` 型が正しく実装されている
- [ ] `extractCommandResults` が簡略化され、複雑な型アサーションが不要
- [ ] すべての既存テストが通過する
- [ ] RedactingHandler との相性が検証されている
- [ ] パフォーマンスが既存実装と同等以上

### 推奨要件

- [ ] コードレビューで設計が承認される
- [ ] E2Eテストで実際のログ出力が期待通り
- [ ] ドキュメントが更新されている
- [ ] `docs/dev/redaction_slice_type_conversion.md` が更新されている

## 制約事項

### 技術的制約

- Go 1.23.10 を使用
- slog パッケージの標準機能のみを使用（サードパーティライブラリは不可）
- RedactingHandler の既存動作を変更しない

### スケジュール制約

- 実装期間: 3日以内
- レビュー期間: 1日
- 検証期間: 1日
- 合計: 5日以内に完了

### リソース制約

- 主担当者: 1名
- レビュアー: 1名以上

## リスク管理

### 識別されたリスク

| リスク | 影響 | 確率 | 対策 |
|-------|-----|-----|-----|
| RedactingHandler との非互換性 | 高 | 低 | 先行検証タスクで確認 |
| ログ出力形式の意図しない変更 | 中 | 中 | E2Eテストで検証 |
| 既存コードへの影響範囲が大きい | 中 | 低 | コンパイルエラーで検出可能 |
| パフォーマンス劣化 | 低 | 低 | ベンチマークテストで測定 |

### 緩和策

1. **段階的な実装**
   - Phase 1: 型定義とテスト
   - Phase 2: extractCommandResults の更新
   - Phase 3: 使用箇所の更新
   - Phase 4: 後方互換コードの削除

2. **十分なテスト**
   - 単体テスト: 各コンポーネントを独立してテスト
   - 統合テスト: RedactingHandler との連携をテスト
   - E2Eテスト: 実際のワークフローで検証

3. **レビュープロセス**
   - 設計レビュー: アーキテクチャ設計書の承認
   - コードレビュー: 実装の品質確認
   - テストレビュー: テストカバレッジの確認

## 関連ドキュメント

- [アーキテクチャ設計書](./02_architecture.md)
- [実装計画](./03_implementation_plan.md)（後で作成予定）
- [RedactingHandler スライス型変換の動作](../../dev/redaction_slice_type_conversion.md)
- [CommandResult ログスキーマ](../../../internal/common/logschema.go)

## Appendix: 他の検討案の詳細

### A1. 正規化レイヤーの導入

**概要:**

RedactingHandler が出力する様々な形式を、共通の内部表現に正規化してから処理する。

**アプローチ:**

1. スライスを統一形式 (`[]any`) に変換
2. 各要素を `slog.Value` に正規化
3. 正規化された値から `commandResultInfo` を抽出

**実装イメージ:**

```go
func normalizeCommandResults(value slog.Value) []commandResultInfo {
    anySlice := toAnySlice(value.Any())
    normalizedValues := normalizeSliceElements(anySlice)
    return extractFromNormalizedValues(normalizedValues)
}
```

**Pros:**
- 各ステップが明確で理解しやすい
- 新しい型が追加されても特定の関数だけ修正すればよい
- テストが容易（各関数を独立してテスト可能）

**Cons:**
- 根本的な解決ではない（対症療法）
- パフォーマンスオーバーヘッド（スライス要素ごとに正規化処理）
- 型安全性の欠如（実行時エラーのリスク）

### A2. 型マッピングテーブルの使用

**概要:**

各型に対する処理をマップで管理し、テーブル駆動で型変換を実施する。

**アプローチ:**

```go
type elementProcessor func(elem any) (slog.Value, bool)

var elementProcessors = []elementProcessor{
    func(elem any) (slog.Value, bool) {
        if cmd, ok := elem.(common.CommandResult); ok {
            return cmd.LogValue(), true
        }
        return slog.Value{}, false
    },
    // 他の型の processor...
}
```

**Pros:**
- 新しい型の追加が容易
- 型ごとの処理が独立
- テーブル駆動で拡張性が高い

**Cons:**
- やや過剰設計の可能性
- ループのオーバーヘッド
- 実行時の型チェックに依存

### A3. インターフェースベースのアプローチ

**概要:**

共通インターフェースを定義し、型変換を抽象化する。Chain of Responsibility パターンを使用。

**アプローチ:**

```go
type CommandResultExtractor interface {
    Extract(value slog.Value) ([]commandResultInfo, bool)
}

type directCommandResultExtractor struct{}
type anySliceExtractor struct{}

func extractCommandResults(value slog.Value) []commandResultInfo {
    extractors := []CommandResultExtractor{
        &directCommandResultExtractor{},
        &anySliceExtractor{},
    }

    for _, extractor := range extractors {
        if results, ok := extractor.Extract(value); ok {
            return results
        }
    }
    return nil
}
```

**Pros:**
- 責務の明確な分離（SRP準拠）
- 拡張性が非常に高い（Open/Closed Principle）
- テストの独立性
- 依存性注入との相性が良い

**Cons:**
- 過剰設計（Over-engineering）のリスク
- コード量の増加（150-200行）
- 認知的負荷の増加（新しい開発者の学習コスト）
- YAGNI 違反の可能性

### A4. 統一的なスライス処理関数の提供

**概要:**

RedactingHandler が処理したスライスを扱うための汎用的なヘルパーパッケージ `internal/slogutil` を作成する。

**アプローチ:**

```go
// internal/slogutil/slice.go
func ResolveSlice(value slog.Value) []slog.Value {
    // reflection を使って任意の型のスライスを処理
    // 各要素を slog.Value に正規化
}

func ResolveElement(elem any) slog.Value {
    // LogValuer の解決
    // slog.Value への変換
    // []slog.Attr の処理
}
```

**Pros:**
- 再利用可能なユーティリティ
- SlackHandler のコードがシンプルになる
- 他の場所でも同様の問題が発生した場合に対応可能

**Cons:**
- 新しいパッケージが必要
- reflection のオーバーヘッド
- 根本的な解決ではない

### 選択しなかった理由のまとめ

上記の代替案（A1-A4）は、いずれも**対症療法的なアプローチ**であり、以下の根本的な問題を解決できない:

1. **型安全性の欠如**: コンパイル時に型チェックができない
2. **パフォーマンス**: reflection や複数段階の型アサーションが必要
3. **複雑性**: 問題を別の場所に移動させただけ
4. **将来の変更への脆弱性**: RedactingHandler の変更に依然として影響を受ける

**正式リリース前**という状況を考慮すると、後方互換性の制約がないため、根本的な解決策（LogValuer 再実装）を選択することが最適である。

リリース後であれば、互換性維持のために A4（統一的なスライス処理関数）を選択していた可能性が高い。
