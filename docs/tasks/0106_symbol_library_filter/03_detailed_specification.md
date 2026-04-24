# 詳細仕様書: シンボル解析のライブラリフィルタ導入

## 1. `binaryanalyzer` パッケージの変更

### 1.1 ファイル：`internal/runner/security/binaryanalyzer/network_symbols.go`

#### 1.1.1 `CategorySyscallWrapper` 定数の追加

```go
const (
    CategorySocket         SymbolCategory = "socket"
    CategoryHTTP           SymbolCategory = "http"
    CategoryTLS            SymbolCategory = "tls"
    CategoryDNS            SymbolCategory = "dns"
    CategoryDynamicLoad    SymbolCategory = "dynamic_load"
    CategorySyscallWrapper SymbolCategory = "syscall_wrapper" // 新規追加
)
```

#### 1.1.2 `IsNetworkCategory` 関数の追加

```go
// IsNetworkCategory は、与えられたカテゴリ文字列がネットワーク系カテゴリか否かを返す。
// "socket", "dns", "tls", "http" が対象。"syscall_wrapper" や "dynamic_load" は false。
func IsNetworkCategory(cat string) bool {
    switch SymbolCategory(cat) {
    case CategorySocket, CategoryDNS, CategoryTLS, CategoryHTTP:
        return true
    }
    return false
}
```

**テスト（`network_symbols_test.go` に追加）**：

| 入力カテゴリ | 期待値 | AC 対応 |
|-----------|-------|--------|
| `"socket"` | `true` | AC-4 |
| `"dns"` | `true` | AC-4 |
| `"tls"` | `true` | AC-4 |
| `"http"` | `true` | AC-4 |
| `"syscall_wrapper"` | `false` | AC-4 |
| `"dynamic_load"` | `false` | — |
| `""` | `false` | — |

### 1.2 `AnalysisOutput.DetectedSymbols` のドキュメント変更

`binaryanalyzer/analyzer.go` の `DetectedSymbols` フィールドコメントを更新する：

```go
// DetectedSymbols contains all symbols imported from libc (ELF) or libSystem (Mach-O).
// Populated for both NetworkDetected and NoNetworkSymbols results.
// Network-related symbols have categories like "socket", "dns", "tls", "http".
// Other libc/libSystem symbols have category "syscall_wrapper".
DetectedSymbols []DetectedSymbol
```

## 2. ELF アナライザの変更

### 2.1 ファイル：`internal/runner/security/elfanalyzer/standard_analyzer.go`

#### 2.1.1 `checkDynamicSymbols` シグネチャ変更

変更前：

```go
func (a *StandardELFAnalyzer) checkDynamicSymbols(dynsyms []elf.Symbol) binaryanalyzer.AnalysisOutput
```

変更後：

```go
func (a *StandardELFAnalyzer) checkDynamicSymbols(elfFile *elf.File) binaryanalyzer.AnalysisOutput
```

呼び出し元（`AnalyzeNetworkSymbols` 内）の変更：

```go
// 変更前
dynsyms, err := elfFile.DynamicSymbols()
...
dynOutput := a.checkDynamicSymbols(dynsyms)

// 変更後
dynOutput := a.checkDynamicSymbols(elfFile)
```

#### 2.1.2 `checkDynamicSymbols` の実装変更

```go
func (a *StandardELFAnalyzer) checkDynamicSymbols(elfFile *elf.File) binaryanalyzer.AnalysisOutput {
    dynsyms, err := elfFile.DynamicSymbols()
    if err != nil {
        if errors.Is(err, elf.ErrNoSymbols) {
            // 静的バイナリ処理は呼び出し元で先に行うので、ここには到達しない
            return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.StaticBinary}
        }
        return binaryanalyzer.AnalysisOutput{
            Result: binaryanalyzer.AnalysisError,
            Error:  fmt.Errorf("failed to read dynamic symbols: %w", err),
        }
    }

    // VERNEED 判定：全 SHN_UNDEF シンボルを走査し、Library が空でないものが
    // 1 つでもあれば VERNEED ありとみなす。全て空の場合のみフォールバックを適用。
    hasAnyUndef := false
    hasVERNEED := false
    for _, sym := range dynsyms {
        if sym.Section == elf.SHN_UNDEF {
            hasAnyUndef = true
            if sym.Library != "" {
                hasVERNEED = true
                break
            }
        }
    }
    // SHN_UNDEF シンボルが存在しない（空の dynsym）場合は static binary として扱う
    _ = hasAnyUndef

    libcInNeeded := false
    if !hasVERNEED {
        libs, _ := elfFile.ImportedLibraries()
        for _, lib := range libs {
            if isLibcLibrary(lib) {
                libcInNeeded = true
                break
            }
        }
    }

    var detected []binaryanalyzer.DetectedSymbol
    var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

    for _, sym := range dynsyms {
        if sym.Section != elf.SHN_UNDEF {
            continue
        }

        // libc シンボルかどうかを判定
        isLibc := false
        if hasVERNEED {
            isLibc = isLibcLibrary(sym.Library)
        } else if libcInNeeded {
            // VERNEED なし・DT_NEEDED に libc あり：STT_FUNC 限定で全シンボルを libc 由来とみなす
            // （libcInNeeded の判定が true の場合、DT_NEEDED に libc パターンが含まれることが保証される）
            isLibc = elf.ST_TYPE(sym.Info) == elf.STT_FUNC
        }

        if isLibc {
            cat := categorizeELFSymbol(sym.Name, a.networkSymbols)
            detected = append(detected, binaryanalyzer.DetectedSymbol{
                Name:     sym.Name,
                Category: cat,
            })
        }

        if binaryanalyzer.IsDynamicLoadSymbol(sym.Name) {
            dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
                Name:     sym.Name,
                Category: "dynamic_load",
            })
        }
    }

    // ネットワーク系カテゴリが存在するかで Result を決定
    hasNetwork := false
    for _, sym := range detected {
        if binaryanalyzer.IsNetworkCategory(sym.Category) {
            hasNetwork = true
            break
        }
    }

    result := binaryanalyzer.NoNetworkSymbols
    if hasNetwork {
        result = binaryanalyzer.NetworkDetected
    }

    return binaryanalyzer.AnalysisOutput{
        Result:             result,
        DetectedSymbols:    detected,
        DynamicLoadSymbols: dynamicLoadSyms,
    }
}
```

#### 2.1.3 ヘルパー関数の追加

```go
// isLibcLibrary は ELF シンボルのライブラリ名が libc パターンに一致するか返す。
// glibc：  "libc.so.6"
// musl：   "libc.musl-x86_64.so.1" 等
func isLibcLibrary(lib string) bool {
    return strings.HasPrefix(lib, "libc.so.") ||
        strings.HasPrefix(lib, "libc.musl-")
}

// categorizeELFSymbol はシンボル名を networkSymbols で検索し、
// 一致すればそのカテゴリを、一致しなければ "syscall_wrapper" を返す。
func categorizeELFSymbol(name string, networkSymbols map[string]binaryanalyzer.SymbolCategory) string {
    if cat, found := networkSymbols[name]; found {
        return string(cat)
    }
    return string(binaryanalyzer.CategorySyscallWrapper)
}
```

#### 2.1.4 `AnalyzeNetworkSymbols` の呼び出し順序変更

現行の `AnalyzeNetworkSymbols` では `DynamicSymbols()` を呼び出した後に `checkDynamicSymbols(dynsyms)` を呼んでいるが、変更後は `elfFile` を直接渡すため、`DynamicSymbols()` の呼び出しを `checkDynamicSymbols` 内部に移す。

`AnalyzeNetworkSymbols` 内の Step 4・Step 5 を以下のように変更する：

```go
// Step 4+5: libc シンボルフィルタと dynamic load シンボルのチェック
dynOutput := a.checkDynamicSymbols(elfFile)
if dynOutput.Result == binaryanalyzer.StaticBinary {
    return a.handleStaticBinary(path, file, contentHash)
}
if dynOutput.Result != binaryanalyzer.NoNetworkSymbols {
    return dynOutput
}
// CGO バイナリフォールバック... （既存ロジック変更なし）
```

**注意**：`DynamicSymbols()` が `ErrNoSymbols` または空スライスを返す場合の静的バイナリ処理は `checkDynamicSymbols` 内で `StaticBinary` を返し、呼び出し元で `handleStaticBinary` にフォールバックする。

### 2.2 テスト変更（`elfanalyzer/analyzer_test.go`）

AC-1 のテストを追加または更新：

- `socket()` と `read()` の両方を libc からインポートする ELF バイナリで `socket` が `"socket"` カテゴリ、`read` が `"syscall_wrapper"` カテゴリであることを確認
- `checkDynamicSymbols` の単体テスト（`sym.Library` あり/なし の両ケース）

AC-2 のテストを追加：

- libc 以外のライブラリのみインポートする ELF バイナリで `DetectedSymbols` が空であることを確認

## 3. Mach-O アナライザの変更

### 3.1 ファイル：`internal/runner/security/machoanalyzer/standard_analyzer.go`

#### 3.1.1 `analyzeSlice` の実装変更

```go
func (a *StandardMachOAnalyzer) analyzeSlice(f *macho.File) binaryanalyzer.AnalysisOutput {
    // 参照ライブラリ一覧を取得（ライブラリ序数解決に使用）
    libs, _ := f.ImportedLibraries()

    var detected []binaryanalyzer.DetectedSymbol
    var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

    if f.Symtab != nil {
        symbols := machoUndefinedSymbols(f)
        for _, sym := range symbols {
            normalized := NormalizeSymbolName(sym.Name)

            if isLibSystemSymbol(sym, libs) {
                cat := categorizeMachoSymbol(normalized, a.networkSymbols)
                detected = append(detected, binaryanalyzer.DetectedSymbol{
                    Name:     normalized,
                    Category: cat,
                })
            }

            if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
                dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
                    Name:     normalized,
                    Category: "dynamic_load",
                })
            }
        }
    } else {
        // Symtab なし：ライブラリ序数不明のためフォールバック
        detected, dynamicLoadSyms = a.analyzeSliceFallback(f, libs)
    }

    // ネットワーク系カテゴリが存在するかで Result を決定
    hasNetwork := false
    for _, sym := range detected {
        if binaryanalyzer.IsNetworkCategory(sym.Category) {
            hasNetwork = true
            break
        }
    }

    result := binaryanalyzer.NoNetworkSymbols
    if hasNetwork {
        result = binaryanalyzer.NetworkDetected
    }

    return binaryanalyzer.AnalysisOutput{
        Result:             result,
        DetectedSymbols:    detected,
        DynamicLoadSymbols: dynamicLoadSyms,
    }
}
```

#### 3.1.2 ヘルパー関数の追加

```go
// machoUndefinedSymbols は Mach-O バイナリの undefined external シンボルを返す。
// Dysymtab がある場合は undef セクションの範囲を使用し、ない場合は全 Symtab をスキャンする。
func machoUndefinedSymbols(f *macho.File) []macho.Symbol {
    const (
        nType = 0x0e
        nUndf = 0x0
        nExt  = 0x01
        nStab = 0xe0
    )
    if f.Symtab == nil {
        return nil
    }
    if f.Dysymtab != nil {
        dt := f.Dysymtab
        end := dt.Iundefsym + dt.Nundefsym
        if end > uint32(len(f.Symtab.Syms)) {
            end = uint32(len(f.Symtab.Syms))
        }
        return f.Symtab.Syms[dt.Iundefsym:end]
    }
    var result []macho.Symbol
    for _, s := range f.Symtab.Syms {
        if s.Type&nStab != 0 {
            continue
        }
        if s.Type&nType == nUndf && s.Type&nExt != 0 {
            result = append(result, s)
        }
    }
    return result
}

// isLibSystemSymbol は Mach-O シンボルが libSystem 由来か否かを判定する。
// two-level namespace の場合は Desc フィールドのライブラリ序数を使用し、
// ライブラリ名を isLibSystemLibrary で確認する。
// 序数が範囲外の場合（SELF / DYNAMIC_LOOKUP / EXECUTABLE）は false を返す。
func isLibSystemSymbol(sym macho.Symbol, libs []string) bool {
    ordinal := int((sym.Desc >> 8) & 0xFF)
    if ordinal < 1 || ordinal > len(libs) {
        return false
    }
    return isLibSystemLibrary(libs[ordinal-1])
}

// isLibSystemLibrary はライブラリパスが libSystem 系か否かを返す。
func isLibSystemLibrary(path string) bool {
    if path == "/usr/lib/libSystem.B.dylib" {
        return true
    }
    base := filepath.Base(path)
    return strings.HasPrefix(base, "libsystem_") &&
        strings.HasSuffix(base, ".dylib")
}

// categorizeMachoSymbol はシンボル名を networkSymbols で検索し、
// 一致すればそのカテゴリを、一致しなければ "syscall_wrapper" を返す。
func categorizeMachoSymbol(name string, networkSymbols map[string]binaryanalyzer.SymbolCategory) string {
    if cat, found := networkSymbols[name]; found {
        return string(cat)
    }
    return string(binaryanalyzer.CategorySyscallWrapper)
}

// analyzeSliceFallback は Symtab がない場合の後退処理。
// libSystem が ImportedLibraries に含まれる場合は全インポートシンボルを libSystem 由来とみなす。
// libSystem がない場合は DetectedSymbols を空で返す（ライブラリフィルタの趣旨を維持）。
func (a *StandardMachOAnalyzer) analyzeSliceFallback(f *macho.File, libs []string) (detected, dynamicLoadSyms []binaryanalyzer.DetectedSymbol) {
    hasLibSystem := false
    for _, lib := range libs {
        if isLibSystemLibrary(lib) {
            hasLibSystem = true
            break
        }
    }

    // libSystem がなければシンボルを記録しない
    if !hasLibSystem {
        return nil, nil
    }

    symbols, err := f.ImportedSymbols()
    if err != nil {
        return nil, nil
    }

    for _, sym := range symbols {
        normalized := NormalizeSymbolName(sym)
        cat := categorizeMachoSymbol(normalized, a.networkSymbols)
        detected = append(detected, binaryanalyzer.DetectedSymbol{
            Name:     normalized,
            Category: cat,
        })
        if binaryanalyzer.IsDynamicLoadSymbol(normalized) {
            dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
                Name:     normalized,
                Category: "dynamic_load",
            })
        }
    }
    return
}
```

### 3.2 テスト変更（`machoanalyzer/analyzer_test.go`）

AC-3 のテストを追加：

- `socket` と `read` の両方を libSystem からインポートするシナリオで両シンボルが `DetectedSymbols` に含まれること
- `socket` のカテゴリが `"socket"`、`read` のカテゴリが `"syscall_wrapper"` であること

`analyzeSlice` の単体テスト：

- Dysymtab あり：ライブラリ序数が正しく機能すること
- Dysymtab なし・Symtab なし：フォールバックが動作すること
- libSystem 以外のライブラリのシンボルが除外されること

## 4. `network_analyzer.go` の変更

### 4.1 ファイル：`internal/runner/security/network_analyzer.go`

#### 4.1.1 `isNetworkViaBinaryAnalysis` の変更箇所

変更対象：`isNetworkViaBinaryAnalysis` 内の以下のコード（行 241-261 付近）：

```go
// 変更前
output := binaryanalyzer.AnalysisOutput{
    DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
    DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
}
if len(data.DetectedSymbols) > 0 || len(data.KnownNetworkLibDeps) > 0 {
    output.Result = binaryanalyzer.NetworkDetected
    ...
} else {
    output.Result = binaryanalyzer.NoNetworkSymbols
}
```

```go
// 変更後
output := binaryanalyzer.AnalysisOutput{
    DetectedSymbols:    convertNetworkSymbolEntries(data.DetectedSymbols),
    DynamicLoadSymbols: convertNetworkSymbolEntries(data.DynamicLoadSymbols),
}
hasNetworkSymbol := false
for _, sym := range data.DetectedSymbols {
    if binaryanalyzer.IsNetworkCategory(sym.Category) {
        hasNetworkSymbol = true
        break
    }
}
if hasNetworkSymbol || len(data.KnownNetworkLibDeps) > 0 {
    output.Result = binaryanalyzer.NetworkDetected
    ...
} else {
    output.Result = binaryanalyzer.NoNetworkSymbols
}
```

### 4.2 テスト変更（`network_analyzer_test.go`）

AC-4 に対応するテストを追加：

```go
// ネットワーク系カテゴリシンボルあり → NetworkDetected
func TestIsNetworkViaBinaryAnalysis_NetworkCategorySymbol(t *testing.T) {
    data := &fileanalysis.SymbolAnalysisData{
        DetectedSymbols: []fileanalysis.DetectedSymbolEntry{
            {Name: "socket", Category: "socket"},
            {Name: "read", Category: "syscall_wrapper"},
        },
    }
    // ...
    // isNetwork = true, isHigh = false
}

// syscall_wrapper のみ → NoNetworkSymbols
func TestIsNetworkViaBinaryAnalysis_SyscallWrapperOnly(t *testing.T) {
    data := &fileanalysis.SymbolAnalysisData{
        DetectedSymbols: []fileanalysis.DetectedSymbolEntry{
            {Name: "read", Category: "syscall_wrapper"},
            {Name: "write", Category: "syscall_wrapper"},
        },
    }
    // ...
    // isNetwork = false, isHigh = false
}
```

## 5. 受け入れ基準とテストの対応

| AC | テスト場所 | テスト内容 |
|----|----------|----------|
| AC-1 | `elfanalyzer/analyzer_test.go` | libc から `socket` + `read` をインポートする ELF バイナリ。`socket` → `"socket"`、`read` → `"syscall_wrapper"` |
| AC-2 | `elfanalyzer/analyzer_test.go` | libc 以外のみインポートする ELF バイナリ。`DetectedSymbols` が空 |
| AC-3 | `machoanalyzer/analyzer_test.go` | libSystem から `socket` + `read` をインポートする Mach-O バイナリ。同上 |
| AC-4 | `security/network_analyzer_test.go` | `"socket"` カテゴリ → `isNetwork=true`、`"syscall_wrapper"` のみ → `isNetwork=false` |
| AC-5 | `make test` + `make lint` | 既存テスト全通過 |

## 6. 変更しないもの

- `networkSymbols` マップの内容（カテゴリ付与のみに使用）
- `DynamicLoadSymbols` の処理ロジック（`IsDynamicLoadSymbol` は変更なし）
- `SyscallAnalysis` の処理（タスク 0105 で対応済み）
- `KnownNetworkLibDeps` の判定ロジック
- `CurrentSchemaVersion`（NFR-3 に従い変更なし）
- `handleAnalysisOutput` 関数（変更なし）
- ELF `handleStaticBinary` / `lookupSyscallAnalysis`（変更なし）

## 7. 実装上の注意点

### 7.1 ELF：VERNEED フォールバックの適用条件（重要）

フォールバックは「SHN_UNDEF シンボルが存在し、**かつそのすべてが `Library == ""`**」の場合のみ適用する。一部でも `Library != ""` のシンボルがあれば VERNEED ありと判断し、フォールバックを混在させない。混在させると `Library == ""` の非 libc シンボル（libm など）が誤って libc 由来と判定される危険がある。

### 7.2 Mach-O：libSystem なし時はシンボルを記録しない

`analyzeSliceFallback` で `ImportedLibraries()` に libSystem が含まれない場合は、`DetectedSymbols: nil` を返す（ネットワーク名フィルタは適用しない）。libSystem を持たない Mach-O バイナリは通常のシステムコールインターフェースを使っていないため、記録対象なしとみなすことが本タスクの趣旨に合致する。

### 7.3 ELF：`checkDynamicSymbols` のリファクタリング

`AnalyzeNetworkSymbols` 内で現在 `elfFile.DynamicSymbols()` を呼んでいる箇所（Step 4）は、`checkDynamicSymbols(elfFile)` 内部に移動する。これにより呼び出し元がシンプルになり、エラー処理も `checkDynamicSymbols` 内で完結する。

### 7.4 Mach-O：`nStab` の除外

Mach-O シンボルテーブルにはデバッグ情報（stabs）が含まれることがある。`machoUndefinedSymbols` では `nStab = 0xe0` マスクで除外する。

### 7.5 Mach-O：ライブラリ序数の特殊値

Desc フィールドのライブラリ序数は 1 始まりで、0・254・255 は特殊値（SELF / DYNAMIC_LOOKUP / EXECUTABLE）。これらは `isLibSystemSymbol` で `false` を返すため、安全にスキップされる。

### 7.6 Mach-O：`analyzeAllFatSlices` への影響

`analyzeAllFatSlices` は `analyzeSlice` の結果を集約するが、`analyzeSlice` のインターフェース（引数・戻り値型）は変わらないため `analyzeAllFatSlices` 自体の変更は不要。
