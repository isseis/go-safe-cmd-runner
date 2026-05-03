# 動的リンクライブラリの再帰的システムコール解析 詳細仕様書

## 1. 変更ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|----------|------|
| `internal/fileanalysis/schema.go` | 変更 | `LibraryAnalysisEntry` 型追加、`Record.LibraryAnalysis` フィールド追加、`SymbolAnalysisData.DetectedLibraryNetworkDeps` フィールド追加、スキーマバージョン更新 |
| `internal/security/binaryanalyzer/syscall_wrapper_libs.go` | 新規 | `IsSyscallWrapperLibrary()` 関数と除外リスト |
| `internal/filevalidator/validator.go` | 変更 | `analyzeLibraries()` と `analyzeOneLibrary()` メソッド追加、`Validator` にキャッシュフィールド追加 |
| `internal/runner/base/security/network_analyzer.go` | 変更 | `DetectedLibraryNetworkDeps` チェック追加 |
| `cmd/record/main.go` | 変更 | `SetLibraryAnalysisEnabled(true)` 呼び出し追加 |

---

## 2. `internal/fileanalysis/schema.go`

### 2.1 スキーマバージョン

```go
// Version 20 adds LibraryAnalysis to Record for per-library syscall/symbol
// analysis results, and DetectedLibraryNetworkDeps to SymbolAnalysisData
// summarising which application libraries had network activity detected.
CurrentSchemaVersion = 20
```

### 2.2 `LibraryAnalysisEntry` 型（新規）

```go
// LibraryAnalysisEntry holds syscall and symbol analysis results for a single
// application-level dynamic library (i.e. not a syscall wrapper such as libc).
type LibraryAnalysisEntry struct {
    // SOName is the DT_NEEDED library name (e.g. "libfoo.so.1").
    SOName string `json:"soname"`

    // Path is the resolved full path to the library file.
    Path string `json:"path"`

    // SyscallAnalysis contains machine-code syscall instruction scan results.
    // nil when the library is not ELF or machine-code analysis was skipped.
    SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`

    // SymbolAnalysis contains .dynsym UNDEF network symbol detection results.
    // nil when symbol analysis was skipped or returned no result.
    SymbolAnalysis *SymbolAnalysisData `json:"symbol_analysis,omitempty"`
}
```

### 2.3 `Record` への追加

```go
// LibraryAnalysis contains per-library analysis results for application libraries
// (excluding syscall wrappers such as libc) found in DynLibDeps.
// nil/empty means library analysis was not performed or found nothing notable.
LibraryAnalysis []LibraryAnalysisEntry `json:"library_analysis,omitempty"`
```

既存フィールド `AnalysisWarnings` の直後に追加する。

### 2.4 `SymbolAnalysisData` への追加

```go
// DetectedLibraryNetworkDeps lists SOName values of application libraries
// in which network-related syscalls or symbols were detected during
// library-level machine-code / dynsym analysis.
// Non-empty means this binary should be treated as network-capable via a
// dependent library.
DetectedLibraryNetworkDeps []string `json:"detected_library_network_deps,omitempty"`
```

---

## 3. `internal/security/binaryanalyzer/syscall_wrapper_libs.go`（新規）

### 3.1 ファイル全体

```go
package binaryanalyzer

// syscallWrapperPrefixes lists SOName prefixes for OS-ABI syscall wrapper
// libraries that are excluded from library-level analysis.
// Matching uses the same safe prefix rule as knownNetworkLibPrefixes:
// the SOName must start with the prefix followed by '.', '-', or a digit.
var syscallWrapperPrefixes = []string{
    "libc",
    "libpthread",
    "libdl",
    "librt",
    "libgcc_s",
    "ld-linux",
    "ld-linux-x86-64",
    "ld-linux-aarch64",
    "linux-vdso",
}

// IsSyscallWrapperLibrary reports whether soname is a known OS-ABI syscall
// wrapper that should be excluded from library-level syscall analysis.
// The SOName must start with the prefix followed immediately by '.', '-', or
// a digit (e.g. "libc.so.6" matches "libc"; "libcpp.so.1" does not).
func IsSyscallWrapperLibrary(soname string) bool {
    for _, prefix := range syscallWrapperPrefixes {
        if matchesKnownPrefix(soname, prefix) {
            return true
        }
    }
    return false
}
```

`matchesKnownPrefix` は既存の `known_network_libs.go` に定義されており、同パッケージ内から
直接呼び出せる。

---

## 4. `internal/filevalidator/validator.go`

### 4.1 `Validator` 構造体への追加フィールド

```go
// libraryCacheEntry holds the analysis result and network-activity flag for
// a single library in the session cache.
type libraryCacheEntry struct {
    entry      fileanalysis.LibraryAnalysisEntry
    hasNetwork bool
}

// libraryAnalysisCache caches per-library analysis results within a single
// record session (keyed by resolved library path).
libraryAnalysisCache   map[string]libraryCacheEntry

// libraryAnalysisEnabled controls whether analyzeLibraries() is called.
// Disabled by default to preserve backward-compatible behaviour.
libraryAnalysisEnabled bool
```

### 4.2 `SetLibraryAnalysisEnabled` メソッド（新規）

```go
// SetLibraryAnalysisEnabled enables or disables library-level syscall and
// symbol analysis during SaveRecord. Default is false.
// Call before the first SaveRecord() invocation.
func (v *Validator) SetLibraryAnalysisEnabled(enabled bool) {
    v.libraryAnalysisEnabled = enabled
    if enabled && v.libraryAnalysisCache == nil {
        v.libraryAnalysisCache = make(map[string]libraryCacheEntry)
    }
}
```

### 4.3 `updateAnalysisRecord` への呼び出し追加

既存の `KnownNetworkLibDeps` ブロックの直後、`analyzeELFSyscalls` の直前に以下を追加する。

```go
// Analyze application libraries for network-related syscalls and symbols.
if err := v.analyzeLibraries(record); err != nil {
    return err
}
```

### 4.4 `analyzeLibraries` メソッド（新規）

```go
// analyzeLibraries runs syscall and symbol analysis on each application-level
// library in record.DynLibDeps and populates record.LibraryAnalysis and
// record.SymbolAnalysis.DetectedLibraryNetworkDeps.
//
// Libraries that are syscall wrappers (libc, ld-linux, etc.) or VDSO are
// skipped. Results are cached by resolved path within the Validator lifetime
// to avoid re-analysing the same library for multiple recorded binaries.
//
// Non-fatal errors (file not found, unsupported arch) are appended to
// record.AnalysisWarnings; fatal I/O errors are returned.
func (v *Validator) analyzeLibraries(record *fileanalysis.Record) error {
    if !v.libraryAnalysisEnabled {
        return nil
    }
    if len(record.DynLibDeps) == 0 {
        return nil
    }

    var entries []fileanalysis.LibraryAnalysisEntry
    var networkSONames []string

    for _, lib := range record.DynLibDeps {
        // Skip VDSO: no filesystem path.
        if _, isVDSO := knownVDSOs[lib.SOName]; isVDSO {
            continue
        }
        // Skip syscall wrapper libraries.
        if binaryanalyzer.IsSyscallWrapperLibrary(lib.SOName) {
            continue
        }

        // Use cached result when available.
        if cached, ok := v.libraryAnalysisCache[lib.Path]; ok {
            entries = append(entries, cached.entry)
            if cached.hasNetwork {
                networkSONames = append(networkSONames, lib.SOName)
            }
            continue
        }

        entry, hasNetwork, warnings, err := v.analyzeOneLibrary(lib)
        if err != nil {
            return err
        }
        for _, w := range warnings {
            record.AnalysisWarnings = append(record.AnalysisWarnings, w)
        }

        v.libraryAnalysisCache[lib.Path] = libraryCacheEntry{entry: *entry, hasNetwork: hasNetwork}
        entries = append(entries, *entry)
        if hasNetwork {
            networkSONames = append(networkSONames, lib.SOName)
        }
    }

    if len(entries) > 0 {
        record.LibraryAnalysis = entries
    }

    if len(networkSONames) > 0 {
        slices.Sort(networkSONames)
        if record.SymbolAnalysis == nil {
            record.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{}
        }
        record.SymbolAnalysis.DetectedLibraryNetworkDeps = networkSONames
    }

    if len(record.AnalysisWarnings) > 0 {
        slices.Sort(record.AnalysisWarnings)
    }

    return nil
}
```

`knownVDSOs` は既存の `internal/dynlib/elfdynlib/analyzer.go` で定義されている
パッケージ非公開変数である。`analyzeLibraries` は `filevalidator` パッケージ内に置くため、
同変数を直接参照できない。

**対処**: `filevalidator` 内に `isKnownVDSO(soname string) bool` ヘルパを追加する。
実装は固定文字列集合との比較で十分であり、`elfdynlib` への依存を追加しない。

```go
// isKnownVDSO reports whether soname is a Linux virtual DSO that has no
// corresponding filesystem path and should be skipped during library analysis.
func isKnownVDSO(soname string) bool {
    switch soname {
    case "linux-vdso.so.1", "linux-gate.so.1", "linux-vdso64.so.1":
        return true
    }
    return false
}
```

`analyzeLibraries` 内の VDSO チェックはこの関数を使う。

### 4.5 `analyzeOneLibrary` メソッド（新規）

```go
// analyzeOneLibrary runs symbol and syscall analysis on a single library.
// hasNetwork is true when the library shows evidence of network activity in
// either its .dynsym or machine-code syscall results.
// Returns the entry, the network flag, non-fatal warnings, and any fatal error.
func (v *Validator) analyzeOneLibrary(lib fileanalysis.LibEntry) (
    entry *fileanalysis.LibraryAnalysisEntry,
    hasNetwork bool,
    warnings []string,
    err error,
) {
    entry = &fileanalysis.LibraryAnalysisEntry{
        SOName: lib.SOName,
        Path:   lib.Path,
    }

    // .dynsym UNDEF symbol analysis.
    // Pass empty contentHash to disable syscall store lookup for library files.
    if v.binaryAnalyzer != nil {
        output := v.binaryAnalyzer.AnalyzeNetworkSymbols(lib.Path, "")
        switch output.Result {
        case binaryanalyzer.NetworkDetected:
            entry.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
                DetectedSymbols:    convertDetectedSymbols(output.DetectedSymbols),
                DynamicLoadSymbols: convertDetectedSymbols(output.DynamicLoadSymbols),
            }
            hasNetwork = true
        case binaryanalyzer.NoNetworkSymbols:
            entry.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
                DetectedSymbols:    convertDetectedSymbols(output.DetectedSymbols),
                DynamicLoadSymbols: convertDetectedSymbols(output.DynamicLoadSymbols),
            }
        case binaryanalyzer.AnalysisError:
            warnings = append(warnings,
                fmt.Sprintf("library symbol analysis failed for %s: %v", lib.SOName, output.Error))
        }
        // StaticBinary / NotSupportedBinary: leave SymbolAnalysis nil.
    }

    // Machine-code syscall instruction scan.
    if v.syscallAnalyzer != nil {
        elfFile, openErr := openELFFile(v.fileSystem, lib.Path)
        if openErr != nil {
            if !errors.Is(openErr, errNotELF) {
                // Unexpected I/O error: warn but do not abort.
                warnings = append(warnings,
                    fmt.Sprintf("failed to open library ELF %s: %v", lib.SOName, openErr))
            }
            // errNotELF: non-ELF library; skip machine-code analysis.
        } else {
            defer func() { _ = elfFile.Close() }()
            syscalls, _, _, analyzeErr := v.syscallAnalyzer.AnalyzeSyscallsFromELF(elfFile)
            if analyzeErr != nil {
                if !errors.Is(analyzeErr, ErrUnsupportedArch) {
                    warnings = append(warnings,
                        fmt.Sprintf("syscall analysis failed for library %s: %v", lib.SOName, analyzeErr))
                }
            } else if len(syscalls) > 0 {
                entry.SyscallAnalysis = &fileanalysis.SyscallAnalysisData{
                    SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
                        DetectedSyscalls: syscalls,
                    },
                }
                // Check if any detected syscall is network-related.
                table, ok := v.syscallAnalyzer.GetSyscallTable(elfFile.Machine)
                if ok {
                    for _, s := range syscalls {
                        if s.Number >= 0 && table.IsNetworkSyscall(s.Number) {
                            hasNetwork = true
                            break
                        }
                    }
                }
            }
        }
    }

    return entry, hasNetwork, warnings, nil
}
```

### 4.6 設計注記：ネットワーク活動判定の責務分離

ネットワーク活動の判定は `analyzeOneLibrary` 内で完結させる。

- **シンボル判定**: `binaryanalyzer.NetworkDetected` が返ったとき `hasNetwork = true` とする
- **syscall 判定**: `v.syscallAnalyzer.GetSyscallTable(elfFile.Machine)` でテーブルを取得し、
  `IsNetworkSyscall()` で判定する。テーブルが取得できない場合（`ok == false`）は
  syscall 由来のネットワーク判定をスキップする

`analyzeLibraries` は `hasNetwork` フラグのみを受け取り、`DetectedLibraryNetworkDeps` への
SOName 追加判断に使う。独立したヘルパ関数は不要であり、`analyzer.go` への参照も生じない。

---

## 5. `internal/runner/base/security/network_analyzer.go`

### 5.1 `isNetworkViaBinaryAnalysis` の変更

既存の `KnownNetworkLibDeps` チェックと同じ位置に以下を追加する。

変更前（抜粋）:
```go
if hasNetworkSymbol || len(data.KnownNetworkLibDeps) > 0 {
```

変更後:
```go
if hasNetworkSymbol || len(data.KnownNetworkLibDeps) > 0 || len(data.DetectedLibraryNetworkDeps) > 0 {
```

`KnownNetworkLibDeps` のログ出力と同様のログを `DetectedLibraryNetworkDeps` 専用に追加する。

```go
if !hasNetworkSymbol && len(data.KnownNetworkLibDeps) == 0 && len(data.DetectedLibraryNetworkDeps) > 0 {
    slog.Info(
        "treating binary as network-capable based on library syscall/symbol analysis",
        "path", cmdPath,
        "detected_library_network_deps", data.DetectedLibraryNetworkDeps,
    )
}
```

---

## 6. `cmd/record/main.go`

既存の `v.SetSyscallAnalyzer(...)` 呼び出しの直後に追加する。

```go
v.SetLibraryAnalysisEnabled(true)
```

---

## 7. テスト仕様

### 7.1 `syscall_wrapper_libs_test.go`（新規）

| テスト名 | 確認内容 |
|---------|---------|
| `TestIsSyscallWrapperLibrary_match` | `libc.so.6`, `libpthread.so.0`, `ld-linux-x86-64.so.2` が `true` を返す |
| `TestIsSyscallWrapperLibrary_noMatch` | `libssl.so.3`, `libcurlso.4`, `libstdc++.so.6` が `false` を返す |
| `TestIsSyscallWrapperLibrary_prefixBoundary` | `libcc.so.1`（`libc` に前方一致するが区切り文字条件を満たさない）が `false` を返す |

### 7.2 `validator_library_analysis_test.go`（新規、`filevalidator` パッケージ）

テストは既存の validator テストパターンに倣い、モック `BinaryAnalyzer` と
モック `SyscallAnalyzerInterface` を注入して行う。

| テスト名 | AC | 確認内容 |
|---------|-----|---------|
| `TestAnalyzeLibraries_networkSymbolDetected` | AC-1 | `libfoo.so` が `socket` を UNDEF に持つ場合、`LibraryAnalysis` に記録され `DetectedLibraryNetworkDeps` に SOName が入る |
| `TestAnalyzeLibraries_networkSyscallDetected` | AC-2 | `libbar.so` が socket 番号の syscall を含む場合、`LibraryAnalysis.SyscallAnalysis` に記録される |
| `TestAnalyzeLibraries_libcExcluded` | AC-3 | `libc.so.6` は `LibraryAnalysis` に含まれない |
| `TestAnalyzeLibraries_ldLinuxExcluded` | AC-4 | `ld-linux-x86-64.so.2` は `LibraryAnalysis` に含まれない |
| `TestAnalyzeLibraries_nonNetworkLib` | AC-7 | `libz.so.1` は `DetectedLibraryNetworkDeps` に含まれない（ネットワーク syscall/シンボルなし） |
| `TestAnalyzeLibraries_sessionCache` | AC-6 | 同じライブラリを 2 回参照すると `analyzeOneLibrary` は 1 回だけ呼ばれる |
| `TestAnalyzeLibraries_missingLibFile` | AC-7 | 存在しないライブラリパスは `AnalysisWarnings` に追加され処理継続 |
| `TestAnalyzeLibraries_disabled` | - | `SetLibraryAnalysisEnabled(false)` のとき `LibraryAnalysis` が nil |

### 7.3 `network_analyzer_test.go` への追加（`runner` パッケージ）

| テスト名 | AC | 確認内容 |
|---------|-----|---------|
| `TestIsNetwork_DetectedLibraryNetworkDeps` | AC-5 | `DetectedLibraryNetworkDeps` が非空のとき `isNetworkViaBinaryAnalysis` が `(true, false)` を返す |

### 7.4 既存テストへの影響

スキーマバージョンが 19→20 に上がるため、schema バージョンを明示しているテストを更新する。

---

## 8. 受け入れ基準との対応

| AC | 対応する実装 |
|----|------------|
| AC-1 | `analyzeOneLibrary` が `.dynsym` 解析結果を `LibraryAnalysis` に記録し `NetworkDetected` のとき `hasNetwork=true`、`analyzeLibraries` が `DetectedLibraryNetworkDeps` にSOName追加 |
| AC-2 | `analyzeOneLibrary` が机械語解析結果を `LibraryAnalysis.SyscallAnalysis` に記録 |
| AC-3 | `IsSyscallWrapperLibrary("libc.so.6")` が `true`、`analyzeLibraries` でスキップ |
| AC-4 | `IsSyscallWrapperLibrary("ld-linux-x86-64.so.2")` が `true`、`analyzeLibraries` でスキップ |
| AC-5 | `isNetworkViaBinaryAnalysis` が `DetectedLibraryNetworkDeps` を参照してネットワーク有りと判定 |
| AC-6 | `libraryAnalysisCache` によりキャッシュヒット時は再解析しない |
| AC-7 | ライブラリファイル不在は `AnalysisWarnings` に追記、処理継続 |
| AC-8 | 既存の `.dynsym`・syscall 解析は変更なし（回帰テスト） |
