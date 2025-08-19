# 詳細仕様書: リアリスティックなDry-Run機能（Resource Manager Pattern）

## 1. API仕様

### 1.1 ResourceManager インターフェース（既実装済み）

#### 1.1.1 ResourceManager メイン仕様
```go
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

#### 1.1.2 Runner拡張メソッド

**PerformDryRun メソッド**
```go
func (r *Runner) PerformDryRun(ctx context.Context, opts DryRunOptions) (*DryRunResult, error)
```

**パラメータ:**
- `ctx context.Context`: キャンセレーション用コンテキスト
- `opts DryRunOptions`: dry-run実行オプション

**戻り値:**
- `*DryRunResult`: 分析結果
- `error`: エラー情報

**動作（Resource Manager Pattern）:**
1. ResourceManager.SetMode(ExecutionModeDryRun, opts) でモード設定
2. 通常実行と完全に同じ`ExecuteAll()`パスを実行
3. ResourceManagerが全副作用を自動的にインターセプト・分析
4. GetDryRunResults()で蓄積された分析結果を取得・返却

**重要**: DryRunAnalyzerは不要。ResourceManagerによる統一的な副作用管理により実現。

### 1.2 DryRunOptions 構造体

```go
type DryRunOptions struct {
    DetailLevel    DetailLevel    `json:"detail_level"`
    OutputFormat   OutputFormat   `json:"output_format"`
    ShowSensitive  bool          `json:"show_sensitive"`
    VerifyFiles    bool          `json:"verify_files"`
    ShowTimings    bool          `json:"show_timings"`
    ShowDependencies bool        `json:"show_dependencies"`
    MaxDepth       int           `json:"max_depth"`          // 変数展開の最大深度
}

type DetailLevel int
const (
    DetailLevelSummary DetailLevel = iota
    DetailLevelDetailed
    DetailLevelFull
)

type OutputFormat int
const (
    OutputFormatText OutputFormat = iota
    OutputFormatJSON
    OutputFormatYAML
)
```

### 1.3 ResourceAnalysis 構造体（既実装済み）

```go
type ResourceAnalysis struct {
    Type        ResourceType              `json:"type"`
    Operation   ResourceOperation         `json:"operation"`
    Target      string                   `json:"target"`
    Parameters  map[string]interface{}   `json:"parameters"`
    Impact      ResourceImpact           `json:"impact"`
    Timestamp   time.Time                `json:"timestamp"`
}

type ResourceType string
const (
    ResourceTypeCommand     ResourceType = "command"
    ResourceTypeFilesystem  ResourceType = "filesystem"
    ResourceTypePrivilege   ResourceType = "privilege"
    ResourceTypeNetwork     ResourceType = "network"
    ResourceTypeProcess     ResourceType = "process"
)

type ResourceOperation string
const (
    OperationCreate   ResourceOperation = "create"
    OperationDelete   ResourceOperation = "delete"
    OperationExecute  ResourceOperation = "execute"
    OperationEscalate ResourceOperation = "escalate"
    OperationSend     ResourceOperation = "send"
)

type ResourceImpact struct {
    Reversible   bool     `json:"reversible"`
    Persistent   bool     `json:"persistent"`
    SecurityRisk string   `json:"security_risk,omitempty"`
    Description  string   `json:"description"`
}
```

### 1.4 DryRunResult 構造体（既実装済み）

```go
type DryRunResult struct {
    Metadata         *ResultMetadata      `json:"metadata"`
    ExecutionPlan    *ExecutionPlan       `json:"execution_plan"`
    ResourceAnalyses []ResourceAnalysis   `json:"resource_analyses"`
    SecurityAnalysis *SecurityAnalysis    `json:"security_analysis"`
    EnvironmentInfo  *EnvironmentInfo     `json:"environment_info"`
    Errors          []DryRunError        `json:"errors"`
    Warnings        []DryRunWarning      `json:"warnings"`
}

type ResultMetadata struct {
    GeneratedAt      time.Time     `json:"generated_at"`
    RunID           string        `json:"run_id"`
    ConfigPath      string        `json:"config_path"`
    EnvironmentFile string        `json:"environment_file"`
    Version         string        `json:"version"`
    Duration        time.Duration `json:"duration"`
}
```

### 1.4 主要データ構造の関係

```mermaid
classDiagram
    class DryRunResult {
        +ResultMetadata metadata
        +ExecutionPlan executionPlan
        +ResourceAnalyses resourceAnalyses
        +SecurityAnalysis securityAnalysis
        +EnvironmentInfo environmentInfo
        +[]DryRunError errors
        +[]DryRunWarning warnings
    }

    class ExecutionPlan {
        +[]GroupPlan groups
        +int totalCommands
        +Duration estimatedDuration
        +bool requiresPrivilege
    }

    class GroupPlan {
        +string name
        +string description
        +int priority
        +string workingDirectory
        +[]ResolvedCommand commands
        +[]string dependencies
        +map[string]string environmentVars
        +Duration estimatedDuration
    }

    class ResolvedCommand {
        +string name
        +string description
        +string commandLine
        +string originalCommand
        +string workingDir
        +Duration timeout
        +string requiredUser
        +PrivilegeInfo privilegeInfo
        +OutputSettings outputSettings
    }

    class SecurityAnalysis {
        +[]SecurityRisk risks
        +[]PrivilegeChange privilegeChanges
        +[]EnvironmentAccess environmentAccess
        +[]FileAccess fileAccess
    }

    DryRunResult --> ExecutionPlan
    DryRunResult --> SecurityAnalysis
    ExecutionPlan --> GroupPlan
    GroupPlan --> ResolvedCommand
    SecurityAnalysis --> SecurityRisk
```

## 2. コンポーネント詳細仕様

### 2.1 ResourceManager 実装（Phase 1完了済み、Phase 2以降で拡張予定）

#### 2.1.1 実装済みパッケージ構造
```
internal/runner/resource/         # ✅ Phase 1 完了
├── manager.go         # ResourceManager インターフェース定義
├── types.go          # 全型定義（DryRunResult, ResourceAnalysis等）
├── manager_test.go   # ResourceManager テスト
└── types_test.go     # 型システム テスト

# Phase 2以降で追加予定:
├── default_manager.go # DefaultResourceManager 実装
├── formatter.go      # 結果フォーマット実装
└── testdata/         # 詳細テストデータ
```

**Phase 1で完了した内容:**
- ResourceManager インターフェース完全定義
- ResourceAnalysis型システム
- DryRunResult型システム完全実装
- ExecutionMode, DetailLevel, OutputFormat等の列挙型
- 基本テストフレームワーク

#### 2.1.2 DefaultResourceManager 実装（Phase 2予定）

Resource Manager Patternの核となる実装：

**主要機能:**
1. **モード管理**: Normal/DryRunの動的切り替え
2. **副作用インターセプション**: 各副作用を適切に処理または分析
3. **分析結果蓄積**: dry-runモードでの詳細な操作記録
4. **既存コンポーネント活用**: 通常実行時は既存のExecutor等を使用

**実装方針:**
- ExecutionMode による条件分岐で実行/分析を切り替え
- dry-runモードでは全操作をResourceAnalysisとして記録
- 既存コンポーネント（CommandExecutor, TempDirManager等）への委譲

#### 2.1.3 Resource Manager Pattern 処理フロー

```mermaid
sequenceDiagram
    participant M as Main
    participant R as Runner
    participant RM as ResourceManager
    participant CE as CommandExecutor
    participant TDM as TempDirManager
    participant PM as PrivilegeManager

    M->>R: PerformDryRun(ctx, opts)
    R->>RM: SetMode(ExecutionModeDryRun, opts)
    R->>R: ExecuteAll(ctx) // 同じ実行パス

    Note over R,PM: 通常実行と完全に同じフロー、副作用のみインターセプト

    R->>RM: CreateTempDir(groupName)
    alt Normal Mode
        RM->>TDM: CreateTempDir(groupName)
    else Dry-Run Mode
        RM->>RM: SimulateTempDir() + RecordAnalysis()
    end

    R->>RM: ExecuteCommand(ctx, cmd, env)
    alt Normal Mode
        RM->>CE: Execute(cmd, env)
    else Dry-Run Mode
        RM->>RM: AnalyzeCommand() + RecordAnalysis()
    end

    R->>RM: WithPrivileges(ctx, fn)
    alt Normal Mode
        RM->>PM: WithPrivileges(ctx, fn)
    else Dry-Run Mode
        RM->>RM: SimulatePrivileges() + RecordAnalysis()
    end

    R->>RM: GetDryRunResults()
    RM-->>R: DryRunResult
    R-->>M: DryRunResult
```

## 2. 実装フェーズと仕様

### 2.1 Phase 1: Foundation（完了済み）
✅ **ResourceManager インターフェース**: `internal/runner/resource/manager.go`
✅ **ExecutionMode 型システム**: Normal/DryRun切り替え
✅ **ResourceAnalysis データ構造**: 副作用操作の詳細記録
✅ **DryRunResult 型階層**: 完全な分析結果構造
✅ **基本テストフレームワーク**: 11テストケース
✅ **Lint対応**: revive警告抑制

### 2.2 Phase 2: DefaultResourceManager実装（次期）
- **DefaultResourceManager構造体**: 副作用の統一管理
- **モード切り替えロジック**: Normal/DryRunの動的制御
- **副作用インターセプション**: 各操作の条件分岐実装
- **分析記録機能**: ResourceAnalysisの蓄積

### 2.3 Phase 3: Runner統合（次期）
- **Runner構造体拡張**: resourceManagerフィールド追加
- **副作用メソッド更新**: 全てResourceManager経由に変更
- **PerformDryRun実装**: SetMode→ExecuteAllの流れ

### 2.4 Phase 4: CLIインターフェース（次期）
- **--dry-runフラグ**: コマンドラインオプション追加
- **結果フォーマッター**: Text/JSON/YAML出力対応
- **ユーザーエクスペリエンス**: 進捗表示・エラー報告

## 3. テスト仕様

### 3.1 実行パス整合性テスト
Resource Manager Patternの最重要テスト項目：

```go
func TestExecutionPathConsistency(t *testing.T) {
    // 1. 通常実行準備段階とdry-run実行で同じ結果を得ることを検証
    // 2. 変数解決、特権分析、ファイル検証の整合性確認
    // 3. ResourceManagerのモード切り替えによる動作差分確認
}
```

### 3.2 副作用インターセプションテスト
```go
func TestSideEffectInterception(t *testing
