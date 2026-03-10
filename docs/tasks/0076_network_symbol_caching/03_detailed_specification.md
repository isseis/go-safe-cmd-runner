# ネットワークシンボル解析結果のキャッシュ 詳細仕様書

## 0. 前提と参照

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)

本仕様書は `SyscallAnalysis`（タスク 0070/0072）の実装パターンを踏襲する。既存の `fileanalysis.syscall_store.go` および `fileanalysis/errors.go` を参照し、重複実装を避ける。

## 1. 型定義

### 1.1 `binaryanalyzer.AnalysisOutput` の変更（`analyzer.go`）

`DynamicLoadSymbols` フィールドを追加し、`HasDynamicLoad` を削除する。スキーマバージョンを上げるため後方互換性は不要。呼び出し元は `len(DynamicLoadSymbols) > 0` で代替する。

```go
type AnalysisOutput struct {
    Result             AnalysisResult
    DetectedSymbols    []DetectedSymbol
    DynamicLoadSymbols []DetectedSymbol  // 追加。HasDynamicLoad の代替
    Error              error
}
```

### 1.2 `fileanalysis` スキーマ変更（`schema.go`）

#### 1.2.1 `Record` 構造体

`HasDynamicLoad bool` を削除し、`NetworkSymbolAnalysis` を追加する：

```go
// 変更前（schema_version: 2）
type Record struct {
    SchemaVersion   int                  `json:"schema_version"`
    FilePath        string               `json:"file_path"`
    ContentHash     string               `json:"content_hash"`
    UpdatedAt       time.Time            `json:"updated_at"`
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    DynLibDeps      *DynLibDepsData      `json:"dyn_lib_deps,omitempty"`
    HasDynamicLoad  bool                 `json:"has_dynamic_load,omitempty"` // ← 削除
}

// 変更後（schema_version: 3）
type Record struct {
    SchemaVersion         int                        `json:"schema_version"`
    FilePath              string                     `json:"file_path"`
    ContentHash           string                     `json:"content_hash"`
    UpdatedAt             time.Time                  `json:"updated_at"`
    SyscallAnalysis       *SyscallAnalysisData       `json:"syscall_analysis,omitempty"`
    DynLibDeps            *DynLibDepsData            `json:"dyn_lib_deps,omitempty"`
    NetworkSymbolAnalysis *NetworkSymbolAnalysisData `json:"network_symbol_analysis,omitempty"` // ← 追加
}
```

#### 1.2.2 新規型定義

```go
// NetworkSymbolAnalysisData holds the network symbol analysis result cached at record time.
// nil means not analyzed (static binary, non-ELF, or old schema record).
type NetworkSymbolAnalysisData struct {
    // AnalyzedAt is when the network symbol analysis was performed.
    AnalyzedAt time.Time `json:"analyzed_at"`

    // HasNetworkSymbols indicates whether any network-related symbols were detected.
    HasNetworkSymbols bool `json:"has_network_symbols"`

    // DetectedSymbols contains all network-related symbols found (excluding dynamic_load category).
    // Empty when HasNetworkSymbols is false.
    DetectedSymbols []DetectedSymbolEntry `json:"detected_symbols,omitempty"`

    // DynamicLoadSymbols contains the dynamic library loading symbols found (dlopen, dlsym, dlvsym).
    // Empty when none were detected.
    // HasDynamicLoad is derived as len(DynamicLoadSymbols) > 0; no separate field.
    DynamicLoadSymbols []DetectedSymbolEntry `json:"dynamic_load_symbols,omitempty"`
}

type DetectedSymbolEntry struct {
    Name     string `json:"name"`
    Category string `json:"category"`
}
```

#### 1.2.3 スキーマバージョン更新

```go
const CurrentSchemaVersion = 3  // 2 → 3
```

### 1.3 `fileanalysis` エラー変数（`errors.go`）

`ErrNoSyscallAnalysis` と並べて追加する：

```go
var (
    ErrHashMismatch            = errors.New("file content hash mismatch")       // 既存
    ErrNoSyscallAnalysis       = errors.New("no syscall analysis data")          // 既存
    ErrNoNetworkSymbolAnalysis = errors.New("no network symbol analysis data")  // 追加
)
```

## 2. アナライザーの変更

### 2.1 `elfanalyzer/standard_analyzer.go` — `checkDynamicSymbols()` の変更

`hasDynamicLoad = true` とするだけでなく、シンボル名を `[]DetectedSymbol` に収集して `DynamicLoadSymbols` に設定する：

```go
func (a *StandardELFAnalyzer) checkDynamicSymbols(dynsyms []elf.Symbol) binaryanalyzer.AnalysisOutput {
    var detected []binaryanalyzer.DetectedSymbol
    var dynamicLoadSyms []binaryanalyzer.DetectedSymbol
    for _, sym := range dynsyms {
        if sym.Section == elf.SHN_UNDEF {
            if cat, found := a.networkSymbols[sym.Name]; found {
                detected = append(detected, binaryanalyzer.DetectedSymbol{
                    Name:     sym.Name,
                    Category: string(cat),
                })
            }
            if binaryanalyzer.IsDynamicLoadSymbol(sym.Name) {
                dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
                    Name:     sym.Name,
                    Category: "dynamic_load",
                })
            }
        }
    }

    if len(detected) > 0 {
        return binaryanalyzer.AnalysisOutput{
            Result:             binaryanalyzer.NetworkDetected,
            DetectedSymbols:    detected,
            DynamicLoadSymbols: dynamicLoadSyms,
        }
    }

    return binaryanalyzer.AnalysisOutput{
        Result:             binaryanalyzer.NoNetworkSymbols,
        DynamicLoadSymbols: dynamicLoadSyms,
    }
}
```

### 2.2 `machoanalyzer/standard_analyzer.go` の変更

本タスクのスコープ外（Mach-O のキャッシュ利用は別タスク）。`DynamicLoadSymbols []DetectedSymbol` フィールド追加によるビルドエラーが発生する場合のみ、最小限の修正（`DynamicLoadSymbols` を空のまま返す等）を行う。収集ロジックは実装しない。

## 3. `filevalidator` の変更

### 3.1 `validator.go` — `saveHash` 関数の変更

`AnalyzeNetworkSymbols` の返り値 `Result` で分岐して `NetworkSymbolAnalysis` を設定する：

```go
// 変更後
if v.binaryAnalyzer != nil {
    output := v.binaryAnalyzer.AnalyzeNetworkSymbols(filePath.String(), contentHash)
    switch output.Result {
    case binaryanalyzer.NetworkDetected, binaryanalyzer.NoNetworkSymbols:
        record.NetworkSymbolAnalysis = &fileanalysis.NetworkSymbolAnalysisData{
            AnalyzedAt:         time.Now(),
            HasNetworkSymbols:  output.Result == binaryanalyzer.NetworkDetected,
            DetectedSymbols:    convertDetectedSymbols(output.DetectedSymbols),
            DynamicLoadSymbols: convertDetectedSymbols(output.DynamicLoadSymbols),
        }
    case binaryanalyzer.StaticBinary, binaryanalyzer.NotSupportedBinary:
        // 静的バイナリ・非 ELF: NetworkSymbolAnalysis を記録しない
    case binaryanalyzer.AnalysisError:
        return fmt.Errorf("network symbol analysis failed: %w", output.Error)
    }
}
```

### 3.2 `convertDetectedSymbols` ヘルパー関数

`filevalidator` パッケージ内のパッケージプライベート関数として定義する（`validator.go` または同パッケージ内の別ファイル）：

```go
func convertDetectedSymbols(syms []binaryanalyzer.DetectedSymbol) []fileanalysis.DetectedSymbolEntry {
    if len(syms) == 0 {
        return nil
    }
    entries := make([]fileanalysis.DetectedSymbolEntry, len(syms))
    for i, s := range syms {
        entries[i] = fileanalysis.DetectedSymbolEntry{Name: s.Name, Category: s.Category}
    }
    return entries
}
```

`nil` を返すことで JSON 出力の `omitempty` と整合する。

## 4. `fileanalysis` パッケージの変更（`network_symbol_store.go`）

`syscall_store.go` と同じ adapter パターンを適用する。`Store` に直接メソッドを生やさず、インターフェース + 非公開実装 + ファクトリの3点セットで構成する。

```go
// NetworkSymbolStore defines the interface for loading network symbol analysis results.
// This interface uses fileanalysis types to avoid import cycles with the security package.
type NetworkSymbolStore interface {
    // LoadNetworkSymbolAnalysis loads the cached network symbol analysis for the given file.
    // Returns (data, nil) if found and hash matches.
    // Returns (nil, ErrRecordNotFound) if record not found.
    // Returns (nil, ErrHashMismatch) if hash does not match.
    // Returns (nil, ErrNoNetworkSymbolAnalysis) if no network symbol analysis exists.
    // Returns (nil, error) on other errors.
    LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*NetworkSymbolAnalysisData, error)
}

// networkSymbolStore implements NetworkSymbolStore backed by Store.
type networkSymbolStore struct {
    store *Store
}

// NewNetworkSymbolStore creates a new NetworkSymbolStore backed by Store.
func NewNetworkSymbolStore(store *Store) NetworkSymbolStore {
    return &networkSymbolStore{store: store}
}

func (s *networkSymbolStore) LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*NetworkSymbolAnalysisData, error) {
    resolvedPath, err := common.NewResolvedPath(filePath)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve path: %w", err)
    }
    record, err := s.store.Load(resolvedPath)
    if err != nil {
        return nil, err
    }
    if record.ContentHash != expectedHash {
        return nil, ErrHashMismatch
    }
    if record.NetworkSymbolAnalysis == nil {
        return nil, ErrNoNetworkSymbolAnalysis
    }
    return record.NetworkSymbolAnalysis, nil
}
```

## 5. `security` パッケージの変更

### 5.1 `NetworkAnalyzer` 構造体の拡張（`network_analyzer.go`）

`store` フィールドの型は `fileanalysis.NetworkSymbolStore` を直接使う（`security` パッケージ独自のインターフェースは定義しない）。インポートサイクルが発生しないことを事前に確認済み（`fileanalysis` → `security` の依存がないため）。

```go
type NetworkAnalyzer struct {
    binaryAnalyzer binaryanalyzer.BinaryAnalyzer
    store          fileanalysis.NetworkSymbolStore  // nil の場合はキャッシュ不使用
}
```

### 5.2 コンストラクタ

```go
// NewNetworkAnalyzer creates a NetworkAnalyzer without a store (cache disabled).
func NewNetworkAnalyzer() *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: NewBinaryAnalyzer()}
}

// NewNetworkAnalyzerWithStore creates a NetworkAnalyzer with a store for cache-based analysis.
// If store is nil, falls back to live binary analysis.
func NewNetworkAnalyzerWithStore(store fileanalysis.NetworkSymbolStore) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: NewBinaryAnalyzer(), store: store}
}
```

### 5.3 store 注入チェーン

store は `runner.go` で生成され、コンストラクタ引数として下位層に渡す。

| 呼び出し階層 | ファイル | 変更内容 |
|------------|---------|---------|
| 1. store 生成 | `runner/runner.go` | `createNormalResourceManager()` 内で `fileanalysis.NewStore(hashDir, getter)` から `*fileanalysis.Store` を生成し、`fileanalysis.NewNetworkSymbolStore(store)` で `fileanalysis.NetworkSymbolStore` に変換して `NewDefaultResourceManager()` に渡す |
| 2. DefaultManager | `resource/default_manager.go` | `NewDefaultResourceManager()` シグネチャに `store fileanalysis.NetworkSymbolStore` を追加し、`NewNormalResourceManagerWithOutput()` に渡す |
| 3. NormalManager | `resource/normal_manager.go` | `NewNormalResourceManagerWithOutput()` シグネチャに `store fileanalysis.NetworkSymbolStore` を追加し、`risk.NewStandardEvaluator(store)` に渡す |
| 4. Evaluator | `risk/evaluator.go` | `NewStandardEvaluator(store fileanalysis.NetworkSymbolStore) Evaluator` に引数追加し、`security.NewNetworkAnalyzerWithStore(store)` を呼び出す |
| 5. NetworkAnalyzer | `security/network_analyzer.go` | `NewNetworkAnalyzerWithStore(store fileanalysis.NetworkSymbolStore)` を本番コードに追加 |

**注意**: `NewDefaultResourceManager()` のシグネチャ変更は呼び出し元（`runner.go`、各テストファイル）にも波及する。テストファイルでは `nil` を渡すことで既存動作を維持する。

### 5.4 `isNetworkViaBinaryAnalysis` の変更

キャッシュ参照ロジックを先頭に追加する：

```go
func (a *NetworkAnalyzer) isNetworkViaBinaryAnalysis(cmdPath string, contentHash string) (isNetwork, isHighRisk bool) {
    // キャッシュ参照（store が設定されている場合のみ）
    if a.store != nil {
        if data, err := a.store.LoadNetworkSymbolAnalysis(cmdPath, contentHash); err == nil {
            output := binaryanalyzer.AnalysisOutput{
                DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
                DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
            }
            if data.HasNetworkSymbols {
                output.Result = binaryanalyzer.NetworkDetected
            } else {
                output.Result = binaryanalyzer.NoNetworkSymbols
            }
            // 既存の AnalysisOutput 処理ロジックに渡す
            return handleAnalysisOutput(output, cmdPath)
        }
        // キャッシュミス（ErrNoNetworkSymbolAnalysis, ErrHashMismatch 等）はフォールバック
        // 旧スキーマはここに到達しない（VerifyGroupFiles が ErrGroupVerificationFailed で先にブロックする）
    }

    // フォールバック: 従来の実行時解析
    output := a.binaryAnalyzer.AnalyzeNetworkSymbols(cmdPath, contentHash)
    return handleAnalysisOutput(output, cmdPath)
}
```

`convertNetworkSymbolEntries` は `fileanalysis.DetectedSymbolEntry` → `binaryanalyzer.DetectedSymbol` の逆変換ヘルパーで、`security` パッケージ内のパッケージプライベート関数として定義する。

### 5.5 テスト用ヘルパーの追加（`network_analyzer_test_helpers.go`）

`NewNetworkAnalyzerWithStore` は本番コード（`network_analyzer.go`）に定義済みのため、テストヘルパーには定義しない。テストヘルパーには BinaryAnalyzer と store を両方差し替えられる関数のみを追加する。

```go
// NewNetworkAnalyzerWithBinaryAnalyzerAndStore creates a NetworkAnalyzer
// with both a custom BinaryAnalyzer and store for testing.
// This function is only available in test builds.
func NewNetworkAnalyzerWithBinaryAnalyzerAndStore(
    analyzer binaryanalyzer.BinaryAnalyzer,
    store fileanalysis.NetworkSymbolStore,
) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: analyzer, store: store}
}
```

## 6. テスト仕様

### 6.1 アナライザーレベルのテスト（`elfanalyzer/standard_analyzer_test.go`）

`machoanalyzer` は本タスクのスコープ外のためテスト追加不要。

| テストケース | 入力 | 期待結果 | 対応 AC |
|------------|------|---------|--------|
| `dlopen` のみを持つ ELF | `dlopen` が `.dynsym` に存在 | `DynamicLoadSymbols: [{dlopen, dynamic_load}]`、`DetectedSymbols: nil` | AC-2 |
| `dlsym` と `dlvsym` を両方持つ ELF | `dlsym`, `dlvsym` が `.dynsym` に存在 | `DynamicLoadSymbols` に両シンボルが列挙される | AC-2 |
| ネットワークシンボルと `dlopen` を同時に持つ ELF | `socket`, `dlopen` が `.dynsym` に存在 | `DetectedSymbols` と `DynamicLoadSymbols` が独立して設定される | AC-2 |
| dynamic_load シンボルを持たない ELF | dynamic_load シンボルなし | `DynamicLoadSymbols: nil` | AC-2 |

### 6.2 `record` 拡張のテスト（`filevalidator/validator_test.go`）

| テストケース | 入力 | 期待結果 | 対応 AC |
|------------|------|---------|--------|
| ネットワークシンボルありの動的 ELF | `Result: NetworkDetected`、`DetectedSymbols: [socket]` | `NetworkSymbolAnalysis.HasNetworkSymbols: true`、`DetectedSymbols` に socket を含む | AC-2 |
| ネットワークシンボルなしの動的 ELF | `Result: NoNetworkSymbols` | `NetworkSymbolAnalysis.HasNetworkSymbols: false`、`DetectedSymbols` が空 | AC-2 |
| `dlopen` のみを持つ動的 ELF | `DynamicLoadSymbols: [dlopen]` | `NetworkSymbolAnalysis.DynamicLoadSymbols` に dlopen を含む | AC-2 |
| 非 ELF ファイル | `Result: NotSupportedBinary` | `NetworkSymbolAnalysis` が `nil` | AC-2 |
| 静的 ELF バイナリ | `Result: StaticBinary` | `NetworkSymbolAnalysis` が `nil` | AC-2 |
| `AnalysisError` | `Result: AnalysisError` | `record` がエラーを返し記録が保存されない | AC-2 |

### 6.3 `runner` キャッシュ利用のテスト（`security/command_analysis_test.go`）

| テストケース | 入力 | 期待結果 | 対応 AC |
|------------|------|---------|--------|
| キャッシュあり・`HasNetworkSymbols: true` | store に `HasNetworkSymbols: true` のデータを設定 | `NetworkDetected` が返され、`BinaryAnalyzer` は呼ばれない | AC-3 |
| キャッシュあり・`HasNetworkSymbols: false` | store に `HasNetworkSymbols: false` のデータを設定 | `NoNetworkSymbols` が返され、`BinaryAnalyzer` は呼ばれない | AC-3 |
| キャッシュなし（`ErrNoNetworkSymbolAnalysis`） | store が `ErrNoNetworkSymbolAnalysis` を返す | `BinaryAnalyzer.AnalyzeNetworkSymbols()` にフォールバック | AC-3 |
| キャッシュあり・`DynamicLoadSymbols` に `dlopen` を含む | `DynamicLoadSymbols: [dlopen]` | `isHighRisk: true` が返される | AC-3 |

### 6.4 統合テスト（`filevalidator/validator_test.go` または `runner` 統合テスト）

| テストケース | 検証内容 | 対応 AC |
|------------|---------|--------|
| `record` → `runner` の正常フロー | キャッシュを利用して正しく判定される | AC-3 |
| 旧スキーマ（`schema_version: 2`）の記録で実行 | `VerifyGroupFiles` が group verification failed を返して実行前に停止する | AC-4 |

## 7. 受け入れ条件との対応

| AC-ID | 要件 | 実装箇所 |
|-------|------|---------|
| AC-1 | `NetworkSymbolAnalysisData` 型の定義と `Record` フィールド追加、`HasDynamicLoad` 削除、スキーマバージョン更新 | § 1.2、§ 1.3 |
| AC-2 | `record` 時の `NetworkSymbolAnalysis` 記録 | § 2.1、§ 2.2、§ 3.1 |
| AC-3 | `runner` 時のキャッシュ利用と `isHighRisk` 導出 | § 4.1、§ 5.4 |
| AC-4 | 旧スキーマの拒否（`VerifyGroupFiles` が `ErrGroupVerificationFailed` を内包する `verification.Error` を返す） | § 1.2.3 |
| AC-5 | 既存機能（`commandProfileDefinitions`、静的バイナリフロー、`DynLibDeps`）への非影響 | store が `nil` の場合のフォールバック（§ 5.2） |

## 8. 変更ファイル一覧

| ファイル | 変更種別 | 内容 |
|---------|---------|------|
| `internal/runner/security/binaryanalyzer/analyzer.go` | 変更 | `AnalysisOutput` に `DynamicLoadSymbols []DetectedSymbol` フィールドを追加し、`HasDynamicLoad bool` を削除 |
| `internal/runner/security/elfanalyzer/standard_analyzer.go` | 変更 | `checkDynamicSymbols()` 内で dynamic_load シンボル名を収集し `AnalysisOutput.DynamicLoadSymbols` に設定 |
| `internal/runner/security/machoanalyzer/standard_analyzer.go` | 変更（最小限） | `DynamicLoadSymbols` フィールド追加に伴うビルド維持のみ。収集ロジックの実装は対象外（別タスク） |
| `internal/fileanalysis/schema.go` | 変更 | `NetworkSymbolAnalysisData` / `DetectedSymbolEntry` 型追加（`DynamicLoadSymbols` フィールド含む）、`HasDynamicLoad` フィールド削除、`CurrentSchemaVersion` を 3 に更新 |
| `internal/fileanalysis/errors.go` | 変更 | `ErrNoNetworkSymbolAnalysis` エラー変数を追加 |
| `internal/fileanalysis/network_symbol_store.go` | 新規 | `syscall_store.go` と同じ adapter パターンで `NetworkSymbolStore` インターフェース・`networkSymbolStore` 非公開実装・`NewNetworkSymbolStore` ファクトリを定義 |
| `internal/filevalidator/validator.go` | 変更 | `saveHash` 内の `binaryAnalyzer` 呼び出しを拡張、`NetworkSymbolAnalysis` を保存 |
| `internal/runner/security/network_analyzer.go` | 変更 | `NetworkAnalyzer` に `NetworkSymbolStore` を追加、`isNetworkViaBinaryAnalysis` にキャッシュ参照ロジックを追加 |
| `internal/runner/security/network_analyzer_test_helpers.go` | 変更 | store ありのテスト用ヘルパー追加 |
| `internal/runner/risk/evaluator.go` | 変更 | `NewStandardEvaluator()` に `store fileanalysis.NetworkSymbolStore` 引数を追加 |
| `internal/runner/resource/normal_manager.go` | 変更 | `NewNormalResourceManagerWithOutput()` シグネチャに `store fileanalysis.NetworkSymbolStore` 引数を追加し、`risk.NewStandardEvaluator(store)` に渡す |
| `internal/runner/resource/default_manager.go` | 変更 | `NewDefaultResourceManager()` シグネチャに `store fileanalysis.NetworkSymbolStore` 引数を追加し、`NewNormalResourceManagerWithOutput()` に渡す |
| `internal/runner/runner.go` | 変更 | `createNormalResourceManager()` 内で `fileanalysis.Store` を生成し `NewDefaultResourceManager()` に渡す |
