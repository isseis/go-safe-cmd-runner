# アーキテクチャー設計書: リアリスティックなDry-Run機能

## 1. 概要

### 1.1 設計目標
- 既存のRunner実行フローを最大限活用し、実際の実行パスに近いdry-run機能を実現
- 副作用なしで詳細な実行計画と検証結果を提供
- 既存コードベースへの影響を最小限に抑制

### 1.2 設計原則
- **単一責任の原則**: dry-run専用のコンポーネントは分析・検証・出力のみを担当
- **開放閉鎖の原則**: 既存のRunnerインターフェースを拡張し、新機能を追加
- **依存性逆転の原則**: 抽象化されたインターフェースを通じて機能を実現

## 2. アーキテクチャー概要

### 2.1 全体構成

```mermaid
graph TD
    A[CLI Entry Point<br/>cmd/runner/main.go] --> B[Mode Detection<br/>--dry-run flag]

    B --> C{Dry-run Mode?}
    C -->|Yes| D[WithDryRun Option<br/>DryRunResourceManager]
    C -->|No| E[Default Option<br/>DefaultResourceManager]

    D --> F[Runner Initialization<br/>with appropriate ResourceManager]
    E --> F

    F --> G[ExecuteAll<br/>unified method for both modes]

    G --> H[ResourceManager<br/>internal/runner/resource/manager.go]

    H --> I[Command Execution]
    H --> J[Filesystem Operations]
    H --> K[Privilege Management]
    H --> L[Network Operations]

    H --> M[Analysis Recording<br/>dry-run only]
    H --> N[Result Formatting<br/>dry-run only]

    style A fill:#e1f5fe
    style B fill:#f3e5f5
    style C fill:#fff9c4
    style D fill:#fce4ec
    style E fill:#e8f5e8
    style F fill:#f3e5f5
    style G fill:#e8f5e8
    style H fill:#fff3e0
    style M fill:#fce4ec
    style N fill:#fce4ec
```

### 2.2 コンポーネント間の関係

```mermaid
graph LR
    subgraph "Core Components"
        A[Runner]
        B[ResourceManager]
        C[VerificationManager]
        D[PrivilegeManager]
        E[CommandExecutor]
    end

    subgraph "Resource Management"
        F[Command Execution]
        G[Filesystem Operations]
        H[Privilege Management]
        I[Network Operations]
    end

    subgraph "Dry-Run Analysis"
        J[Analysis Recording]
        K[Security Analysis]
        L[Result Aggregation]
    end

    A --> B
    B --> F
    B --> G
    B --> H
    B --> I

    B --> J
    B --> K
    B --> L

    C --> B
    D --> B
    E --> B

    style A fill:#e3f2fd
    style B fill:#fff3e0
    style J fill:#fce4ec
    style K fill:#fce4ec
    style L fill:#fce4ec
```

**コンポーネントの役割:**
- **Runner**: 既存機能を保持し、初期化時に適切なResourceManagerを設定
- **ResourceManager**: すべての副作用を統一的に管理（モードに応じた実装を最初から選択）
- **Analysis Recording**: dry-runモードでの詳細な分析情報記録

## 3. コンポーネント設計

### 3.1 Resource Manager Pattern

#### 3.1.1 統一された実行フロー
Resource Manager Patternにより、通常実行とdry-runの両方で同一の`ExecuteAll`メソッドを使用し、100%の実行パス整合性を保証します。

```go
type Runner struct {
    // existing fields...
    resourceManager ResourceManager  // 新規追加
}

// NewRunner creates a new command runner with appropriate ResourceManager
func NewRunner(config *runnertypes.Config, options ...Option) (*Runner, error) {
    // ... existing initialization code ...

    // Check if dry-run mode is requested
    if opts.dryRun {
        // Create DryRunResourceManager with specified options
        opts.resourceManager = resource.NewDryRunResourceManager(
            opts.executor,
            fs,
            opts.privilegeManager,
            opts.dryRunOptions,
        )
    } else {
        // Create DefaultResourceManager for normal execution
        opts.resourceManager = resource.NewDefaultResourceManager(
            opts.executor,
            fs,
            opts.privilegeManager,
            resource.ExecutionModeNormal,
            &resource.DryRunOptions{},
        )
    }

    // ... rest of initialization ...
}

// GetDryRunResults returns dry-run analysis results if available
func (r *Runner) GetDryRunResults() *resource.DryRunResult {
    return r.resourceManager.GetDryRunResults()
}
```

#### 3.1.2 ResourceManager インターフェース
すべての副作用を統一的に管理するインターフェース（Phase 2で実装完了）：

```go
// ResourceManager manages all side-effects (commands, filesystem, privileges, etc.)
type ResourceManager interface {
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
    SendNotification(message string, details map[string]any) error

    // Dry-run results (returns nil for normal execution mode)
    GetDryRunResults() *DryRunResult
}

// DryRunResourceManager extends ResourceManager with dry-run specific functionality
type DryRunResourceManager interface {
    ResourceManager

    // Dry-run specific
    RecordAnalysis(analysis *ResourceAnalysis)
}
```

#### 3.1.3 DefaultResourceManager 委譲パターン (Phase 2実装完了)
実際の実装では、委譲パターンによるファサード型ResourceManagerを採用しています：

```go
// DefaultResourceManager provides a mode-aware facade that delegates to
// NormalResourceManager or DryRunResourceManagerImpl depending on ExecutionMode.
type DefaultResourceManager struct {
    mode   ExecutionMode
    normal *NormalResourceManager
    dryrun *DryRunResourceManagerImpl
}

// activeManager returns the manager corresponding to the current execution mode.
func (d *DefaultResourceManager) activeManager() ResourceManager {
    if d.mode == ExecutionModeDryRun {
        return d.dryrun
    }
    return d.normal
}

// GetMode returns the current execution mode.
func (d *DefaultResourceManager) GetMode() ExecutionMode { return d.mode }

// 各操作は現在のモードに応じて適切なマネージャに委譲
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
    return d.activeManager().ExecuteCommand(ctx, cmd, group, env)
}

// Dry-run結果は dry-runモード時のみ取得可能（通常時は nil）
func (d *DefaultResourceManager) GetDryRunResults() *DryRunResult {
    if d.mode == ExecutionModeDryRun {
        return d.dryrun.GetDryRunResults()
    }
    return nil
}
```

このパターンにより：
- **実行パス整合性**: 両モードで同一のインターフェースとフローを使用
- **初期化時の最適化**: 開始時にモードが決定され、適切なマネージャーを選択
- **状態管理**: dry-run分析結果を適切に管理

### 3.2 実装済み型システム

Resource Manager Patternにより、すべての型定義は`internal/runner/resource`パッケージに統合されています：

#### 3.2.1 ResourceAnalysis システム
```go
// 各副作用操作の詳細な分析情報
type ResourceAnalysis struct {
    Type        ResourceType           `json:"type"`        // command, filesystem, privilege, network, process
    Operation   ResourceOperation      `json:"operation"`   // create, delete, execute, escalate, send
    Target      string                 `json:"target"`      // 操作対象（コマンド、パス等）
    Parameters  map[string]interface{} `json:"parameters"`  // 操作パラメータ
    Impact      ResourceImpact         `json:"impact"`      // 操作の影響
    Timestamp   time.Time              `json:"timestamp"`   // 操作時刻
}
```

#### 3.2.2 DryRunResult システム
```go
// dry-run分析の完全な結果
type DryRunResult struct {
    Metadata         *ResultMetadata     `json:"metadata"`
    ExecutionPlan    *ExecutionPlan      `json:"execution_plan"`
    ResourceAnalyses []ResourceAnalysis  `json:"resource_analyses"`
    SecurityAnalysis *SecurityAnalysis   `json:"security_analysis"`
    EnvironmentInfo  *EnvironmentInfo    `json:"environment_info"`
    Errors          []DryRunError       `json:"errors"`
    Warnings        []DryRunWarning     `json:"warnings"`
}
```

#### 3.2.3 セキュリティ分析システム
```go
type SecurityAnalysis struct {
    Risks              []SecurityRisk      `json:"risks"`
    PrivilegeChanges   []PrivilegeChange   `json:"privilege_changes"`
    EnvironmentAccess  []EnvironmentAccess `json:"environment_access"`
    FileAccess         []FileAccess        `json:"file_access"`
}

type SecurityRisk struct {
    Level       RiskLevel `json:"level"`        // Low, Medium, High, Critical
    Type        RiskType  `json:"type"`         // PrivilegeEscalation, DataExposure, etc.
    Description string    `json:"description"`
    Command     string    `json:"command"`
    Group       string    `json:"group"`
    Mitigation  string    `json:"mitigation"`
}

type PrivilegeChange struct {
    Group       string `json:"group"`
    Command     string `json:"command"`
    FromUser    string `json:"from_user"`
    ToUser      string `json:"to_user"`
    Mechanism   string `json:"mechanism"`  // sudo, setuid, etc.
}
```

#### 3.3.3 検証結果
```go
type VerificationResults struct {
    ConfigVerification    *FileVerification    `json:"config_verification"`
    EnvironmentVerification *FileVerification  `json:"environment_verification"`
    GlobalVerification    *verification.Result `json:"global_verification"`
    Errors               []error               `json:"errors"`
}

type FileVerification struct {
    FilePath   string        `json:"file_path"`
    Verified   bool          `json:"verified"`
    HashValue  string        `json:"hash_value"`
    Algorithm  string        `json:"algorithm"`
    Duration   int64         `json:"duration_ms"` // Duration in milliseconds
    Error      error         `json:"error,omitempty"`
}
```

#### 3.3.4 実行結果
```go
// ExecutionResult unified result for both normal and dry-run
type ExecutionResult struct {
    ExitCode int               `json:"exit_code"`
    Stdout   string            `json:"stdout"`
    Stderr   string            `json:"stderr"`
    Duration int64             `json:"duration_ms"` // Duration in milliseconds
    DryRun   bool              `json:"dry_run"`
    Analysis *ResourceAnalysis `json:"analysis,omitempty"`
}

// DryRunResult represents the complete result of a dry-run analysis
type DryRunResult struct {
    Metadata         *ResultMetadata    `json:"metadata"`
    ExecutionPlan    *ExecutionPlan     `json:"execution_plan"`
    ResourceAnalyses []ResourceAnalysis `json:"resource_analyses"`
    SecurityAnalysis *SecurityAnalysis  `json:"security_analysis"`
    EnvironmentInfo  *EnvironmentInfo   `json:"environment_info"`
    Errors           []DryRunError      `json:"errors"`
    Warnings         []DryRunWarning    `json:"warnings"`
}

// ResultMetadata contains metadata about the dry-run result
type ResultMetadata struct {
    GeneratedAt     time.Time     `json:"generated_at"`
    RunID           string        `json:"run_id"`
    ConfigPath      string        `json:"config_path"`
    EnvironmentFile string        `json:"environment_file"`
    Version         string        `json:"version"`
    Duration        time.Duration `json:"duration"`
}

// ExecutionStatus represents the status of command execution
type ExecutionStatus string

const (
    ExecutionStatusPending   ExecutionStatus = "pending"
    ExecutionStatusRunning   ExecutionStatus = "running"
    ExecutionStatusCompleted ExecutionStatus = "completed"
    ExecutionStatusFailed    ExecutionStatus = "failed"
    ExecutionStatusCancelled ExecutionStatus = "cancelled"
    ExecutionStatusTimeout   ExecutionStatus = "timeout"
)
```

### 3.4 DryRun Formatter

#### 3.4.1 フォーマッター構成
```go
// Formatter formats dry-run results for output
type Formatter interface {
    FormatResult(result *DryRunResult, opts FormatterOptions) (string, error)
    FormatSummary(result *DryRunResult) (string, error)
    FormatDetailed(result *DryRunResult) (string, error)
    FormatErrors(errors []error) (string, error)
}

type FormatterOptions struct {
    Format        OutputFormat  // Text, JSON, YAML
    DetailLevel   DetailLevel   // Summary, Detailed, Full
    ShowSensitive bool         // Show sensitive information (masked)
    ColorOutput   bool         // Use colored output for terminals
}
```

## 4. 実行フロー設計

### 4.1 統一実行フロー（Resource Manager Pattern）

```mermaid
flowchart TD
    A[CLI Flag Detection<br/>--dry-run or normal] --> B{Dry-run Mode?}
    B -->|Yes| C[WithDryRun Option<br/>DryRunResourceManager]
    B -->|No| D[Default Option<br/>DefaultResourceManager]

    C --> E[Runner Initialization<br/>with appropriate ResourceManager]
    D --> E

    E --> F[ExecuteAll Method<br/>unified execution path]
    F --> G[Resource Operations<br/>through ResourceManager]

    G --> H{ResourceManager Type?}
    H -->|NormalResourceManager| I[Actual Side Effects<br/>Command, File, Privilege, Network]
    H -->|DryRunResourceManager| J[Simulated Operations<br/>and Analysis Recording]

    I --> K[Normal Result]
    J --> L[Dry-Run Analysis Result<br/>GetDryRunResults]

    L --> M[Result Formatting<br/>and Output]

    style A fill:#ffecb3
    style B fill:#fff9c4
    style C fill:#fce4ec
    style D fill:#e8f5e8
    style E fill:#c8e6c9
    style F fill:#e8f5e8
    style G fill:#fff3e0
    style H fill:#fff9c4
    style I fill:#e8f5e8
    style J fill:#fce4ec
    style L fill:#fce4ec
    style M fill:#f8bbd9
```

### 4.2 詳細実行パス

#### 4.2.1 実行パス統一の仕組み
Resource Manager Patternにより、通常実行とdry-runは完全に同じコードパスを通ります：

1. **初期化フェーズ**：
   - CLI flags解析でdry-runモード判定
   - モードに応じた適切なResourceManager作成
   - Runner初期化（既存パス）

2. **実行フェーズ**：
   - `ExecuteAll` → `ExecuteGroup` → `executeCommandInGroup`（既存パス）
   - 全副作用がResourceManagerを経由
   - ResourceManagerの実装に応じて実際実行 or シミュレーション

3. **結果フェーズ**：
   - Normal: 既存の実行結果
   - Dry-Run: ResourceManagerが蓄積した分析結果を`GetDryRunResults()`で取得

#### 4.2.2 副作用インターセプション
各副作用操作でResourceManagerが自動的に処理を分岐：

- **コマンド実行**: `ExecuteCommand()` → 実行 or 分析記録
- **ファイルシステム**: `CreateTempDir()` → 作成 or シミュレーション
- **特権管理**: `WithPrivileges()` → 昇格 or 分析記録
- **ネットワーク**: `SendNotification()` → 送信 or 分析記録

## 5. 既存コードとの統合

### 5.1 最小限の変更による統合
Resource Manager Patternにより、既存コードへの変更を最小限に抑えます：

```go
// Runner構造体への追加（最小限）
type Runner struct {
    // 既存フィールド（変更なし）
    config              *runnertypes.Config
    envVars             map[string]string
    validator           *security.Validator
    verificationManager *verification.Manager
    envFilter           *environment.Filter
    runID               string

    // 新規追加（1フィールドのみ）
    resourceManager     ResourceManager
}

// 既存メソッドは変更なし（内部でresourceManagerを使用するよう更新）
func (r *Runner) ExecuteAll(ctx context.Context) error {
    // 既存のロジック、ただし副作用はresourceManager経由
}

// 新規メソッド追加（シンプル）
func (r *Runner) GetDryRunResults() *resource.DryRunResult {
    return r.resourceManager.GetDryRunResults()  // dry-runモードでのみ結果返却
}

// WithDryRun option function for initialization
func WithDryRun(dryRunOptions *resource.DryRunOptions) Option {
    return func(opts *runnerOptions) {
        opts.dryRun = true
        opts.dryRunOptions = dryRunOptions
    }
}
```

### 5.2 既存インターフェース活用
ResourceManagerが既存コンポーネントを内部で活用：

- **CommandExecutor**: 通常実行時はそのまま使用
- **PrivilegeManager**: 通常実行時はそのまま使用
- **VerificationManager**: 両モードで共通使用
- **TempDirManager**: 通常実行時はそのまま使用

### 5.3 完全な後方互換性
- 既存のTOML設定ファイル形式：完全互換
- 既存のCLIインターフェース：変更なし
- 既存のAPI：変更なし

## 6. 設計の利点

### 6.1 Resource Manager Pattern の利点
- **完全な実行パス整合性**: 通常実行とdry-runで100%同じコードパス
- **包括的副作用管理**: すべての副作用を統一的にインターセプション
- **最小限のコード変更**: 既存機能への影響を最小化
- **拡張可能性**: 新しい副作用タイプの追加が容易
- **初期化時最適化**: モードが決定された段階で適切なマネージャーを選択

### 6.2 テスト戦略
- **統一テストケース**: 通常実行とdry-runで同じテストを使用可能
- **初期化テスト**: WithDryRunオプションでdry-runモードの初期化テスト
- **副作用分離**: dry-runでは副作用なしの完全テスト実行

### 6.3 運用上の利点
- **一貫性保証**: 実行パスの乖離を構造的に防止
- **包括的分析**: 実行前にすべての副作用を事前分析
- **安全な検証**: 副作用なしの完全な事前検証

## 7. 実装状況

### 7.1 Phase 1 完了済み（Foundation）
- ✅ **ResourceManager インターフェース**: 完全実装済み
- ✅ **ExecutionMode と関連型**: 完全実装済み
- ✅ **ResourceAnalysis データ構造**: 完全実装済み
- ✅ **DryRunResult 型システム**: 完全実装済み
- ✅ **基本テストフレームワーク**: 完全実装済み
- ✅ **Lint 対応**: 完全対応済み

### 7.2 実装パッケージ構成
```
internal/runner/resource/
├── manager.go         # ResourceManager インターフェース定義
├── types.go          # 全型定義（DryRunResult, ResourceAnalysis等）
├── manager_test.go   # ResourceManager テスト
└── types_test.go     # 型システム テスト
```

### 7.3 次期実装（Phase 2以降）
- **DefaultResourceManager 実装**
- **Runner 統合**
- **CLI インターフェース追加**
- **フォーマッター実装**

## 8. 実行パス整合性保証

### 8.1 Resource Manager Pattern による保証
Resource Manager Patternにより、以下の方法で実行パス整合性を構造的に保証：

1. **同一実行パス**: 両モードで `ExecuteAll()` を使用し、通常実行と完全に同じパスを使用
2. **副作用インターセプション**: ResourceManager が全副作用を統一的に処理（実行 or シミュレーション）
3. **モード透過性**: 実行ロジックはモードを意識せず、ResourceManager の実装が自動的に処理を分岐

### 8.2 実装上の保証メカニズム
- **統一インターフェース**: ResourceManager による全副作用の抽象化
- **実装の共有**: 通常実行とdry-runで同じコード実行
- **自動テスト**: 統一テストケースによる継続的な整合性検証

---

**Resource Manager Pattern採用により、従来のPerformDryRunメソッドによる一時的な差し替えは不要となり、初期化時の適切なResourceManager選択によるより簡潔で効率的なアーキテクチャを実現しました。**
