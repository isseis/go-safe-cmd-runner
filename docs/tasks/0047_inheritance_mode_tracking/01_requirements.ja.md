# 要件定義書: 環境変数継承モードの明示的追跡

## 1. プロジェクト概要

### 1.1 目的
TOML設定ファイル解析時に環境変数allowlistの継承モード（`InheritanceMode`）を判定し、`RuntimeGroup`に記録することで、コードの可読性と保守性を向上させる。

### 1.2 背景
現在、`InheritanceMode`型は定義されているが、プロダクションコードでは使用されていない。継承モードの判定は各所で`len(group.EnvAllowed)`などを用いて重複して行われており、特に`PrintFromEnvInheritance`関数は継承モードを推測しながら表示処理を行うため、コードが読みにくくなっている。

### 1.3 スコープ
- **対象範囲**: 環境変数allowlistの継承モード追跡機能の実装
- **対象外**: その他の設定項目（`env_import`, `vars`など）の継承モード追跡は本プロジェクトの範囲外

## 2. 現状分析

### 2.1 InheritanceMode型の定義
[internal/runner/runnertypes/config.go:13-27](../../../internal/runner/runnertypes/config.go#L13-L27)にて定義されている:

```go
type InheritanceMode int

const (
    // InheritanceModeInherit: グループがグローバルallowlistを継承
    // env_allowlistフィールドが未定義（nilスライス）の場合
    InheritanceModeInherit InheritanceMode = iota

    // InheritanceModeExplicit: グループが独自のallowlistを使用
    // env_allowlistフィールドに値がある場合: ["VAR1", "VAR2"]
    InheritanceModeExplicit

    // InheritanceModeReject: グループがすべての環境変数を拒否
    // env_allowlistフィールドが明示的に空の場合: []
    InheritanceModeReject
)
```

`String()`メソッドも実装済み（[config.go:162-173](../../../internal/runner/runnertypes/config.go#L162-L173)）:
- `InheritanceModeInherit` → "inherit"
- `InheritanceModeExplicit` → "explicit"
- `InheritanceModeReject` → "reject"

### 2.2 現状の問題点

#### 2.2.1 型が使用されていない
`InheritanceMode`型は定義されているが、プロダクションコードでは使用されていない。

**該当箇所:**
- [internal/runner/config/validation.go:328-359](../../../internal/runner/config/validation.go#L328-L359) - `analyzeInheritanceMode`関数は`InheritanceMode`型を使用せず、`group.EnvAllowed`を直接チェック

#### 2.2.2 継承モード判定ロジックの重複
継承モードの判定が各所で重複している:

```go
// 例1: validation.go
if group.EnvAllowed == nil {
    // Inherit mode
} else if len(group.EnvAllowed) == 0 {
    // Reject mode
} else {
    // Explicit mode
}

// 例2: inheritance.go
if len(group.EnvAllowed) > 0 {
    // Override
} else {
    // Inherit
}
```

**該当箇所:**
- [internal/runner/config/validation.go:329-358](../../../internal/runner/config/validation.go#L329-L358)
- internal/runner/debug/inheritance.go:80-96

#### 2.2.3 PrintFromEnvInheritance関数の可読性
internal/runner/debug/inheritance.go:14-98の`PrintFromEnvInheritance`関数は:
- `group.EnvAllowed`の値を解析して継承モードを推測
- 推測結果に基づいて異なる表示メッセージを出力
- 条件分岐が多く、コードの意図が不明瞭

特に78-96行目のallowlist継承表示部分:
```go
if len(group.EnvAllowed) > 0 {
    // Override case
    fmt.Fprintf(w, "  Group overrides Global allowlist\n")
    // ... 詳細表示
} else {
    // Inherit case
    fmt.Fprintf(w, "  Inheriting Global allowlist\n")
    // ... 詳細表示
}
```

この実装では:
- 拒否モード（`env_allowlist = []`）が明示的に扱われていない
- 継承モードの判定ロジックが表示ロジックと混在
- テストしにくく、保守が困難

### 2.3 dry-runモードでの表示
現在のdry-runモードでは、環境変数allowlistの継承モードは直接表示されていない:
- [internal/runner/resource/formatter.go](../../../internal/runner/resource/formatter.go) - 継承モードの表示なし
- [internal/runner/resource/dryrun_manager.go](../../../internal/runner/resource/dryrun_manager.go) - 継承モード情報を追跡していない

## 3. 提案する改善策

### 3.1 基本方針
TOML設定ファイル解析時に環境変数allowlistの継承モードを判定し、`RuntimeGroup`構造体に記録する。

### 3.2 実装アプローチ

#### 3.2.1 RuntimeGroup構造体の拡張
`RuntimeGroup`に継承モード情報を追加:

```go
type RuntimeGroup struct {
    // ... 既存フィールド

    // EnvAllowlistInheritanceMode は環境変数allowlistの継承モード
    EnvAllowlistInheritanceMode InheritanceMode
}
```

#### 3.2.2 継承モード判定ロジックの集約

##### 3.2.2.1 実行順序の課題
現在の実装では、設定検証（`config/validation.go`）が設定展開（`config/expansion.go`）より先に実行される。この実行順序により、検証フェーズで`RuntimeGroup`の新フィールドを使用することができない。

##### 3.2.2.2 解決策：共有ヘルパー関数
継承モード判定ロジックを共有パッケージ（例: `runnertypes`）に独立したヘルパー関数として実装:

```go
// internal/runner/runnertypes/inheritance.go (新規ファイル)
package runnertypes

// DetermineInheritanceMode は GroupSpec から継承モードを判定する
func DetermineInheritanceMode(envAllowed []string) InheritanceMode {
    if envAllowed == nil {
        return InheritanceModeInherit
    }
    if len(envAllowed) == 0 {
        return InheritanceModeReject
    }
    return InheritanceModeExplicit
}
```

この関数を以下の2箇所で使用:

1. **`config/validation.go`**: `analyzeInheritanceMode`関数で検証に使用
2. **`config/expansion.go`**: `ExpandGroup`関数で`RuntimeGroup`フィールドに設定

判定ロジック:
1. `envAllowed == nil` → `InheritanceModeInherit`
2. `len(envAllowed) == 0` → `InheritanceModeReject`
3. `len(envAllowed) > 0` → `InheritanceModeExplicit`

##### 3.2.2.3 実装メリット
- ロジック集約: 継承モード判定が単一箇所に集約
- 既存フロー維持: 検証→展開の実行順序を変更不要
- テスタビリティ: 判定ロジックを独立してテスト可能
- 再利用性: 検証と展開の両方で同じロジックを使用

#### 3.2.3 PrintFromEnvInheritance関数の簡素化
`RuntimeGroup.EnvAllowlistInheritanceMode`を参照することで、継承モード判定ロジックを削除し、表示ロジックを簡素化:

```go
// Before: 継承モードを推測
if len(group.EnvAllowed) > 0 {
    // Override case
} else {
    // Inherit case
}

// After: 明示的な継承モード使用
switch runtimeGroup.EnvAllowlistInheritanceMode {
case InheritanceModeInherit:
    // ...
case InheritanceModeExplicit:
    // ...
case InheritanceModeReject:
    // ...
}
```

### 3.3 期待される効果

#### 3.3.1 可読性の向上
- 継承モードが明示的になり、コードの意図が明確
- `PrintFromEnvInheritance`などのデバッグ関数が単純化

#### 3.3.2 保守性の向上
- 継承モード判定ロジックが1箇所に集約
- 判定ロジックの変更時に影響範囲が限定的

#### 3.3.3 一貫性の向上
- `InheritanceMode`型が実際に使用され、型の存在意義が明確
- 型システムによる安全性向上

#### 3.3.4 テスタビリティの向上
- 継承モード判定ロジックを独立してテスト可能
- 表示ロジックと判定ロジックの分離

### 3.4 YAGNI原則との整合性
この変更はYAGNI（You Aren't Gonna Need It）原則に反しない:

1. **既存の型の活用**: `InheritanceMode`型は既に定義済み
2. **重複コードの削減**: 継承モード判定は既に複数箇所で実行されている
3. **実際の使用例**: dry-runモードで既に表示が必要
4. **保守性の改善**: 将来の変更を容易にするための適切なリファクタリング

## 4. 影響範囲

### 4.1 変更が必要なファイル

#### 4.1.1 型定義とヘルパー関数
- `internal/runner/runnertypes/runtime.go` - `RuntimeGroup`構造体にフィールド追加
- `internal/runner/runnertypes/inheritance.go` - 継承モード判定ヘルパー関数（新規ファイル）

#### 4.1.2 継承モード判定・設定
- `internal/runner/config/validation.go` - `analyzeInheritanceMode`でヘルパー関数を使用
- `internal/runner/config/expansion.go` - `ExpandGroup`でヘルパー関数を使用し`RuntimeGroup`に設定

#### 4.1.3 継承モード使用
- `internal/runner/debug/inheritance.go` - `PrintFromEnvInheritance`関数の簡素化

#### 4.1.4 テストコード
- `internal/runner/runnertypes/runtime_test.go` - 新フィールドのテスト
- `internal/runner/runnertypes/inheritance_test.go` - ヘルパー関数のテスト（新規ファイル）
- `internal/runner/config/expansion_test.go` - 継承モード設定のテスト
- `internal/runner/config/validator_test.go` - 継承モード検証のテスト
- `internal/runner/debug/inheritance_test.go` - 簡素化された表示ロジックのテスト（存在する場合）

### 4.2 影響を受けるが変更不要なファイル
- `internal/runner/resource/formatter.go` - 将来的に継承モード表示を追加可能（本プロジェクトでは対象外）
- `internal/runner/resource/dryrun_manager.go` - 同上

## 5. 制約条件

### 5.1 技術的制約
- Go 1.23.10を使用
- 既存のTOML設定ファイル形式との互換性を維持
- 既存のAPI（公開インターフェース）を破壊しない

### 5.2 品質要件
- すべてのユニットテストが成功すること
- `make lint`がエラーなく完了すること
- `make fmt`によるフォーマットが適用されていること
- 既存の動作を変更しないこと（リファクタリングのみ）

## 6. 非機能要件

### 6.1 性能
- TOML解析時の継承モード判定は軽量な処理（条件分岐のみ）
- 実行時のオーバーヘッドはゼロ（既存の判定ロジックを置き換えるだけ）

### 6.2 保守性
- 継承モード判定ロジックが1箇所に集約され、保守が容易
- 型安全性が向上し、バグの早期発見が可能

### 6.3 可読性
- コードの意図が明確になり、新規開発者の理解が容易
- デバッグ時の情報が明示的

## 7. 実装計画

### 7.1 実装順序
1. **フェーズ1**: ヘルパー関数の実装
   - `internal/runner/runnertypes/inheritance.go`の作成
   - `DetermineInheritanceMode`関数の実装
   - ユニットテスト作成

2. **フェーズ2**: `RuntimeGroup`構造体の拡張
   - `EnvAllowlistInheritanceMode`フィールド追加
   - アクセサメソッド追加（必要に応じて）

3. **フェーズ3**: 検証・展開での使用
   - `config/validation.go`でヘルパー関数を使用
   - `config/expansion.go`でヘルパー関数を使用し`RuntimeGroup`に設定
   - ユニットテスト作成・更新

4. **フェーズ4**: 既存コードの書き換え
   - `debug/inheritance.go`の`PrintFromEnvInheritance`の簡素化
   - 関連テストの更新

5. **フェーズ5**: テストと検証
   - 全テストの実行と修正
   - Lint/フォーマットチェック
   - 動作確認

### 7.2 リスク管理
- **リスク**: 既存の動作を誤って変更する可能性
  - **対策**: 各フェーズでテストを実行し、動作を確認

- **リスク**: 判定ロジックの実装ミス
  - **対策**: 包括的なユニットテストでカバー

## 8. 今後の拡張可能性

本プロジェクトの範囲外だが、将来的に以下の拡張が可能:

### 8.1 他の設定項目への適用
- `EnvImport`の継承モード追跡
- `Vars`の継承モード追跡
- `VerifyFiles`の継承モード追跡

### 8.2 dry-runモードでの表示
本タスクで実装する`EnvAllowlistInheritanceMode`フィールドを活用し、dry-runモードの出力を改善可能:

#### 8.2.1 表示要件
各グループについて以下の情報を明示的に表示:

1. **継承モードの表示**
   - `inherit`: "Inheriting Global allowlist"
   - `explicit`: "Using group-specific allowlist"
   - `reject`: "Rejecting all environment variables"

2. **詳細情報の表示**
   - **inheritモード**:
     ```
     Group: example-group
       Environment Variable Allowlist: Inheriting Global allowlist
       Global allowlist: [VAR1, VAR2, VAR3]
     ```

   - **explicitモード**:
     ```
     Group: example-group
       Environment Variable Allowlist: Using group-specific allowlist
       Group allowlist: [VAR4, VAR5]
     ```

   - **rejectモード**:
     ```
     Group: example-group
       Environment Variable Allowlist: Rejecting all environment variables
       (No environment variables will be inherited)
     ```

#### 8.2.2 実装候補箇所
- `internal/runner/resource/formatter.go` - フォーマット関数に継承モード表示を追加
- `internal/runner/resource/dryrun_manager.go` - dry-run実行時の情報収集

#### 8.2.3 期待される効果
- ユーザーが各グループの環境変数継承動作を理解しやすくなる
- 設定ファイルのデバッグが容易になる
- 明示的な空allowlist（`env_allowlist = []`）とallowlist未定義の違いが明確になる

### 8.3 バリデーション強化
- 継承モードに基づく警告・エラーメッセージの改善
- 設定ファイルの最適化提案

## 9. 参考情報

### 9.1 関連ドキュメント
- [docs/dev/config-inheritance-behavior.ja.md](../../dev/config-inheritance-behavior.ja.md) - 継承動作の詳細説明
- [docs/dev/security-architecture.ja.md](../../dev/security-architecture.ja.md) - セキュリティアーキテクチャ

### 9.2 関連タスク
- Task 0011: Allowlist refinement - `InheritanceMode`型の初期定義
- Task 0032: Allowlist map optimization - 継承動作の最適化検討
- Task 0033: Vars/env separation - 変数システムの分離

## 10. 承認

本要件定義書に基づき、詳細設計・実装を進める。

---

**作成日**: 2025-10-29
**対象バージョン**: v1.x
**プロジェクトID**: 0047_inheritance_mode_tracking
