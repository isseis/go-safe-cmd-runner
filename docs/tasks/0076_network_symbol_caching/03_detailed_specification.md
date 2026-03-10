# ネットワークシンボル解析結果のキャッシュ 詳細仕様書

## 0. 前提と参照

- 要件定義: [01_requirements.md](01_requirements.md)
- アーキテクチャ設計: [02_architecture.md](02_architecture.md)

本仕様書は `SyscallAnalysis`（タスク 0070/0072）の実装パターンを踏襲する。既存の `fileanalysis.syscall_store.go` および `fileanalysis/errors.go` を参照し、重複実装を避ける。

## 1. 型定義

### 1.1 `binaryanalyzer.AnalysisOutput` の変更（`analyzer.go`）

`DynamicLoadSymbols` フィールドを追加する。`HasDynamicLoad` は後方互換性のために残す。

```go
type AnalysisOutput struct {
    Result          AnalysisResult
    DetectedSymbols []DetectedSymbol
    HasDynamicLoad  bool             // 後方互換。len(DynamicLoadSymbols) > 0 と常に等価
    DynamicLoadSymbols []DetectedSymbol  // 追加
    Error           error
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

### 1.4 `security.NetworkSymbolStore` インターフェース（`network_analyzer.go`）

```go
// NetworkSymbolStore provides cached network symbol analysis results.
type NetworkSymbolStore interface {
    LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*fileanalysis.NetworkSymbolAnalysisData, error)
}
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
            HasDynamicLoad:     len(dynamicLoadSyms) > 0,
            DynamicLoadSymbols: dynamicLoadSyms,
        }
    }

    return binaryanalyzer.AnalysisOutput{
        Result:             binaryanalyzer.NoNetworkSymbols,
        HasDynamicLoad:     len(dynamicLoadSyms) > 0,
        DynamicLoadSymbols: dynamicLoadSyms,
    }
}
```

### 2.2 `machoanalyzer/standard_analyzer.go` の変更

`hasDynamicLoad = true` とする箇所でシンボル名も収集し、返す `AnalysisOutput` の `DynamicLoadSymbols` に設定する。ELF と同様のパターンを適用する（`var dynamicLoadSyms []binaryanalyzer.DetectedSymbol` を宣言し、検出時に `append`）。

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

## 4. `fileanalysis.Store` の変更

### 4.1 `LoadNetworkSymbolAnalysis` メソッド（`store.go`）

`SyscallAnalysisStore.LoadSyscallAnalysis` の実装に倣う：

```go
// LoadNetworkSymbolAnalysis loads the cached network symbol analysis for the given file.
// Returns (nil, ErrHashMismatch) if the hash does not match the stored record.
// Returns (nil, ErrNoNetworkSymbolAnalysis) if no network symbol analysis exists in the record.
func (s *Store) LoadNetworkSymbolAnalysis(filePath string, expectedHash string) (*NetworkSymbolAnalysisData, error) {
    record, err := s.loadRecord(filePath)
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

```go
type NetworkAnalyzer struct {
    binaryAnalyzer binaryanalyzer.BinaryAnalyzer
    store          NetworkSymbolStore  // nil の場合はキャッシュ不使用
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
func NewNetworkAnalyzerWithStore(store NetworkSymbolStore) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: NewBinaryAnalyzer(), store: store}
}
```

### 5.3 store 注入チェーン

| 呼び出し階層 | 変更内容 |
|------------|---------|
| `resource/normal_manager.go` | `risk.NewStandardEvaluator(analysisStore)` に変更（`analysisStore` は既存の `fileanalysis.Store` インスタンスを流用） |
| `risk/evaluator.go` | `NewStandardEvaluator(store security.NetworkSymbolStore) Evaluator` に引数追加 |
| `security/network_analyzer.go` | `NewNetworkAnalyzerWithStore(store)` を新規追加 |

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
                HasDynamicLoad:     len(data.DynamicLoadSymbols) > 0,
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
    }

    // フォールバック: 従来の実行時解析
    output := a.binaryAnalyzer.AnalyzeNetworkSymbols(cmdPath, contentHash)
    return handleAnalysisOutput(output, cmdPath)
}
```

`convertNetworkSymbolEntries` は `fileanalysis.DetectedSymbolEntry` → `binaryanalyzer.DetectedSymbol` の逆変換ヘルパーで、`security` パッケージ内のパッケージプライベート関数として定義する。

### 5.5 テスト用ヘルパーの追加（`network_analyzer_test_helpers.go`）

```go
// NewNetworkAnalyzerWithStore creates a NetworkAnalyzer with a custom store for testing.
// This function is only available in test builds.
func NewNetworkAnalyzerWithStore(store NetworkSymbolStore) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: NewBinaryAnalyzer(), store: store}
}

// NewNetworkAnalyzerWithBinaryAnalyzerAndStore creates a NetworkAnalyzer
// with both a custom BinaryAnalyzer and store for testing.
// This function is only available in test builds.
func NewNetworkAnalyzerWithBinaryAnalyzerAndStore(
    analyzer binaryanalyzer.BinaryAnalyzer,
    store NetworkSymbolStore,
) *NetworkAnalyzer {
    return &NetworkAnalyzer{binaryAnalyzer: analyzer, store: store}
}
```

## 6. テスト仕様

### 6.1 アナライザーレベルのテスト（`elfanalyzer/standard_analyzer_test.go`、`machoanalyzer/standard_analyzer_test.go`）

| テストケース | 入力 | 期待結果 | 対応 AC |
|------------|------|---------|--------|
| `dlopen` のみを持つバイナリ | `dlopen` が `.dynsym` に存在 | `DynamicLoadSymbols: [{dlopen, dynamic_load}]`、`HasDynamicLoad: true`、`DetectedSymbols: nil` | AC-2 |
| `dlsym` と `dlvsym` を両方持つバイナリ | `dlsym`, `dlvsym` が `.dynsym` に存在 | `DynamicLoadSymbols` に両シンボルが列挙される | AC-2 |
| ネットワークシンボルと `dlopen` を同時に持つバイナリ | `socket`, `dlopen` が `.dynsym` に存在 | `DetectedSymbols` と `DynamicLoadSymbols` が独立して設定される | AC-2 |
| dynamic_load シンボルを持たないバイナリ | dynamic_load シンボルなし | `DynamicLoadSymbols: nil`、`HasDynamicLoad: false` | AC-2 |

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
| 旧スキーマ（`schema_version: 2`）の記録で実行 | `SchemaVersionMismatchError` でブロックされる | AC-4 |

## 7. 受け入れ条件との対応

| AC-ID | 要件 | 実装箇所 |
|-------|------|---------|
| AC-1 | `NetworkSymbolAnalysisData` 型の定義と `Record` フィールド追加、`HasDynamicLoad` 削除、スキーマバージョン更新 | § 1.2、§ 1.3 |
| AC-2 | `record` 時の `NetworkSymbolAnalysis` 記録 | § 2.1、§ 2.2、§ 3.1 |
| AC-3 | `runner` 時のキャッシュ利用と `isHighRisk` 導出 | § 4.1、§ 5.4 |
| AC-4 | 旧スキーマの拒否（既存の `SchemaVersionMismatchError` 機構で対応） | § 1.2.3 |
| AC-5 | 既存機能（`commandProfileDefinitions`、静的バイナリフロー、`DynLibDeps`）への非影響 | store が `nil` の場合のフォールバック（§ 5.2） |
