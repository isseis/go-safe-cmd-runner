# vars テーブル形式への変更 - 要件定義書

## 1. 概要

### 1.1 背景

現在の TOML 設定では、ローカル変数 `vars` は文字列配列として定義されている。

```toml
vars = [
    "base_dir=/opt/myapp",
    "env_type=production",
    "config_path=%{base_dir}/%{env_type}/config.yml"
]
```

この形式には以下の問題がある：

1. **型の制限**: 文字列のみをサポートし、文字列配列を扱えない
2. **パース処理の複雑さ**: `"key=value"` の分割ロジックが必要で、エラーハンドリングが複雑
3. **TOMLらしくない記法**: TOMLの標準的な構造を活用していない
4. **拡張性の低さ**: 将来的に新しい型を追加する際の柔軟性が低い

現在の実装では、配列形式の利点として**定義順序が保証**されており、`ProcessVars` 関数（[internal/runner/config/expansion.go:231-285](internal/runner/config/expansion.go:231-285)）が順次展開を行っている。ただし、既存の `ExpandString` 関数は再帰的に依存変数を解決するため、実際には処理順序に依存しない設計になっている。

コマンドテンプレート機能（Task 0062）の導入に伴い、テンプレートパラメータとして文字列配列を渡す必要性が生じた。この要求に対応するため、`vars` の定義形式をテーブルベース（`[vars]` セクション）に変更する。テーブル形式では反復順序が不定だが、既存の `ExpandString` の再帰展開機能により、依存関係は自動的に解決される。

### 1.1.1 変更対象の範囲

本変更は以下のレベルで定義される `vars` すべてに適用される：
- **グローバルレベル**: `[global]` セクション（現在は `global.vars = [...]`、変更後は `[global.vars]`）
- **グループレベル**: `[[groups]]` 内（現在は `vars = [...]`、変更後は `[groups.vars]`）
- **コマンドレベル**: `[[groups.commands]]` 内（現在は `vars = [...]`、変更後は `[groups.commands.vars]`）

これらすべてのレベルで一貫したテーブル形式を採用する。

### 1.2 目的

`vars` の定義形式を配列ベースからテーブルベースに変更し、以下を実現する：

1. 文字列と文字列配列の両方をサポート
2. TOMLネイティブの型システムを活用
3. パース処理の簡略化（`"key=value"` 分割ロジックが不要）
4. 将来の拡張性の確保
5. 既存の `ExpandString` 再帰展開機能を活用した依存関係解決

### 1.3 スコープ

#### 対象範囲 (In Scope)

- `vars` の定義形式を `[vars]` セクション形式に変更（global、group、command の全レベル）
- 文字列型と文字列配列型のサポート
- 変数名の重複検出（TOMLパーサーが自動検出）
- 既存の `ExpandString` を活用した依存関係解決（再帰展開）
- 既存の変数展開機能（`%{variable}`）との統合
- 既存のサンプル設定ファイル（`sample/*.toml`）およびテストファイルの更新

#### 対象外 (Out of Scope)

- 後方互換性の維持（配列ベース形式のサポート）
- 文字列・文字列配列以外の型（数値、真偽値など）
- 変数定義の条件分岐
- ネストした変数定義
- 移行ツールの提供（必要に応じて別タスクとして検討）

## 2. 機能要件

### 2.1 テーブル形式での変数定義

#### F-001: `[vars]` セクションでの変数定義

**概要**: TOML の `[vars]` セクションで変数を定義する。

**フォーマット**:
```toml
[vars]
variable_name = "string_value"
array_variable = ["value1", "value2", "value3"]
```

**制約**:
- 変数名は英字またはアンダースコアで始まり、英数字とアンダースコアのみ使用可能（既存の `ValidateVariableName` を使用）
- 同じ変数名が複数定義された場合はエラー（TOMLパーサーが自動検出）
- 値の型は文字列または文字列配列のみ（それ以外の型はエラー）
- 変数名に予約語は使用不可（`__runner_` で始まる名前は予約済み）
- 配列変数の要素数は最大1000個まで（DoS防止）
- 文字列値の最大長は10KB（DoS防止）

**例**:
```toml
[vars]
base_dir = "/opt/myapp"
env_type = "production"
config_path = "%{base_dir}/%{env_type}/config.yml"
include_files = [
    "%{base_dir}/config.yml",
    "%{base_dir}/secrets.yml"
]
```

### 2.2 変数展開の依存関係解決

#### F-002: 既存の `ExpandString` を活用した再帰展開

**概要**: 変数定義内で他の変数を参照する場合、既存の `ExpandString` 関数の再帰展開機能により依存関係を解決する。

**動作**:
1. `map[string]interface{}` から変数を1つずつ取得（順序は不定）
2. 各変数値に対して `ExpandString` を呼び出し
3. `ExpandString` が `%{variable}` を検出すると、再帰的に依存変数を展開
4. 循環依存は `expandStringRecursive` の visited マップで検出（既存実装）

**実装方針**:
- 既存の `ExpandString` 関数（[expansion.go:26-34](internal/runner/config/expansion.go:26-34)）をそのまま活用
- 処理順序に依存しない設計のため、`map` の反復順序が不定でも問題なし

**例**:
```toml
[vars]
# map の反復順序は不定だが、ExpandString が依存を自動解決
base_dir = "/opt/myapp"
env_type = "production"
config_path = "%{base_dir}/%{env_type}/config.yml"
```

展開プロセス例（仮に `config_path` が最初に処理される場合）:
1. `config_path` の値 `"%{base_dir}/%{env_type}/config.yml"` を展開
2. `%{base_dir}` を検出 → `base_dir` の値を再帰的に展開 → `/opt/myapp`
3. `%{env_type}` を検出 → `env_type` の値を再帰的に展開 → `production`
4. 結果: `/opt/myapp/production/config.yml`

#### F-003: 循環依存の検出

**概要**: 変数定義間に循環依存が存在する場合はエラーとして検出する。

**動作**:
- 既存の `expandStringRecursive` 関数の visited マップにより検出
- エラーメッセージで循環依存のパスを表示（既存実装）

**例（エラーケース）**:
```toml
[vars]
a = "%{b}"
b = "%{c}"
c = "%{a}"  # 循環依存: a → b → c → a
```

**エラーメッセージ例**:
```
circular reference detected in vars: variable 'a' -> 'b' -> 'c' -> 'a'
```

**実装**:
- 既存の `ErrCircularReferenceDetail` エラー型を使用（[expansion.go:111-118](internal/runner/config/expansion.go:111-118)）
- 新たな実装は不要

### 2.3 配列変数の展開

#### F-004: 配列要素内での変数展開

**概要**: 配列変数の各要素内で変数展開を行う。

**動作**:
- 配列の各要素を個別に変数展開
- 展開後も配列構造を維持

**例**:
```toml
[vars]
base_dir = "/opt/myapp"
include_files = [
    "%{base_dir}/config.yml",
    "%{base_dir}/secrets.yml"
]
```

展開後:
```toml
include_files = [
    "/opt/myapp/config.yml",
    "/opt/myapp/secrets.yml"
]
```

### 2.4 型検証

#### F-005: 変数値の型チェック

**概要**: `vars` の値が文字列または文字列配列であることを検証する。

**動作**:
- TOMLパース時に型を検証
- サポートされていない型（数値、真偽値、ネストしたテーブルなど）が指定された場合はエラー
- 配列内に非文字列要素（数値、真偽値など）が含まれる場合はエラー

**エラーメッセージ例**:
```
variable "count" has unsupported type int64: only string and []string are supported
variable "mixed_array" has invalid array element at index 2: expected string, got int64
```

#### F-006: 配列変数の制限

**概要**: 配列変数のサイズに制限を設け、リソース枯渇攻撃を防止する。

**動作**:
- 配列要素数の上限: 1000個
- 各文字列要素の最大長: 10KB
- 制限を超えた場合は設定ファイル読み込み時にエラー

**エラーメッセージ例**:
```
variable "large_array" exceeds maximum array size: got 1500, max 1000
variable "long_value" value exceeds maximum length: got 15000 bytes, max 10240 bytes
```

## 3. 非機能要件

### 3.1 互換性 (Compatibility)

#### NF-001: 後方互換性の非維持

**要件**: 既存の配列ベース形式（`vars = ["key=value"]`）はサポートしない。

**理由**:
- 前提条件として後方互換性は考慮不要
- 2つの形式を同時サポートすることによる複雑性を避ける

**移行方針**:
- 既存の設定ファイルは手動で新形式に移行
- 移行ツールの提供は対象外（必要に応じて別タスクとして検討）

### 3.2 保守性 (Maintainability)

#### NF-002: 明確なエラーメッセージ

**要件**: 変数定義に関するエラーは明確で分かりやすいメッセージを表示すること。

**エラーメッセージ例**:
- `circular dependency detected in vars: a -> b -> c -> a`
- `variable "config_path" references undefined variable "base_dir"`
- `variable "count" has unsupported type int64: only string and []string are supported`
- `invalid variable name "__runner_reserved": names starting with "__runner_" are reserved`
- `duplicate variable name "base_dir"`（TOMLパーサーが検出）
- `variable "mixed" has invalid array element at index 0: expected string, got float64`

**エラーメッセージ設計原則**:
- エラー発生箇所を特定可能（レベル、フィールド名、変数名）
- 期待される形式と実際の入力を表示
- 可能な場合は修正方法のヒントを提供

#### NF-003: テストカバレッジ

**要件**: 変数定義と展開機能の全ての分岐をカバーするテストを作成すること。

**確認項目**:
- 正常系: 文字列変数、配列変数、変数参照、依存関係解決
- 異常系: 循環依存、未定義変数参照、不正な型、不正な変数名
- エッジケース: 空文字列、空配列、特殊文字、自己参照

### 3.3 性能 (Performance)

#### NF-004: オーバーヘッドの最小化

**要件**: 依存関係解決によるオーバーヘッドを最小限に抑えること。

**期待値**:
- 変数展開処理は設定ファイル読み込み時に1回のみ実行
- 通常のユースケース（数個〜数十個の変数）では無視できるオーバーヘッド
- 変数数が100個を超える場合でも、読み込み時間の増加は100ms以内

**制限値**:
- 最大変数数: 1000個/レベル（global、group、command それぞれ）
- 配列要素数: 最大1000個/変数
- 文字列長: 最大10KB/値
- 再帰深度: 最大100（既存の `MaxRecursionDepth`）

**監視項目** (SRE観点):
- 設定ファイル読み込み時間のメトリクス
- 変数展開でのエラー率
- 制限値到達頻度のログ

### 3.4 セキュリティ (Security)

#### NF-005: 変数名のバリデーション

**要件**: 変数名は既存の `ValidateVariableName` で検証すること。

**検証内容**:
- 英字またはアンダースコアで始まる
- 英数字とアンダースコアのみ使用
- 予約語（`__runner_` で始まる名前）を拒否

#### NF-006: 変数値のセキュリティ検証は使用時点で実施

**設計判断**: 変数値（文字列および配列の各要素）に対するセキュリティ検証は、vars定義時ではなく、最終的な使用時点（cmd, args, env等への展開時）で実施する。

**理由**:
- **コンテキスト依存の要件**: 環境変数、コマンドパス、出力パスなど、使用箇所によって検証要件が異なる
- **DRY原則**: 既存の検証ロジック（`ValidateEnvironmentValue`, `validateCommandPath`, `ValidateOutputPath`等）を再利用
- **柔軟性**: varsは内部的なテンプレート変数であり、使用時点まで最終的なコンテキストが確定しない
- **責任範囲の明確化**: vars段階では構造的検証、使用時点では意味的検証という分離により保守性が向上

**vars定義時の検証範囲**（本タスクの対象）:
- 変数名のバリデーション（`ValidateVariableName` 使用）
- 型検証（文字列または文字列配列のみ）
- サイズ制限（配列要素数、文字列長）
- 展開深さ制限（循環依存検出）

**使用時点での検証**（既存実装、本タスクの対象外）:
- 環境変数として使用: `ValidateEnvironmentValue`
- コマンドパスとして使用: `validateCommandPath`
- 出力パスとして使用: `ValidateOutputPath`

#### NF-007: 展開の深さ制限

**要件**: 変数展開の深さに制限を設け、無限ループや DoS 攻撃を防止すること。

**実装方針**:
- 既存の `MaxRecursionDepth` (100) がそのまま適用される（[expansion.go:19-20](internal/runner/config/expansion.go:19-20)）
- 新たな実装は不要

#### NF-008: リソース制限

**要件**: 設定ファイルの処理によるリソース枯渇攻撃を防止すること。

**制限値**:
| リソース | 制限値 | 根拠 |
|---------|--------|------|
| 変数数/レベル | 1000 | 実用上十分、メモリ使用量を制限 |
| 配列要素数/変数 | 1000 | 展開処理時間を制限 |
| 文字列長/値 | 10KB | メモリ使用量を制限 |
| 展開深度 | 100 | スタックオーバーフロー防止（既存） |

**エラーハンドリング**:
- 制限超過時は明確なエラーメッセージで即座に拒否
- パニックではなくエラーとして返却

## 4. 実装計画

### 4.1 実装フェーズ

#### Phase 1: 型定義の変更

**対象ファイル**:
- `internal/runner/runnertypes/spec.go` - `Vars` フィールドの型変更

**実装内容**:
- `Vars []string` から `Vars map[string]interface{}` に変更
- または、より型安全な `Vars map[string]VarValue` を定義
- `GlobalSpec`, `GroupSpec`, `CommandSpec` すべてで型を変更

**型定義例**:
```go
// VarValue represents a variable value that can be either a string or a string array.
// This is used for type-safe handling of vars defined in TOML configuration.
type VarValue struct {
    Type   VarType  // The type of the variable value
    String string   // Value when Type == VarTypeString
    Array  []string // Value when Type == VarTypeArray
}

type VarType int

const (
    VarTypeString VarType = iota
    VarTypeArray
)

// String returns the string representation of VarValue for debugging.
func (v VarValue) String() string {
    switch v.Type {
    case VarTypeString:
        return v.String
    case VarTypeArray:
        return fmt.Sprintf("%v", v.Array)
    default:
        return "<unknown>"
    }
}
```

**代替案: `map[string]interface{}` の直接使用**:
- TOMLパーサーが返す型をそのまま使用
- 型アサーション時にエラーハンドリングが必要
- より柔軟だが、型安全性が低下
- 実装の簡便さを優先する場合はこちらを選択

**推奨**: `map[string]interface{}` を使用し、`ProcessVars` 内で型検証と変換を行う。これにより TOML パース層と処理層の分離が明確になる。

**テスト**:
- TOML パース時の型変換テスト
- 不正な型の拒否テスト
- 配列内の非文字列要素の検出テスト

#### Phase 2: 変数展開ロジックの更新

**対象ファイル**:
- `internal/runner/config/expansion.go` - `ProcessVars` 関数の更新

**実装内容**:
- `ProcessVars` の引数を `vars []string` から `vars map[string]interface{}` に変更
- 配列変数の各要素に対する変数展開ロジック追加
- 既存の `ExpandString` を活用した再帰展開（実装済み）
- エラーハンドリングの強化
- サイズ制限の実装（配列要素数、文字列長）

**実装例**:
```go
// Maximum limits to prevent resource exhaustion
const (
    MaxVarsPerLevel    = 1000
    MaxArrayElements   = 1000
    MaxStringValueLen  = 10 * 1024 // 10KB
)

func ProcessVars(vars map[string]interface{}, baseExpandedVars map[string]string, level string) (map[string]string, map[string][]string, error) {
    // Check total variable count
    if len(vars) > MaxVarsPerLevel {
        return nil, nil, fmt.Errorf("too many variables in %s: got %d, max %d", level, len(vars), MaxVarsPerLevel)
    }

    expandedStrings := maps.Clone(baseExpandedVars)
    expandedArrays := make(map[string][]string)

    for varName, rawValue := range vars {
        // Validate variable name
        if err := validateVariableName(varName, level, "vars"); err != nil {
            return nil, nil, err
        }

        switch v := rawValue.(type) {
        case string:
            // Validate string length
            if len(v) > MaxStringValueLen {
                return nil, nil, fmt.Errorf("variable %q value too long: got %d bytes, max %d", varName, len(v), MaxStringValueLen)
            }
            // Use existing ExpandString
            expanded, err := ExpandString(v, expandedStrings, level, "vars")
            if err != nil {
                return nil, nil, err
            }
            expandedStrings[varName] = expanded

        case []interface{}:
            // Validate array size
            if len(v) > MaxArrayElements {
                return nil, nil, fmt.Errorf("variable %q array too large: got %d elements, max %d", varName, len(v), MaxArrayElements)
            }
            // Convert and expand each element
            expandedArray := make([]string, len(v))
            for i, elem := range v {
                str, ok := elem.(string)
                if !ok {
                    return nil, nil, fmt.Errorf("variable %q has invalid array element at index %d: expected string, got %T", varName, i, elem)
                }
                if len(str) > MaxStringValueLen {
                    return nil, nil, fmt.Errorf("variable %q array element %d too long: got %d bytes, max %d", varName, i, len(str), MaxStringValueLen)
                }
                expanded, err := ExpandString(str, expandedStrings, level, fmt.Sprintf("vars[%s][%d]", varName, i))
                if err != nil {
                    return nil, nil, err
                }
                expandedArray[i] = expanded
            }
            expandedArrays[varName] = expandedArray

        default:
            return nil, nil, fmt.Errorf("variable %q has unsupported type %T: only string and []string are supported", varName, rawValue)
        }
    }

    return expandedStrings, expandedArrays, nil
}
```

**設計上の決定: 配列変数の格納方法**

配列変数は文字列変数とは別のマップ（`map[string][]string`）に格納する。これにより：
1. 型安全性が向上（`map[string]interface{}` を避ける）
2. 文字列変数と配列変数の区別が明確
3. 展開後の使用箇所で適切な型が利用可能

**RuntimeGroup/RuntimeCommand への影響**:
- `ExpandedVars map[string]string` は維持
- `ExpandedArrayVars map[string][]string` を新規追加

**テスト**:
- 文字列変数の展開テスト（既存テストを更新）
- 配列変数の展開テスト（新規）
- 変数参照を含む配列要素の展開テスト（新規）
- 循環依存の検出テスト（既存が動作することを確認）
- サイズ制限のテスト（新規）
- 配列内の型エラー検出テスト（新規）

#### Phase 3: バリデーションの統合

**対象ファイル**:
- `internal/runner/config/expansion.go`（既存ファイルに統合）

**実装内容**:
- 変数名のバリデーション（`ValidateVariableName` 使用）- 既存ロジックを再利用
- サイズ制限の検証（Phase 2 で実装）
- 展開深さ制限の適用（既存）

**設計方針**: 変数値のセキュリティ検証（危険パターン検出など）は実施しない。これらは最終的な使用時点（cmd, args, env等への展開時）で既存の検証ロジック（`ValidateEnvironmentValue`, `validateCommandPath`, `ValidateOutputPath`等）により実施される。vars段階では構造的検証（型、サイズ、命名規則）のみを行う。

**テスト**:
- 不正な変数名の拒否テスト（既存を更新）
- サイズ制限超過のテスト（新規）
- 展開深さ制限のテスト（既存を確認）

#### Phase 4: 統合テスト

**対象ファイル**:
- `internal/runner/config/loader_test.go` - エンドツーエンドテスト
- `internal/runner/config/expansion_test.go` - ユニットテスト

**実装内容**:
- TOML 読み込み → 依存解決 → 変数展開の全フロー
- 既存のテストの更新（配列形式からテーブル形式への変更）
- サンプル設定ファイルの更新

#### Phase 5: サンプルファイルとドキュメントの更新

**対象ファイル**:
- `sample/*.toml` - すべてのサンプル設定ファイル
- `README.md`, `README.ja.md` - ドキュメント更新
- `docs/user/*.md` - ユーザードキュメント更新

**実装内容**:
- すべてのサンプルファイルを新形式に変換
- ドキュメントの例を更新
- 移行ガイドの作成（必要に応じて）

### 4.2 影響を受けるファイル一覧

| ファイル | 変更内容 |
|---------|---------|
| `internal/runner/runnertypes/spec.go` | `Vars` フィールドの型変更 |
| `internal/runner/runnertypes/runtime.go` | `ExpandedArrayVars` フィールド追加 |
| `internal/runner/config/expansion.go` | `ProcessVars` の更新 |
| `internal/runner/config/expansion_test.go` | テスト更新 |
| `internal/runner/config/loader.go` | TOML パース部分の更新（必要に応じて） |
| `internal/runner/config/loader_test.go` | 統合テスト更新 |
| `sample/*.toml` | すべてのサンプルファイル |
| `cmd/runner/testdata/*.toml` | テストデータファイル |

### 4.3 リスクと対策

#### Risk-001: 型変換エラーのハンドリング

**リスク**: TOMLパーサーが返す `interface{}` の型アサーションでパニックが発生する可能性。

**対策**:
- 型アサーション時に ok チェックを実施（type switch を使用）
- 不正な型の場合は明確なエラーメッセージを返す
- パニックリカバリーは行わない（エラーとして適切に処理）

#### Risk-002: 配列変数の文字列変数への格納

**リスク**: 配列変数を展開した結果を `map[string]string` にどう格納するか（配列は文字列にできない）。

**対策**:
- 配列変数は展開結果を別の構造（`map[string][]string`）に格納
- `RuntimeGroup`, `RuntimeCommand` に `ExpandedArrayVars` フィールドを追加
- 文字列変数と配列変数は別々に管理し、使用時に適切な方を参照

#### Risk-003: 循環依存の検出

**リスク**: 既存の `ExpandString` の循環依存検出が、テーブル形式でも正しく動作するか。

**対策**:
- 既存の `expandStringRecursive` は visited マップで循環依存を検出（[expansion.go:111-118](internal/runner/config/expansion.go:111-118)）
- この仕組みは処理順序に依存しないため、テーブル形式でもそのまま動作
- テストケースで動作を確認

#### Risk-004: TOML パーサーの配列型処理

**リスク**: TOML パーサー（BurntSushi/toml）が配列を `[]interface{}` として返すため、各要素の型チェックが必要。

**対策**:
- 配列の各要素に対して型アサーションを実施
- 非文字列要素が含まれる場合は詳細なエラーメッセージ（インデックス、期待型、実際の型）を返す

#### Risk-005: 既存設定ファイルの互換性

**リスク**: 後方互換性を維持しないため、既存の設定ファイルがすべて動作しなくなる。

**対策**:
- リリースノートで明確に破壊的変更を記載
- 変換例をドキュメントに記載
- 旧形式を検出した場合は、新形式への移行方法を示すエラーメッセージを表示

```go
// 旧形式検出時のエラーメッセージ例
if _, ok := vars.([]interface{}); ok {
    return fmt.Errorf("vars array format is no longer supported; please migrate to table format: [vars] section")
}
```

#### Risk-006: map の反復順序の非決定性

**リスク**: Go の map 反復順序は非決定的であり、デバッグやログ出力時に結果が安定しない可能性。

**対策**:
- 変数展開ロジック自体は順序に依存しない設計（既存の再帰展開による）
- ログ出力やエラーメッセージではソート済みのキーリストを使用
- テストでは個別の変数値を検証（全体の順序に依存しない）

## 5. 変換例

### 5.1 シンプルな変数定義

**変更前（配列ベース）**:
```toml
vars = [
    "base_dir=/opt/myapp",
    "env_type=production"
]
```

**変更後（テーブルベース）**:
```toml
[vars]
base_dir = "/opt/myapp"
env_type = "production"
```

### 5.2 変数参照を含む定義

**変更前（配列ベース）**:
```toml
vars = [
    "base_dir=/opt/myapp",
    "env_type=production",
    "config_path=%{base_dir}/%{env_type}/config.yml"
]
```

**変更後（テーブルベース）**:
```toml
[vars]
base_dir = "/opt/myapp"
env_type = "production"
config_path = "%{base_dir}/%{env_type}/config.yml"
```

### 5.3 配列変数の定義（新規機能）

**変更後（テーブルベース）**:
```toml
[vars]
base_dir = "/opt/myapp"
include_files = [
    "%{base_dir}/config.yml",
    "%{base_dir}/secrets.yml",
    "%{base_dir}/credentials.yml"
]
```

展開後:
```toml
include_files = [
    "/opt/myapp/config.yml",
    "/opt/myapp/secrets.yml",
    "/opt/myapp/credentials.yml"
]
```

### 5.4 グループレベルでの変数定義

**変更前（配列ベース）**:
```toml
[[groups]]
name = "deploy"
vars = [
    "deploy_target=production",
    "deploy_path=/var/www/%{deploy_target}"
]
```

**変更後（テーブルベース）**:
```toml
[[groups]]
name = "deploy"

[groups.vars]
deploy_target = "production"
deploy_path = "/var/www/%{deploy_target}"
```

### 5.5 コマンドレベルでの変数定義

**変更前（配列ベース）**:
```toml
[[groups.commands]]
name = "backup"
vars = [
    "backup_suffix=.bak"
]
cmd = "/usr/bin/cp"
args = ["config.yml", "config.yml%{backup_suffix}"]
```

**変更後（テーブルベース）**:
```toml
[[groups.commands]]
name = "backup"
cmd = "/usr/bin/cp"
args = ["config.yml", "config.yml%{backup_suffix}"]

[groups.commands.vars]
backup_suffix = ".bak"
```

## 6. セキュリティ考慮事項

### 6.1 脅威モデル

テーブル形式の `vars` において想定する脅威：

1. **設定ファイル改ざん**: 攻撃者が TOML 設定ファイルを改ざんし、悪意のある変数値を注入する
2. **DoS攻撃**: 大量の変数定義や深い依存関係により、設定ファイル読み込みを遅延させる
3. **型システム悪用**: 不正な型を使用してパース処理やメモリ使用を攻撃する

**注記**: コマンドインジェクションやパストラバーサルなどのコンテキスト依存の脅威は、vars定義段階では対象外とする。これらは変数が最終的に使用される時点（cmd, args, env等への展開時）で既存のセキュリティ検証により防御される。

### 6.2 セキュリティ設計原則

1. **Validation at Input（入力時検証）**:
   - **構造的検証**: 変数名のバリデーション（`ValidateVariableName`）、型検証、サイズ制限
   - **意味的検証**: 使用時点で実施（コンテキスト依存のため）

2. **Fail-Safe Defaults（安全側への失敗）**:
   - 不正な入力はエラーとして拒否
   - 不明な型は許可ではなく拒否

3. **Defense in Depth（多層防御）**:
   - **vars定義時**: 構造的検証（命名規則、型、サイズ）
   - **変数展開時**: 再帰深さ制限、循環依存検出
   - **使用時**: コンテキスト別セキュリティ検証（`ValidateEnvironmentValue`, `validateCommandPath`, `ValidateOutputPath`等）

4. **Separation of Concerns（関心の分離）**:
   - vars層: テンプレート変数の構造的整合性を保証
   - 使用層: 実際の使用コンテキストに応じたセキュリティを保証

### 6.3 検証チェックリスト

実装時に確認すべきセキュリティ項目：

**vars定義時の構造的検証**（本タスクで実装）:
- [ ] 変数名のバリデーション（`ValidateVariableName` 使用）
- [ ] 予約プレフィックス（`__runner_`）の拒否
- [ ] 不正な型の拒否（文字列・配列以外）
- [ ] 配列内の非文字列要素の検出と拒否
- [ ] 変数数の制限（1000個/レベル）
- [ ] 配列要素数の制限（1000個/変数）
- [ ] 文字列長の制限（10KB/値）
- [ ] 旧形式（配列ベース）の検出と明確なエラーメッセージ

**変数展開時の検証**（既存実装を活用）:
- [ ] 循環依存の検出と拒否（`expandStringRecursive` の visited マップ）
- [ ] 未定義変数参照の検出と拒否（`ExpandString` の既存機能）
- [ ] 展開深さ制限の適用（`MaxRecursionDepth`）

**使用時点のセキュリティ検証**（既存実装、本タスクの対象外）:
- 環境変数として使用: `ValidateEnvironmentValue` による危険パターン検出
- コマンドパスとして使用: `validateCommandPath` によるパス検証
- 出力パスとして使用: `ValidateOutputPath` によるパストラバーサル防止

## 7. 運用考慮事項 (SRE観点)

### 7.1 監視とアラート

**メトリクス**:
- 設定ファイル読み込み時間（P50, P95, P99）
- 変数展開エラー率
- 制限値到達イベント数

**ログ**:
- 設定ファイル読み込み成功/失敗
- 変数展開エラー（詳細なコンテキスト付き）
- 制限値に近づいた場合の警告（例: 変数数が800を超えた場合）

### 7.2 トラブルシューティング

**よくある問題と対処法**:

| 問題 | 原因 | 対処法 |
|------|------|--------|
| `vars array format is no longer supported` | 旧形式の設定ファイル | テーブル形式に移行 |
| `circular dependency detected` | 変数間の循環参照 | 依存関係を見直し |
| `undefined variable` | 未定義の変数を参照 | 変数定義を追加、またはスペルミス確認 |
| `unsupported type` | 非文字列/非配列の値 | 値を文字列または文字列配列に修正 |

### 7.3 ロールバック計画

本変更は破壊的変更のため、以下のロールバック計画を準備：

1. **設定ファイルのバックアップ**: デプロイ前に既存設定をバックアップ
2. **バイナリのバージョン管理**: 旧バージョンのバイナリを保持
3. **ロールバック手順**:
   - 旧バージョンのバイナリをデプロイ
   - バックアップした設定ファイルを復元
   - サービス再起動

### 7.4 デプロイ戦略

**推奨手順**:
1. 開発環境で新形式の設定ファイルをテスト
2. ステージング環境でエンドツーエンドテスト
3. 本番環境へのデプロイ（カナリアリリース推奨）
4. モニタリングで異常がないことを確認
5. 全面展開

## 8. 用語集

- **テーブル形式 (Table Format)**: TOML の `[section]` 記法を使用した変数定義形式
- **配列ベース形式 (Array-based Format)**: 従来の `vars = ["key=value"]` 形式
- **依存グラフ (Dependency Graph)**: 変数間の依存関係を表すグラフ
- **循環依存 (Circular Dependency)**: 変数 A が B に依存し、B が A に依存するような状態
- **変数展開 (Variable Expansion)**: `%{variable}` 形式の変数参照を実際の値に置き換える処理
- **再帰展開 (Recursive Expansion)**: 変数参照を検出した際に、その変数の値を再帰的に展開する処理
- **予約プレフィックス (Reserved Prefix)**: システムが内部使用のために予約している変数名の接頭辞（`__runner_`）

## 9. 参考資料

### 9.1 関連ファイル

- `internal/runner/runnertypes/spec.go` - 設定ファイルの型定義
- `internal/runner/runnertypes/runtime.go` - ランタイム型定義
- `internal/runner/config/loader.go` - TOML 読み込み処理
- `internal/runner/config/expansion.go` - 変数展開ロジック
- `internal/runner/config/validation.go` - バリデーションロジック
- `internal/runner/security/environment_validation.go` - セキュリティ検証
- `sample/variable_expansion_advanced.toml` - サンプル設定ファイル

### 9.2 関連タスク

- Task 0026: 変数展開機能の実装
- Task 0033: vars/env の分離
- Task 0062: コマンドテンプレート機能（配列変数のユースケース）

### 9.3 外部参考資料

- [TOML v1.0.0 Specification](https://toml.io/en/v1.0.0)
- [BurntSushi/toml Go Library](https://github.com/BurntSushi/toml)

### 9.4 設計上の議論

この要件定義書は、以下の議論に基づいて作成された：

1. **問題の発見**: 配列ベース形式では文字列配列を扱えない
2. **トリガー**: コマンドテンプレート機能（Task 0062）で配列パラメータのサポートが必要
3. **解決策の検討**: テーブル形式への変更を決定
4. **Pros/Cons分析**: 定義順序の重要性、型の多様性、保守性などを検討
5. **最終決定**: `[vars]` セクション形式を採用、依存解決は既存の再帰展開で実現

## 10. 変更履歴

| 日付 | バージョン | 変更内容 |
|------|-----------|---------|
| 2025-12-04 | 1.0 | 初版作成 |
| 2025-12-04 | 1.1 | レビュー反映: 変更対象範囲の明確化、リソース制限の追加、運用考慮事項の追加、リスク項目の拡充 |
| 2025-12-04 | 1.2 | セキュリティ設計の明確化: 変数値のセキュリティ検証を使用時点で実施する設計に変更。vars段階では構造的検証のみ実施し、意味的検証（危険パターン検出等）は最終使用時点（cmd, args, env等への展開時）で既存の検証ロジックにより実施する方針を明記 |
