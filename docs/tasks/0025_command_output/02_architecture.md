# アーキテクチャ設計書：コマンド出力キャプチャ機能

## 1. 概要 (Overview)

### 1.1 目的
本文書は、go-safe-cmd-runnerにおけるコマンド出力キャプチャ機能の高レベルアーキテクチャを定義する。この機能により、実行されるコマンドの標準出力を指定されたファイルに安全かつ効率的に保存できるようになる。

### 1.2 設計原則
- **セキュリティファースト**: パストラバーサル攻撃の防止、適切なファイル権限管理
- **権限分離**: コマンド実行権限と出力ファイル権限の明確な分離
- **既存アーキテクチャとの整合性**: 現在のResourceManager/Executor パターンとの統合
- **Dry-Run対応**: 実行前の出力予測と検証
- **エラーハンドリング**: 部分失敗の適切な処理とクリーンアップ

## 2. システムアーキテクチャ概要

### 2.1 コンポーネント配置

```mermaid
graph TD
    A[Main Application<br/>cmd/runner/main.go] --> B[Runner<br/>internal/runner/runner.go]
    B --> B1[ExecuteAll]
    B --> B2[ExecuteGroup]
    B --> B3[executeCommandIn...]
    B --> C[ResourceManager<br/>internal/runner/resource/manager.go]
    C --> C1[NormalResource<br/>Manager]
    C --> C2[DryRunResource<br/>Manager]
    C --> C3[OutputCapture<br/>Manager ← 新規]
    C --> D[Executor<br/>internal/runner/executor/]
    D --> D1[executeNormal]
    D --> D2[executeWithUser...]
    D --> D3[executeCommandWith...]
```

### 2.2 データフロー概要

```mermaid
graph LR
    A[Configuration<br/>TOML Parse] --> B[Runner]
    B --> C[ResourceManager<br/>Command Execution]
    C --> D[OutputCaptureManager<br/>File Validation]
    D --> E[File System<br/>Permission Control]

    C --> F[Stdout Stream]
    F --> G[Runner Stdout<br/>常に出力]
    F --> H{Output指定あり?}
    H -->|Yes| I[Tee to File<br/>Buffer Management]
    I --> J[Atomic File Write]
    J --> E
```

## 3. 新規コンポーネント設計

### 3.1 OutputCaptureManager
新たに導入するコアコンポーネントで、出力キャプチャのライフサイクル全体を管理する。

#### 3.1.1 責任範囲
- 出力パスの検証とセキュリティチェック
- ディレクトリの自動作成と権限管理
- ストリーミング書き込みとバッファ管理
- 出力サイズ制限の監視
- 原子的ファイル操作の実装
- エラー時のクリーンアップ

#### 3.1.2 インターフェース
```go
type OutputCaptureManager interface {
    // 出力キャプチャの準備（事前検証）
    PrepareOutput(outputPath string, workDir string, maxSize int64) (*OutputCapture, error)

    // ストリーミング出力書き込み
    WriteOutput(capture *OutputCapture, data []byte) error

    // 出力完了と最終化
    FinalizeOutput(capture *OutputCapture) error

    // エラー時のクリーンアップ
    CleanupOutput(capture *OutputCapture) error

    // Dry-Run用の分析
    AnalyzeOutput(outputPath string, workDir string) (*OutputAnalysis, error)
}
```

### 3.2 OutputCapture 構造体
個別の出力キャプチャセッションを管理する。

```go
type OutputCapture struct {
    OutputPath   string    // 最終出力先パス
    TempPath     string    // 一時ファイルパス
    TempFile     *os.File  // 一時ファイルハンドル
    MaxSize      int64     // 最大出力サイズ
    CurrentSize  int64     // 現在の書き込みサイズ
    StartTime    time.Time // 開始時刻
}
```

### 3.3 OutputAnalysis 構造体 (Dry-Run用)
```go
type OutputAnalysis struct {
    OutputPath      string        // 出力先パス
    ResolvedPath    string        // 解決済み絶対パス
    DirectoryExists bool          // ディレクトリ存在確認
    WritePermission bool          // 書き込み権限確認
    EstimatedSize   string        // 推定サイズ ("Unknown"等)
    SecurityRisk    SecurityLevel // セキュリティリスク評価
    MaxSizeLimit    int64         // サイズ制限
}
```

## 4. 既存コンポーネントとの統合

### 4.1 ResourceManager 統合パターン
既存のResourceManagerインターフェースを拡張し、出力キャプチャ機能を統合する。

#### 4.1.1 NormalResourceManager 拡張
```go
type NormalResourceManager struct {
    // 既存フィールド
    executor         executor.CommandExecutor
    pathResolver     PathResolver
    privilegeManager runnertypes.PrivilegeManager

    // 新規追加フィールド
    outputManager    OutputCaptureManager  // 出力キャプチャ管理
    maxOutputSize    int64                // デフォルト出力サイズ制限
}
```

#### 4.1.2 DryRunResourceManager 拡張
```go
type DryRunResourceManager struct {
    // 既存フィールド
    executor         executor.CommandExecutor
    pathResolver     PathResolver

    // 新規追加フィールド
    outputManager    OutputCaptureManager  // 出力分析用
}
```

### 4.2 Command構造体拡張
既存のCommand構造体にoutputフィールドを追加する。

```go
type Command struct {
    // 既存フィールド
    Name         string   `toml:"name"`
    Description  string   `toml:"description"`
    Cmd          string   `toml:"cmd"`
    Args         []string `toml:"args"`
    // ... その他既存フィールド

    // 新規追加フィールド
    Output       string   `toml:"output"`  // 標準出力の書き込み先ファイル
}
```

### 4.3 GlobalConfig構造体拡張
グローバル設定にデフォルト出力サイズ制限を追加する。

```go
type GlobalConfig struct {
    // 既存フィールド
    Timeout           int      `toml:"timeout"`
    WorkDir           string   `toml:"workdir"`
    // ... その他既存フィールド

    // 新規追加フィールド
    MaxOutputSize     int64    `toml:"max_output_size"`  // デフォルト出力サイズ制限
}
```

## 5. 実行フロー設計

### 5.1 通常実行モードのフロー

```mermaid
graph TD
    A[Runner.ExecuteGroup] --> B[グループ前処理<br/>環境変数解決等]
    B --> C[For each command in group]
    C --> D[ResourceManager.ExecuteCommand]
    D --> E{outputフィールドチェック}
    E -->|Empty| F[通常実行]
    E -->|Not Empty| G[出力キャプチャ実行]
    G --> H[OutputManager.PrepareOutput]
    H --> H1[パス検証・セキュリティチェック]
    H1 --> H2[ディレクトリ作成]
    H2 --> H3[一時ファイル作成]
    H3 --> I[Executor.Execute + 出力キャプチャ]
    I --> I1[プロセス実行]
    I1 --> I2[stdout → Runner標準出力 + ファイル]
    I2 --> I3[サイズ制限監視]
    I3 --> J[OutputManager.FinalizeOutput]
    J --> J1[一時ファイル → 最終ファイル移動<br/>権限0600は移動時に継承]
    J1 --> J2[権限確認・必要に応じて修正]
    F --> K[実行結果返却<br/>Runner標準出力のみ]
    J2 --> K
    K --> L{次のコマンドあり?}
    L -->|Yes| C
    L -->|No| M[完了]
```

### 5.2 Dry-Runモードのフロー

```mermaid
graph TD
    A[Runner.ExecuteGroup<br/>DryRun Mode] --> B[グループ前処理]
    B --> C[For each command in group]
    C --> D[DryRunResourceManager.ExecuteCommand]
    D --> E{outputフィールドチェック}
    E -->|Empty| F[通常のDry-Run処理]
    E -->|Not Empty| G[出力分析]
    G --> H[OutputManager.AnalyzeOutput]
    H --> H1[パス解決・セキュリティ分析]
    H1 --> H2[権限確認]
    H2 --> H3[リスク評価]
    H3 --> I[分析結果をDryRunResultに追加]
    F --> J[模擬実行結果返却]
    I --> J
    J --> K{次のコマンドあり?}
    K -->|Yes| C
    K -->|No| L[完了]
```

## 6. セキュリティアーキテクチャ

### 6.1 権限分離モデル

```mermaid
graph TB
    subgraph "Process Model"
        A[Real UID RUID<br/>元の実行ユーザー] --> A1[出力ファイル権限]
        B[Effective UID EUID<br/>run_as_userの値] --> B1[コマンド実行権限]
    end

    subgraph "権限の使い分け"
        C[Output File Creation<br/>Always use RUID]
        D[Command Execution<br/>Use EUID if set]
    end
```

### 6.2 セキュリティ検証フロー

```mermaid
graph TD
    A[セキュリティ検証開始] --> B[Path Validation]
    B --> B1[相対パス正規化]
    B1 --> B2[パストラバーサル検出<br/>../ 等]
    B2 --> B3[シンボリックリンク検証]

    B3 --> C[Permission Check]
    C --> C1[実UID権限での書き込み可能性確認]
    C1 --> C2[ディレクトリ作成権限確認]
    C2 --> C3[既存ファイル上書き権限確認]

    C3 --> D[Security Risk Assessment]
    D --> D1[パス危険度評価]
    D1 --> D2[システムディレクトリアクセス検出]
    D2 --> D3[機密領域への出力検出]

    D3 --> E[検証完了]
```

### 6.3 ファイル権限管理

```mermaid
graph TB
    subgraph "出力ファイルの権限設定"
        A[File Permission: 0600 固定]
        B[Owner: Real UID 実行者]
        C[Group: Real GID 実行者のプライマリグループ]
        D[理由: 機密情報保護のため制限的な権限設定]
    end
```

## 7. エラーハンドリング戦略

### 7.1 エラー分類と対応
```go
type OutputCaptureError struct {
    Type    ErrorType
    Path    string
    Phase   ExecutionPhase
    Cause   error
}

type ErrorType int
const (
    ErrorTypePathValidation ErrorType = iota  // パス検証エラー
    ErrorTypePermission                       // 権限エラー
    ErrorTypeFileSystem                       // ファイルシステムエラー
    ErrorTypeSizeLimit                        // サイズ制限エラー
    ErrorTypeCleanup                          // クリーンアップエラー
)

type ExecutionPhase int
const (
    PhasePreparation ExecutionPhase = iota    // 準備段階
    PhaseExecution                            // 実行段階
    PhaseFinalization                         // 完了段階
    PhaseCleanup                              // クリーンアップ段階
)
```

### 7.2 エラー復旧戦略

| エラー段階 | 対応戦略 | 影響範囲 |
|------------|----------|----------|
| 準備段階失敗 | コマンド実行中止 | 該当コマンドのみ |
| 実行中ファイルエラー | プロセス強制終了 | 該当コマンドのみ |
| サイズ制限超過 | プロセス強制終了 | 該当コマンドのみ |
| 完了段階失敗 | 一時ファイル削除 | 該当コマンドのみ |
| クリーンアップ失敗 | ログ記録・継続 | 影響なし |

## 8. パフォーマンス設計

### 8.1 メモリ効率戦略
```go
// ストリーミング書き込み設定
const (
    DefaultBufferSize = 64 * 1024    // 64KB バッファ（bufio.Writerが自動管理）
    MaxBufferSize     = 1024 * 1024  // 1MB 最大バッファ
)

// サイズ制限
const (
    DefaultMaxOutputSize = 10 * 1024 * 1024  // 10MB デフォルト制限
    AbsoluteMaxSize      = 100 * 1024 * 1024 // 100MB 絶対制限
)
```

### 8.2 I/O効率戦略

```mermaid
graph TB
    subgraph "書き込みパターン"
        A[1. バッファリング書き込み]
        A --> A1[bufio.Writerによる自動バッファ管理]
        A --> A2[適切なバッファサイズ設定]

        B[2. 原子的ファイル操作]
        B --> B1[一時ファイル → 最終ファイル移動]
        B --> B2[失敗時の部分ファイル残存防止]

        C[3. 非同期I/O利用 将来拡張]
        C --> C1[プロセス実行と並行書き込み]
        C --> C2[CPU使用率の最適化]
    end
```

## 9. 拡張性設計

### 9.1 将来拡張ポイント

```mermaid
graph TB
    subgraph "拡張可能な機能領域"
        A[1. 出力フォーマット変換]
        A --> A1[JSON/XML変換]
        A --> A2[エンコーディング変換]

        B[2. 圧縮・暗号化]
        B --> B1[gzip/bzip2圧縮]
        B --> B2[AES暗号化]

        C[3. 複数出力先]
        C --> C1[ファイル + ネットワーク]
        C --> C2[ログシステム統合]

        D[4. リアルタイム処理]
        D --> D1[ストリーミング変換]
        D --> D2[リアルタイム通知]
    end
```

### 9.2 インターフェース設計
```go
// プラグイン可能な出力プロセッサ（将来拡張）
type OutputProcessor interface {
    ProcessOutput(data []byte) ([]byte, error)
    Finalize() error
}

// 設定可能な出力ハンドラー（将来拡張）
type OutputHandler interface {
    HandleOutput(capture *OutputCapture, data []byte) error
    SupportedSchemes() []string  // file://, s3://, http:// etc.
}
```

## 10. テスト戦略アーキテクチャ

### 10.1 テストレベル設計

```mermaid
graph TB
    subgraph "テスト階層"
        A[Unit Tests]
        A --> A1[OutputCaptureManager]
        A --> A2[Path validation]
        A --> A3[Security checks]
        A --> A4[File operations]

        B[Integration Tests]
        B --> B1[ResourceManager integration]
        B --> B2[Command execution flow]
        B --> B3[Error handling scenarios]

        C[Security Tests]
        C --> C1[Path traversal attacks]
        C --> C2[Privilege escalation attempts]
        C --> C3[File permission validation]

        D[Performance Tests]
        D --> D1[Large output handling]
        D --> D2[Memory usage patterns]
        D --> D3[Concurrent execution]
    end
```

### 10.2 モック戦略
```go
// テスト用モックインターフェース
type MockOutputCaptureManager struct {
    PrepareOutputFunc    func(string, string, int64) (*OutputCapture, error)
    WriteOutputFunc      func(*OutputCapture, []byte) error
    FinalizeOutputFunc   func(*OutputCapture) error
    CleanupOutputFunc    func(*OutputCapture) error
}

// ExtendedFileSystemのモック
type MockExtendedFileSystem struct {
    // common.FileSystemの埋め込み
    *common.DefaultFileSystem

    // 拡張機能のモック関数
    CreateTempFileFunc  func(dir, pattern string) (*os.File, error)
    StatFunc           func(path string) (os.FileInfo, error)
    ChownFunc          func(path string, uid, gid int) error
    ChmodFunc          func(path string, mode os.FileMode) error
    RenameFunc         func(oldpath, newpath string) error
    OpenFunc           func(name string) (*os.File, error)
    OpenFileFunc       func(name string, flag int, perm os.FileMode) (*os.File, error)
}
```

## 11. 運用考慮事項

### 11.1 監視・ログ戦略
```go
// ログ出力項目
type OutputCaptureMetrics struct {
    CommandName     string        `json:"command_name"`
    OutputPath      string        `json:"output_path"`
    OutputSize      int64         `json:"output_size"`
    Duration        time.Duration `json:"duration"`
    Success         bool          `json:"success"`
    ErrorType       string        `json:"error_type,omitempty"`
    SecurityRisk    string        `json:"security_risk"`
}
```

### 11.2 リソース管理

```mermaid
graph TB
    subgraph "リソース制限"
        A[ファイルディスクリプタ管理]
        A --> A1[一時ファイル数の制限]
        A --> A2[適切なクリーンアップ]

        B[ディスク容量管理]
        B --> B1[出力サイズ制限の実装]
        B --> B2[一時ファイル容量監視]

        C[メモリ使用量管理]
        C --> C1[バッファサイズの制限]
        C --> C2[ガベージコレクション最適化]
    end
```

この設計により、既存のgo-safe-cmd-runnerアーキテクチャと整合性を保ちながら、安全で効率的なコマンド出力キャプチャ機能を実現できる。
