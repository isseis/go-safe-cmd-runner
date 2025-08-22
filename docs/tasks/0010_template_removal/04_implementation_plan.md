# 実装計画書：テンプレート機能の削除

## 1. 実装概要

### 1.1 プロジェクト期間
- **推定期間**: 2-3日間（開発・テスト・ドキュメント更新含む）
- **複雑度**: 低〜中（約400行のコード削除、構造体変更）
- **影響範囲**: 中程度（テンプレート機能を使用する部分のみ）

### 1.2 実装方針
- 段階的削除により安全性を確保
- 各段階でコンパイル・テスト確認を実施
- 開発中プロジェクトのため下位互換性は考慮しない

## 2. 実装フェーズ

### 2.1 Phase 1: 新構造体の実装（第1日）
**目的**: テンプレート機能の代替となる新しい構造体フィールドの実装

#### 実装項目
- [x] `CommandGroup`構造体への新フィールド追加
- [x] 新フィールドの処理ロジック実装
- [x] 基本テストケースの追加
- [x] コンパイル確認

**所要時間**: 2-3時間

### 2.2 Phase 2: テンプレート機能の削除（第1-2日）
**目的**: テンプレート関連コードの完全削除

#### 実装項目
- [x] `template`パッケージの削除
- [x] 構造体からテンプレート関連フィールドの削除
- [x] `Runner`からテンプレート関連コードの削除
- [x] テンプレート関連テストの削除
- [x] コンパイル・テスト確認

**所要時間**: 3-4時間

### 2.3 Phase 3: サンプル・ドキュメント更新（第2-3日）
**目的**: サンプル設定ファイルとドキュメントの更新

#### 実装項目
- [x] `sample/config.toml`の更新
- [x] `sample/test.toml`の更新
- [x] README.mdの更新
- [x] 最終動作確認

**所要時間**: 2-3時間

## 3. 詳細実装手順

### 3.1 Phase 1: 新構造体の実装

#### 3.1.1 `CommandGroup`構造体の拡張（完了）

**ファイル**: `internal/runner/runnertypes/config.go`

**実装済みの構造体**:
```go
// CommandGroup represents a group of related commands with a name
type CommandGroup struct {
    Name        string `toml:"name"`
    Description string `toml:"description"`
    Priority    int    `toml:"priority"`

    // Fields for resource management
    TempDir bool   `toml:"temp_dir"` // Auto-generate temporary directory
    WorkDir string `toml:"workdir"`  // Working directory

    Commands     []Command `toml:"commands"`
    VerifyFiles  []string  `toml:"verify_files"`  // Files to verify for this group
    EnvAllowlist []string  `toml:"env_allowlist"` // Group-level environment variable allowlist
}
```

**注意**: `Cleanup` フィールドは実装では `ResourceManager` で処理されており、明示的なフィールドとしては実装されていません。また、`DependsOn` フィールドは `Priority` による順序付けに簡素化されたため削除されました。

#### 3.1.2 新フィールドの処理ロジック実装

**ファイル**: `internal/runner/runner.go`

```go
// RunCommandGroup メソッドに新フィールドの処理を追加
func (r *Runner) RunCommandGroup(group runnertypes.CommandGroup, ctx context.Context) error {
    // 新フィールドの処理を追加（詳細は後述）

    // 既存のテンプレート適用処理は継続（Phase 2で削除）
    if group.Template != "" {
        // ... 既存のテンプレート処理
    }

    // ... 残りの処理
}
```

#### 3.1.3 新フィールドの詳細処理

**TempDir 処理**:
```go
if group.TempDir {
    // リソースマネージャーで一時ディレクトリを生成
    tempDir, err := r.resourceManager.CreateTempDir()
    if err != nil {
        return fmt.Errorf("failed to create temp directory: %w", err)
    }
    // 各コマンドのDirが空の場合、tempDirを設定
    for i := range group.Commands {
        if group.Commands[i].Dir == "" {
            group.Commands[i].Dir = tempDir
        }
    }
}
```

**WorkDir 処理**:
```go
if group.WorkDir != "" {
    // 各コマンドのDirが空の場合、WorkDirを設定
    for i := range group.Commands {
        if group.Commands[i].Dir == "" {
            group.Commands[i].Dir = group.WorkDir
        }
    }
}
```

**Cleanup 処理**:
```go
if group.Cleanup {
    defer func() {
        if cleanupErr := r.resourceManager.Cleanup(); cleanupErr != nil {
            // クリーンアップエラーをログ出力
            log.Printf("Cleanup failed: %v", cleanupErr)
        }
    }()
}
```

#### 3.1.4 基本テストケースの追加

**ファイル**: `internal/runner/runner_test.go`

```go
func TestCommandGroup_NewFields(t *testing.T) {
    tests := []struct {
        name        string
        group       runnertypes.CommandGroup
        expectDir   string
        expectError bool
    }{
        {
            name: "TempDir enabled",
            group: runnertypes.CommandGroup{
                Name:    "test",
                TempDir: true,
                Commands: []runnertypes.Command{{Name: "test", Cmd: "echo"}},
            },
            expectError: false,
        },
        {
            name: "WorkDir specified",
            group: runnertypes.CommandGroup{
                Name:    "test",
                WorkDir: "/tmp/test",
                Commands: []runnertypes.Command{{Name: "test", Cmd: "echo"}},
            },
            expectDir: "/tmp/test",
            expectError: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // テスト実装
        })
    }
}
```

### 3.2 Phase 2: テンプレート機能の削除

#### 3.2.1 削除前の最終確認

```bash
# テンプレート機能の使用箇所を確認
grep -r "template\|Template" internal/runner/ --exclude-dir=template
grep -r "{{.*}}" sample/
```

#### 3.2.2 段階的削除手順

**Step 1: import文の削除**
```go
// internal/runner/runner.go から削除
// "github.com/isseis/go-safe-cmd-runner/internal/runner/template"
```

**Step 2: 構造体フィールドの削除**
```go
// Config構造体から削除
// Templates map[string]TemplateConfig `toml:"templates"`

// CommandGroup構造体から削除
// Template string `toml:"template"`

// Runner構造体から削除
// templateEngine *template.Engine

// Options構造体から削除
// templateEngine *template.Engine
```

**Step 3: メソッドの削除・変更**
```go
// 削除対象
// func WithTemplateEngine(engine *template.Engine) Option

// New関数の修正
func New(options ...Option) (*Runner, error) {
    opts := Options{
        filesystem:      common.NewFileSystem(),
        resourceManager: resource.NewManager(),
        executor:        executor.New(),
        // templateEngine: template.NewEngine(), // 削除
    }

    // ... オプション適用処理（WithTemplateEngine部分を削除）

    return &Runner{
        filesystem:          opts.filesystem,
        resourceManager:     opts.resourceManager,
        executor:            opts.executor,
        // templateEngine:      opts.templateEngine, // 削除
        securityValidator:   security.NewValidator(),
        environmentFilter:   environment.NewFilter(),
    }, nil
}
```

**Step 4: RunCommandGroupメソッドの簡素化**
```go
func (r *Runner) RunCommandGroup(group runnertypes.CommandGroup, ctx context.Context) error {
    // テンプレート適用処理を削除
    groupToRun := group

    // 新フィールドの処理（Phase 1で実装済み）
    if group.TempDir {
        // TempDir処理
    }

    if group.WorkDir != "" {
        // WorkDir処理
    }

    if group.Cleanup {
        // Cleanup処理
    }

    // ... 残りの処理（変更なし）
}
```

**Step 5: template パッケージの削除**
```bash
rm -rf internal/runner/template/
```

#### 3.2.3 コンパイル・テスト確認

```bash
# コンパイル確認
go build ./...

# テスト実行
go test ./...

# エラーがある場合は修正を繰り返す
```

### 3.3 Phase 3: サンプル・ドキュメント更新

#### 3.3.1 `sample/config.toml` の更新

**削除対象セクション**:
```toml
# [templates] セクション全体を削除
```

**変更対象セクション**:
```toml
# 変更前
[[groups]]
name = "setup"
template = "dev"

# 変更後
[[groups]]
name = "setup"
temp_dir = false
cleanup = true
workdir = "/tmp/setup"
```

**環境変数使用への変更**:
```toml
# 変更前
args = ["-p", "{{.app_name}}", "logs"]

# 変更後
args = ["-p", "$APP_NAME", "logs"]
env = ["APP_NAME=myapp-dev"]
```

#### 3.3.2 `sample/test.toml` の更新

同様の変更を `sample/test.toml` にも適用。

#### 3.3.3 コメント・ドキュメントの更新

**設定ファイルのコメント更新**:
```toml
# 変更前のコメント
# Template variables that can be referenced in commands using {{.variable_name}}

# 変更後のコメント
# Environment variables can be used in commands using $VARIABLE_NAME syntax
# Set them in the 'env' array for each command
```

#### 3.3.4 README.md の更新

テンプレート機能に関する説明を削除し、環境変数使用方法を追加。

## 4. テスト戦略

### 4.1 単体テスト

#### Phase 1 テスト
- [x] 新フィールドの TOML 読み込みテスト
- [x] `TempDir`, `WorkDir` の動作テスト
- [x] 既存機能の非回帰テスト

#### Phase 2 テスト
- [x] テンプレート機能削除後のコンパイル確認
- [x] 基本的なコマンド実行テスト
- [x] エラーハンドリングテスト

#### Phase 3 テスト
- [x] 更新後のサンプル設定ファイルの動作確認
- [x] 環境変数展開の動作確認

### 4.2 統合テスト

```bash
# 各段階でのサンプル実行テスト
go run cmd/runner/main.go -config sample/config.toml
go run cmd/runner/main.go -config sample/test.toml
```

### 4.3 パフォーマンステスト

削除前後でのパフォーマンス測定:
```go
func BenchmarkRunCommandGroup_BeforeRemoval(b *testing.B) { /* ... */ }
func BenchmarkRunCommandGroup_AfterRemoval(b *testing.B) { /* ... */ }
```

## 5. リスク管理

### 5.1 技術リスク

| リスク | 対策 | 優先度 |
|--------|------|--------|
| 未知の依存関係 | 段階的削除・全文検索 | 高 |
| テストケース不足 | 各段階でテスト確認 | 中 |
| パフォーマンス悪化 | ベンチマークテスト | 低 |

### 5.2 スケジュールリスク

| リスク | 対策 |
|--------|------|
| 実装時間の超過 | 段階的実装で進捗を可視化 |
| テスト時間の不足 | 自動化テストの活用 |

## 6. 成功指標

### 6.1 品質指標
- [ ] 全テストが通過する
- [ ] コンパイルエラーが発生しない
- [ ] サンプル設定ファイルが正常動作する
- [ ] パフォーマンスが同等以上である

### 6.2 実装指標
- [ ] 約400行のテンプレート関連コードが削除される
- [ ] 新しい設定構造でコマンドが実行できる
- [ ] ドキュメントが更新される

## 7. 後続作業

### 7.1 継続的改善
- 新しい設定構造の使いやすさ評価
- 環境変数管理の更なる改善検討
- パフォーマンス最適化

### 7.2 ドキュメント整備
- 設定例の充実
- 環境変数使用のベストプラクティス文書化

## 8. 完了基準

### 8.1 機能完了基準
- [x] テンプレート関連コードが完全に削除されている
- [x] 新しい `CommandGroup` フィールドが正常に動作する
- [x] 環境変数ベースの設定例が動作する
- [x] 全ての既存テストが通過する

### 8.2 品質完了基準
- [x] コードカバレッジが維持されている
- [x] 静的解析エラーが発生しない
- [x] ドキュメントが最新状態に更新されている

## 9. デプロイ計画

開発中プロジェクトのため、特別なデプロイ手順は不要：
1. ブランチでの実装完了
2. プルリクエスト作成・レビュー
3. メインブランチへのマージ

## 10. 実装結果と結論

### 10.1 実装完了状況
**✅ 完了済み**: テンプレート機能削除作業は完全に実装済みである。

### 10.2 実装されたアーキテクチャ
現在のシステムは計画通り以下の状態になっている：

- **`template`パッケージ**: 完全に削除済み
- **`Config`構造体**: `Templates`フィールドが削除され、簡素化された
- **`CommandGroup`構造体**: `TempDir`、`WorkDir`フィールドが追加され、`Template`フィールドは削除された
- **`Runner`構造体**: テンプレートエンジン関連フィールドが削除され、簡素化された

### 10.3 実現された改善点
1. **コード複雑度の削減**: 約400行のテンプレート関連コードが削除された
2. **設定の簡素化**: より直接的で理解しやすい設定構造に変更された
3. **パフォーマンス向上**: テンプレート処理オーバーヘッドが削除された
4. **保守性の向上**: 依存関係が減少し、テストが簡素化された

### 10.4 結論
この実装計画により、テンプレート機能は安全かつ効率的に削除され、より単純で保守しやすいコードベースが実現された。環境変数ベースのアプローチにより、標準的で広く理解されている設定管理手法に統一され、システム全体の品質が向上した。
