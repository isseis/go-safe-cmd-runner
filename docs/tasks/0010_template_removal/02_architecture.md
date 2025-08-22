# 設計方針書：テンプレート機能の削除

## 2.1. 概要

要件定義書に基づき、go-safe-cmd-runner からテンプレート機能を完全に削除するための設計方針を定める。
この変更により、コードベースが簡素化され、より直接的で理解しやすい設定構造に変更される。

## 2.2. アーキテクチャ

### 2.2.1. 現在のアーキテクチャ

現在のシステムは以下のコンポーネントでテンプレート機能を実現している：

```
internal/runner/
├── template/
│   └── template.go          # テンプレートエンジン（約400行）
├── runnertypes/
│   └── config.go           # TemplateConfig構造体定義
└── runner.go               # Runner構造体（templateEngine フィールド）
```

**依存関係：**
- `Runner` → `template.Engine`
- `template.Engine` → `runnertypes.TemplateConfig`
- `CommandGroup` → `Template` フィールド（テンプレート名参照）

### 2.2.2. 削除後のアーキテクチャ

テンプレート機能削除後のシンプルなアーキテクチャ：

```
internal/runner/
├── runnertypes/
│   └── config.go           # 簡素化されたConfig構造体
└── runner.go               # 簡素化されたRunner構造体
```

**変更点：**
- `internal/runner/template/` パッケージ全体を削除
- `Runner` 構造体から `templateEngine` フィールドを削除
- `Config` 構造体から `Templates` フィールドを削除
- `CommandGroup` 構造体から `Template` フィールドを削除

### 2.2.3. 設定構造の変更

#### 変更前の設定構造

```go
type Config struct {
    Version   string                    `toml:"version"`
    Global    GlobalConfig              `toml:"global"`
    Templates map[string]TemplateConfig `toml:"templates"` // 削除対象
    Groups    []CommandGroup            `toml:"groups"`
}

type TemplateConfig struct {
    Description string            `toml:"description"`
    TempDir     bool              `toml:"temp_dir"`
    Cleanup     bool              `toml:"cleanup"`
    WorkDir     string            `toml:"workdir"`
    Variables   map[string]string `toml:"variables"`
}

type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`
    Template     string    `toml:"template"` // 削除対象
    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    EnvAllowlist []string  `toml:"env_allowlist"`
}
```

#### 変更後の設定構造

```go
type Config struct {
    Version string         `toml:"version"`
    Global  GlobalConfig   `toml:"global"`
    Groups  []CommandGroup `toml:"groups"`
}

type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`

    // テンプレートから移動されたフィールド
    TempDir      bool      `toml:"temp_dir"`   // 一時ディレクトリ自動生成
    Cleanup      bool      `toml:"cleanup"`    // 自動クリーンアップ
    WorkDir      string    `toml:"workdir"`    // 作業ディレクトリ

    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    EnvAllowlist []string  `toml:"env_allowlist"`
}
```

## 2.3. コンポーネント設計

### 2.3.1. 削除対象コンポーネント

#### `template.Engine` 構造体
- **責務**: テンプレート管理、変数展開、循環依存検出
- **削除理由**: 複雑性に対して得られる利益が少ない
- **影響範囲**: `Runner` 構造体から参照されている

#### `template` パッケージの主要機能
- `RegisterTemplate()`: テンプレート登録
- `ApplyTemplate()`: テンプレート適用
- `expandString()`: 変数展開（Go text/template使用）
- `detectCircularDependencies()`: 循環依存検出
- `extractVariableReferences()`: 変数参照抽出

### 2.3.2. 変更対象コンポーネント

#### `Runner` 構造体の簡素化
```go
// 変更前
type Runner struct {
    // ... 他のフィールド
    templateEngine *template.Engine // 削除対象
}

// 変更後
type Runner struct {
    // ... 他のフィールド（templateEngine削除）
}
```

#### `CommandGroup` の機能拡張
テンプレートから移動する3つのプロパティを追加：
- `TempDir bool`: 一時ディレクトリの自動生成フラグ
- `Cleanup bool`: 実行後の自動クリーンアップフラグ
- `WorkDir string`: 作業ディレクトリの指定

### 2.3.3. 実行時の動作変更

#### 変更前の処理フロー
1. 設定ファイル読み込み
2. テンプレート登録 (`templateEngine.RegisterTemplate()`)
3. コマンドグループ処理時にテンプレート適用 (`templateEngine.ApplyTemplate()`)
4. 変数展開処理 (`{{.variableName}}` → 実際の値)
5. コマンド実行

#### 変更後の処理フロー
1. 設定ファイル読み込み
2. コマンドグループを直接処理（テンプレート適用をスキップ）
3. 環境変数展開処理 (`$VARIABLE` → 実際の値、既存機能)
4. コマンド実行

## 2.4. データフローの変更

### 2.4.1. 設定ファイルの処理フロー

#### 変更前
```
TOML設定ファイル
    ↓
Config構造体（Templates含む）
    ↓
templateEngine.RegisterTemplate()
    ↓
CommandGroup処理時
    ↓
templateEngine.ApplyTemplate()
    ↓
変数展開（{{.var}} → 値）
    ↓
コマンド実行
```

#### 変更後
```
TOML設定ファイル
    ↓
Config構造体（Templates削除）
    ↓
CommandGroup直接処理
    ↓
環境変数展開（$VAR → 値）
    ↓
コマンド実行
```

### 2.4.2. 設定例の変更

#### 変更前（テンプレート使用）
```toml
[templates.dev]
description = "Development environment"
temp_dir = true
cleanup = true
workdir = "/tmp/dev"
variables = { app_name = "myapp-dev", port = "3000" }

[[groups]]
name = "build"
template = "dev"
[[groups.commands]]
cmd = "echo"
args = ["Building {{.app_name}} on port {{.port}}"]
```

#### 変更後（直接設定）
```toml
[[groups]]
name = "build"
temp_dir = true
cleanup = true
workdir = "/tmp/dev"
[[groups.commands]]
cmd = "echo"
args = ["Building $APP_NAME on port $PORT"]  # 環境変数使用
env = ["APP_NAME=myapp-dev", "PORT=3000"]    # 環境変数定義
```

## 2.5. 実装戦略

### 2.5.1. 段階的削除アプローチ

1. **準備段階**: 新しい `CommandGroup` 構造の実装
   - `TempDir`, `Cleanup`, `WorkDir` フィールドの追加
   - これらのフィールドを処理するロジックの実装

2. **削除段階**: テンプレート機能の段階的削除
   - `Runner` 構造体からテンプレート関連フィールドの削除
   - `Config` 構造体からテンプレート関連フィールドの削除
   - `template` パッケージの削除

3. **更新段階**: サンプルとドキュメントの更新
   - サンプル設定ファイルの環境変数ベース変更
   - ドキュメントの更新

### 2.5.2. 互換性への配慮

開発中プロジェクトのため、下位互換性は考慮しない：
- 既存のテンプレート機能使用設定は動作しなくなる
- エラーハンドリングは通常のTOML解析エラーに委ねる
- 移行ツールは提供しない

## 2.6. パフォーマンス影響

### 2.6.1. 改善される点

1. **初期化時間の短縮**:
   - テンプレートエンジンの初期化処理が不要
   - テンプレート登録処理が不要

2. **実行時オーバーヘッドの削減**:
   - テンプレート適用処理が不要
   - 変数展開処理が不要（Go text/template）
   - 循環依存検出が不要

3. **メモリ使用量の削減**:
   - テンプレートエンジンのメモリ使用量削減
   - テンプレート定義データの保持が不要

### 2.6.2. メンテナンス性の向上

1. **コード複雑度の削減**: 約400行のテンプレート関連コード削除
2. **理解しやすさの向上**: より直接的な設定構造
3. **テスト負荷の軽減**: テンプレート関連テストケースの削除

## 2.7. 実装結果と結論

### 2.7.1 実装完了状況
**✅ 実装完了**: このアーキテクチャ設計書で計画されたテンプレート機能削除は完全に実装済みである。

### 2.7.2 実現されたアーキテクチャ
現在の go-safe-cmd-runner は設計通りより単純で理解しやすいアーキテクチャを実現している：

**削除された要素**:
- `internal/runner/template/` パッケージ全体
- `Runner` 構造体の `templateEngine` フィールド
- `Config` 構造体の `Templates` フィールド
- `CommandGroup` 構造体の `Template` フィールド

**追加された要素**:
- `CommandGroup` 構造体の `TempDir` および `WorkDir` フィールド
- 環境変数ベースの設定管理

### 2.7.3 効果
テンプレート機能の削除により、環境変数ベースのアプローチに統一され、標準的で広く理解されている設定管理手法が採用されている。これによりシステム全体の保守性が大幅に向上した。
