# 要件定義書：テンプレート機能の削除

## 1. 概要 (Overview)

**目的:** このドキュメントは、go-safe-cmd-runner からテンプレート機能を完全に削除することに関する要件を定義する。

**背景:**
現在のテンプレート機能は、CommandGroup に対してテンプレート設定を適用する機能を提供している。しかし、以下の理由により機能の削除を決定した：

1. **限定的な再利用性**: コマンド構造自体（`[[groups.commands]]`）はテンプレート化できず、変数展開のみの限定的な機能
2. **複雑性と利益の不均衡**: テンプレートエンジン、変数展開、循環依存検出などの実装複雑性に対して得られる利益が少ない
3. **環境変数との重複**: テンプレート変数で実現できることは環境変数や `.env` ファイルで代替可能
4. **保守コストの増大**: 約400行のテンプレート関連コードの維持コスト

## 2. 目的とスコープ (Goals and Scope)

### 2.1. 目的 (Goals)
- テンプレート機能を完全に削除し、コードベースを簡素化する
- 設定構造を直接的で理解しやすいものに変更する
- システムの保守性と可読性を向上させる

### 2.2. スコープ (In Scope)
- `internal/runner/template` パッケージの完全削除
- `runnertypes.TemplateConfig` 構造体の削除
- `runnertypes.Config.Templates` フィールドの削除
- `runnertypes.CommandGroup.Template` フィールドの削除
- テンプレート機能で提供していた `TempDir`, `Cleanup`, `WorkDir` プロパティを `CommandGroup` に直接移動
- テンプレートエンジン関連コードの削除（`Runner.templateEngine` フィールドを含む）
- サンプル設定ファイルの更新（環境変数ベースの例に変更）
- ドキュメントの更新

### 2.3. スコープ外 (Out of Scope)
- 環境変数管理の新機能追加（既存の環境変数サポートを活用）
- 設定ファイル形式の変更（TOML形式は継続）
- 既存のコマンド実行ロジックの変更
- ファイル検証機能やセキュリティ機能の変更

## 3. 現在の状況 (Current State)

### 3.1. 削除対象のコンポーネント

**削除対象のファイル:**
- `internal/runner/template/template.go` (約400行)

**削除対象の構造体とフィールド:**
```go
// 削除対象の構造体
type TemplateConfig struct {
    Description string            `toml:"description"`
    TempDir     bool              `toml:"temp_dir"`
    Cleanup     bool              `toml:"cleanup"`
    WorkDir     string            `toml:"workdir"`
    Variables   map[string]string `toml:"variables"`
}

// Config から削除されるフィールド
type Config struct {
    // ...
    Templates map[string]TemplateConfig `toml:"templates"` // 削除対象
    // ...
}

// CommandGroup から削除されるフィールド
type CommandGroup struct {
    // ...
    Template string `toml:"template"` // 削除対象
    // ...
}
```

**削除対象の Runner コンポーネント:**
```go
type Runner struct {
    // ...
    templateEngine *template.Engine // 削除対象
    // ...
}

// 削除対象の関数
func WithTemplateEngine(engine *template.Engine) Option
```

### 3.2. 現在のテンプレート使用パターン

**サンプル設定ファイルでの使用例:**
```toml
[templates.dev]
description = "Development environment template"
[templates.dev.variables]
app_name = "myapp-dev"
port = "3000"
env_type = "development"

[[groups]]
template = "dev"
[[groups.commands]]
args = ["-p", "{{.app_name}}", "logs", "tmp"]
```

## 4. 要件 (Requirements)

### 4.1. 機能要件 (Functional Requirements)

**FR-1: テンプレート機能の完全削除**
- `internal/runner/template` パッケージを削除する
- テンプレート関連のすべての構造体、関数、メソッドを削除する
- テンプレート変数展開機能（`{{.variableName}}`）を削除する

**FR-2: 設定構造の簡素化**
- `TemplateConfig` 構造体を削除する
- `Config.Templates` フィールドを削除する
- `CommandGroup.Template` フィールドを削除する

**FR-3: テンプレート機能の代替実装**
- `TemplateConfig` の `TempDir`, `Cleanup`, `WorkDir` プロパティを `CommandGroup` に直接移動する
- これらのプロパティはグループレベルで直接設定可能にする

**FR-4: 環境変数ベースの設定への移行**
- テンプレート変数の代替として環境変数や `.env` ファイルの使用を推奨する
- サンプル設定ファイルを環境変数ベースの例に更新する

### 4.2. 非機能要件 (Non-Functional Requirements)

**NFR-1: コードの簡素化**
- テンプレート関連コード（約400行）の削除により、コードベースが簡素化される
- 循環依存検出、テンプレート変数展開などの複雑なロジックが削除される

**NFR-2: 保守性の向上**
- より直接的で理解しやすい設定構造により保守性が向上する
- テンプレートエンジンの維持コストが削減される

## 5. 成功基準 (Success Criteria)

### 5.1. 削除の完了基準
- [x] `internal/runner/template` パッケージが完全に削除されている
- [x] テンプレート関連のすべての構造体とフィールドが削除されている
- [x] すべての既存テストが通過する（テンプレート関連テストを除く）
- [x] 新しい設定構造でサンプル設定ファイルが正常に動作する

### 5.2. 移行の完了基準
- [x] `CommandGroup` に `TempDir`, `WorkDir` フィールドが追加されている
- [x] サンプル設定ファイルが環境変数ベースの例に更新されている

### 5.3. ドキュメントの更新基準
- [x] README.md からテンプレート機能の説明が削除されている
- [x] 環境変数の使用方法に関する説明が追加されている
- [x] 設定例がテンプレート非使用の形式に更新されている

## 6. リスク分析 (Risk Analysis)

### 6.1. 技術リスク
**リスク:** テンプレート機能に依存している未知のコードが存在する可能性
**対策:** 全コードベースでテンプレート関連の使用箇所を徹底的に検索し、依存関係を特定する

### 6.2. 運用リスク
**リスク:** ドキュメントと実装の不整合
**対策:** 削除作業と並行してドキュメントを更新する

## 7. 実装計画 (Implementation Plan)

### 7.1. 段階的な削除計画
1. **第1段階**: 新しい `CommandGroup` 構造の実装（`TempDir`, `Cleanup`, `WorkDir` フィールドの追加）
2. **第2段階**: テンプレート機能の削除（パッケージ・構造体・フィールドの削除）
3. **第3段階**: サンプルファイルとドキュメントの更新
4. **第4段階**: テスト実行と最終検証

### 7.2. 設定例の更新
サンプル設定ファイルを環境変数ベースの例に更新する。

**更新例:**
```toml
# 変更前（テンプレート使用）
[templates.dev]
variables = { app_name = "myapp-dev", port = "3000" }

[[groups]]
template = "dev"
[[groups.commands]]
args = ["-p", "{{.app_name}}"]

# 変更後（環境変数使用）
[[groups]]
temp_dir = false
cleanup = false
workdir = ""
[[groups.commands]]
args = ["-p", "$APP_NAME"]  # 環境変数使用
```

## 8. 実装完了状況 (Implementation Status)

### 8.1. 実装完了確認
**✅ 実装完了**: 本要件定義書で定義されたテンプレート機能削除は完全に実装済みである。

### 8.2. 実装された変更内容
- **削除されたコンポーネント**: `internal/runner/template`パッケージ（約400行）完全削除
- **構造体の変更**: `Config`、`CommandGroup`、`Runner`構造体からテンプレート関連フィールド削除
- **新機能の追加**: `CommandGroup`に`TempDir`、`WorkDir`フィールド追加
- **設定変更**: サンプル設定ファイルの環境変数ベース変更完了

### 8.3. 検証結果
- **✅ 機能テスト**: 全ての既存テストが正常に通過
- **✅ 統合テスト**: サンプル設定ファイルでの動作確認完了
- **✅ 品質確認**: コードカバレッジ維持、静的解析エラー0件

## 9. 結論 (Conclusion)

テンプレート機能の削除により、go-safe-cmd-runner はより簡潔で保守しやすいコードベースとなった。環境変数ベースのアプローチにより、標準的でより広く理解されている設定管理方法に統一された。この変更により、システムの複雑性が大幅に減少し、開発・保守コストの削減が実現されている。
