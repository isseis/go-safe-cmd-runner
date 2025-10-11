# Allowlist データ構造最適化 - 要件定義書

## 1. 背景

### 1.1 問題の発見
commit b52dbccbadc83fe42b8a9fb1de38df27ff0f07b5 でのリファクタリング中に、allowlist の管理において非効率なデータ構造変換が発見された：

1. `Filter.globalAllowlist` は内部で `map[string]struct{}` として保持
2. `ResolveAllowlistConfiguration()` で map → slice に変換（[filter.go:179-182](../../../internal/runner/environment/filter.go#L179-L182)）
3. `AllowlistResolution` の初期化時に slice → map に再変換（[filter.go:185-188](../../../internal/runner/environment/filter.go#L185-L188)）

### 1.2 根本原因
`AllowlistResolution` 構造体が公開フィールドとして slice を保持しているため：

```go
type AllowlistResolution struct {
    GroupAllowlist  []string  // 公開フィールド（slice）
    GlobalAllowlist []string  // 公開フィールド（slice）
    EffectiveList   []string  // 公開フィールド（slice）

    // 内部的に map も持っている
    groupAllowlistSet  map[string]struct{}
    globalAllowlistSet map[string]struct{}
}
```

### 1.3 パフォーマンスへの影響
- allowlist の主な用途は「特定の変数が許可されているか」の O(1) 検索
- 不要な変換により、メモリアロケーションと CPU サイクルを浪費
- 大規模な allowlist（数百〜数千の変数）では影響が顕著になる可能性

## 2. 目的

### 2.1 主要目的
allowlist のデータ構造を一貫して map ベースで管理し、不要な変換を排除する

### 2.2 副次的目的
- コードの明確性向上：データ構造の用途が明確になる
- メンテナンス性向上：変換ロジックの削減
- パフォーマンス改善：不要なアロケーションの削減

## 3. 要件

### 3.1 機能要件

#### FR-1: 内部データ構造の統一
- **要件**: allowlist は内部で一貫して `map[string]struct{}` として管理する
- **理由**: O(1) 検索が主な用途であり、map が最適なデータ構造

#### FR-2: 外部インターフェースの維持
- **要件**: 既存の公開 API を可能な限り維持する
- **理由**: 既存コードへの影響を最小化

#### FR-3: slice への変換は必要時のみ
- **要件**: テスト、ログ出力、デバッグ等、本当に必要な場合のみ slice に変換
- **理由**: 不要な変換の排除

### 3.2 非機能要件

#### NFR-1: パフォーマンス
- **要件**: map ⇔ slice の不要な変換を排除
- **測定基準**: 変換回数の削減

#### NFR-2: 後方互換性
- **要件**: 既存のテストが全て通過すること
- **測定基準**: `make test` が成功

#### NFR-3: コードの明確性
- **要件**: データ構造の用途が明確であること
- **測定基準**: コードレビューでの可読性評価

### 3.3 制約条件

#### C-1: TOML 設定ファイルとの互換性
- TOML ファイルでは allowlist は配列として定義される
- 設定読み込み時に slice → map への変換は必要（1回のみ）

#### C-2: テスト容易性
- テストコードで allowlist の内容を検証する必要がある
- 順序に依存しない比較が必要

#### C-3: ログ出力の可読性
- デバッグログでは allowlist の内容を人間が読める形式で出力
- slice への変換が必要な場合がある

## 4. 対象範囲

### 4.1 対象コンポーネント
- `internal/runner/environment/filter.go`
  - `Filter` 構造体
  - `ResolveAllowlistConfiguration()` メソッド
- `internal/runner/runnertypes/config.go`
  - `AllowlistResolution` 構造体
  - `IsAllowed()` メソッド
  - setter メソッド群

### 4.2 対象外
- TOML 設定ファイルの構造（変更なし）
- `Config`, `GlobalConfig`, `CommandGroup` の `EnvAllowlist []string` フィールド（変更なし）
- 既存の公開 API の動作（互換性を維持）

## 5. 成功基準

### 5.1 機能的成功基準
- ✅ 全ての既存テストが通過
- ✅ allowlist の検証ロジックが正しく動作
- ✅ 継承モード（inherit/explicit/reject）が正しく機能

### 5.2 技術的成功基準
- ✅ map → slice → map の不要な変換が排除されている
- ✅ `ResolveAllowlistConfiguration()` で slice への変換が不要
- ✅ コードの可読性が向上している

### 5.3 パフォーマンス基準
- ✅ allowlist 検索は O(1) で動作
- ✅ 不要なメモリアロケーションが削減されている

## 6. リスク分析

### 6.1 技術的リスク

#### R-1: 既存コードへの影響
- **リスク**: `AllowlistResolution` の構造変更により既存コードが動作しなくなる
- **影響度**: 中
- **対策**: 段階的なリファクタリング、包括的なテスト

#### R-2: テストコードの修正が必要
- **リスク**: slice ベースの比較を使用しているテストが失敗する
- **影響度**: 低
- **対策**: map ベースの比較ヘルパー関数の追加

### 6.2 運用リスク

#### R-3: デバッグの困難化
- **リスク**: map の内容が直接見えないためデバッグが難しくなる
- **影響度**: 低
- **対策**: 適切なログ出力、getter メソッドの追加

## 7. 次のステップ

1. 現状分析：既存コードの詳細な調査
2. アーキテクチャ設計：リファクタリング戦略の立案
3. 詳細仕様：データ構造とインターフェースの詳細設計
4. 実装計画：段階的な実装手順の策定
5. 実装とテスト：TDD アプローチでの実装

## 8. 参考情報

### 8.1 関連 commit
- b52dbccbadc83fe42b8a9fb1de38df27ff0f07b5: allowlist 継承ロジックの統一化
- afecbb1: allowlist 継承ロジックの抽出

### 8.2 関連ファイル
- [internal/runner/environment/filter.go](../../../internal/runner/environment/filter.go)
- [internal/runner/runnertypes/config.go](../../../internal/runner/runnertypes/config.go)

### 8.3 関連タスク
- Task 0011: Allowlist refinement
- Task 0031: Global group environment variable management
