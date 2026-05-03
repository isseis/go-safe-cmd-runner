# 詳細仕様書: ELF バイナリ VERNEED なし時の名前ベースシンボル検出

## 1. 変更ファイル一覧

| ファイル | 変更種別 | 概要 |
|---------|---------|------|
| `internal/security/elfanalyzer/standard_analyzer.go` | 修正 | `checkDynamicSymbols` の分類ロジック変更、`categorizeELFSymbol` 削除 |
| `internal/security/elfanalyzer/testing/helpers.go` | 修正 | `CreateELFWithSymbols` ヘルパー追加 |
| `internal/security/elfanalyzer/analyzer_test.go` | 修正 | AC-1〜AC-6 対応テストの追加、既存テストの更新 |

## 1.1 責務分担に関する前提

本タスクでは `record` / `runner` の責務分担を変更しない。

- `record` は `checkDynamicSymbols` の結果を `fileanalysis.SymbolAnalysisData` へ保存する際、`networkSymbols` を使って runner 向けの正規化済み特徴量を生成する
- JSON に保存される `detected_symbols` はシンボル名のみであり、カテゴリは保持しない
- `runner` は保存済みシンボル名に対して `binaryanalyzer.IsNetworkSymbol()` を再適用し、カテゴリとネットワーク性を再導出して実行時リスク判定に利用する

したがって、本タスクが変更するのは「record が保存する解析結果の正規化ルール」であり、実行可否や `risk_level` の最終判断ロジック自体ではない。

## 2. `standard_analyzer.go` の変更

### 2.1 `checkDynamicSymbols` の修正

`internal/security/elfanalyzer/standard_analyzer.go` の `checkDynamicSymbols` を以下のとおり変更する。

**変更前（抜粋）:**

```go
// VERNEED judgment: scan all SHN_UNDEF symbols and check if any Library field is non-empty.
hasVERNEED := slices.ContainsFunc(dynsyms, func(s elf.Symbol) bool {
    return s.Section == elf.SHN_UNDEF && s.Library != ""
})
// hasVERNEED implies hasAnyUndef; only scan again if VERNEED was not found.
hasAnyUndef := hasVERNEED || slices.ContainsFunc(dynsyms, func(s elf.Symbol) bool {
    return s.Section == elf.SHN_UNDEF
})

// If no undefined symbols exist, this is a statically linked or import-free binary
if !hasAnyUndef {
    return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}
}

var detected []binaryanalyzer.DetectedSymbol
var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

for _, sym := range dynsyms {
    if sym.Section != elf.SHN_UNDEF {
        continue
    }

    // Determine if symbol is from libc
    isLibc := false
    if hasVERNEED {
        isLibc = isLibcLibrary(sym.Library)
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
```

**変更後:**

```go
hasAnyUndef := slices.ContainsFunc(dynsyms, func(s elf.Symbol) bool {
    return s.Section == elf.SHN_UNDEF
})

// If no undefined symbols exist, this is a statically linked or import-free binary
if !hasAnyUndef {
    return binaryanalyzer.AnalysisOutput{Result: binaryanalyzer.NoNetworkSymbols}
}

var detected []binaryanalyzer.DetectedSymbol
var dynamicLoadSyms []binaryanalyzer.DetectedSymbol

for _, sym := range dynsyms {
    if sym.Section != elf.SHN_UNDEF {
        continue
    }

    // Step 1: name-based detection — applies regardless of sym.Library or VERNEED presence.
    if cat, found := a.networkSymbols[sym.Name]; found {
        detected = append(detected, binaryanalyzer.DetectedSymbol{
            Name:     sym.Name,
            Category: string(cat),
        })
    } else if isLibcLibrary(sym.Library) {
        // Step 2: libc symbols not in networkSymbols are syscall wrappers.
        // On VERNEED-less binaries (musl) sym.Library is always empty,
        // so isLibcLibrary returns false and syscall_wrapper is never assigned.
        detected = append(detected, binaryanalyzer.DetectedSymbol{
            Name:     sym.Name,
            Category: string(binaryanalyzer.CategorySyscallWrapper),
        })
    }

    if binaryanalyzer.IsDynamicLoadSymbol(sym.Name) {
        dynamicLoadSyms = append(dynamicLoadSyms, binaryanalyzer.DetectedSymbol{
            Name:     sym.Name,
            Category: "dynamic_load",
        })
    }
}
```

### 2.2 `categorizeELFSymbol` の削除

`categorizeELFSymbol` は変更後の `checkDynamicSymbols` から呼ばれなくなるため、削除する。

**削除対象:**

```go
// categorizeELFSymbol returns the category of the symbol using networkSymbols,
// or "syscall_wrapper" if not found.
func categorizeELFSymbol(name string, networkSymbols map[string]binaryanalyzer.SymbolCategory) string {
    if cat, found := networkSymbols[name]; found {
        return string(cat)
    }
    return string(binaryanalyzer.CategorySyscallWrapper)
}
```

### 2.3 コメント更新

`checkDynamicSymbols` の関数コメントを以下のとおり更新する。

**変更前:**
```go
// checkDynamicSymbols extracts all libc symbols from the given ELF file and categorizes them.
// Returns DetectedSymbols containing both network and non-network libc symbols.
// Non-network libc symbols are assigned category "syscall_wrapper".
```

**変更後:**
```go
// checkDynamicSymbols extracts network-related and libc symbols from the given ELF file.
// For each SHN_UNDEF symbol, applies a two-step filter:
//  1. If the symbol name is in networkSymbols, record it with the corresponding category.
//  2. Otherwise, if the symbol is from libc, record it as "syscall_wrapper".
// This handles both VERNEED-present (glibc) and VERNEED-absent (musl) binaries.
```

## 3. `testing/helpers.go` の変更

`internal/security/elfanalyzer/testing/helpers.go` に `CreateELFWithSymbols` を追加する。テストが必要とするのは「任意のシンボル名を持ち、VERNEED を持たない（musl 相当）ELF」であるため、VERNEED あり ELF の生成は対象外とする（AC-5 は既存 testdata を使用）。

`CreateDynamicELFFile` は `CreateELFWithSymbols` に委譲してコードの重複を排除する。

```go
// CreateDynamicELFFile creates a minimal dynamic ELF file for testing.
// Deprecated: use CreateELFWithSymbols directly.
func CreateDynamicELFFile(t *testing.T, path string) {
    t.Helper()
    CreateELFWithSymbols(t, path, []SymbolSpec{{Name: "__libc_start_main"}})
}
```

```go
// SymbolSpec defines a symbol to include in a test ELF binary.
type SymbolSpec struct {
    Name string
}

// CreateELFWithSymbols creates a minimal dynamic ELF64 LE file at the given path.
// All symbols are SHN_UNDEF with no VERNEED section (musl-style binary).
// The generated ELF has the following sections:
//   [0] null
//   [1] .dynsym  (null symbol + one entry per SymbolSpec)
//   [2] .dynstr  (string table for .dynsym)
//   [3] .shstrtab (section name string table)
func CreateELFWithSymbols(t *testing.T, path string, symbols []SymbolSpec) {
    t.Helper()

    const (
        elfHeaderSize  = 64
        sectionHdrSize = 64
        numSections    = 4
        elf64SymSize   = 24
        stInfoShift    = 4
        phentSize      = 56
        shstrndx       = 3
        dynstrIdx      = 2
    )

    // .shstrtab
    shstrtab := []byte("\x00.dynsym\x00.dynstr\x00.shstrtab\x00")
    const (
        shOffNull     = 0
        shOffDynsym   = 1
        shOffDynstr   = 9
        shOffShstrtab = 17
    )

    // .dynstr: "\x00" + name1 + "\x00" + name2 + "\x00" + ...
    dynstr := []byte{0}
    nameOffsets := make([]int, len(symbols))
    for i, s := range symbols {
        nameOffsets[i] = len(dynstr)
        dynstr = append(dynstr, []byte(s.Name)...)
        dynstr = append(dynstr, 0)
    }

    // .dynsym: null symbol + one entry per SymbolSpec
    dynsymData := make([]byte, (1+len(symbols))*elf64SymSize)
    for i := range symbols {
        off := (1 + i) * elf64SymSize
        sym := dynsymData[off : off+elf64SymSize]
        binary.LittleEndian.PutUint32(sym[0:4], uint32(nameOffsets[i]))
        sym[4] = byte(elf.STT_FUNC) | byte(elf.STB_GLOBAL<<stInfoShift)
        binary.LittleEndian.PutUint16(sym[6:8], uint16(elf.SHN_UNDEF))
    }

    // Layout: ELF header | section headers | .dynsym | .dynstr | .shstrtab
    shdrsOffset := int64(elfHeaderSize)
    shdrsSize := int64(numSections * sectionHdrSize)
    dynsymOffset := shdrsOffset + shdrsSize
    dynstrOffset := dynsymOffset + int64(len(dynsymData))
    shstrtabOffset := dynstrOffset + int64(len(dynstr))

    buf := &bytes.Buffer{}

    // ELF header
    elfHdr := make([]byte, elfHeaderSize)
    copy(elfHdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
    elfHdr[4] = byte(elf.ELFCLASS64)
    elfHdr[5] = byte(elf.ELFDATA2LSB)
    elfHdr[6] = byte(elf.EV_CURRENT)
    elfHdr[7] = byte(elf.ELFOSABI_NONE)
    binary.LittleEndian.PutUint16(elfHdr[16:18], uint16(elf.ET_EXEC))
    binary.LittleEndian.PutUint16(elfHdr[18:20], uint16(elf.EM_X86_64))
    binary.LittleEndian.PutUint32(elfHdr[20:24], uint32(elf.EV_CURRENT))
    binary.LittleEndian.PutUint64(elfHdr[40:48], uint64(shdrsOffset)) //nolint:gosec
    binary.LittleEndian.PutUint16(elfHdr[52:54], uint16(elfHeaderSize))
    binary.LittleEndian.PutUint16(elfHdr[54:56], phentSize)
    binary.LittleEndian.PutUint16(elfHdr[56:58], 0)
    binary.LittleEndian.PutUint16(elfHdr[58:60], uint16(sectionHdrSize))
    binary.LittleEndian.PutUint16(elfHdr[60:62], uint16(numSections))
    binary.LittleEndian.PutUint16(elfHdr[62:64], shstrndx)
    buf.Write(elfHdr)

    // Section header helper
    writeSHdr := func(nameIdx uint32, shType elf.SectionType, flags elf.SectionFlag,
        offset, size, link, info uint64, entSize uint32,
    ) {
        sh := make([]byte, sectionHdrSize)
        binary.LittleEndian.PutUint32(sh[0:4], nameIdx)
        binary.LittleEndian.PutUint32(sh[4:8], uint32(shType))
        binary.LittleEndian.PutUint64(sh[8:16], uint64(flags))
        binary.LittleEndian.PutUint64(sh[24:32], offset)
        binary.LittleEndian.PutUint64(sh[32:40], size)
        binary.LittleEndian.PutUint32(sh[40:44], uint32(link)) //nolint:gosec
        binary.LittleEndian.PutUint32(sh[44:48], uint32(info)) //nolint:gosec
        binary.LittleEndian.PutUint64(sh[48:56], 1)
        binary.LittleEndian.PutUint32(sh[56:60], entSize)
        buf.Write(sh)
    }

    writeSHdr(shOffNull, elf.SHT_NULL, 0, 0, 0, 0, 0, 0)
    writeSHdr(uint32(shOffDynsym), elf.SHT_DYNSYM, elf.SHF_ALLOC,
        uint64(dynsymOffset), uint64(len(dynsymData)), dynstrIdx, 1, elf64SymSize) //nolint:gosec
    writeSHdr(uint32(shOffDynstr), elf.SHT_STRTAB, elf.SHF_ALLOC,
        uint64(dynstrOffset), uint64(len(dynstr)), 0, 0, 0) //nolint:gosec
    writeSHdr(uint32(shOffShstrtab), elf.SHT_STRTAB, 0,
        uint64(shstrtabOffset), uint64(len(shstrtab)), 0, 0, 0) //nolint:gosec

    buf.Write(dynsymData)
    buf.Write(dynstr)
    buf.Write(shstrtab)

    err := os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec
    require.NoError(t, err)
}
```

## 4. `analyzer_test.go` の変更

### 4.1 既存テストの更新

`TestStandardELFAnalyzer_LibcSymbolFiltering` の `"non-libc symbols are not recorded"` サブテストを更新する。サブテスト名も動作変更に合わせてリネームする。

**変更前（サブテスト名 + assert）:**
```go
t.Run("non-libc symbols are not recorded", func(t *testing.T) {
    // ...
    for _, sym := range output.DetectedSymbols {
        assert.NotEqual(t, "SSL_CTX_new", sym.Name,
            "SSL_CTX_new (from libssl) must not appear in DetectedSymbols")
        assert.NotEqual(t, "SSL_CTX_free", sym.Name,
            "SSL_CTX_free (from libssl) must not appear in DetectedSymbols")
    }
})
```

**変更後（サブテスト名 + assert）:**
```go
t.Run("non-libc network symbols recorded with correct category", func(t *testing.T) {
    // After task 0109, Step 1 records networkSymbols matches regardless of Library.
    // SSL_CTX_new is in networkSymbols (tls category) and must appear in DetectedSymbols.
    // ...
    foundSSL := false
    for _, sym := range output.DetectedSymbols {
        if sym.Name == "SSL_CTX_new" {
            assert.Equal(t, "tls", sym.Category,
                `SSL_CTX_new should have category "tls"`)
            foundSSL = true
        }
    }
    assert.True(t, foundSSL, `SSL_CTX_new should now appear in DetectedSymbols`)
})
```

また `TestStandardELFAnalyzer_AnalyzeNetworkSymbols` の `"binary with ssl symbols"` ケースも期待値を更新する。

**変更前:**
```go
{
    name:           "binary with ssl symbols",
    filename:       "with_ssl.elf",
    expectedResult: binaryanalyzer.NoNetworkSymbols,
    expectSymbols:  true,
},
```

**変更後:**
```go
{
    name:           "binary with ssl symbols",
    filename:       "with_ssl.elf",
    expectedResult: binaryanalyzer.NetworkDetected,
    expectSymbols:  true,
},
```

### 4.2 新規テストの追加

以下のテスト関数を `analyzer_test.go` に追加する。

> **JSON スキーマとの関係**: 各テストは `binaryanalyzer.AnalysisOutput`（内部表現）を検証する。`sym.Category` は `binaryanalyzer.DetectedSymbol` の内部フィールドであり、JSON `symbol_analysis.detected_symbols` には schema v18 以降保存されない（`category` フィールド削除）。JSON にはシンボル名のみが `[]string` で保存される（schema v19 以降）。

```go
// TestCheckDynamicSymbols_NameBasedFilter verifies that checkDynamicSymbols applies
// name-based detection for VERNEED-absent binaries (musl-style) and correctly handles
// mixed-library VERNEED-absent binaries.
func TestCheckDynamicSymbols_NameBasedFilter(t *testing.T) {
    tmpDir := commontesting.SafeTempDir(t)
    analyzer := NewStandardELFAnalyzer(nil)

    t.Run("no-VERNEED binary importing socket yields NetworkDetected with socket category", func(t *testing.T) {
        path := filepath.Join(tmpDir, "socket_only.elf")
        elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
            {Name: "socket"},
        })

        output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

        require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
        require.NotEmpty(t, output.DetectedSymbols)
        found := false
        for _, sym := range output.DetectedSymbols {
            if sym.Name == "socket" {
                assert.Equal(t, "socket", sym.Category)
                found = true
            }
        }
        assert.True(t, found, `"socket" must be in DetectedSymbols`)
    })

    t.Run("no-VERNEED binary importing SSL_CTX_new yields NetworkDetected with tls category", func(t *testing.T) {
        path := filepath.Join(tmpDir, "ssl_only.elf")
        elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
            {Name: "SSL_CTX_new"},
        })

        output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

        require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
        found := false
        for _, sym := range output.DetectedSymbols {
            if sym.Name == "SSL_CTX_new" {
                assert.Equal(t, "tls", sym.Category)
                found = true
            }
        }
        assert.True(t, found, `"SSL_CTX_new" must be in DetectedSymbols`)
    })

    t.Run("no-VERNEED binary importing only non-network symbols yields NoNetworkSymbols", func(t *testing.T) {
        path := filepath.Join(tmpDir, "read_only.elf")
        elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
            {Name: "read"},
        })

        output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

        assert.Equal(t, binaryanalyzer.NoNetworkSymbols, output.Result)
        assert.Empty(t, output.DetectedSymbols)
    })

    t.Run("no-VERNEED binary with mixed symbols records only networkSymbols matches", func(t *testing.T) {
        path := filepath.Join(tmpDir, "mixed_symbols.elf")
        elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
            {Name: "socket"},
            {Name: "SSL_CTX_new"},
            {Name: "pthread_create"}, // not in networkSymbols, must not be recorded
        })

        output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

        require.Equal(t, binaryanalyzer.NetworkDetected, output.Result)
        names := make(map[string]string)
        for _, sym := range output.DetectedSymbols {
            names[sym.Name] = sym.Category
        }
        assert.Equal(t, "socket", names["socket"])
        assert.Equal(t, "tls", names["SSL_CTX_new"])
        assert.NotContains(t, names, "pthread_create",
            "pthread_create is not in networkSymbols and must not appear in DetectedSymbols")
    })

    t.Run("dlopen in no-VERNEED binary appears in DynamicLoadSymbols", func(t *testing.T) {
        path := filepath.Join(tmpDir, "dlopen_socket.elf")
        elfanalyzertesting.CreateELFWithSymbols(t, path, []elfanalyzertesting.SymbolSpec{
            {Name: "dlopen"},
            {Name: "socket"},
        })

        output := analyzer.AnalyzeNetworkSymbols(path, "sha256:dummy")

        found := false
        for _, sym := range output.DynamicLoadSymbols {
            if sym.Name == "dlopen" {
                assert.Equal(t, "dynamic_load", sym.Category)
                found = true
            }
        }
        assert.True(t, found, `"dlopen" must appear in DynamicLoadSymbols`)
    })
}
```

## 5. `import` の変更

`standard_analyzer.go` から `hasVERNEED` 削除後は `slices.ContainsFunc` が `hasAnyUndef` の1か所のみになる。`slices` パッケージのインポートは引き続き必要。`categorizeELFSymbol` 削除後も `isLibcLibrary`・`isELFMagic` で `path/filepath`・`strings` は使用するため、インポート変更は不要。

## 6. テストデータ

| テストデータ | 用途 | 変更 |
|-------------|------|------|
| `testdata/with_socket.elf` | AC-5（VERNEED あり・glibc） | 変更なし |
| `testdata/with_ssl.elf` | AC-5（VERNEED あり・libssl）、既存テスト更新 | 変更なし（テスト期待値を更新） |
| `CreateELFWithSymbols` 生成 | AC-1〜AC-4、AC-6（VERNEED なし） | 新規ヘルパーで動的生成 |

## 7. `record` コマンドと JSON スキーマの関係

本タスクの変更は `checkDynamicSymbols`（内部）のみに影響し、`record` コマンドの出力仕様は変わらない。参考として現行の出力仕様を記載する。

| 項目 | デフォルト出力 | `--debug-info` 追加時 |
|------|--------------|----------------------|
| `symbol_analysis.detected_symbols` | シンボル名のみ `[]string` | 同じ（変化なし） |
| `symbol_analysis.dynamic_load_symbols` | シンボル名のみ `[]string` | 同じ（変化なし） |
| `syscall_analysis.detected_syscalls[].occurrences` | 含まれない | 含まれる |
| `syscall_analysis.determination_stats` | 含まれない | 含まれる |

`Category`（`socket` / `tls` / `dns` / `http` / `syscall_wrapper`）は schema v18 以降 JSON に保存されない。`runner` はシンボル名から `binaryanalyzer.IsNetworkSymbol()` を呼び出してカテゴリを再導出する。

このスキーマは現行の責務分担を反映している。すなわち `record` は runner が参照すべきシンボル名集合を保存し、`runner` がその集合を実行時ポリシーへ解釈する。生の全インポートシンボル一覧を保存する設計には変更しない。
