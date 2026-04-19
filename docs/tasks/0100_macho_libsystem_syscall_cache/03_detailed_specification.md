# libSystem.dylib syscall ラッパー関数キャッシュ 詳細仕様書

## 1. ファイル一覧と役割

本タスクで変更・新規作成するファイル一覧。対応する要件・アーキテクチャドキュメントのセクションを参照として記載する。

| ファイル | 変更種別 | 主な内容 | アーキテクチャ参照 |
|---------|---------|---------|---------|
| `internal/libccache/schema.go` | 拡張 | 新定数 3 件追加 | §5.1 |
| `internal/libccache/macos_syscall_table.go` | 新規 | `MacOSSyscallTable` 型・BSD syscall テーブル | §3.1 |
| `internal/libccache/macho_analyzer.go` | 新規 | `MachoLibSystemAnalyzer`（Mach-O 関数単位解析、x16 後方スキャン） | §3.2 |
| `internal/libccache/macho_cache.go` | 新規 | `MachoLibSystemCacheManager`（Mach-O 用キャッシュ管理） | §3.5 |
| `internal/libccache/adapters.go` | 拡張 | `MachoLibSystemAdapter` 追加 | §3.6 |
| `internal/libccache/matcher.go` | 拡張 | `MatchWithMethod()` 追加 | §6 |
| `internal/machodylib/libsystem_resolver.go` | 新規 | `LibSystemKernelSource`、`ResolveLibSystemKernel()` | §3.4 |
| `internal/machodylib/dyld_extractor.go` | 新規 | non-Darwin スタブ `ExtractLibSystemKernelFromDyldCache()` | §3.3 |
| `internal/machodylib/dyld_extractor_darwin.go` | 新規 | Darwin 実装 (`blacktop/ipsw/pkg/dyld` 使用) | §3.3 |
| `internal/filevalidator/validator.go` | 拡張 | `LibSystemCacheInterface`、`libSystemCache` フィールド、`analyzeLibSystemImports()`、svc+libSystem マージロジック | §3.7 |
| `internal/runner/security/machoanalyzer/symbol_normalizer.go` | 拡張 | `normalizeSymbolName` → `NormalizeSymbolName` エクスポート | §3.7.2 |
| `internal/runner/security/network_analyzer.go` | 拡張 | `syscallAnalysisHasNetworkSignal()` 追加、`isNetworkViaBinaryAnalysis()` 拡張 | §3.8 |
| `cmd/record/main.go` | 拡張 | `MachoLibSystemAdapter` 初期化・注入 | §3.6 |

## 2. `internal/libccache/schema.go` の拡張

### 2.1 追加する定数

```go
// SourceLibsystemSymbolImport is the value of SyscallInfo.Source for syscalls
// detected via libSystem import symbol matching.
const SourceLibsystemSymbolImport = "libsystem_symbol_import"

// DeterminationMethodLibCacheMatch indicates the syscall was determined via
// libSystem function-level analysis cache matching.
const DeterminationMethodLibCacheMatch = "lib_cache_match"

// DeterminationMethodSymbolNameMatch indicates the syscall was determined via
// symbol name-only matching (fallback path).
const DeterminationMethodSymbolNameMatch = "symbol_name_match"
```

既存の `SourceLibcSymbolImport = "libc_symbol_import"` は変更しない。

## 3. `internal/libccache/macos_syscall_table.go`（新規）

### 3.1 型定義とテーブル

```go
//go:build !integration

package libccache

// macOSSyscallEntry is the internal representation of a BSD syscall entry.
type macOSSyscallEntry struct {
    name      string
    isNetwork bool
}

// MacOSSyscallTable implements SyscallNumberTable for macOS arm64 BSD syscalls.
type MacOSSyscallTable struct{}

// macOSSyscallEntries defines the macOS arm64 BSD syscall table.
// Keys are syscall numbers without the BSD class prefix 0x2000000.
var macOSSyscallEntries = map[int]macOSSyscallEntry{
    3:   {name: "read",        isNetwork: false},
    4:   {name: "write",       isNetwork: false},
    5:   {name: "open",        isNetwork: false},
    6:   {name: "close",       isNetwork: false},
    27:  {name: "recvmsg",     isNetwork: true},
    28:  {name: "sendmsg",     isNetwork: true},
    29:  {name: "recvfrom",    isNetwork: true},
    30:  {name: "accept",      isNetwork: true},
    31:  {name: "getpeername", isNetwork: true},
    32:  {name: "getsockname", isNetwork: true},
    74:  {name: "mprotect",    isNetwork: false},
    97:  {name: "socket",      isNetwork: true},
    98:  {name: "connect",     isNetwork: true},
    104: {name: "bind",        isNetwork: true},
    105: {name: "setsockopt",  isNetwork: true},
    106: {name: "listen",      isNetwork: true},
    118: {name: "getsockopt",  isNetwork: true},
    133: {name: "sendto",      isNetwork: true},
    134: {name: "shutdown",    isNetwork: true},
    135: {name: "socketpair",  isNetwork: true},
}

// GetSyscallName implements SyscallNumberTable.
func (t MacOSSyscallTable) GetSyscallName(number int) string {
    if e, ok := macOSSyscallEntries[number]; ok {
        return e.name
    }
    return ""
}

// IsNetworkSyscall implements SyscallNumberTable.
func (t MacOSSyscallTable) IsNetworkSyscall(number int) bool {
    if e, ok := macOSSyscallEntries[number]; ok {
        return e.isNetwork
    }
    return false
}

// networkSyscallWrapperNames lists network-related syscall wrapper names used by
// the fallback matching path in FR-3.4.2.
// sendmmsg / recvmmsg are Linux-specific and are therefore excluded on macOS.
var networkSyscallWrapperNames = []string{
    "socket", "connect", "bind", "listen", "accept",
    "sendto", "recvfrom", "sendmsg", "recvmsg",
    "socketpair", "shutdown", "setsockopt", "getsockopt",
    "getpeername", "getsockname",
}
```

### 3.2 `SyscallNumberTable` インターフェース適合確認

`MacOSSyscallTable` が `libccache.SyscallNumberTable` を実装することをコンパイル時に確認するため、以下の行を追加する。

```go
var _ SyscallNumberTable = MacOSSyscallTable{}
```

## 4. `internal/libccache/macho_analyzer.go`（新規）

### 4.1 定数と型定義

```go
package libccache

import (
    "debug/macho"
    "encoding/binary"
    "fmt"
    "sort"

    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
)

// maxBackwardScanInstructions is the maximum number of instructions scanned backward from svc.
const maxBackwardScanInstructions = 16

// bsdSyscallClassPrefix is the macOS arm64 BSD syscall class prefix (0x2000000).
const bsdSyscallClassPrefix = 0x2000000

// svcMacOSEncoding is the little-endian uint32 encoding of the "svc #0x80" instruction.
// ARM64 encoding: 0xD4001001.
const svcMacOSEncoding = uint32(0xD4001001)

// MachoLibSystemAnalyzer analyzes a libsystem_kernel.dylib Mach-O file and returns
// a list of syscall wrapper functions.
type MachoLibSystemAnalyzer struct{}
```

### 4.2 `Analyze()` メソッドの実装

```go
// Analyze scans exported functions in machoFile and returns WrapperEntry values
// for functions recognized as syscall wrappers.
// Returns *elfanalyzer.UnsupportedArchitectureError for non-arm64 architectures.
// The returned slice is sorted by Number and then by Name.
func (a *MachoLibSystemAnalyzer) Analyze(machoFile *macho.File) ([]WrapperEntry, error) {
    if machoFile.Cpu != macho.CpuArm64 {
        return nil, &elfanalyzer.UnsupportedArchitectureError{Machine: fmt.Sprintf("%v", machoFile.Cpu)}
    }

    // Get the __TEXT,__text section.
    textSection := machoFile.Section("__text")
    if textSection == nil || textSection.Seg != "__TEXT" {
        return nil, nil
    }
    code, err := textSection.Data()
    if err != nil {
        return nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
    }
    textBase := textSection.Addr // Virtual address base.

    // Enumerate externally defined symbols from LC_SYMTAB.
    if machoFile.Symtab == nil {
        return nil, nil
    }

    // Sort by address to estimate function sizes.
    syms := filterFunctionSymbols(machoFile.Symtab.Syms)
    sort.Slice(syms, func(i, j int) bool {
        return syms[i].Value < syms[j].Value
    })

    textEnd := textBase + uint64(len(code))
    var entries []WrapperEntry

    for i, sym := range syms {
        // Estimate function size because Mach-O symtab has no st_size equivalent.
        var funcEnd uint64
        if i+1 < len(syms) {
            funcEnd = syms[i+1].Value
        } else {
            funcEnd = textEnd
        }
        if sym.Value >= funcEnd || sym.Value < textBase || funcEnd > textEnd {
            continue
        }
        funcSize := funcEnd - sym.Value

        // Apply the size filter from FR-3.2.2.
        if funcSize > MaxWrapperFunctionSize {
            continue
        }

        // Slice out the function bytes.
        startOff := int(sym.Value - textBase)  //nolint:gosec // G115: sym.Value >= textBase
        endOff := int(funcEnd - textBase)      //nolint:gosec // G115: funcEnd <= textEnd
        funcCode := code[startOff:endOff]

        // Detect svc #0x80 and resolve the BSD syscall number by scanning backward for x16 setup.
        number, ok := analyzeWrapperFunction(funcCode)
        if !ok {
            continue
        }
        entries = append(entries, WrapperEntry{Name: sym.Name, Number: number})
    }

    // Sort by Number and then by Name as required by FR-3.1.1.
    sort.Slice(entries, func(i, j int) bool {
        if entries[i].Number != entries[j].Number {
            return entries[i].Number < entries[j].Number
        }
        return entries[i].Name < entries[j].Name
    })

    return entries, nil
}
```

### 4.3 シンボルフィルタリング

```go
// filterFunctionSymbols returns only externally defined function symbols from Symtab.
// Externally defined means the symbol is section-defined and not undefined.
// macOS Mach-O symbol type flags:
//   N_EXT    = 0x01 (external)
//   N_TYPE   = 0x0E (type mask)
//   N_SECT   = 0x0E (defined in section)
//   N_UNDF   = 0x00 (undefined)
func filterFunctionSymbols(syms []macho.Symbol) []macho.Symbol {
    var result []macho.Symbol
    for _, s := range syms {
        // Exclude undefined symbols (imports).
        if s.Sect == 0 {
            continue
        }
        // Exclude debug symbols (N_STAB: type >= 0x20).
        if s.Type >= 0x20 {
            continue
        }
        // Keep only external symbols (N_EXT bit set).
        if s.Type&0x01 == 0 {
            continue
        }
        result = append(result, s)
    }
    return result
}
```

### 4.4 ラッパー関数解析（x16 後方スキャン）

```go
// analyzeWrapperFunction analyzes funcCode, which contains one function body,
// and returns a single BSD syscall number. It returns (0, false) if the
// function contains no svc or if multiple distinct syscall numbers are found.
func analyzeWrapperFunction(funcCode []byte) (int, bool) {
    var foundNumbers []int
    const instrLen = 4

    for i := 0; i+instrLen <= len(funcCode); i += instrLen {
        word := binary.LittleEndian.Uint32(funcCode[i:])
        if word != svcMacOSEncoding {
            continue
        }
        // Found svc #0x80. Scan backward to find the immediate loaded into x16.
        num, ok := backwardScanX16(funcCode, i)
        if !ok {
            return 0, false
        }
        foundNumbers = append(foundNumbers, num)
    }

    if len(foundNumbers) == 0 {
        return 0, false
    }

    // Verify that all svc instructions resolve to the same BSD number (FR-3.2.3).
    first := foundNumbers[0]
    for _, n := range foundNumbers[1:] {
        if n != first {
            return 0, false
        }
    }
    return first, true
}

// backwardScanX16 walks backward from the svc #0x80 instruction at funcCode[svcOffset]
// and looks for an immediate-load sequence into x16. When found, it returns the
// syscall number with the BSD class prefix removed. The scan is limited to
// maxBackwardScanInstructions instructions.
//
// Supported instruction patterns:
//   - MOVZ X16, #imm               (single-instruction case when imm < 0x10000)
//   - MOVZ X16, #hi, LSL #16       (upper 16 bits only)
//   - MOVZ X16, #hi, LSL #16       (preceding instruction)
//     MOVK X16, #lo                (following instruction immediately before svc)
//
// Typical macOS BSD syscall pattern (example: socket = 0x2000061):
//   MOVZ X16, #0x200, LSL #16    ; x16 = 0x02000000
//   MOVK X16, #0x61              ; x16 = 0x02000061
//   SVC  #0x80
//
// Because the scan runs backward from the instruction nearest to svc, it can observe
// MOVK first and then combine it with a later-observed MOVZ.
// MOVK X16 does not terminate the scan; it records a partial value and continues.
//
// arm64asm is intentionally not used here. Only a small subset of fixed encodings
// is needed, and direct decoding keeps the dependency surface small.
func backwardScanX16(funcCode []byte, svcOffset int) (int, bool) {
    const instrLen = 4

    // MOVZ X16, #imm, LSL #shift encoding:
    //   [31]:   sf=1 (64-bit)
    //   [30:29]: opc=10 (MOVZ)
    //   [28:23]: 100101
    //   [22:21]: hw (shift: 00=0, 01=16, 10=32, 11=48)
    //   [20:5]:  imm16
    //   [4:0]:   Rd=16 (x16)
    //
    // MOVZ X16, #imm (shift=0): 0xD280_0010 | (imm16 << 5)
    // MOVZ X16, #imm, LSL #16: 0xD2A0_0010 | (imm16 << 5)
    //
    // MOVK X16, #imm, LSL #shift encoding:
    //   [31]:   sf=1
    //   [30:29]: opc=11 (MOVK)
    //   [28:23]: 100101
    //   [22:21]: hw
    //   [20:5]:  imm16
    //   [4:0]:   Rd=16 (x16)
    //
    // MOVK X16, #imm (shift=0):    0xF280_0010 | (imm16 << 5)
    // MOVK X16, #imm, LSL #16:     0xF2A0_0010 | (imm16 << 5)

    const (
        movzX16Base  = uint32(0xD2800010) // MOVZ X16, #0, LSL #0
        movzX16Lsl16 = uint32(0xD2A00010) // MOVZ X16, #0, LSL #16
        movkX16Base  = uint32(0xF2800010) // MOVK X16, #0, LSL #0
        movkX16Lsl16 = uint32(0xF2A00010) // MOVK X16, #0, LSL #16
        imm16Mask    = uint32(0x001FFFE0) // bits[20:5]
        imm16Shift   = 5
    )

    // Scan backward from the instruction immediately before svc.
    startIdx := svcOffset/instrLen - 1
    endIdx := startIdx - maxBackwardScanInstructions
    if endIdx < 0 {
        endIdx = -1
    }

    // Keep partial values so a MOVZ+MOVK sequence can be reconstructed.
    // -1 means "not observed yet".
    x16Lo := -1 // Lower 16 bits recorded from MOVK X16, #imm, LSL #0.
    x16Hi := -1 // Upper 16 bits recorded from MOVK X16, #imm, LSL #16.

    for i := startIdx; i > endIdx; i-- {
        off := i * instrLen
        if off < 0 {
            break
        }
        word := binary.LittleEndian.Uint32(funcCode[off:])

        // MOVZ X16, #imm (LSL #0) terminates the sequence and sets the low bits.
        // Combine it with a previously observed MOVK #hi if present.
        if word&^imm16Mask == movzX16Base {
            lo := int((word & imm16Mask) >> imm16Shift)
            hi := 0
            if x16Hi >= 0 {
                hi = x16Hi
            }
            return stripBSDPrefix(hi | lo), true
        }

        // MOVZ X16, #imm, LSL #16 terminates the sequence and sets the high bits.
        // Combine it with a previously observed MOVK #lo if present.
        if word&^imm16Mask == movzX16Lsl16 {
            hi := int((word & imm16Mask) >> imm16Shift) << 16
            lo := 0
            if x16Lo >= 0 {
                lo = x16Lo
            }
            return stripBSDPrefix(hi | lo), true
        }

        // MOVK X16, #imm (LSL #0): record the low 16 bits and continue scanning.
        if word&^imm16Mask == movkX16Base {
            x16Lo = int((word & imm16Mask) >> imm16Shift)
            continue
        }

        // MOVK X16, #imm, LSL #16: record the high 16 bits and continue scanning.
        if word&^imm16Mask == movkX16Lsl16 {
            x16Hi = int((word & imm16Mask) >> imm16Shift) << 16
            continue
        }

        // Stop when a control-flow instruction is reached.
        if isControlFlowInstruction(word) {
            break
        }

        // Stop when some other instruction writes to x16.
        if writesX16NotMovzMovk(word) {
            break
        }
    }
    return 0, false
}

// stripBSDPrefix removes the BSD class prefix (0x2000000) as required by FR-3.2.3.
func stripBSDPrefix(value int) int {
    if value >= bsdSyscallClassPrefix {
        return value - bsdSyscallClassPrefix
    }
    return value
}
```

### 4.5 制御フロー・レジスタ書き込み判定

ARM64 命令を arm64asm ライブラリなしでデコードするためのシンプルな判定関数。

```go
// isControlFlowInstruction reports whether an ARM64 instruction is a control-flow instruction.
// It recognizes B / BL / BLR / BR / RET / CBZ / CBNZ / TBZ / TBNZ.
// ARM64 instructions are fixed-width 4-byte little-endian values.
func isControlFlowInstruction(word uint32) bool {
    // B:  [31:26] = 000101 (0b000101 = 5)
    // BL: [31:26] = 100101 (0b100101 = 37)
    if word>>26 == 0b000101 || word>>26 == 0b100101 {
        return true
    }
    // BLR / BR / RET: [31:22] = 1101011000 (0b1101011000)
    // BR:  full encoding = 0xD61F0000 | (Rn << 5)
    // BLR: full encoding = 0xD63F0000 | (Rn << 5)
    // RET: full encoding = 0xD65F0000 | (Rn << 5)
    if word>>22 == 0b1101011000 {
        return true
    }
    // CBZ / CBNZ: [30:24] = 0110100 / 0110101
    // [30:25] = 011010, [24] = 0 (CBZ) or 1 (CBNZ)
    if (word>>25)&0x3F == 0b011010 {
        return true
    }
    // TBZ / TBNZ: [30:25] = 011011
    if (word>>25)&0x3F == 0b011011 {
        return true
    }
    return false
}

// writesX16NotMovzMovk detects 64-bit instructions that write to x16,
// excluding MOVZ and MOVK.
// It is used after MOVZ/MOVK handling inside backwardScanX16.
//
// MOVZ/MOVK share a fixed [28:23] = 100101 pattern.
// When that pattern is present, this helper returns false because the caller
// already handled those instructions.
func writesX16NotMovzMovk(word uint32) bool {
    // MOVZ/MOVK: [28:23] = 100101, with Rd encoded in [4:0].
    // Reaching this point means either MOVZ/MOVK targeting another register
    // or a different encoding, so perform a generic x16-write check.
    bits28_23 := (word >> 23) & 0x3F
    if bits28_23 == 0b100101 {
        // MOVZ or MOVK encoding for some Rd: already handled by the caller.
        return false
    }
    // 64-bit instruction (sf=1) with Rd=16 ([4:0] = 0b10000).
    return word>>31 == 1 && word&0x1F == 0x10
}
```

**設計上の注意**: `backwardScanX16` のループ内では MOVZ/MOVK の検出を最初に行い、
それ以外の場合のみ `writesX16NotMovzMovk` を呼び出す構造としている。
これにより MOVK X16 がスキャンを中断することなく部分値を積み上げられる。

## 5. `internal/libccache/macho_cache.go`（新規）

### 5.1 型定義とコンストラクタ

```go
package libccache

import (
    "bytes"
    "debug/macho"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/isseis/go-safe-cmd-runner/internal/filevalidator/pathencoding"
)

// MachoLibSystemCacheManager manages read and write of libSystem analysis cache files.
// Uses the same LibcCacheFile schema and cache directory as LibcCacheManager.
type MachoLibSystemCacheManager struct {
    cacheDir string
    analyzer *MachoLibSystemAnalyzer
    pathEnc  *pathencoding.SubstitutionHashEscape
}

// NewMachoLibSystemCacheManager creates a new MachoLibSystemCacheManager.
// cacheDir is the path to the cache directory (created automatically if it does not exist).
func NewMachoLibSystemCacheManager(cacheDir string) (*MachoLibSystemCacheManager, error) {
    if err := os.MkdirAll(cacheDir, cacheDirPerm); err != nil {
        return nil, fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
    }
    return &MachoLibSystemCacheManager{
        cacheDir: cacheDir,
        analyzer: &MachoLibSystemAnalyzer{},
        pathEnc:  pathencoding.NewSubstitutionHashEscape(),
    }, nil
}
```

### 5.2 `GetOrCreate()` メソッドの実装

```go
// GetOrCreate returns cached wrappers, or analyzes libsystem_kernel and creates cache on miss.
// libPath is used for cache file naming and the lib_path field (install name or real file path).
// libHash is the "sha256:<hex>" validity hash.
// getData is a callback that returns Mach-O bytes on cache miss only.
func (m *MachoLibSystemCacheManager) GetOrCreate(
    libPath, libHash string,
    getData func() ([]byte, error),
) ([]WrapperEntry, error) {
    encodedName, err := m.pathEnc.Encode(libPath)
    if err != nil {
        return nil, fmt.Errorf("failed to encode libsystem path: %w", err)
    }
    cacheFilePath := filepath.Join(m.cacheDir, encodedName)

    // Load and validate any existing cache entry (FR-3.1.4).
    if data, readErr := os.ReadFile(cacheFilePath); readErr == nil { //nolint:nestif,gosec // G304: cacheFilePath = cacheDir + pathEnc.Encode(libPath), both trusted
        var cache LibcCacheFile
        if jsonErr := json.Unmarshal(data, &cache); jsonErr == nil {
            if cache.SchemaVersion == LibcCacheSchemaVersion && cache.LibHash == libHash {
                return cache.SyscallWrappers, nil
            }
        }
    }

    // Cache miss: obtain Mach-O bytes through getData() and analyze them.
    machoBytes, err := getData()
    if err != nil {
        return nil, fmt.Errorf("%w: %w", ErrLibcFileNotAccessible, err)
    }

    machoFile, err := macho.NewFile(bytes.NewReader(machoBytes))
    if err != nil {
        return nil, fmt.Errorf("failed to parse Mach-O bytes: %w", err)
    }
    defer func() { _ = machoFile.Close() }()

    wrappers, err := m.analyzer.Analyze(machoFile)
    if err != nil {
        return nil, err
    }

    // Write the cache file atomically, same as the ELF path.
    cache := LibcCacheFile{
        SchemaVersion:   LibcCacheSchemaVersion,
        LibPath:         libPath,
        LibHash:         libHash,
        AnalyzedAt:      time.Now().UTC().Format(time.RFC3339),
        SyscallWrappers: wrappers,
    }
    cacheData, err := json.MarshalIndent(cache, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
    }
    if err := writeFileAtomic(cacheFilePath, cacheData, cacheFilePerm); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrCacheWriteFailed, err)
    }

    return wrappers, nil
}
```

## 6. `internal/libccache/matcher.go` の拡張

### 6.1 `MatchWithMethod()` の追加

```go
// MatchWithMethod is equivalent to Match, but allows the caller to specify
// the DeterminationMethod explicitly.
// It is used by the Mach-O path to set "lib_cache_match" on cache hits and
// "symbol_name_match" on fallback.
func (m *ImportSymbolMatcher) MatchWithMethod(
    importSymbols []string,
    wrappers []WrapperEntry,
    determinationMethod string,
) []common.SyscallInfo {
    // Build a symbol-name to WrapperEntry map.
    wrapperMap := make(map[string]WrapperEntry, len(wrappers))
    for _, w := range wrappers {
        wrapperMap[w.Name] = w
    }

    // Deduplicate by Number, keeping the lexicographically smallest symbol name.
    candidate := make(map[int]WrapperEntry)
    for _, sym := range importSymbols {
        w, ok := wrapperMap[sym]
        if !ok {
            continue
        }
        prev, seen := candidate[w.Number]
        if !seen || w.Name < prev.Name {
            candidate[w.Number] = w
        }
    }

    result := make([]common.SyscallInfo, 0, len(candidate))
    for _, w := range candidate {
        info := common.SyscallInfo{
            Number:              w.Number,
            Name:                m.syscallTable.GetSyscallName(w.Number),
            IsNetwork:           m.syscallTable.IsNetworkSyscall(w.Number),
            Location:            0,
            DeterminationMethod: determinationMethod,
            Source:              SourceLibsystemSymbolImport,
        }
        if info.Name == "" {
            info.Name = w.Name
        }
        result = append(result, info)
    }
    sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })

    return result
}
```

## 7. `internal/libccache/adapters.go` の拡張

### 7.1 `MachoLibSystemAdapter` の追加

```go
// MachoLibSystemAdapter implements filevalidator.LibSystemCacheInterface
// by combining MachoLibSystemCacheManager and ImportSymbolMatcher.
type MachoLibSystemAdapter struct {
    cacheMgr     *MachoLibSystemCacheManager
    fs           safefileio.FileSystem
    syscallTable SyscallNumberTable
}

// NewMachoLibSystemAdapter creates a new MachoLibSystemAdapter.
func NewMachoLibSystemAdapter(
    cacheMgr *MachoLibSystemCacheManager,
    fs safefileio.FileSystem,
) *MachoLibSystemAdapter {
    return &MachoLibSystemAdapter{
        cacheMgr:     cacheMgr,
        fs:           fs,
        syscallTable: MacOSSyscallTable{},
    }
}
```

### 7.2 `GetSyscallInfos()` の実装

```go
// GetSyscallInfos resolves libsystem_kernel.dylib source from dynLibDeps,
// gets/creates the wrapper cache, matches importSymbols against the cache,
// and returns detected SyscallInfo entries.
//
// Fallback conditions (FR-3.4.1):
//   - dynLibDeps does not contain a libSystem-family library
//   - dyld shared cache extraction also failed
func (a *MachoLibSystemAdapter) GetSyscallInfos(
    dynLibDeps []fileanalysis.LibEntry,
    importSymbols []string,
) ([]common.SyscallInfo, error) {
    source, err := machodylib.ResolveLibSystemKernel(dynLibDeps, a.fs)
    if err != nil {
        return nil, err
    }

    if source == nil {
        reason := classifyLibSystemFallbackReason(dynLibDeps)

        // Fallback to name-only matching (FR-3.4.2).
        slog.Info("libSystem cache unavailable; falling back to symbol-name matching",
            "reason", reason)
        result := a.fallbackNameMatch(importSymbols)
        slog.Info("libSystem fallback matching completed",
            "reason", reason,
            "detected_syscalls", len(result))
        return result, nil
    }

    // Load or create the cache.
    wrappers, err := a.cacheMgr.GetOrCreate(source.Path, source.Hash, source.GetData)
    if err != nil {
        var archErr *elfanalyzer.UnsupportedArchitectureError
        if errors.As(err, &archErr) {
            slog.Info("Skipping libsystem_kernel.dylib analysis because the library is not arm64",
                "machine", archErr.Machine)
            return nil, nil
        }
        return nil, err
    }

    // Match imported symbols against the cache (FR-3.3.2).
    matcher := NewImportSymbolMatcher(a.syscallTable)
    return matcher.MatchWithMethod(importSymbols, wrappers, DeterminationMethodLibCacheMatch), nil
}

// classifyLibSystemFallbackReason classifies the fallback reason required by FR-3.4.3.
// If DynLibDeps has no libSystem umbrella or kernel entry, the reason is
// "missing_libsystem_dependency". Otherwise the resolver already attempted filesystem and
// dyld cache resolution and the reason is "dyld_extraction_unavailable".
func classifyLibSystemFallbackReason(dynLibDeps []fileanalysis.LibEntry) string {
    const (
        umbrellaInstallName = "/usr/lib/libSystem.B.dylib"
        kernelBaseName      = "libsystem_kernel.dylib"
    )

    for _, entry := range dynLibDeps {
        if entry.SOName == umbrellaInstallName || filepath.Base(entry.SOName) == kernelBaseName {
            return "dyld_extraction_unavailable"
        }
    }
    return "missing_libsystem_dependency"
}

// fallbackNameMatch implements the symbol-name fallback defined in FR-3.4.2.
// It matches importSymbols against the macOS network-related syscall wrapper list
// and returns the resulting SyscallInfo entries.
func (a *MachoLibSystemAdapter) fallbackNameMatch(importSymbols []string) []common.SyscallInfo {
    // Build a set of imported symbols.
    symSet := make(map[string]bool, len(importSymbols))
    for _, s := range importSymbols {
        symSet[s] = true
    }

    var result []common.SyscallInfo
    for _, name := range networkSyscallWrapperNames {
        if !symSet[name] {
            continue
        }
        // Reverse-lookup the syscall number from the macOS syscall table.
        number := -1
        for num, entry := range macOSSyscallEntries {
            if entry.name == name {
                number = num
                break
            }
        }
        if number < 0 {
            continue
        }
        result = append(result, common.SyscallInfo{
            Number:              number,
            Name:                name,
            IsNetwork:           a.syscallTable.IsNetworkSyscall(number),
            Location:            0,
            DeterminationMethod: DeterminationMethodSymbolNameMatch,
            Source:              SourceLibsystemSymbolImport,
        })
    }

    // Sort by Number.
    sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })
    return result
}
```

**Import 追加**: `adapters.go` に以下を追加する。

```go
import (
    // Add to the existing import list.
    "errors"
    "log/slog"
    "path/filepath"
    "sort"

    "github.com/isseis/go-safe-cmd-runner/internal/common"
    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/machodylib"
    "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)
```

## 8. `internal/machodylib/dyld_extractor.go`（新規 - non-Darwin スタブ）

```go
//go:build !darwin

package machodylib

// LibSystemKernelBytes holds the in-memory bytes of libsystem_kernel.dylib
// extracted from the dyld shared cache, together with its SHA-256 hash.
type LibSystemKernelBytes struct {
    Data []byte // Mach-O bytes.
    Hash string // "sha256:<hex>" (SHA-256 of Data).
}

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache. On non-Darwin platforms, always returns nil, nil.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
    return nil, nil
}
```

## 9. `internal/machodylib/dyld_extractor_darwin.go`（新規 - Darwin 実装）

```go
//go:build darwin

package machodylib

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "log/slog"
    "os"

    "github.com/blacktop/ipsw/pkg/dyld"
)

// dyldSharedCachePaths is the ordered list of dyld shared cache paths to try (FR-3.1.6).
var dyldSharedCachePaths = []string{
    "/System/Library/dyld/dyld_shared_cache_arm64e",
    "/System/Library/dyld/dyld_shared_cache_arm64",
}

// libsystemKernelInstallName is the install name of libsystem_kernel.dylib inside the dyld shared cache.
const libsystemKernelInstallName = "/usr/lib/system/libsystem_kernel.dylib"

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache.
//
// On failure (cache not found, image not found, or extraction failure),
// return nil, nil and let the caller fall back as defined in FR-3.1.6.
// Emit logs at slog.Info level.
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
    // Try the configured dyld shared cache paths (FR-3.1.6).
    var cachePath string
    for _, p := range dyldSharedCachePaths {
        if _, err := os.Stat(p); err == nil {
            cachePath = p
            break
        }
    }
    if cachePath == "" {
        slog.Info("dyld shared cache not found; applying fallback",
            "tried", dyldSharedCachePaths)
        return nil, nil
    }

    // Parse the shared cache using blacktop/ipsw/pkg/dyld.
    f, err := dyld.Open(cachePath)
    if err != nil {
        slog.Info("Failed to open dyld shared cache; applying fallback",
            "path", cachePath, "error", err)
        return nil, nil
    }
    defer func() { _ = f.Close() }()

    // Locate the libsystem_kernel.dylib image.
    image := f.Image(libsystemKernelInstallName)
    if image == nil {
        slog.Info("libsystem_kernel.dylib was not found in the dyld shared cache; applying fallback",
            "cache_path", cachePath,
            "install_name", libsystemKernelInstallName)
        return nil, nil
    }

    // Materialize the image as Mach-O bytes.
    // Concrete pkg/dyld API details are isolated in a helper.
    machoBytes, err := extractMachOImageBytes(f, image)
    if err != nil {
        slog.Info("Failed to obtain libsystem_kernel.dylib bytes; applying fallback",
            "error", err)
        return nil, nil
    }

    // Compute the SHA-256 hash as required by FR-3.1.4 and FR-3.1.6.
    h := sha256.Sum256(machoBytes)
    hash := fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))

    return &LibSystemKernelBytes{
        Data: machoBytes,
        Hash: hash,
    }, nil
}

// extractMachOImageBytes hides pkg/dyld API details and returns a standalone Mach-O byte slice
// for the selected image. This helper is the only place allowed to depend on concrete
// pkg/dyld method names so future API changes do not affect resolver logic or tests.
func extractMachOImageBytes(cache *dyld.File, image *dyld.CacheImage) ([]byte, error)
```

## 10. `internal/machodylib/libsystem_resolver.go`（新規）

### 10.1 型定義

```go
package machodylib

import (
    "fmt"
    "log/slog"
    "os"
    "path/filepath"

    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// LibSystemKernelSource represents the resolved source of libsystem_kernel.dylib.
type LibSystemKernelSource struct {
    // Path is used for the lib_path field and cache file naming.
    // Filesystem case: the resolved library path.
    // dyld shared cache case: the install name "/usr/lib/system/libsystem_kernel.dylib".
    Path string
    // Hash is the cache validity hash in "sha256:<hex>" format.
    Hash string
    // GetData returns Mach-O bytes and is called only on cache miss.
    GetData func() ([]byte, error)
}

// wellKnownLibsystemKernelPath is the well-known filesystem path for libsystem_kernel.dylib.
const wellKnownLibsystemKernelPath = "/usr/lib/system/libsystem_kernel.dylib"

// libSystemBDylibInstallName is the install name of the umbrella framework.
const libSystemBDylibInstallName = "/usr/lib/libSystem.B.dylib"

// libsystemKernelBaseName is the base name of libsystem_kernel.dylib.
const libsystemKernelBaseName = "libsystem_kernel.dylib"
```

### 10.2 `ResolveLibSystemKernel()` の実装

```go
// ResolveLibSystemKernel resolves the libsystem_kernel.dylib source from DynLibDeps.
//
// Returns nil, nil when:
//   - no libSystem-family library is present in DynLibDeps (non-libSystem binary)
//   - dyld shared cache extraction also failed (fallback path)
// Returns error only for unrecoverable conditions (permission errors, hash computation failures).
func ResolveLibSystemKernel(
    dynLibDeps []fileanalysis.LibEntry,
    fs safefileio.FileSystem,
) (*LibSystemKernelSource, error) {
    // Step 1: Collect libSystem umbrella and kernel candidates from DynLibDeps (FR-3.1.5).
    candidates := findLibSystemCandidates(dynLibDeps)

    if !candidates.HasLibSystem {
        // No libSystem-family library in DynLibDeps: return nil (fallback condition 2).
        return nil, nil
    }

    // Step 2: Choose a filesystem candidate path in this order:
    // direct kernel entry, umbrella re-export, then the well-known path.
    candidatePath, err := resolveLibSystemKernelPath(candidates, fs)
    if err != nil {
        return nil, err
    }

    // Step 3: Use the filesystem path if one was resolved.
    if candidatePath != "" {
        // Filesystem path resolved: compute its hash.
        hash, err := computeFileHash(fs, candidatePath)
        if err != nil {
            return nil, fmt.Errorf("failed to compute hash for %s: %w", candidatePath, err)
        }
        path := candidatePath
        return &LibSystemKernelSource{
            Path: path,
            Hash: hash,
            GetData: func() ([]byte, error) {
                return os.ReadFile(path) //nolint:gosec // G304: path is a system library path from DynLibDeps or well-known
            },
        }, nil
    }

    // Step 4: Try dyld shared cache extraction (FR-3.1.6).
    extracted, err := ExtractLibSystemKernelFromDyldCache()
    if err != nil {
        // Unexpected error. Normally the extractor returns nil, nil on fallback cases.
        return nil, fmt.Errorf("dyld shared cache extraction failed unexpectedly: %w", err)
    }
    if extracted == nil {
        // Extraction failed: return nil and let the caller enter the fallback path.
        slog.Info("dyld shared cache extraction for libsystem_kernel.dylib also failed; applying fallback",
            "candidate_path", candidatePath)
        return nil, nil
    }

    // Extraction succeeded: use the install name as the cache path (FR-3.1.6).
    data := extracted.Data
    return &LibSystemKernelSource{
        Path:    libsystemKernelInstallName,
        Hash:    extracted.Hash,
        GetData: func() ([]byte, error) { return data, nil },
    }, nil
}
```

### 10.3 ヘルパー関数

```go
// libSystemCandidates holds the relevant DynLibDeps entries for FR-3.1.5 resolution.
type libSystemCandidates struct {
    Umbrella     *fileanalysis.LibEntry
    Kernel       *fileanalysis.LibEntry
    HasLibSystem bool
}

// findLibSystemCandidates collects libSystem umbrella and kernel candidates from DynLibDeps (FR-3.1.5).
//
// Return value interpretation:
//   - Kernel != nil:
//       libsystem_kernel.dylib is directly present in DynLibDeps.
//   - Kernel == nil, Umbrella != nil:
//       Only libSystem.B.dylib is present in DynLibDeps.
//       ResolveLibSystemKernel first resolves LC_REEXPORT_DYLIB from the umbrella binary.
//   - HasLibSystem == false:
//       No libSystem-family dependency is present.
//       ResolveLibSystemKernel returns nil, nil and lets the caller fall back.
func findLibSystemCandidates(dynLibDeps []fileanalysis.LibEntry) libSystemCandidates {
    var result libSystemCandidates
    for i, entry := range dynLibDeps {
        if entry.SOName == libSystemBDylibInstallName {
            e := dynLibDeps[i]
            result.Umbrella = &e
            result.HasLibSystem = true
        }

        // A direct kernel dependency takes precedence as the filesystem source.
        if filepath.Base(entry.SOName) == libsystemKernelBaseName {
            e := dynLibDeps[i]
            result.Kernel = &e
            result.HasLibSystem = true
        }
    }
    return result
}

// resolveLibSystemKernelPath decides the filesystem path candidate according to FR-3.1.5.
// Order:
//   1. direct kernel Path from DynLibDeps
//   2. umbrella file on disk -> LC_REEXPORT_DYLIB -> task 0096 resolver reuse
//   3. well-known path /usr/lib/system/libsystem_kernel.dylib
//   4. empty string (caller proceeds to dyld shared cache extraction)
func resolveLibSystemKernelPath(candidates libSystemCandidates, fs safefileio.FileSystem) (string, error)
```

## 11. `internal/filevalidator/validator.go` の拡張

### 11.1 インターフェース定義とフィールド追加

```go
// LibSystemCacheInterface abstracts libSystem wrapper cache operations for Mach-O.
type LibSystemCacheInterface interface {
    // GetSyscallInfos resolves the libsystem_kernel.dylib source from dynLibDeps,
    // matches importSymbols against the cache, and returns the detected syscalls.
    // Returns nil, nil when libSystem is not in dynLibDeps or all fallback paths failed.
    GetSyscallInfos(
        dynLibDeps []fileanalysis.LibEntry,
        importSymbols []string,
    ) ([]common.SyscallInfo, error)
}
```

`Validator` 構造体に `libSystemCache LibSystemCacheInterface` フィールドを追加する（`libcCache` フィールドの後）。

```go
// SetLibSystemCache injects the LibSystemCacheInterface used during record operations.
func (v *Validator) SetLibSystemCache(m LibSystemCacheInterface) {
    v.libSystemCache = m
}
```

### 11.2 `updateAnalysisRecord()` の変更

タスク 0097 後の現行 svc スキャン処理を以下のように変更する。

**変更前の該当箇所** (`internal/filevalidator/validator.go` の svc スキャン部分):

```go
// Current Mach-O arm64 svc #0x80 scan.
{
    addrs, svcErr := machoanalyzer.CollectSVCAddressesFromFile(filePath.String(), v.fileSystem)
    if svcErr != nil {
        return fmt.Errorf("mach-o svc scan failed: %w", svcErr)
    }
    if len(addrs) > 0 {
        record.SyscallAnalysis = buildSVCSyscallAnalysis(addrs)
    }
}
```

**変更後**:

```go
// Mach-O arm64 svc #0x80 scan and libSystem import-symbol matching.
// Merge both results and store them in record.SyscallAnalysis (task 0100).
{
    addrs, svcErr := machoanalyzer.CollectSVCAddressesFromFile(filePath.String(), v.fileSystem)
    if svcErr != nil {
        return fmt.Errorf("mach-o svc scan failed: %w", svcErr)
    }
    svcEntries := buildSVCSyscallEntries(addrs)

    libsysEntries, libsysErr := v.analyzeLibSystemImports(record, filePath.String())
    if libsysErr != nil {
        return fmt.Errorf("libSystem import analysis failed: %w", libsysErr)
    }

    merged := mergeSyscallInfos(svcEntries, libsysEntries)
    if len(merged) > 0 {
        record.SyscallAnalysis = buildMachoSyscallAnalysisData(svcEntries, libsysEntries)
    }
}
```

### 11.3 新規ヘルパー関数の実装

```go
// buildSVCSyscallEntries converts a list of svc #0x80 addresses into []common.SyscallInfo.
// It returns nil when addrs is empty.
func buildSVCSyscallEntries(addrs []uint64) []common.SyscallInfo {
    if len(addrs) == 0 {
        return nil
    }
    syscalls := make([]common.SyscallInfo, len(addrs))
    for i, addr := range addrs {
        syscalls[i] = common.SyscallInfo{
            Number:              -1,
            IsNetwork:           false,
            Location:            addr,
            DeterminationMethod: "direct_svc_0x80",
            Source:              "direct_svc_0x80",
        }
    }
    return syscalls
}

// buildMachoSyscallAnalysisData merges svc and libSystem entries and constructs
// SyscallAnalysisData.
// AnalysisWarnings is populated only when svc entries exist.
// DetectedSyscalls is sorted by Number as required by FR-3.3.3.
func buildMachoSyscallAnalysisData(
    svcEntries []common.SyscallInfo,
    libsysEntries []common.SyscallInfo,
) *fileanalysis.SyscallAnalysisData {
    merged := mergeSyscallInfos(svcEntries, libsysEntries)

    var warnings []string
    if len(svcEntries) > 0 {
        warnings = []string{"svc #0x80 detected: direct syscall bypassing libSystem.dylib"}
    }

    return &fileanalysis.SyscallAnalysisData{
        SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
            Architecture:     "arm64",
            AnalysisWarnings: warnings,
            DetectedSyscalls: merged,
        },
    }
}

// mergeSyscallInfos combines svc entries and libSystem entries and sorts them by Number.
// svc entries use Number=-1 and remain distinct by Location.
// libSystem entries use Number>=0 and are already deduplicated inside GetSyscallInfos.
// Their Number ranges do not overlap, so no extra deduplication is required.
//
// After sorting, svc entries appear first because -1 < 0 < positive numbers.
// The runner identifies svc entries by DeterminationMethod rather than Number.
func mergeSyscallInfos(svcEntries, libsysEntries []common.SyscallInfo) []common.SyscallInfo {
    if len(svcEntries) == 0 && len(libsysEntries) == 0 {
        return nil
    }
    merged := make([]common.SyscallInfo, 0, len(svcEntries)+len(libsysEntries))
    merged = append(merged, svcEntries...)
    merged = append(merged, libsysEntries...)
    sort.Slice(merged, func(i, j int) bool {
        return merged[i].Number < merged[j].Number
    })
    return merged
}

// analyzeLibSystemImports obtains imported symbols from the target Mach-O binary
// and matches them against the libSystem cache (FR-3.3.2).
// It returns nil, nil when v.libSystemCache is nil, DynLibDeps is empty, or the file is not Mach-O.
func (v *Validator) analyzeLibSystemImports(
    record *fileanalysis.Record,
    filePath string,
) ([]common.SyscallInfo, error) {
    if v.libSystemCache == nil || len(record.DynLibDeps) == 0 {
        return nil, nil
    }

    // Obtain Mach-O imported symbols using debug/macho.
    // Return nil, nil for non-Mach-O files such as ELF binaries.
    importSymbols, err := getMachoImportSymbols(v.fileSystem, filePath)
    if err != nil || importSymbols == nil {
        return nil, err
    }

    // Strip the Mach-O underscore prefix before matching (FR-3.3.2).
    // "_socket" -> "socket", "_socket$UNIX2003" -> "socket"
    normalized := make([]string, len(importSymbols))
    for i, sym := range importSymbols {
        normalized[i] = machoanalyzer.NormalizeSymbolName(sym)
    }

    return v.libSystemCache.GetSyscallInfos(record.DynLibDeps, normalized)
}

// getMachoImportSymbols opens filePath as a Mach-O file and returns imported symbols.
// It returns nil, nil for non-Mach-O files.
func getMachoImportSymbols(fs safefileio.FileSystem, filePath string) ([]string, error) {
    f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
    if err != nil {
        return nil, err
    }
    defer func() { _ = f.Close() }()

    mf, err := macho.NewFile(f)
    if err != nil {
        // Non-Mach-O file such as ELF: skip it.
        return nil, nil //nolint:nilerr // Mach-O parse failure means a non-Mach-O file; callers expect nil, nil
    }
    defer func() { _ = mf.Close() }()

    syms, err := mf.ImportedSymbols()
    if err != nil {
        return nil, fmt.Errorf("failed to get imported symbols: %w", err)
    }
    return syms, nil
}
```

**既存の `buildSVCSyscallAnalysis` は削除し `buildSVCSyscallEntries` + `buildMachoSyscallAnalysisData` に置き換える。**

## 12. `internal/runner/security/machoanalyzer/symbol_normalizer.go` の拡張

`normalizeSymbolName` をエクスポートする（`filevalidator` から再利用するため）。

```go
// NormalizeSymbolName strips the leading underscore and version suffix
// from a macOS imported symbol name.
// Keep normalizeSymbolName (lowercase) for package-internal compatibility.
func NormalizeSymbolName(name string) string {
    return normalizeSymbolName(name)
}
```

## 13. `internal/runner/security/network_analyzer.go` の拡張

### 13.1 新規ヘルパー関数

```go
// syscallAnalysisHasNetworkSignal checks whether SyscallAnalysisResult contains
// any IsNetwork==true entry (FR-3.6.2 priority 2).
func syscallAnalysisHasNetworkSignal(result *fileanalysis.SyscallAnalysisResult) bool {
    if result == nil {
        return false
    }
    for _, entry := range result.DetectedSyscalls {
        if entry.IsNetwork {
            return true
        }
    }
    return false
}
```

### 13.2 `isNetworkViaBinaryAnalysis()` の拡張

**挿入位置**: `network_analyzer.go` の `isNetworkViaBinaryAnalysis()` 内で、
svc 結果を処理する switch 文のケース後（`// No svc #0x80 signal:` コメントが付いた
`if a.syscallStore != nil { ... }` ブロックの直後）、かつ `if data == nil { return false, false }` の前に挿入する。

具体的には現行コードの以下の位置の直後:

```go
// Existing code (unchanged).
case errors.Is(svcErr, fileanalysis.ErrNoSyscallAnalysis):
    // Schema v15+ guarantee: svc scan was performed and found nothing.
    // Fall through to SymbolAnalysis-based decision.
// ... other cases ...
}
// Insert the IsNetwork check here.
```

**`ErrNoSyscallAnalysis` は変更しない**: システムコールを直接呼び出していない
バイナリは `SyscallAnalysis` フィールドを持たない。「解析が実施された」という
保証は `SchemaVersion` が担うため、`ErrNoSyscallAnalysis` は「解析済みだが未検出」を
意味する正常ケースである（ELF バイナリと同一ポリシー）。

**追加コード**:

```go
// IsNetwork check: look for libSystem-derived network syscalls
// (FR-3.6.2 priority 2).
// Because the direct_svc_0x80 check happens inside the switch earlier,
// reaching this point means no svc #0x80 signal was detected.
if syscallAnalysisHasNetworkSignal(svcResult) {
    slog.Info("SyscallAnalysis cache indicates libSystem network syscall",
        "path", cmdPath)
    return true, false
}

// No svc #0x80 signal: determine result from SymbolAnalysis.
// Existing code continues with the data == nil { return false, false } path.
```

**優先順位の実現**: `direct_svc_0x80` チェック（switch 内の `syscallAnalysisHasSVCSignal` 呼び出し）が
この `IsNetwork` チェックより先に行われるため、FR-3.6.2 の優先順位 1 → 2 → 3 が自然に実現される。

## 14. `cmd/record/main.go` の拡張

`MachoLibSystemAdapter` を初期化して `Validator` に注入する。

```go
// Add this immediately after the existing LibcCacheManager initialization.
machoLibSystemCacheMgr, err := libccache.NewMachoLibSystemCacheManager(libCacheDir)
if err != nil {
    log.Fatalf("failed to create Mach-O libSystem cache manager: %v", err)
}
machoAdapter := libccache.NewMachoLibSystemAdapter(machoLibSystemCacheMgr, fs)
fv.SetLibSystemCache(machoAdapter)
```

`libCacheDir` は ELF 版 `LibcCacheManager` と同じディレクトリを使用する（FR-3.1.3）。

## 15. テスト仕様

### 15.1 受け入れ条件とテストの対応

| AC | テストファイル | テスト関数名（案） |
|----|--------------|----------------|
| AC-1 | `internal/libccache/macos_syscall_table_test.go` | `TestMacOSSyscallTable_NetworkEntries`, `TestMacOSSyscallTable_SocketNumber` |
| AC-2 | `internal/libccache/macho_analyzer_test.go`, `internal/libccache/macho_cache_test.go` | `TestMachoLibSystemAnalyzer_Analyze_SvcDetection`, `TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge`, `TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls`, `TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval`, `TestMachoLibSystemCacheManager_CreateAndLoad` |
| AC-3 | `internal/libccache/macho_cache_test.go`, `internal/filevalidator/validator_macho_test.go` | `TestMachoLibSystemCacheManager_InvalidatesOnHashMismatch`, `TestMachoLibSystemCacheManager_InvalidatesOnSchemaMismatch`, `TestMachoLibSystemCacheManager_ReparsesBrokenCache`, `TestUpdateAnalysisRecord_LibSystemUnreadable`, `TestUpdateAnalysisRecord_LibSystemExportSymbolsFailure`, `TestUpdateAnalysisRecord_LibSystemCacheWriteFailure` |
| AC-4 | `internal/libccache/adapters_test.go`, `internal/filevalidator/validator_macho_test.go` | `TestMachoLibSystemAdapter_Fallback_NoLibSystem`, `TestMachoLibSystemAdapter_Fallback_DyldExtractFail`, `TestMachoLibSystemAdapter_Fallback_LogsReasonAndCount` |
| AC-5 | `internal/libccache/adapters_test.go`, `internal/filevalidator/validator_macho_test.go` | `TestMachoLibSystemAdapter_ImportSymbolMatching`, `TestMachoLibSystemAdapter_DedupsByNumber`, `TestAnalyzeLibSystemImports_NormalizesUnderscoreSymbols`, `TestUpdateAnalysisRecord_MergesDirectSVCAndLibSystemEntries` |
| AC-6 | `internal/runner/security/network_analyzer_test.go` | `TestIsNetworkViaBinaryAnalysis_LibSystemNetworkSyscall`, `TestIsNetworkViaBinaryAnalysis_DirectSVCOverridesLibSystem`, `TestIsNetworkViaBinaryAnalysis_LibSystemNonNetworkFallsBackToSymbols` |
| AC-7 | `make test` | 全テスト |

### 15.2 `macho_analyzer_test.go`（主要テストケース）

```go
// TestMachoLibSystemAnalyzer_Analyze_SvcDetection covers the main success path for
// size filtering and svc detection.
func TestMachoLibSystemAnalyzer_Analyze_SvcDetection(t *testing.T) {
    // Build a test Mach-O binary in memory.
    // Include an arm64 CPU type, a __TEXT,__text section, and a Symtab.
    // Place MOVZ X16, #97 (socket) immediately before svc #0x80 (0xD4001001).
}

// TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge verifies that functions larger than 256 bytes are excluded.
func TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls verifies that functions with multiple distinct syscall numbers are excluded.
func TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval verifies removal of the 0x2000000 BSD prefix (AC-2).
func TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_MultipleSameSyscall verifies that multiple svc instructions with the same syscall number are accepted.
func TestMachoLibSystemAnalyzer_Analyze_MultipleSameSyscall(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_NonArm64 verifies that non-arm64 inputs return UnsupportedArchitectureError.
func TestMachoLibSystemAnalyzer_Analyze_NonArm64(t *testing.T) { ... }
```

**テスト用 Mach-O バイナリ生成**: `debug/macho` の `NewFile` はバイナリ形式を必要とするため、
最小限の Mach-O ヘッダを手動で組み立てるか、既存の `internal/runner/security/machoanalyzer` の
テストパターンを参考にする。テスト用バイナリはファイルに書き出してテンプレートとして使用するか、
`bytes.Buffer` に直接組み立てる。

### 15.3 `adapters_test.go`（主要テストケース）

```go
// TestMachoLibSystemAdapter_GetSyscallInfos_CacheHit covers the normal cache-hit path.
// Use a mock resolver that returns LibSystemKernelSource and an existing cache file.
func TestMachoLibSystemAdapter_GetSyscallInfos_CacheHit(t *testing.T) { ... }

// TestMachoLibSystemAdapter_GetSyscallInfos_Fallback_NoLibSystem covers the fallback path when DynLibDeps has no libSystem dependency.
func TestMachoLibSystemAdapter_GetSyscallInfos_Fallback_NoLibSystem(t *testing.T) { ... }

// TestMachoLibSystemAdapter_Fallback_DetectedSyscalls_Source verifies that Source is "libsystem_symbol_import" during fallback (AC-4).
func TestMachoLibSystemAdapter_Fallback_DetectedSyscalls_Source(t *testing.T) { ... }

// TestMachoLibSystemAdapter_DeterminationMethod_LibCacheMatch verifies that DeterminationMethod is "lib_cache_match" on cache hit (AC-5).
func TestMachoLibSystemAdapter_DeterminationMethod_LibCacheMatch(t *testing.T) { ... }

// TestMachoLibSystemAdapter_DedupsByNumber verifies deduplication by Number (AC-5).
func TestMachoLibSystemAdapter_DedupsByNumber(t *testing.T) { ... }

// TestMachoLibSystemAdapter_NonArm64Library_SkipsGracefully verifies that non-arm64 libraries are skipped without returning an error (FR-3.3.1).
func TestMachoLibSystemAdapter_NonArm64Library_SkipsGracefully(t *testing.T) { ... }

// TestMachoLibSystemAdapter_Fallback_LogsReasonAndCount verifies that the fallback reason and detected count are logged via slog.Info (AC-4).
func TestMachoLibSystemAdapter_Fallback_LogsReasonAndCount(t *testing.T) { ... }
```

### 15.4 統合テスト条件

`//go:build !integration` または `t.Skip()` でプラットフォームをチェックする。

```go
// TestMachoLibSystem_Integration_DyldSharedCache runs only on macOS arm64.
func TestMachoLibSystem_Integration_DyldSharedCache(t *testing.T) {
    if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
        t.Skip("macOS arm64 only")
    }
    // Analyze a dynamic Mach-O binary through the record command and verify that
    // SyscallAnalysis contains detected network syscalls.
}

// TestMachoLibSystem_Integration_FallbackWhenDyldExtractionFails verifies that
// the final fallback path activates and record still succeeds when dyld extraction fails.
func TestMachoLibSystem_Integration_FallbackWhenDyldExtractionFails(t *testing.T) { ... }
```

## 16. build タグ方針

| ファイル | build タグ | 理由 |
|---------|-----------|------|
| `dyld_extractor.go` | `//go:build !darwin` | non-Darwin スタブ |
| `dyld_extractor_darwin.go` | `//go:build darwin` | Darwin 実装 |
| `macho_analyzer.go` | なし | `debug/macho` は全プラットフォームで利用可能 |
| `macos_syscall_table.go` | なし | プラットフォーム非依存 |
| `macho_cache.go` | なし | プラットフォーム非依存 |

`blacktop/ipsw/pkg/dyld` は Darwin でのみビルドされるため `go.mod` への追加は Darwin 環境でのみ有効となる。
`go build` の Linux 環境では `dyld_extractor.go`（スタブ）のみがコンパイルされ、
`github.com/blacktop/ipsw/pkg/dyld` への依存は発生しない。

## 17. エラー定義

Mach-O 版で新しい sentinel error は追加しない。既存の `libccache` エラーを再利用し、
必要なら呼び出し側でコンテキストを付けてラップする。

```go
// Reuse existing errors to avoid a parallel Mach-O-only error hierarchy.
// - file access failure: ErrLibcFileNotAccessible
// - export symbol retrieval failure: ErrExportSymbolsFailed
// - cache write failure: ErrCacheWriteFailed
```

**実装注意**: エラーメッセージの可読性が必要な場合は
`fmt.Errorf("libsystem cache error: %w", ErrLibcFileNotAccessible)` のように文脈を追加する。

## 18. `go.mod` / `go.sum` の更新

`blacktop/ipsw/pkg/dyld` を依存として追加する。

```
go get github.com/blacktop/ipsw@<pinned-version>
```

**注意**: `blacktop/ipsw` は大きなモジュールであり、`pkg/dyld` サブパッケージのみが必要なため、
Go モジュールの最小バージョン選択（MVS）により実際の依存グラフが肥大化する可能性がある。
本タスクでは通常の `go.mod` 依存追加を採用し、コミット時には `latest` ではなく
具体バージョンを固定する。ベンダリングは採用しない。
