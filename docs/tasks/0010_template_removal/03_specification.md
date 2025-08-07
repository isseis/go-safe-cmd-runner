# 詳細仕様書：テンプレート機能の削除

## 3.1. 概要

このドキュメントは、`02_architecture.md`で定義された設計方針に基づき、テンプレート機能削除の各ステップとコンポーネントの具体的な実装仕様を定義する。

## 3.2. 削除対象の詳細仕様

### 3.2.1. ファイル削除リスト

**完全削除対象:**
```
internal/runner/template/
└── template.go                # テンプレートエンジン実装（約400行）
```

**修正対象ファイル:**
```
internal/runner/runnertypes/config.go   # 構造体定義の変更
internal/runner/runner.go               # Runner構造体とメソッドの変更
sample/config.toml                      # サンプル設定ファイルの更新
sample/test.toml                        # テスト用設定ファイルの更新
```

### 3.2.2. 削除対象コード詳細

#### `template.go` の削除内容

**削除対象の構造体とインターフェース:**
```go
// Template represents a template definition
type Template struct {
    Name        string            `toml:"name"`
    Description string            `toml:"description"`
    TempDir     bool              `toml:"temp_dir"`
    Cleanup     bool              `toml:"cleanup"`
    WorkDir     string            `toml:"workdir"`
    Variables   map[string]string `toml:"variables"`
}

// Engine manages template expansion and application
type Engine struct {
    templates map[string]*Template
    variables map[string]string
}
```

**削除対象のエラー定義:**
```go
var (
    ErrTemplateNotFound   = errors.New("template not found")
    ErrUndefinedVariable  = errors.New("undefined template variable")
    ErrCircularDependency = errors.New("circular template dependency detected")
    ErrInvalidTemplate    = errors.New("invalid template syntax")
    ErrEmptyTemplateName  = errors.New("template name cannot be empty")
    ErrNilTemplate        = errors.New("template cannot be nil")
)
```

**削除対象の主要メソッド:**
```go
func NewEngine() *Engine
func (e *Engine) RegisterTemplate(name string, tmpl *Template) error
func (e *Engine) GetTemplate(name string) (*Template, error)
func (e *Engine) SetVariable(key, value string)
func (e *Engine) SetVariables(vars map[string]string)
func (e *Engine) ApplyTemplate(group *runnertypes.CommandGroup, templateName string) (*runnertypes.CommandGroup, error)
func (e *Engine) ValidateTemplate(name string) error
func (e *Engine) ListTemplates() []string
func (e *Engine) detectCircularDependencies(variables map[string]string) error
func (e *Engine) expandString(input string, variables map[string]string) (string, error)
func (e *Engine) extractVariableReferences(input string) []string
```

## 3.3. 構造体変更の詳細仕様

### 3.3.1. `runnertypes/config.go` の変更

#### 変更前
```go
package runnertypes

// Config represents the root configuration structure
type Config struct {
    Version   string                    `toml:"version"`
    Global    GlobalConfig              `toml:"global"`
    Templates map[string]TemplateConfig `toml:"templates"`
    Groups    []CommandGroup            `toml:"groups"`
}

// TemplateConfig represents a template configuration
type TemplateConfig struct {
    Description string            `toml:"description"`
    TempDir     bool              `toml:"temp_dir"`
    Cleanup     bool              `toml:"cleanup"`
    WorkDir     string            `toml:"workdir"`
    Variables   map[string]string `toml:"variables"`
}

// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`
    Template     string    `toml:"template"`
    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    EnvAllowlist []string  `toml:"env_allowlist"`
}
```

#### 変更後
```go
package runnertypes

// Config represents the root configuration structure
type Config struct {
    Version string         `toml:"version"`
    Global  GlobalConfig   `toml:"global"`
    Groups  []CommandGroup `toml:"groups"`
}

// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
    Name         string    `toml:"name"`
    Description  string    `toml:"description"`
    Priority     int       `toml:"priority"`
    DependsOn    []string  `toml:"depends_on"`

    // テンプレートから移動したフィールド
    TempDir      bool      `toml:"temp_dir"`   // 一時ディレクトリ自動生成
    Cleanup      bool      `toml:"cleanup"`    // 自動クリーンアップ
    WorkDir      string    `toml:"workdir"`    // 作業ディレクトリ

    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`
    EnvAllowlist []string  `toml:"env_allowlist"`
}

// TemplateConfig構造体は完全に削除
```

### 3.3.2. `runner.go` の変更

#### 変更対象の構造体とメソッド

**Options 構造体:**
```go
// 変更前
type Options struct {
    filesystem      common.FileSystem
    resourceManager resource.Manager
    executor        executor.Executor
    templateEngine  *template.Engine  // 削除対象
}

// 変更後
type Options struct {
    filesystem      common.FileSystem
    resourceManager resource.Manager
    executor        executor.Executor
}
```

**Runner 構造体:**
```go
// 変更前
type Runner struct {
    filesystem          common.FileSystem
    resourceManager     resource.Manager
    executor            executor.Executor
    templateEngine      *template.Engine  // 削除対象
    securityValidator   security.Validator
    environmentFilter   environment.Filter
}

// 変更後
type Runner struct {
    filesystem          common.FileSystem
    resourceManager     resource.Manager
    executor            executor.Executor
    securityValidator   security.Validator
    environmentFilter   environment.Filter
}
```

#### 削除対象の関数とメソッド

```go
// 削除対象
func WithTemplateEngine(engine *template.Engine) Option

// New関数内の削除対象部分
if opts.templateEngine == nil {
    opts.templateEngine = template.NewEngine()
}
// ...
templateEngine:      opts.templateEngine,
```

#### 変更対象のメソッド

**`RunCommandGroup` メソッドの変更:**
```go
// 変更前（テンプレート適用処理を含む）
func (r *Runner) RunCommandGroup(group runnertypes.CommandGroup, ctx context.Context) error {
    // Apply template to the group if specified
    groupToRun := group
    if group.Template != "" {
        appliedGroup, err := r.templateEngine.ApplyTemplate(&group, group.Template)
        if err != nil {
            return fmt.Errorf("failed to apply template %s to group %s: %w", group.Template, group.Name, err)
        }
        groupToRun = *appliedGroup
    }

    // ... 残りの処理
}

// 変更後（テンプレート適用処理を削除）
func (r *Runner) RunCommandGroup(group runnertypes.CommandGroup, ctx context.Context) error {
    // テンプレート適用処理を削除し、groupを直接使用
    groupToRun := group

    // ... 残りの処理（変更なし）
}
```

## 3.4. 設定ファイル変更の詳細仕様

### 3.4.1. `sample/config.toml` の変更

#### 削除対象セクション
```toml
# Template definitions for reusable command configurations
# Templates allow you to define common settings and variables that can be
# applied to multiple command groups
[templates]

# Development environment template
[templates.dev]
description = "Development environment template"

# Template variables that can be referenced in commands using {{.variable_name}}
[templates.dev.variables]
app_name = "myapp-dev"
port = "3000"
env_type = "development"

# Production environment template
[templates.prod]
description = "Production environment template"

[templates.prod.variables]
app_name = "myapp-prod"
port = "8080"
env_type = "production"
```

#### 変更対象セクション例

**変更前:**
```toml
[[groups]]
name = "setup"
description = "Initial setup and preparation"
priority = 1
depends_on = []
template = "dev"

[[groups.commands]]
name = "create_workspace"
description = "Create workspace directories"
cmd = "mkdir"
args = ["-p", "{{.app_name}}", "logs", "tmp"]
timeout = 30
```

**変更後:**
```toml
[[groups]]
name = "setup"
description = "Initial setup and preparation"
priority = 1
depends_on = []
temp_dir = false
cleanup = false
workdir = "/tmp/cmd-runner"

[[groups.commands]]
name = "create_workspace"
description = "Create workspace directories"
cmd = "mkdir"
args = ["-p", "$APP_NAME", "logs", "tmp"]
env = ["APP_NAME=myapp-dev"]
timeout = 30
```

### 3.4.2. 環境変数使用パターン

#### コマンド引数での環境変数使用
```toml
[[groups.commands]]
cmd = "echo"
args = ["Building $APP_NAME on port $PORT"]
env = [
    "APP_NAME=myapp-dev",
    "PORT=3000",
    "ENV_TYPE=development"
]
```

#### 外部環境変数の参照
```toml
[[groups.commands]]
cmd = "deploy"
args = ["--env", "${DEPLOY_ENV:-development}"]  # デフォルト値付き
```

## 3.5. テスト変更の詳細仕様

### 3.5.1. 削除対象テスト

**テンプレート関連のテストファイル:**
- `internal/runner/template/template_test.go`（存在する場合）
- `runner_test.go` 内のテンプレート関連テストケース

**削除対象テストケース例:**
```go
// 削除対象
func TestWithTemplateEngine(t *testing.T) { /* ... */ }
func TestRunCommandGroup_WithTemplate(t *testing.T) { /* ... */ }
func TestTemplateVariableExpansion(t *testing.T) { /* ... */ }
```

### 3.5.2. 変更対象テスト

**`CommandGroup` 構造体のテスト:**
```go
func TestCommandGroup_NewFields(t *testing.T) {
    group := runnertypes.CommandGroup{
        Name:     "test",
        TempDir:  true,
        Cleanup:  true,
        WorkDir:  "/tmp/test",
    }

    // 新フィールドの動作確認
    assert.True(t, group.TempDir)
    assert.True(t, group.Cleanup)
    assert.Equal(t, "/tmp/test", group.WorkDir)
}
```

**設定ファイル読み込みテスト:**
```go
func TestLoadConfig_WithoutTemplate(t *testing.T) {
    configData := `
version = "1.0"
[[groups]]
name = "test"
temp_dir = true
cleanup = true
workdir = "/tmp"
`

    // テンプレートなしでの読み込み確認
    // ...
}
```

## 3.6. 実装手順の詳細

### 3.6.1. 第1段階：新しい構造体の実装

1. **`runnertypes/config.go` の修正**
   ```bash
   # CommandGroup に新フィールドを追加
   TempDir      bool      `toml:"temp_dir"`
   Cleanup      bool      `toml:"cleanup"`
   WorkDir      string    `toml:"workdir"`
   ```

2. **新フィールドの処理ロジック実装**
   - リソースマネージャーでの `TempDir` 処理
   - 実行後の `Cleanup` 処理
   - `WorkDir` の適用処理

### 3.6.2. 第2段階：テンプレート機能の削除

1. **import文の削除**
   ```go
   // runner.go から削除
   "github.com/isseis/go-safe-cmd-runner/internal/runner/template"
   ```

2. **構造体フィールドの削除**
   ```go
   // Config から削除
   Templates map[string]TemplateConfig `toml:"templates"`

   // CommandGroup から削除
   Template string `toml:"template"`

   // Runner から削除
   templateEngine *template.Engine
   ```

3. **ディレクトリの削除**
   ```bash
   rm -rf internal/runner/template/
   ```

### 3.6.3. 第3段階：設定ファイルとドキュメントの更新

1. **サンプルファイルの更新**
   - `sample/config.toml`: 全テンプレート記述を環境変数使用に変更
   - `sample/test.toml`: 同様に更新

2. **コメントとドキュメントの更新**
   - テンプレート機能に関する説明を削除
   - 環境変数使用方法の説明を追加

### 3.6.4. 第4段階：検証

1. **コンパイル確認**
   ```bash
   go build ./...
   ```

2. **テスト実行**
   ```bash
   go test ./...
   ```

3. **サンプル実行確認**
   ```bash
   go run cmd/runner/main.go -config sample/config.toml
   ```

## 3.7. エラーハンドリング

### 3.7.1. 想定されるエラーケース

1. **TOML解析エラー**: 削除されたフィールドを含む設定ファイル
   - 通常のTOML解析エラーとして処理
   - 特別なエラーメッセージは提供しない

2. **フィールド型エラー**: 新フィールドの型不整合
   - 標準的なTOML型エラーとして処理

### 3.7.2. ログ出力

削除に伴う警告やエラーログは追加しない：
- テンプレート関連のログ出力を削除
- 新フィールド使用時の通常ログは既存パターンに従う

## 3.8. パフォーマンス検証

### 3.8.1. 測定対象

1. **初期化時間**: 設定ファイル読み込み〜Runner生成
2. **実行時間**: CommandGroup実行時間
3. **メモリ使用量**: テンプレートエンジン削除による削減量

### 3.8.2. 期待される改善

1. **初期化時間**: テンプレート登録処理の削除により短縮
2. **実行時間**: テンプレート適用処理の削除により短縮
3. **メモリ使用量**: テンプレートデータ保持の削除により削減

## 3.9. 結論

この実装仕様に従ってテンプレート機能を削除することで、go-safe-cmd-runner はより単純で効率的なシステムとなる。各段階での検証を通じて、安全かつ確実な削除作業を実施できる。
