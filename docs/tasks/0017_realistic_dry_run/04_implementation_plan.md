# 実装計画書: リアリスティックなDry-Run機能

## 1. 実装概要

### 1.1 目標
通常実行パスと完全に同じフローを辿りながら、すべての副作用（コマンド実行、ファイルシステム操作、特権昇格、ネットワーク通信）をインターセプトし、詳細な分析結果を提供するdry-run機能を実装する。

### 1.2 実装アプローチ
**Resource Manager Pattern** を採用し、すべての副作用を `ResourceManager` インターフェース経由で実行。モードに応じて実際の処理またはシミュレーションを選択する。

### 1.3 実装スコープ
- ResourceManager インターフェースとDefaultResourceManager実装
- Runner構造体のResourceManager統合
- dry-run結果フォーマッター
- 包括的テストスイート
- ドキュメント整備

## 2. 段階的実装計画

### Phase 1: Foundation（基盤構築）
**期間**: 2-3日
**目標**: ResourceManagerインターフェースの基盤を構築

#### 2.1.1 作業項目
- [ ] ResourceManager インターフェース定義
- [ ] ExecutionMode と関連型の定義
- [ ] ResourceAnalysis データ構造の実装
- [ ] 基本的なテストフレームワーク構築

#### 2.1.2 成果物
```
internal/runner/resource/
├── manager.go         # ResourceManager インターフェース
├── types.go          # ExecutionMode, ResourceAnalysis等
└── manager_test.go   # 基本テスト

internal/runner/dryrun/
├── types.go         # DryRunResult等
└── types_test.go    # 型テスト
```

#### 2.1.3 実装詳細

**ResourceManager インターフェース**
```go
// internal/runner/resource/manager.go
package resource

type ExecutionMode int

const (
    ExecutionModeNormal ExecutionMode = iota
    ExecutionModeDryRun
)

type ResourceManager interface {
    // Mode management
    SetMode(mode ExecutionMode, opts *DryRunOptions)
    GetMode() ExecutionMode

    // Command execution
    ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error)

    // Filesystem operations
    CreateTempDir(groupName string) (string, error)
    CleanupTempDir(tempDirPath string) error
    CleanupAllTempDirs() error

    // Privilege management
    WithPrivileges(ctx context.Context, fn func() error) error
    IsPrivilegeEscalationRequired(cmd runnertypes.Command) (bool, error)

    // Network operations
    SendNotification(message string, details map[string]interface{}) error

    // Dry-run specific
    GetDryRunResults() *DryRunResult
    RecordAnalysis(analysis *ResourceAnalysis)
}
```

#### 2.1.4 検証基準
- [ ] インターフェースがコンパイル可能
- [ ] 基本的な型定義のテストが通過
- [ ] モックResourceManagerの作成と動作確認

---

### Phase 2: Core Implementation（コア実装）
**期間**: 4-5日
**目標**: DefaultResourceManagerの完全実装

#### 2.2.1 作業項目
- [ ] DefaultResourceManager の実装
- [ ] コマンド実行のインターセプション
- [ ] ファイルシステム操作のインターセプション
- [ ] 特権管理のインターセプション
- [ ] ネットワーク操作のインターセプション
- [ ] リソース分析ロジックの実装

#### 2.2.2 成果物
```
internal/runner/resource/
├── manager.go
├── default_manager.go    # DefaultResourceManager実装
├── analysis.go          # 分析ロジック
├── types.go
├── default_manager_test.go
└── analysis_test.go

internal/runner/dryrun/
├── types.go
├── formatter.go         # 結果フォーマッター
└── formatter_test.go
```

#### 2.2.3 実装詳細

**DefaultResourceManager のコマンド実行**
```go
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
    switch d.mode {
    case ExecutionModeNormal:
        // 通常実行：既存executorを使用
        result, err := d.executor.Execute(ctx, cmd, env)
        if err != nil {
            return nil, err
        }
        return &ExecutionResult{
            ExitCode: result.ExitCode,
            Stdout:   result.Stdout,
            Stderr:   result.Stderr,
            Duration: time.Since(start),
            DryRun:   false,
        }, nil

    case ExecutionModeDryRun:
        // dry-run: 分析のみ
        start := time.Now()
        analysis := d.analyzeCommand(ctx, cmd, group, env)

        d.recordAnalysis(&analysis)

        return &ExecutionResult{
            ExitCode: 0,
            Stdout:   fmt.Sprintf("[DRY-RUN] Would execute: %s", cmd.Command),
            Stderr:   "",
            Duration: time.Since(start),
            DryRun:   true,
            Analysis: &analysis,
        }, nil
    }
}
```

#### 2.2.4 検証基準
- [ ] 通常実行モードでの完全な動作
- [ ] dry-runモードでの適切なシミュレーション
- [ ] リソース分析の正確性
- [ ] すべての副作用タイプの適切なインターセプション

---

### Phase 3: Runner Integration（Runner統合）
**期間**: 3-4日
**目標**: 既存RunnerへのResourceManager統合

#### 2.3.1 作業項目
- [ ] Runner構造体のResourceManager フィールド追加
- [ ] `NewRunner` 関数の更新
- [ ] `executeCommandInGroup` のResourceManager使用への変更
- [ ] `ExecuteGroup` の一時ディレクトリ処理更新
- [ ] 特権管理処理の更新
- [ ] 通知機能の更新
- [ ] `PerformDryRun` メソッドの実装

#### 2.3.2 成果物
```
internal/runner/
├── runner.go            # ResourceManager統合済み
├── runner_test.go       # 更新されたテスト
└── options.go          # WithResourceManager オプション追加
```

#### 2.3.3 実装詳細

**Runner構造体の変更**
```go
type Runner struct {
    config              *runnertypes.Config
    envVars             map[string]string
    validator           *security.Validator
    verificationManager *verification.Manager
    envFilter           *environment.Filter
    runID               string

    // ★新規追加：すべての副作用を管理
    resourceManager     resource.ResourceManager
}
```

**executeCommandInGroup の変更**
```go
func (r *Runner) executeCommandInGroup(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup) (*executor.Result, error) {
    // 環境変数解決（既存ロジック）
    resolvedCmd, env, err := r.prepareCommandExecution(cmd, group)
    if err != nil {
        return nil, err
    }

    // ★変更：resourceManagerを使用
    result, err := r.resourceManager.ExecuteCommand(ctx, resolvedCmd, group, env)
    if err != nil {
        return nil, err
    }

    // 既存形式に変換
    return &executor.Result{
        ExitCode: result.ExitCode,
        Stdout:   result.Stdout,
        Stderr:   result.Stderr,
    }, nil
}
```

**PerformDryRun メソッドの実装**
```go
func (r *Runner) PerformDryRun(ctx context.Context, opts dryrun.DryRunOptions) (*dryrun.DryRunResult, error) {
    // ResourceManagerをdry-runモードに設定
    r.resourceManager.SetMode(resource.ExecutionModeDryRun, &opts)

    // 通常実行と同じパスを実行
    err := r.ExecuteAll(ctx)
    if err != nil {
        return nil, fmt.Errorf("dry-run analysis failed: %w", err)
    }

    // 結果を取得
    return r.resourceManager.GetDryRunResults(), nil
}
```

#### 2.3.4 検証基準
- [ ] 既存のすべてのテストが通過
- [ ] 通常実行の動作が変わらないことを確認
- [ ] dry-run機能の基本動作確認
- [ ] すべてのResourceManager操作が適切に呼び出される

---

### Phase 4: Output & Formatting（出力・フォーマット）
**期間**: 2-3日
**目標**: 包括的な出力機能の実装

#### 2.4.1 作業項目
- [ ] テキストフォーマッターの実装
- [ ] JSONフォーマッターの実装
- [ ] YAMLフォーマッターの実装
- [ ] 詳細レベル別の出力制御
- [ ] セキュリティ情報のマスキング機能
- [ ] CLI統合（main.go の更新）

#### 2.4.2 成果物
```
internal/runner/dryrun/
├── formatter.go          # インターフェース定義
├── text_formatter.go     # テキスト出力
├── json_formatter.go     # JSON出力
├── yaml_formatter.go     # YAML出力
├── formatter_test.go     # フォーマッターテスト
└── testdata/            # 期待値ファイル

cmd/runner/
└── main.go              # dry-run フラグ統合
```

#### 2.4.3 実装詳細

**テキストフォーマッター**
```go
func (f *textFormatter) FormatResult(result *DryRunResult, opts FormatterOptions) (string, error) {
    var buf strings.Builder

    // 1. ヘッダー情報
    f.writeHeader(&buf, result.Metadata)

    // 2. サマリー情報
    f.writeSummary(&buf, result)

    // 3. リソース分析結果
    if opts.DetailLevel >= DetailLevelDetailed {
        f.writeResourceAnalyses(&buf, result.ResourceAnalyses, opts)
    }

    // 4. セキュリティ分析
    if result.SecurityAnalysis != nil {
        f.writeSecurityAnalysis(&buf, result.SecurityAnalysis, opts)
    }

    // 5. エラーと警告
    f.writeErrorsAndWarnings(&buf, result.Errors, result.Warnings)

    return buf.String(), nil
}
```

**main.go の更新**
```go
// 既存のdry-run処理を置き換え
if *dryRun {
    opts := dryrun.DryRunOptions{
        DetailLevel:   dryrun.DetailLevelDetailed,
        OutputFormat:  dryrun.OutputFormatText,
        ShowSensitive: false,
        VerifyFiles:   true,
    }

    result, err := runner.PerformDryRun(ctx, opts)
    if err != nil {
        return fmt.Errorf("dry-run failed: %w", err)
    }

    formatter := dryrun.NewTextFormatter()
    output, err := formatter.FormatResult(result, dryrun.FormatterOptions{
        DetailLevel: opts.DetailLevel,
        Format:      opts.OutputFormat,
    })
    if err != nil {
        return fmt.Errorf("formatting failed: %w", err)
    }

    fmt.Print(output)
    return nil
}
```

#### 2.4.4 検証基準
- [ ] 全出力形式での正常なフォーマット
- [ ] 詳細レベル別の出力確認
- [ ] 機密情報の適切なマスキング
- [ ] 大規模設定での出力パフォーマンス確認

---

### Phase 5: Comprehensive Testing（包括的テスト）
**期間**: 3-4日
**目標**: 実行パス整合性の完全なテスト体制構築

#### 2.5.1 作業項目
- [ ] 実行パス整合性テストの実装
- [ ] パフォーマンステストの実装
- [ ] エラーシナリオのテスト
- [ ] セキュリティ分析のテスト
- [ ] CI/CD パイプラインの更新
- [ ] ベンチマークテストの実装

#### 2.5.2 成果物
```
internal/runner/resource/
└── integration_test.go   # 統合テスト

internal/runner/dryrun/
├── consistency_test.go   # 実行パス整合性テスト
├── performance_test.go   # パフォーマンステスト
└── security_test.go      # セキュリティテスト

.github/workflows/
└── dry-run-consistency.yml # CI/CD パイプライン
```

#### 2.5.3 実装詳細

**実行パス整合性テスト**
```go
func TestExecutionPathConsistency(t *testing.T) {
    tests := []struct {
        name           string
        config         *runnertypes.Config
        envVars        map[string]string
        expectedDiffs  []string // 許容される差分
    }{
        {
            name: "basic command execution",
            config: testConfig,
            envVars: map[string]string{"TEST": "value"},
            expectedDiffs: []string{}, // 差分なしが期待
        },
        // ... その他のテストケース
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // 1. 通常実行の準備段階の情報収集
            normalData := captureNormalExecutionData(t, tt.config, tt.envVars)

            // 2. dry-run実行
            dryRunResult := performDryRun(t, tt.config, tt.envVars)

            // 3. 結果比較
            diffs := compareExecutionPaths(normalData, dryRunResult)
            assertAcceptableDifferences(t, diffs, tt.expectedDiffs)
        })
    }
}
```

**CI/CD パイプライン**
```yaml
name: Dry-Run Consistency Check

on: [push, pull_request]

jobs:
  consistency-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v3
        with:
          go-version: 1.23

      - name: Run Consistency Tests
        run: |
          go test -v ./internal/runner/dryrun -run TestExecutionPathConsistency

      - name: Run Performance Benchmarks
        run: |
          go test -bench=BenchmarkDryRunPerformance ./internal/runner/dryrun

      - name: Security Analysis Tests
        run: |
          go test -v ./internal/runner/dryrun -run TestSecurityAnalysis
```

#### 2.5.4 検証基準
- [ ] すべての整合性テストが通過
- [ ] パフォーマンス要件（要件書の5.1項）を満たす
- [ ] セキュリティ分析の精度確認
- [ ] CI/CDでの自動テスト実行

---

### Phase 6: Documentation & Finalization（ドキュメント・完成）
**期間**: 2日
**目標**: ドキュメント整備と最終調整

#### 2.6.1 作業項目
- [ ] README.md の更新
- [ ] API ドキュメントの作成
- [ ] 使用例の追加
- [ ] トラブルシューティングガイド
- [ ] パフォーマンスチューニングガイド

#### 2.6.2 成果物
```
docs/
├── dry-run-usage.md         # 使用方法
├── api-reference.md         # API リファレンス
├── troubleshooting.md       # トラブルシューティング
└── performance-tuning.md   # パフォーマンスチューニング

README.md                    # 更新済み
```

## 3. リスク管理

### 3.1 技術的リスク

| リスク | 影響度 | 対策 |
|--------|---------|------|
| 既存機能への影響 | 高 | 段階的統合とテスト徹底 |
| パフォーマンス劣化 | 中 | ベンチマーク監視とプロファイリング |
| 特権管理の複雑さ | 中 | 既存PrivilegeManagerの活用 |

### 3.2 スケジュールリスク

| リスク | 対策 |
|--------|------|
| Phase 2の実装複雑さ | 早期プロトタイプで検証 |
| テスト工数の増加 | 並行テスト作成 |

## 4. 品質保証

### 4.1 テスト戦略
- **単体テスト**: 各コンポーネント90%以上のカバレッジ
- **統合テスト**: 実行パス整合性の完全検証
- **パフォーマンステスト**: 要件書記載の性能基準達成
- **セキュリティテスト**: 機密情報漏洩防止の確認

### 4.2 レビュー体制
- Phase毎のコードレビュー
- アーキテクチャレビュー（Phase 2完了時）
- セキュリティレビュー（Phase 5完了時）

## 5. 完了基準

### 5.1 機能要件
- [ ] 通常実行パスとの100%整合性
- [ ] すべての副作用の適切なインターセプション
- [ ] 詳細な分析結果の提供
- [ ] 複数出力形式のサポート

### 5.2 非機能要件
- [ ] パフォーマンス要件の達成
- [ ] セキュリティ要件の満足
- [ ] 既存機能の無影響

### 5.3 品質要件
- [ ] テストカバレッジ90%以上
- [ ] すべてのCI/CDテストの通過
- [ ] ドキュメントの完備

## 6. デリバリー計画

**合計期間**: 16-19日（約3-4週間）

**マイルストーン**:
- Week 1 終了: Phase 1-2 完了
- Week 2 終了: Phase 3-4 完了
- Week 3 終了: Phase 5 完了
- Week 4 初期: Phase 6 完了、リリース準備
