# 動的リンクライブラリの再帰的システムコール解析 アーキテクチャ設計書

## 1. 設計目標

- `record` 時に実行ファイルの DynLibDeps（推移的依存含む）をライブラリ単位で解析し、
  ネットワーク系システムコールを検出する
- 既存の解析エンジン（`SyscallAnalyzerInterface`・`binaryanalyzer.BinaryAnalyzer`）を再利用し、
  新規アルゴリズムを最小限に抑える
- セッション内キャッシュで重複解析を防ぎ、`record` コマンドの実行時間増加を抑制する
- runner 側の判定ロジック変更を最小限に抑える（既存の `SymbolAnalysisData` フィールドを拡張）

---

## 2. 全体フロー

### 2.1 record コマンドの解析フロー（変更後）

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    BIN[("実行ファイル\n(ELF)")] --> DEPS["analyzeDynLibDeps()\n既存: 推移的依存解決"]
    DEPS --> DYNLIB[("record.DynLibDeps\n全推移的ライブラリ")]
    DYNLIB --> SYM["AnalyzeNetworkSymbols()\n既存: .dynsym UNDEF 解析"]
    SYM --> SYMA[("record.SymbolAnalysis\n既存フィールド")]
    DYNLIB --> KNOWN["SOName 既知リスト照合\n既存: KnownNetworkLibDeps"]
    KNOWN --> SYMA

    DYNLIB --> LIBANA["analyzeLibraries()\n新規"]
    LIBANA --> WRAP{"syscall wrapper\nSOName?"}
    WRAP -->|"Yes (libc等)"| SKIP["スキップ"]
    WRAP -->|"No"| CACHE{"セッション\nキャッシュ?"}
    CACHE -->|"Hit"| ENTRY[("LibraryAnalysisEntry")]
    CACHE -->|"Miss"| ANALYZE["ライブラリ解析\n.dynsym + syscall命令"]
    ANALYZE --> ENTRY
    ENTRY --> LIBDEPS[("record.LibraryAnalysis\n新規フィールド")]
    ENTRY --> SUMMARY["ネットワーク検出?\n→ SOName を収集"]
    SUMMARY --> NETDEPS["record.SymbolAnalysis\n.DetectedLibraryNetworkDeps\n新規フィールド"]

    class BIN,DYNLIB,SYMA,LIBDEPS,NETDEPS data;
    class DEPS,SYM,KNOWN,LIBANA,WRAP,CACHE,ANALYZE,SUMMARY process;
    class ENTRY enhanced;
```

**凡例（Legend）**

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    D1[("データ")] --> P1["既存コンポーネント"] --> E1["新規 / 拡張コンポーネント"]
    class D1 data
    class P1 process
    class E1 enhanced
```

### 2.2 ライブラリ単体の解析フロー

```mermaid
flowchart LR
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    LIBPATH[("lib.Path\n(解決済みパス)")] --> OPEN["safefileio.SafeOpenFile()"]
    OPEN --> DYNSYM["BinaryAnalyzer\n.AnalyzeNetworkSymbols(path, '')"]
    OPEN --> ELFOPEN["elf.NewFile()"]
    ELFOPEN --> SYSCALL["SyscallAnalyzerInterface\n.AnalyzeSyscallsFromELF(elfFile)"]
    DYNSYM --> SYMOUT[("SymbolAnalysisData\n{DetectedSymbols}")]
    SYSCALL --> SYSOUT[("SyscallAnalysisData\n{DetectedSyscalls}")]
    SYMOUT --> ENTRY[("LibraryAnalysisEntry\n{SOName, Path,\nSymbolAnalysis,\nSyscallAnalysis}")]
    SYSOUT --> ENTRY

    class LIBPATH,SYMOUT,SYSOUT,ENTRY data;
    class OPEN,DYNSYM,ELFOPEN,SYSCALL process;
```

### 2.3 runner のリスク判定フロー（変更後）

```mermaid
flowchart TD
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    STORE[("fileanalysis.Store")] --> SYMLOAD["LoadNetworkSymbolAnalysis()"]
    SYMLOAD --> DATA[("SymbolAnalysisData")]
    DATA --> CHECK1{"DetectedSymbols\nに network?"}
    DATA --> CHECK2{"KnownNetworkLibDeps\n非空?"}
    DATA --> CHECK3{"DetectedLibraryNetworkDeps\n非空? (新規)"}
    CHECK1 -->|"Yes"| NET["ネットワーク有り"]
    CHECK2 -->|"Yes"| NET
    CHECK3 -->|"Yes"| NET
    CHECK1 -->|"No"| AND{"全て No?"}
    CHECK2 -->|"No"| AND
    CHECK3 -->|"No"| AND
    AND --> NONNET["ネットワークなし"]

    class STORE,DATA data;
    class SYMLOAD,CHECK1,CHECK2,CHECK3,AND process;
    class NET,NONNET enhanced;
```

---

## 3. コンポーネント変更一覧

```mermaid
graph TB
    classDef data fill:#e6f7ff,stroke:#1f77b4,stroke-width:1px,color:#0b3d91;
    classDef process fill:#fff1e6,stroke:#ff7f0e,stroke-width:1px,color:#8a3e00;
    classDef enhanced fill:#e8f5e8,stroke:#2e8b57,stroke-width:2px,color:#006400;

    subgraph fileanalysis["internal/fileanalysis"]
        SCHEMA["schema.go\n+ LibraryAnalysisEntry 型\n+ Record.LibraryAnalysis フィールド\n+ SymbolAnalysisData.DetectedLibraryNetworkDeps フィールド\n+ CurrentSchemaVersion 19→20"]
    end

    subgraph binaryanalyzer["internal/security/binaryanalyzer"]
        WRAP["syscall_wrapper_libs.go (新規)\n+ syscallWrapperPrefixes リスト\n+ IsSyscallWrapperLibrary() 関数"]
    end

    subgraph filevalidator["internal/filevalidator"]
        VAL["validator.go\n+ Validator.libraryAnalysisCache フィールド\n+ analyzeLibraries() メソッド\n+ SetLibraryAnalysisEnabled() メソッド"]
    end

    subgraph runner_security["internal/runner/base/security"]
        NET["network_analyzer.go\n+ DetectedLibraryNetworkDeps チェック追加"]
    end

    WRAP -->|"uses"| VAL
    SCHEMA -->|"uses"| VAL
    SCHEMA -->|"uses"| NET

    class SCHEMA data;
    class WRAP,VAL,NET enhanced;
```

---

## 4. データ構造の変更

### 4.1 新規型: `LibraryAnalysisEntry`（`internal/fileanalysis/schema.go`）

```go
type LibraryAnalysisEntry struct {
    SOName          string               `json:"soname"`
    Path            string               `json:"path"`
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
    SymbolAnalysis  *SymbolAnalysisData  `json:"symbol_analysis,omitempty"`
}
```

### 4.2 `Record` への追加（`internal/fileanalysis/schema.go`）

```go
// LibraryAnalysis contains per-library analysis results for application libraries
// (excluding syscall wrappers) found in DynLibDeps.
LibraryAnalysis []LibraryAnalysisEntry `json:"library_analysis,omitempty"`
```

### 4.3 `SymbolAnalysisData` への追加（`internal/fileanalysis/schema.go`）

```go
// DetectedLibraryNetworkDeps lists SOName values of application libraries
// in which network syscalls or symbols were detected via library-level analysis.
DetectedLibraryNetworkDeps []string `json:"detected_library_network_deps,omitempty"`
```

### 4.4 スキーマバージョン: 19 → 20

---

## 5. 新規コンポーネント: `syscall_wrapper_libs.go`

### 5.1 配置パッケージ

`internal/security/binaryanalyzer/syscall_wrapper_libs.go`（新規）

既存の `known_network_libs.go` と同パッケージに置く。

`matchesKnownPrefix()` ヘルパーは既存の `known_network_libs.go` にある。
`IsSyscallWrapperLibrary` はこの関数を再利用してプレフィックス照合を行う。

### 5.2 公開 API

```go
func IsSyscallWrapperLibrary(soname string) bool
```

---

## 6. `Validator` への追加

### 6.1 セッション内キャッシュ

`Validator` に以下のフィールドを追加する。

```go
libraryAnalysisCache    map[string]*fileanalysis.LibraryAnalysisEntry  // keyed by resolved path
libraryAnalysisEnabled  bool
```

### 6.2 `analyzeLibraries()` メソッド

`updateAnalysisRecord` 内で `analyzeELFSyscalls` の直前に呼び出す。

```
Input:  record *fileanalysis.Record（DynLibDeps 設定済み）
Output: record.LibraryAnalysis および record.SymbolAnalysis.DetectedLibraryNetworkDeps を設定
```

処理フロー:
1. `libraryAnalysisEnabled` が false → 早期リターン
2. `record.DynLibDeps` をイテレート
3. VDSO (`knownVDSOs`) は除外
4. `IsSyscallWrapperLibrary(lib.SOName)` が true → 除外
5. `libraryAnalysisCache[lib.Path]` にヒット → キャッシュから取得
6. キャッシュミス → `analyzeOneLibrary(lib)` を呼ぶ → キャッシュに格納
7. 結果を `record.LibraryAnalysis` に追加
8. ネットワーク系 syscall またはシンボルが検出された場合は SOName を `DetectedLibraryNetworkDeps` に追加

### 6.3 `analyzeOneLibrary()` メソッド

```
Input:  lib fileanalysis.LibEntry
Output: *fileanalysis.LibraryAnalysisEntry, error
```

処理フロー:
1. `v.binaryAnalyzer.AnalyzeNetworkSymbols(lib.Path, "")` で `.dynsym` 解析（空のハッシュを渡して syscall store lookup を無効化）
2. ELF ファイルを `openELFFile` で開く（存在しない・ELF でない場合は SymbolAnalysis のみ）
3. ELF ファイルが開けた場合: `v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)` で機械語解析
4. 結果を `LibraryAnalysisEntry` に詰めて返す

エラー処理: ファイルオープン失敗・機械語解析エラーは `AnalysisWarnings` に追記して継続。

### 6.4 `SetLibraryAnalysisEnabled()` メソッド

```go
func (v *Validator) SetLibraryAnalysisEnabled(enabled bool)
```

`cmd/record/main.go` で依存注入時に呼び出す。デフォルトは `false`（既存の動作を維持）。

---

## 7. runner 側の変更

`internal/runner/base/security/network_analyzer.go` の
`isNetworkViaBinaryAnalysis` の `SymbolAnalysisData` 判定部分に
`DetectedLibraryNetworkDeps` のチェックを追加する。

既存の `KnownNetworkLibDeps` チェックと同じ位置・同じ判定基準で追加する。

---

## 8. 有効化フロー（cmd/record/main.go）

```
既存: v.SetELFDynLibAnalyzer(...)
既存: v.SetBinaryAnalyzer(...)
既存: v.SetSyscallAnalyzer(...)
新規: v.SetLibraryAnalysisEnabled(true)  // 新しい SetLibraryAnalysisEnabled を呼ぶ
```
