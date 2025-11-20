# Dry-Run モードでのファイル検証機能 - アーキテクチャ設計書

## 1. 概要

### 1.1 目的

本ドキュメントは、dry-run モードでのファイルハッシュ検証機能の高レベルなアーキテクチャ設計を記述する。本機能により、dry-run 実行時にもファイル検証を行い、検証失敗を警告として記録することで、本番実行前のリスク検出を可能にする。

### 1.2 設計原則

1. **Warn-Only Mode**: 検証失敗時もプログラムを継続実行し、結果を記録
2. **既存パターンの活用**: 既存の Resource Manager Pattern と検証ロジックを再利用
3. **最小限の変更**: 通常実行モードの検証動作は変更せず、dry-run モード特有の拡張のみを実装
4. **可視性の向上**: 検証結果を DryRunResult に統合し、既存の出力フォーマッタで表示
5. **副作用の抑制**: dry-run モードでは永続的な副作用を発生させない（従来通り）

### 1.3 スコープ

**対象範囲:**
- dry-run モードでのファイル検証実行（warn-only モード）
- 検証結果の記録と DryRunResult への統合
- TEXT/JSON フォーマッタでの検証結果表示
- **読み取り専用検証**: ファイル検証は読み取り専用操作であり、永続的な副作用を発生させない

**対象外:**
- 通常実行モードの変更（既存の厳格な検証を維持）
- 新規コマンドライン引数の追加
- 複数の検証モード（warn-only のみを実装）
- **ファイルシステムへの書き込み**: dry-run モードではハッシュファイルの作成・更新を行わない
- **ネットワーク通信**: dry-run モードでは Slack 通知等を送信しない（従来通り）

## 2. システムアーキテクチャ

### 2.1 全体構成

```mermaid
graph TD
    A[CLI Entry Point<br/>runner --dry-run] --> B[Verification Manager<br/>NewManagerForDryRun]

    B --> C{File Validator<br/>Enabled?}
    C -->|No 現在| D[Skip Verification<br/>verifyFileWithFallback returns nil]
    C -->|Yes 本実装| E[Execute Verification<br/>Warn-Only Mode]

    E --> F[Collect Results<br/>FileVerificationSummary]

    A --> G[Runner Initialization<br/>with DryRunResourceManager]

    G --> H[Execute Groups<br/>Same Path as Normal Mode]

    H --> I[Verification Calls<br/>VerifyConfigFile<br/>VerifyGlobalFiles<br/>VerifyGroupFiles]

    I --> E

    F --> J[DryRunResult<br/>with FileVerification Field]

    J --> K[Formatter<br/>Text/JSON]

    K --> L[Output<br/>Verification Summary<br/>+ Failures]

    style B fill:#fff3e0
    style E fill:#fce4ec
    style F fill:#e3f2fd
    style J fill:#c8e6c9
    style K fill:#f3e5f5
```

**主要コンポーネント:**

1. **Verification Manager**: ファイル検証の中核ロジック（既存）
2. **File Validator**: ハッシュ検証の実行（既存、dry-run で有効化）
3. **Result Collector**: 検証結果の収集（新規）
4. **DryRunResult**: dry-run 結果の統合データ構造（拡張）
5. **Formatter**: 検証結果の表示（拡張）

### 2.2 コンポーネント間の関係

```mermaid
classDiagram
    class VerificationManager {
        -fileValidator FileValidator
        -isDryRun bool
        -resultCollector ResultCollector
        +VerifyConfigFile(path) error
        +VerifyGlobalFiles(global) error
        +VerifyGroupFiles(group) error
        +GetVerificationSummary() FileVerificationSummary
        -verifyFileWithFallback(path) error
    }

    class FileValidator {
        +Verify(filePath) error
        +VerifyAndRead(filePath) ([]byte, error)
    }

    class ResultCollector {
        -failures []FileVerificationFailure
        -totalFiles int
        -verifiedFiles int
        -skippedFiles int
        +RecordSuccess(filePath, context)
        +RecordFailure(filePath, reason, level, context)
        +RecordSkip(filePath, context)
        +GetSummary() FileVerificationSummary
    }

    class DryRunResult {
        +FileVerification FileVerificationSummary
        +ResourceAnalyses []ResourceAnalysis
        +SecurityAnalysis SecurityAnalysis
    }

    class FileVerificationSummary {
        +TotalFiles int
        +VerifiedFiles int
        +SkippedFiles int
        +FailedFiles int
        +Duration time.Duration
        +HashDirStatus HashDirectoryStatus
        +Failures []FileVerificationFailure
    }

    class FileVerificationFailure {
        +Path string
        +Reason VerificationFailureReason
        +Level string
        +Message string
        +Context string
    }

    class TextFormatter {
        +FormatResult(result, opts) string
        -writeFileVerification(buf, summary)
    }

    class JSONFormatter {
        +FormatResult(result, opts) string
    }

    VerificationManager --> FileValidator : uses
    VerificationManager --> ResultCollector : uses
    VerificationManager ..> DryRunResult : produces
    ResultCollector --> FileVerificationSummary : creates
    FileVerificationSummary --> FileVerificationFailure : contains
    DryRunResult --> FileVerificationSummary : contains
    TextFormatter ..> DryRunResult : formats
    JSONFormatter ..> DryRunResult : formats
```

### 2.3 検証モードの比較

| 項目 | 通常実行モード | Dry-Run モード（現在） | Dry-Run モード（本実装） |
|------|--------------|---------------------|---------------------|
| File Validator | 有効 | **無効** | **有効（warn-only）** |
| 検証失敗時の動作 | プログラム終了 | - | **継続実行** |
| 検証結果の記録 | なし | - | **ResultCollector** |
| ログ出力 | ERROR（終了） | - | **WARN/ERROR（継続）** |
| 出力フォーマット | - | - | **TEXT/JSON に追加** |

## 3. 検証フロー設計

### 3.1 検証実行フロー

```mermaid
sequenceDiagram
    participant M as Main
    participant VM as VerificationManager
    participant FV as FileValidator
    participant RC as ResultCollector
    participant DR as DryRunResult

    M->>VM: NewManagerForDryRun()
    activate VM
    VM->>VM: fileValidator = New(...)
    Note over VM: 現在は nil、本実装で有効化
    VM->>RC: New ResultCollector
    VM-->>M: manager
    deactivate VM

    M->>VM: VerifyConfigFile(configPath)
    activate VM
    VM->>RC: startTimer()
    VM->>VM: ensureHashDirectoryValidated()
    alt Hash Directory Exists
        VM->>FV: Verify(configPath)
        activate FV
        FV->>FV: Check hash file
        alt Hash File Not Found
            FV-->>VM: error (hash file not found)
            VM->>RC: RecordFailure(path, "hash_file_not_found", "warn", "config")
        else Hash Mismatch
            FV-->>VM: error (hash mismatch)
            VM->>RC: RecordFailure(path, "hash_mismatch", "error", "config")
        else Hash OK
            FV-->>VM: nil
            VM->>RC: RecordSuccess(path, "config")
        end
        deactivate FV
    else Hash Directory Not Found
        VM->>RC: RecordInfo("Hash directory not found")
        Note over VM: Skip all verification
    end

    Note over VM: Dry-Run: Continue even on failure
    VM-->>M: nil (always success)
    deactivate VM

    M->>VM: VerifyGlobalFiles(global)
    Note over VM,RC: Same pattern as VerifyConfigFile

    M->>VM: VerifyGroupFiles(group)
    Note over VM,RC: Same pattern as VerifyConfigFile

    M->>VM: GetVerificationSummary()
    activate VM
    VM->>RC: GetSummary()
    activate RC
    RC-->>VM: FileVerificationSummary
    deactivate RC
    VM-->>M: FileVerificationSummary
    deactivate VM

    M->>DR: AddFileVerification(summary)
    M->>DR: Format and Output
```

### 3.2 検証失敗時の処理フロー

```mermaid
flowchart TD
    A[File Verification] --> B{Hash Directory<br/>Exists?}

    B -->|No| C[Log: INFO<br/>Hash directory not found]
    C --> D[Record to Summary<br/>HashDirStatus.Exists = false]
    D --> E[Skip Verification<br/>Continue Execution]

    B -->|Yes| F{Hash File<br/>Exists?}

    F -->|No| G[Log: WARN<br/>Hash file not found]
    G --> H[Record Failure<br/>Level: warn<br/>Reason: hash_file_not_found]
    H --> E

    F -->|Yes| I{Hash<br/>Matches?}

    I -->|No| J[Log: ERROR<br/>Hash mismatch<br/>Security Risk: High]
    J --> K[Record Failure<br/>Level: error<br/>Reason: hash_mismatch]
    K --> E

    I -->|Yes| L[Log: DEBUG<br/>Verification success]
    L --> M[Record Success<br/>Increment verifiedFiles]
    M --> E

    E --> N{Standard<br/>System Path?}
    N -->|Yes| O[Log: INFO<br/>Skipped standard path]
    O --> P[Record Skip<br/>Increment skippedFiles]
    P --> E

    N -->|No| E

    E --> Q[Continue to Next File<br/>No Program Exit]

    style C fill:#e3f2fd
    style G fill:#fff9c4
    style J fill:#ffcdd2
    style M fill:#c8e6c9
    style O fill:#e3f2fd
    style Q fill:#c8e6c9
```

### 3.3 通常実行モードとの動作比較

```mermaid
graph LR
    subgraph "通常実行モード"
        A1[File Verification] --> B1{Verification<br/>Success?}
        B1 -->|Yes| C1[Continue]
        B1 -->|No| D1[Log ERROR]
        D1 --> E1[Exit 1]
    end

    subgraph "Dry-Run モード（本実装）"
        A2[File Verification] --> B2{Verification<br/>Success?}
        B2 -->|Yes| C2[Record Success]
        B2 -->|No| D2[Log WARN/ERROR]
        D2 --> E2[Record Failure]
        E2 --> F2[Continue]
        C2 --> F2
        F2 --> G2[Add to DryRunResult]
    end

    style E1 fill:#ffcdd2
    style F2 fill:#c8e6c9
    style G2 fill:#e3f2fd
```

## 4. 副作用抑制の設計（従来要件の確認）

### 4.1 Dry-Run モードの副作用抑制原則

**重要**: dry-run モードでは、従来通り永続的な副作用を発生させない。本機能追加（ファイル検証の有効化）も、この原則に従う。

#### 4.1.1 副作用の分類

```mermaid
graph TD
    A[Dry-Run Operations] --> B[Read-Only Operations<br/>許可]
    A --> C[Side-Effect Operations<br/>禁止]

    B --> B1[File Read<br/>✓ 設定ファイル読み取り<br/>✓ ハッシュファイル読み取り<br/>✓ 検証対象ファイル読み取り]
    B --> B2[Memory Operations<br/>✓ 実行計画生成<br/>✓ セキュリティ分析<br/>✓ 検証結果記録]
    B --> B3[Standard Output<br/>✓ TEXT/JSON 出力<br/>✓ ログメッセージ]

    C --> C1[File Write<br/>✗ ハッシュファイル作成<br/>✗ コマンド出力ファイル<br/>✗ 一時ファイル作成]
    C --> C2[Network Communication<br/>✗ Slack 通知<br/>✗ 外部サービス通信]
    C --> C3[System State Change<br/>✗ コマンド実行<br/>✗ 権限変更<br/>✗ 環境変数永続化]

    style B fill:#c8e6c9
    style C fill:#ffcdd2
    style B1 fill:#e3f2fd
    style B2 fill:#e3f2fd
    style B3 fill:#e3f2fd
    style C1 fill:#ffebee
    style C2 fill:#ffebee
    style C3 fill:#ffebee
```

#### 4.1.2 ファイル検証の副作用分析

**本機能追加で実施する操作:**

| 操作 | 分類 | 副作用 | 説明 |
|------|-----|--------|------|
| ハッシュディレクトリの確認 | Read-Only | なし | ディレクトリの存在確認のみ |
| ハッシュファイルの読み取り | Read-Only | なし | ファイル内容を読み取るのみ |
| 検証対象ファイルの読み取り | Read-Only | なし | SHA-256 計算のため読み取るのみ |
| ハッシュ値の計算 | Memory | なし | メモリ内でハッシュ計算 |
| 検証結果の記録 | Memory | なし | `ResultCollector` への記録（メモリ内） |
| ログ出力 | Standard Output | なし | 標準出力/標準エラー出力のみ |

**実施しない操作:**

| 操作 | 理由 |
|------|------|
| ハッシュファイルの作成・更新 | 永続的な副作用（ファイル書き込み） |
| 検証失敗によるプログラム終了 | dry-run の基本動作（継続実行） |
| 検証結果のファイル保存 | 永続的な副作用（要件外） |

### 4.2 副作用抑制の実装保証

#### 4.2.1 実装レベルの保証

```go
// verifyFileWithFallback の実装（疑似コード）
func (m *Manager) verifyFileWithFallback(filePath string, context string) error {
    if m.fileValidator == nil {
        // Hash directory not found - read-only check, no side effect
        if m.isDryRun && m.resultCollector != nil {
            m.resultCollector.RecordSkip(filePath, context, "hash_directory_not_found")
        }
        return nil  // No file creation, no network communication
    }

    // Execute verification - READ-ONLY operation
    err := m.fileValidator.Verify(filePath)
    // Verify reads:
    //   1. Hash file (read-only)
    //   2. Target file (read-only, for SHA-256 calculation)
    // No writes, no network communication

    if m.isDryRun && m.resultCollector != nil {
        if err != nil {
            // Record failure in MEMORY (ResultCollector)
            m.resultCollector.RecordFailure(filePath, err, context)
            // Log to STANDARD ERROR (no file write)
            logVerificationFailure(...)
        } else {
            // Record success in MEMORY
            m.resultCollector.RecordSuccess(filePath, context)
        }
        return nil  // Continue execution, no program exit
    }

    // Normal mode: strict mode (may exit on error)
    return err
}
```

#### 4.2.2 テストによる検証

**副作用不在のテスト:**

```go
// TestDryRunNoSideEffects verifies that dry-run mode has no side effects
func TestDryRunNoSideEffects(t *testing.T) {
    // Setup: Monitor filesystem and network
    fileMonitor := NewFileSystemMonitor()
    networkMonitor := NewNetworkMonitor()

    // Execute dry-run with file verification
    result, err := RunDryRunWithVerification(config)

    // Verify: No file writes
    assert.Empty(t, fileMonitor.GetFileWrites(),
        "Dry-run should not write any files")

    // Verify: No network communication
    assert.Empty(t, networkMonitor.GetNetworkRequests(),
        "Dry-run should not send any network requests")

    // Verify: Exit code is 0 (even with verification failures)
    assert.Equal(t, 0, result.ExitCode,
        "Dry-run should always exit with code 0")

    // Verify: Verification results are in memory
    assert.NotNil(t, result.FileVerification,
        "Verification results should be in DryRunResult")
}
```

### 4.3 Resource Manager Pattern との整合性

dry-run モードでの副作用抑制は、既存の Resource Manager Pattern と一貫している：

```mermaid
graph TD
    A[Resource Manager<br/>Pattern] --> B[DryRunResourceManager]
    A --> C[NormalResourceManager]

    B --> B1[ExecuteCommand<br/>✗ 実行しない<br/>✓ 分析のみ]
    B --> B2[CreateTempDir<br/>✗ 作成しない<br/>✓ パス生成のみ]
    B --> B3[SendNotification<br/>✗ 送信しない<br/>✓ 記録のみ]

    C --> C1[ExecuteCommand<br/>✓ 実際に実行]
    C --> C2[CreateTempDir<br/>✓ 実際に作成]
    C --> C3[SendNotification<br/>✓ 実際に送信]

    D[ファイル検証<br/>本機能] --> B4[Verify<br/>✓ 読み取り専用<br/>✓ メモリ内記録<br/>✗ ファイル書き込みなし]
    D --> C4[Verify<br/>✓ 読み取り専用<br/>✓ エラー時終了<br/>✗ ファイル書き込みなし]

    style B fill:#e3f2fd
    style C fill:#c8e6c9
    style D fill:#fff9c4
    style B1 fill:#e1f5fe
    style B2 fill:#e1f5fe
    style B3 fill:#e1f5fe
    style B4 fill:#fff9c4
```

**重要な一貫性:**
- **コマンド実行**: 両モードで実行しない（分析のみ）
- **一時ファイル**: 両モードで作成しない
- **Slack 通知**: 両モードで送信しない
- **ファイル検証**: **両モードで読み取り専用**（本機能追加でも変更なし）

## 5. データフロー設計

### 5.1 検証結果の集約

```mermaid
flowchart LR
    A[VerifyConfigFile] --> E[ResultCollector]
    B[VerifyGlobalFiles] --> E
    C[VerifyGroupFiles] --> E
    D[VerifyEnvFile] --> E

    E --> F{Collect Results}

    F --> G[FileVerificationSummary<br/>totalFiles<br/>verifiedFiles<br/>failedFiles<br/>skippedFiles]

    F --> H[HashDirectoryStatus<br/>path<br/>exists<br/>validated]

    F --> I[Failures Array<br/>path<br/>reason<br/>level<br/>message<br/>context]

    G --> J[DryRunResult<br/>file_verification field]
    H --> J
    I --> J

    J --> K[Formatter<br/>Text/JSON]

    K --> L[User Output]

    style E fill:#fff3e0
    style J fill:#c8e6c9
    style K fill:#f3e5f5
```

### 5.2 検証コンテキストの伝播

各検証呼び出し元から、検証失敗時の文脈情報を伝播する：

| 検証メソッド | Context Value | 説明 |
|------------|--------------|------|
| `VerifyConfigFile` | `"config"` | 設定ファイル自体の検証 |
| `VerifyGlobalFiles` | `"global"` | グローバル検証ファイル |
| `VerifyGroupFiles` | `"group:<name>"` | 特定グループの検証ファイル |
| `VerifyEnvFile` | `"env"` | 環境変数ファイルの検証 |

この文脈情報により、ユーザーはどの段階で検証が失敗したかを理解できる。

## 6. 拡張ポイント設計

### 6.1 Verification Manager の拡張

**現在の実装:**
```go
func (m *Manager) verifyFileWithFallback(filePath string) error {
    if m.fileValidator == nil {
        // File validator is disabled (e.g., in dry-run mode) - skip verification
        return nil
    }
    return m.fileValidator.Verify(filePath)
}
```

**拡張後の実装:**
```go
func (m *Manager) verifyFileWithFallback(filePath string, context string) error {
    if m.fileValidator == nil {
        // Development environment without hash directory
        return nil
    }

    err := m.fileValidator.Verify(filePath)

    if m.isDryRun {
        // Warn-only mode: record result and continue
        if err != nil {
            m.resultCollector.RecordFailure(filePath, err, context)
        } else {
            m.resultCollector.RecordSuccess(filePath, context)
        }
        return nil  // Always return nil in dry-run
    }

    // Normal mode: return error (strict mode)
    return err
}
```

### 6.2 DryRunResult の拡張

**現在の構造:**
```go
type DryRunResult struct {
    Metadata         *ResultMetadata       `json:"metadata"`
    Status           ExecutionStatus       `json:"status"`
    Phase            ExecutionPhase        `json:"phase"`
    Error            *ExecutionError       `json:"error,omitempty"`
    Summary          *ExecutionSummary     `json:"summary"`
    ResourceAnalyses []ResourceAnalysis    `json:"resource_analyses"`
    SecurityAnalysis *SecurityAnalysis     `json:"security_analysis"`
    EnvironmentInfo  *EnvironmentInfo      `json:"environment_info"`
    Errors           []DryRunError         `json:"errors"`
    Warnings         []DryRunWarning       `json:"warnings"`
}
```

**拡張後の構造:**
```go
type DryRunResult struct {
    // ... existing fields ...
    FileVerification *FileVerificationSummary `json:"file_verification,omitempty"`  // 新規追加
}
```

**ExecutionStatus の変更:**

ファイル検証失敗を適切に表現するため、`ExecutionStatus` 型を整理：

**現在の定義:**
```go
type ExecutionStatus string

const (
    StatusSuccess ExecutionStatus = "success"
    StatusError   ExecutionStatus = "error"
    StatusPartial ExecutionStatus = "partial"  // 未使用のため削除
)
```

**変更後の定義:**
```go
type ExecutionStatus string

const (
    StatusSuccess ExecutionStatus = "success"
    StatusError   ExecutionStatus = "error"
)
```

**ステータス設定ロジック:**
- ファイル検証失敗がある場合: `StatusError`（ただし exit code は 0）
- 全ての検証が成功した場合: `StatusSuccess`
- dry-run 処理自体の致命的エラー: `StatusError`（exit code は 1）

**設計判断の根拠:**
- ファイル検証失敗（特にハッシュ不一致）はセキュリティ上重大な問題
- `status: "error"` により JSON パーサーが適切に警告を表示可能
- exit code 0 は維持（dry-run は診断ツールとして動作）
- `StatusPartial` は使用されていないため削除

### 6.3 Formatter の拡張

**Text Formatter:**
```go
func (f *TextFormatter) FormatResult(result *DryRunResult, opts FormatterOptions) (string, error) {
    // ... existing sections ...

    // 新規追加: File Verification セクション
    if result.FileVerification != nil {
        f.writeFileVerification(&buf, result.FileVerification)
    }

    // ... existing sections ...
}
```

**JSON Formatter:**
- `DryRunResult` の JSON シリアライゼーションで自動的に `file_verification` フィールドが含まれる
- 追加の実装は不要

## 7. エラー処理設計

### 7.1 検証失敗の分類

```mermaid
graph TD
    A[Verification Failure] --> B{Failure Type}

    B --> C[Hash Directory Not Found]
    B --> D[Hash File Not Found]
    B --> E[Hash Mismatch]
    B --> F[File Read Error]
    B --> G[Permission Denied]
    B --> H[Standard Path Skipped]

    C --> I[Level: INFO<br/>Continue: Yes<br/>Record: HashDirStatus]
    D --> J[Level: WARN<br/>Continue: Yes<br/>Record: Failure]
    E --> K[Level: ERROR<br/>Continue: Yes<br/>Record: Failure<br/>Security Risk: High]
    F --> L[Level: ERROR<br/>Continue: Yes<br/>Record: Failure]
    G --> M[Level: ERROR<br/>Continue: Yes<br/>Record: Failure]
    H --> N[Level: INFO<br/>Continue: Yes<br/>Record: Skip]

    style I fill:#e3f2fd
    style J fill:#fff9c4
    style K fill:#ffcdd2
    style L fill:#ffcdd2
    style M fill:#ffcdd2
    style N fill:#e3f2fd
```

### 7.2 ログレベルの決定基準

| 失敗理由 | ログレベル | 根拠 |
|---------|----------|------|
| Hash Directory Not Found | INFO | 開発環境では正常な状態 |
| Hash File Not Found | WARN | 本番環境では設定ミスの可能性（セキュリティリスクは中） |
| Hash Mismatch | ERROR | ファイル改ざんの可能性（セキュリティリスクは高） |
| File Read Error | ERROR | システムの問題（本番実行で確実に失敗） |
| Standard Path Skipped | INFO | 意図的なスキップ |

## 8. セキュリティ設計

### 8.1 センシティブ情報の扱い

検証結果にセンシティブ情報が含まれる可能性を考慮：

1. **ファイルパス**: 検証失敗時に記録するが、`--show-sensitive` フラグに従ってマスク
2. **ハッシュ値**: ERROR レベルのログで expected/actual を出力するが、詳細は制限
3. **エラーメッセージ**: システム内部情報の漏洩を防ぐため、汎用的なメッセージを使用

### 7.2 検証バイパスの防止

```mermaid
graph TD
    A[Verification Request] --> B{Execution Mode}

    B -->|Normal Mode| C[Strict Verification<br/>FileValidator.Verify]
    C --> D{Verification Result}
    D -->|Success| E[Continue]
    D -->|Failure| F[Log ERROR + Exit 1]

    B -->|Dry-Run Mode| G[Warn-Only Verification<br/>FileValidator.Verify]
    G --> H{Verification Result}
    H -->|Success| I[Record Success]
    H -->|Failure| J[Record Failure<br/>Log WARN/ERROR]
    I --> K[Continue]
    J --> K

    style C fill:#c8e6c9
    style F fill:#ffcdd2
    style G fill:#fff9c4
    style K fill:#e3f2fd
```

**重要な原則:**
- **通常実行モードでは検証をバイパスしない**: dry-run モードの拡張は、通常実行モードのセキュリティを低下させない
- **File Validator は同一のロジックを使用**: dry-run と通常モードで検証ロジックを共有し、整合性を保証

## 9. パフォーマンス設計

### 9.1 検証の並列化

既存の実装では、ファイル検証は順次実行されるが、将来的に並列化を検討する場合の設計：

```go
// Future optimization (not in scope for this task)
func (m *Manager) VerifyGlobalFilesParallel(runtimeGlobal *runnertypes.RuntimeGlobal) (*Result, error) {
    var wg sync.WaitGroup
    resultChan := make(chan verificationResult, len(runtimeGlobal.ExpandedVerifyFiles))

    for _, filePath := range runtimeGlobal.ExpandedVerifyFiles {
        wg.Add(1)
        go func(path string) {
            defer wg.Done()
            err := m.verifyFileWithFallback(path, "global")
            resultChan <- verificationResult{path: path, err: err}
        }(filePath)
    }

    // ... collect results ...
}
```

現時点では並列化を実装しないが、`ResultCollector` は並行アクセスに対応した設計とする（mutex を使用）。

### 9.2 検証コストの最小化

```mermaid
graph TD
    A[File Verification Request] --> B{Hash Directory<br/>Validated?}

    B -->|No| C[Validate Once]
    C --> D[Cache Result]
    D --> E{Directory Exists?}

    B -->|Yes, Cached| E

    E -->|No| F[Skip All Verification<br/>Return Early]

    E -->|Yes| G{Standard Path?}

    G -->|Yes| H[Skip Verification<br/>Record Skip]

    G -->|No| I[Execute Verification<br/>FileValidator.Verify]

    style C fill:#fff9c4
    style D fill:#e3f2fd
    style F fill:#c8e6c9
    style H fill:#c8e6c9
```

**最適化ポイント:**
1. **Hash Directory 検証のキャッシュ**: 初回のみ検証し、結果をキャッシュ
2. **早期リターン**: Hash Directory が存在しない場合、全検証をスキップ
3. **Standard Path のスキップ**: `/bin`, `/usr/bin` 等の標準パスは検証をスキップ

## 10. テスト戦略

### 10.1 テストレベル

```mermaid
graph TD
    A[Testing Strategy] --> B[Unit Tests]
    A --> C[Integration Tests]
    A --> D[E2E Tests]

    B --> B1[ResultCollector]
    B --> B2[FileVerificationSummary]
    B --> B3[verifyFileWithFallback]

    C --> C1[VerifyConfigFile<br/>with dry-run]
    C --> C2[VerifyGlobalFiles<br/>with dry-run]
    C --> C3[VerifyGroupFiles<br/>with dry-run]
    C --> C4[Formatter Integration]

    D --> D1[runner --dry-run<br/>Hash Dir Not Found]
    D --> D2[runner --dry-run<br/>Hash File Not Found]
    D --> D3[runner --dry-run<br/>Hash Mismatch]
    D --> D4[runner --dry-run<br/>All Success]

    style B fill:#e3f2fd
    style C fill:#fff9c4
    style D fill:#c8e6c9
```

### 9.2 主要テストケース

| レベル | テストケース | 検証内容 |
|--------|------------|---------|
| **Unit** | ResultCollector.RecordFailure | 失敗記録の正確性 |
| **Unit** | ResultCollector.GetSummary | サマリー計算の正確性 |
| **Integration** | VerifyConfigFile (dry-run, hash not found) | WARN ログ + 継続実行 |
| **Integration** | VerifyGlobalFiles (dry-run, hash mismatch) | ERROR ログ + 継続実行 |
| **Integration** | TextFormatter with FileVerification | TEXT 出力の正確性 |
| **Integration** | JSONFormatter with FileVerification | JSON 出力の正確性 |
| **E2E** | runner --dry-run (no hash dir) | INFO ログ + exit 0 |
| **E2E** | runner --dry-run (hash file not found) | WARN ログ + exit 0 + JSON 出力 |
| **E2E** | runner --dry-run (hash mismatch) | ERROR ログ + exit 0 + verification section |

## 11. 段階的実装計画

### 11.1 実装フェーズ

```mermaid
gantt
    title Dry-Run File Verification Implementation
    dateFormat  YYYY-MM-DD
    section Phase 1
    ResultCollector Implementation    :p1, 2024-01-01, 2d
    Unit Tests                        :p1t, after p1, 1d
    section Phase 2
    Verification Manager Extension    :p2, after p1t, 2d
    Integration Tests                 :p2t, after p2, 1d
    section Phase 3
    DryRunResult Extension            :p3, after p2t, 1d
    Formatter Extension               :p3f, after p3, 2d
    Format Tests                      :p3t, after p3f, 1d
    section Phase 4
    E2E Tests                         :p4, after p3t, 2d
    Documentation                     :p4d, after p4, 1d
```

### 10.2 フェーズ詳細

**Phase 1: Result Collector Implementation**
- `ResultCollector` 構造体の実装
- `RecordSuccess`, `RecordFailure`, `RecordSkip` メソッド
- `GetSummary` メソッド
- ユニットテスト

**Phase 2: Verification Manager Extension**
- `Manager` 構造体に `resultCollector` フィールドを追加
- `NewManagerForDryRun` で `ResultCollector` を初期化
- `verifyFileWithFallback` を拡張（context パラメータ追加、warn-only モード実装）
- `GetVerificationSummary` メソッドの追加
- 統合テスト

**Phase 3: DryRunResult and Formatter Extension**
- `DryRunResult` に `FileVerification` フィールドを追加
- `TextFormatter.writeFileVerification` メソッドの実装
- JSON 出力の検証
- フォーマッタテスト

**Phase 4: E2E Testing and Documentation**
- E2E テストの実装（各検証失敗パターン）
- ドキュメント更新
- パフォーマンステスト

## 12. 今後の拡張可能性

### 12.1 将来的な機能拡張

1. **並列検証**: 大量ファイルの並列検証によるパフォーマンス向上
2. **検証モード選択**: `--verify-mode=disabled|warn|strict` のような明示的な制御
3. **検証結果のエクスポート**: 検証結果を別ファイルに保存（監査ログ）
4. **差分検証**: 前回の検証結果との差分表示

### 12.2 拡張時の考慮事項

```mermaid
graph TD
    A[Future Extensions] --> B{Extension Type}

    B --> C[Parallel Verification]
    B --> D[Verification Modes]
    B --> E[Result Export]
    B --> F[Diff Reporting]

    C --> G[Design Consideration:<br/>Thread-safe ResultCollector<br/>Context propagation]

    D --> H[Design Consideration:<br/>Mode enum extension<br/>Backward compatibility]

    E --> I[Design Consideration:<br/>Export format<br/>Sensitive data handling]

    F --> J[Design Consideration:<br/>State management<br/>Storage location]

    style G fill:#fff9c4
    style H fill:#fff9c4
    style I fill:#fff9c4
    style J fill:#fff9c4
```

## 13. まとめ

### 13.1 アーキテクチャの特徴

1. **既存パターンとの調和**: Resource Manager Pattern と同様の設計原則を踏襲
2. **最小限の侵襲性**: 通常実行モードには影響を与えず、dry-run モードのみを拡張
3. **可視性の向上**: 検証結果を DryRunResult に統合し、既存の出力インフラを活用
4. **段階的実装**: フェーズ分けにより、リスクを最小化しながら実装

### 13.2 成功基準

- [ ] dry-run モードでファイル検証が実行される
- [ ] 検証失敗時も dry-run は継続実行される（exit 0）
- [ ] 検証結果が DryRunResult に記録される
- [ ] TEXT/JSON フォーマットで検証結果が適切に表示される
- [ ] 通常実行モードの動作は変更されない
- [ ] 既存の dry-run テストが全て成功する
