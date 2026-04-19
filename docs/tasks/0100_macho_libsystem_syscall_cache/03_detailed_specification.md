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
//go:build !integration_only

package libccache

// macOSSyscallEntry は BSD syscall エントリの内部表現。
type macOSSyscallEntry struct {
    name      string
    isNetwork bool
}

// MacOSSyscallTable implements SyscallNumberTable for macOS arm64 BSD syscalls.
type MacOSSyscallTable struct{}

// macOSSyscallEntries は macOS arm64 の BSD syscall テーブル。
// キーは BSD クラスプレフィックス 0x2000000 を除いた syscall 番号。
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

// networkSyscallWrapperNames は FR-3.4.2 のフォールバック照合に使用する
// ネットワーク関連 syscall ラッパー関数名のリスト。
// sendmmsg / recvmmsg は Linux 固有であり macOS には存在しないため含めない。
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

// maxBackwardScanInstructions は x16 後方スキャンの最大命令数。
const maxBackwardScanInstructions = 16

// bsdSyscallClassPrefix は macOS arm64 BSD syscall クラスプレフィックス（0x2000000）。
const bsdSyscallClassPrefix = 0x2000000

// svcMacOSEncoding は "svc #0x80" 命令のリトルエンディアン uint32 エンコーディング。
// ARM64 エンコーディング: 0xD4001001
const svcMacOSEncoding = uint32(0xD4001001)

// MachoLibSystemAnalyzer analyzes a libsystem_kernel.dylib Mach-O file and returns
// a list of syscall wrapper functions.
type MachoLibSystemAnalyzer struct{}
```

### 4.2 `Analyze()` メソッドの実装

```go
// Analyze scans exported functions in machoFile and returns WrapperEntry values
// for functions recognized as syscall wrappers.
// arm64 以外のアーキテクチャは *elfanalyzer.UnsupportedArchitectureError を返す。
// 戻り値は Number 昇順・同一 Number 内で Name 昇順にソートされる。
func (a *MachoLibSystemAnalyzer) Analyze(machoFile *macho.File) ([]WrapperEntry, error) {
    if machoFile.Cpu != macho.CpuArm64 {
        return nil, &elfanalyzer.UnsupportedArchitectureError{Machine: fmt.Sprintf("%v", machoFile.Cpu)}
    }

    // __TEXT,__text セクションを取得する。
    textSection := machoFile.Section("__text")
    if textSection == nil || textSection.Seg != "__TEXT" {
        return nil, nil
    }
    code, err := textSection.Data()
    if err != nil {
        return nil, fmt.Errorf("failed to read __TEXT,__text section: %w", err)
    }
    textBase := textSection.Addr // 仮想アドレスベース

    // LC_SYMTAB から外部定義シンボルを列挙する。
    if machoFile.Symtab == nil {
        return nil, nil
    }

    // アドレス昇順でソートしてシンボルサイズを推定する。
    syms := filterFunctionSymbols(machoFile.Symtab.Syms)
    sort.Slice(syms, func(i, j int) bool {
        return syms[i].Value < syms[j].Value
    })

    textEnd := textBase + uint64(len(code))
    var entries []WrapperEntry

    for i, sym := range syms {
        // 関数サイズを推定する（Mach-O symtab には st_size 相当がないため）。
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

        // サイズフィルタ（FR-3.2.2）。
        if funcSize > MaxWrapperFunctionSize {
            continue
        }

        // 関数バイト列をスライスする。
        startOff := int(sym.Value - textBase)  //nolint:gosec // G115: sym.Value >= textBase
        endOff := int(funcEnd - textBase)      //nolint:gosec // G115: funcEnd <= textEnd
        funcCode := code[startOff:endOff]

        // svc #0x80 を検出し、x16 後方スキャンで BSD syscall 番号を特定する。
        number, ok := analyzeWrapperFunction(funcCode)
        if !ok {
            continue
        }
        entries = append(entries, WrapperEntry{Name: sym.Name, Number: number})
    }

    // Number 昇順・同一 Number 内 Name 昇順でソートする（FR-3.1.1）。
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
// filterFunctionSymbols は Symtab から外部定義済み関数シンボルのみを返す。
// 外部定義: (sym.Type & 0x0E) == 0x0E かつ (sym.Type & 0x01) == 0（未定義でない）
// macOS Mach-O のシンボルタイプフラグ:
//   N_EXT    = 0x01 (external)
//   N_TYPE   = 0x0E (type mask)
//   N_SECT   = 0x0E (defined in section)
//   N_UNDF   = 0x00 (undefined)
func filterFunctionSymbols(syms []macho.Symbol) []macho.Symbol {
    var result []macho.Symbol
    for _, s := range syms {
        // 未定義シンボル（import）を除外する。
        if s.Sect == 0 {
            continue
        }
        // デバッグシンボル（N_STAB: type >= 0x20）を除外する。
        if s.Type >= 0x20 {
            continue
        }
        // 外部シンボルのみを対象とする（N_EXT ビットが立っていること）。
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
// analyzeWrapperFunction は funcCode（1 つの関数のバイト列）を解析し、
// 単一の BSD syscall 番号を返す。複数の異なる番号を含む場合や svc が存在しない場合は
// (0, false) を返す。
func analyzeWrapperFunction(funcCode []byte) (int, bool) {
    var foundNumbers []int
    const instrLen = 4

    for i := 0; i+instrLen <= len(funcCode); i += instrLen {
        word := binary.LittleEndian.Uint32(funcCode[i:])
        if word != svcMacOSEncoding {
            continue
        }
        // svc #0x80 を発見した。直前から x16 への即値設定を後方スキャンする。
        num, ok := backwardScanX16(funcCode, i)
        if !ok {
            return 0, false
        }
        foundNumbers = append(foundNumbers, num)
    }

    if len(foundNumbers) == 0 {
        return 0, false
    }

    // すべての svc で同一 BSD 番号であることを確認する（FR-3.2.3）。
    first := foundNumbers[0]
    for _, n := range foundNumbers[1:] {
        if n != first {
            return 0, false
        }
    }
    return first, true
}

// backwardScanX16 は funcCode[svcOffset] の svc #0x80 より前の命令を遡り、
// x16 レジスタへの即値設定命令を探す。見つかった場合は BSD クラスプレフィックスを
// 除去した syscall 番号を返す。最大 maxBackwardScanInstructions 命令を遡る。
//
// 対応する命令パターン:
//   - MOVZ X16, #imm               （imm < 0x10000: 1 命令で完結）
//   - MOVZ X16, #hi, LSL #16       （上位16ビットのみ設定）
//   - MOVZ X16, #hi, LSL #16       （先行）
//     MOVK X16, #lo                 （後続 = svc 直前）
//
// macOS BSD syscall の典型的なパターン（例: socket = 0x2000061）:
//   MOVZ X16, #0x200, LSL #16    ; x16 = 0x02000000
//   MOVK X16, #0x61              ; x16 = 0x02000061
//   SVC  #0x80
//
// 後方スキャンでは svc に近い命令から遡るため、MOVK を先に観測し、
// その後 MOVZ を観測したときに組み合わせて返す。
// MOVK X16 命令はスキャンを中断せず部分値を記録して継続する。
//
// arm64asm ライブラリは使用せず、固定エンコーディングで直接デコードする。
// これはシンプルな即値ロード命令のみを対象とし、asm ライブラリ依存を避けるため。
func backwardScanX16(funcCode []byte, svcOffset int) (int, bool) {
    const instrLen = 4

    // MOVZ X16, #imm, LSL #shift エンコーディング:
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
    // MOVK X16, #imm, LSL #shift エンコーディング:
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

    // 後方スキャン: svcOffset の直前から最大 maxBackwardScanInstructions 命令を遡る。
    startIdx := svcOffset/instrLen - 1
    endIdx := startIdx - maxBackwardScanInstructions
    if endIdx < 0 {
        endIdx = -1
    }

    // 後方スキャンで MOVZ+MOVK シーケンスを組み立てるための部分値を保持する。
    // -1 は「未観測」を意味する。
    x16Lo := -1 // MOVK X16, #imm, LSL #0 で記録した下位16ビット
    x16Hi := -1 // MOVK X16, #imm, LSL #16 で記録した上位16ビット（シフト済み）

    for i := startIdx; i > endIdx; i-- {
        off := i * instrLen
        if off < 0 {
            break
        }
        word := binary.LittleEndian.Uint32(funcCode[off:])

        // MOVZ X16, #imm (LSL #0): x16 の下位ビットを設定する終端命令。
        // 先に観測した MOVK #hi があれば組み合わせる。
        if word&^imm16Mask == movzX16Base {
            lo := int((word & imm16Mask) >> imm16Shift)
            hi := 0
            if x16Hi >= 0 {
                hi = x16Hi
            }
            return stripBSDPrefix(hi | lo), true
        }

        // MOVZ X16, #imm, LSL #16: x16 の上位ビットを設定する終端命令。
        // 先に観測した MOVK #lo があれば組み合わせる。
        if word&^imm16Mask == movzX16Lsl16 {
            hi := int((word & imm16Mask) >> imm16Shift) << 16
            lo := 0
            if x16Lo >= 0 {
                lo = x16Lo
            }
            return stripBSDPrefix(hi | lo), true
        }

        // MOVK X16, #imm (LSL #0): 下位16ビットを記録して走査を継続する。
        // 後方スキャンでは svc に近い側から遡るため MOVK を先に観測する。
        if word&^imm16Mask == movkX16Base {
            x16Lo = int((word & imm16Mask) >> imm16Shift)
            continue
        }

        // MOVK X16, #imm, LSL #16: 上位16ビットを記録して走査を継続する。
        if word&^imm16Mask == movkX16Lsl16 {
            x16Hi = int((word & imm16Mask) >> imm16Shift) << 16
            continue
        }

        // 制御フロー命令（B, BL, RET, CBZ, CBNZ 等）に達したらスキャンを中断する。
        if isControlFlowInstruction(word) {
            break
        }

        // 上記 MOVZ/MOVK 以外で x16 を書き込む命令に達したらスキャンを中断する。
        if writesX16NotMovzMovk(word) {
            break
        }
    }
    return 0, false
}

// stripBSDPrefix は BSD クラスプレフィックス（0x2000000）を除去する（FR-3.2.3）。
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
// isControlFlowInstruction は ARM64 命令が制御フロー命令かどうかを判定する。
// B / BL / BLR / BR / RET / CBZ / CBNZ / TBZ / TBNZ を対象とする。
// arm64 では命令は固定 4 バイトでリトルエンディアン。
func isControlFlowInstruction(word uint32) bool {
    // B:  [31:26] = 000101 (0b000101 = 5)
    // BL: [31:26] = 100101 (0b100101 = 37)
    if word>>26 == 0b000101 || word>>26 == 0b100101 {
        return true
    }
    // BLR / BR / RET: [31:22] = 1101011000 (0b1101011000)
    // BR:  全体 = 0xD61F0000 | (Rn << 5)
    // BLR: 全体 = 0xD63F0000 | (Rn << 5)
    // RET: 全体 = 0xD65F0000 | (Rn << 5)
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

// writesX16NotMovzMovk は MOVZ/MOVK 以外で x16 レジスタを書き込む 64-bit 命令を検出する。
// backwardScanX16 ループ内で MOVZ/MOVK を先に処理した後、残る x16 書き込み命令の
// 検出に使用する。
//
// MOVZ/MOVK のエンコーディングは [28:23] = 100101 (0b100101) の固定パターンを持つ。
// この範囲に該当する場合は false を返す（呼び出し元で処理済み）。
func writesX16NotMovzMovk(word uint32) bool {
    // MOVZ/MOVK: [28:23] = 100101、かつ Rd = X16 ([4:0] = 0x10)
    // ここに達した場合は MOVZ/MOVK の X16 以外の Rd への書き込みか、
    // または別命令エンコーディングのため、X16 書き込みか否かを判定する。
    bits28_23 := (word >> 23) & 0x3F
    if bits28_23 == 0b100101 {
        // MOVZ または MOVK エンコーディング（任意の Rd）→ 呼び出し元で処理済み
        return false
    }
    // 64-bit 命令 (sf=1) かつ Rd=16 ([4:0] = 0b10000)
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
// libPath はキャッシュファイル命名および lib_path フィールドに使用するパス（インストール名または実ファイルパス）。
// libHash は "sha256:<hex>" 形式のハッシュ（キャッシュ有効性判定に使用）。
// getData はキャッシュミス時のみ呼び出される Mach-O バイト列取得コールバック。
func (m *MachoLibSystemCacheManager) GetOrCreate(
    libPath, libHash string,
    getData func() ([]byte, error),
) ([]WrapperEntry, error) {
    encodedName, err := m.pathEnc.Encode(libPath)
    if err != nil {
        return nil, fmt.Errorf("failed to encode libsystem path: %w", err)
    }
    cacheFilePath := filepath.Join(m.cacheDir, encodedName)

    // 既存キャッシュの読み込みと有効性確認（FR-3.1.4）。
    if data, readErr := os.ReadFile(cacheFilePath); readErr == nil { //nolint:nestif,gosec // G304: cacheFilePath = cacheDir + pathEnc.Encode(libPath), both trusted
        var cache LibcCacheFile
        if jsonErr := json.Unmarshal(data, &cache); jsonErr == nil {
            if cache.SchemaVersion == LibcCacheSchemaVersion && cache.LibHash == libHash {
                return cache.SyscallWrappers, nil
            }
        }
    }

    // キャッシュミス: getData() で Mach-O バイト列を取得して解析する。
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

    // キャッシュファイルをアトミック書き込みする（ELF 版と同一）。
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
// MatchWithMethod は Match と同様だが、DeterminationMethod を外部から指定できる。
// Mach-O 版でキャッシュヒット時 "lib_cache_match" / フォールバック時 "symbol_name_match"
// を設定するために使用する。
func (m *ImportSymbolMatcher) MatchWithMethod(
    importSymbols []string,
    wrappers []WrapperEntry,
    determinationMethod string,
) []common.SyscallInfo {
    // wrappers からシンボル名 → WrapperEntry のマップを作成する。
    wrapperMap := make(map[string]WrapperEntry, len(wrappers))
    for _, w := range wrappers {
        wrapperMap[w.Name] = w
    }

    // 同一 Number に対してシンボル名が最小のものを採用する（Match と同一の dedup）。
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
// フォールバック条件（FR-3.4.1）:
//   - libSystem 系ライブラリが dynLibDeps に含まれていない
//   - dyld shared cache からの抽出も失敗した場合
func (a *MachoLibSystemAdapter) GetSyscallInfos(
    dynLibDeps []fileanalysis.LibEntry,
    importSymbols []string,
) ([]common.SyscallInfo, error) {
    source, err := machodylib.ResolveLibSystemKernel(dynLibDeps, a.fs)
    if err != nil {
        return nil, err
    }

    if source == nil {
        // フォールバック: シンボル名単体一致（FR-3.4.2）。
        slog.Info("libSystem キャッシュ解析不可: シンボル名単体一致にフォールバック",
            "reason", "libsystem_kernel.dylib not available (filesystem or dyld cache)")
        return a.fallbackNameMatch(importSymbols), nil
    }

    // キャッシュ取得または生成する。
    wrappers, err := a.cacheMgr.GetOrCreate(source.Path, source.Hash, source.GetData)
    if err != nil {
        return nil, err
    }

    // インポートシンボルとキャッシュを照合する（FR-3.3.2）。
    matcher := NewImportSymbolMatcher(a.syscallTable)
    return matcher.MatchWithMethod(importSymbols, wrappers, DeterminationMethodLibCacheMatch), nil
}

// fallbackNameMatch は FR-3.4.2 のシンボル名単体一致フォールバックを実装する。
// macOS syscall テーブルのネットワーク関連 syscall 名リストと importSymbols を照合し、
// 一致した名前の SyscallInfo を生成する。
func (a *MachoLibSystemAdapter) fallbackNameMatch(importSymbols []string) []common.SyscallInfo {
    // インポートシンボルのセットを作成する。
    symSet := make(map[string]bool, len(importSymbols))
    for _, s := range importSymbols {
        symSet[s] = true
    }

    var result []common.SyscallInfo
    for _, name := range networkSyscallWrapperNames {
        if !symSet[name] {
            continue
        }
        // macOS syscall テーブルから番号を逆引きする。
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

    // Number 昇順でソートする。
    sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })
    slog.Info("libSystem フォールバック照合完了",
        "detected_syscalls", len(result))
    return result
}
```

**Import 追加**: `adapters.go` に以下を追加する。

```go
import (
    // 既存の import に追加
    "log/slog"
    "sort"

    "github.com/isseis/go-safe-cmd-runner/internal/common"
    "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
    "github.com/isseis/go-safe-cmd-runner/internal/machodylib"
    "github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)
```

## 8. `internal/machodylib/dyld_extractor.go`（新規 - non-Darwin スタブ）

```go
//go:build !darwin

package machodylib

// LibSystemKernelBytes holds the in-memory bytes of libsystem_kernel.dylib
// extracted from the dyld shared cache, along with its SHA-256 hash.
type LibSystemKernelBytes struct {
    Data []byte // Mach-O バイト列
    Hash string // "sha256:<hex>" (Data の SHA-256)
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

// dyldSharedCachePaths は試行する dyld shared cache のパス順序（FR-3.1.6）。
var dyldSharedCachePaths = []string{
    "/System/Library/dyld/dyld_shared_cache_arm64e",
    "/System/Library/dyld/dyld_shared_cache_arm64",
}

// libsystemKernelInstallName は dyld shared cache 内の libsystem_kernel.dylib のインストール名。
const libsystemKernelInstallName = "/usr/lib/system/libsystem_kernel.dylib"

// ExtractLibSystemKernelFromDyldCache extracts libsystem_kernel.dylib from the
// dyld shared cache.
//
// 失敗時（キャッシュが見つからない、イメージが見つからない、展開失敗）は
// nil, nil を返してフォールバックへ移行する（FR-3.1.6）。
// slog.Info レベルでログを出力する。
func ExtractLibSystemKernelFromDyldCache() (*LibSystemKernelBytes, error) {
    // dyld shared cache ファイルの試行（FR-3.1.6）。
    var cachePath string
    for _, p := range dyldSharedCachePaths {
        if _, err := os.Stat(p); err == nil {
            cachePath = p
            break
        }
    }
    if cachePath == "" {
        slog.Info("dyld shared cache が見つかりません: フォールバックを適用します",
            "tried", dyldSharedCachePaths)
        return nil, nil
    }

    // blacktop/ipsw/pkg/dyld を使用して shared cache を解析する。
    f, err := dyld.Open(cachePath)
    if err != nil {
        slog.Info("dyld shared cache のオープンに失敗: フォールバックを適用します",
            "path", cachePath, "error", err)
        return nil, nil
    }
    defer func() { _ = f.Close() }()

    // libsystem_kernel.dylib イメージを取得する。
    image := f.Image(libsystemKernelInstallName)
    if image == nil {
        slog.Info("dyld shared cache 内に libsystem_kernel.dylib が見つかりません: フォールバックを適用します",
            "cache_path", cachePath,
            "install_name", libsystemKernelInstallName)
        return nil, nil
    }

    // イメージをバイト列に展開する。
    data, err := image.GetMacho()
    if err != nil {
        slog.Info("libsystem_kernel.dylib の展開に失敗: フォールバックを適用します",
            "cache_path", cachePath, "error", err)
        return nil, nil
    }

    machoBytes, err := data.FileSetFileData()
    if err != nil {
        // GetMacho() 直後に MachO バイト列を取得する別メソッドを試みる。
        // blacktop/ipsw API の具体的なメソッドは実装フェーズで確認する。
        slog.Info("libsystem_kernel.dylib バイト列の取得に失敗: フォールバックを適用します",
            "error", err)
        return nil, nil
    }

    // SHA-256 ハッシュを計算する（FR-3.1.4 補足・FR-3.1.6）。
    h := sha256.Sum256(machoBytes)
    hash := fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))

    return &LibSystemKernelBytes{
        Data: machoBytes,
        Hash: hash,
    }, nil
}
```

**実装注意**: `blacktop/ipsw/pkg/dyld` の正確な API（`Image.GetMacho()` の戻り値型、バイト列取得メソッド名）は、実装フェーズで `pkg/dyld` のドキュメントと実コードを参照して確定する。
API が変更されている場合は `image.Data()` や `image.Bytes()` 等の代替メソッドを使用する。

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
    // Path は lib_path フィールドおよびキャッシュファイル命名に使用するパス。
    // ファイルシステム経由: 実体ファイルパス。
    // dyld shared cache 経由: インストール名 "/usr/lib/system/libsystem_kernel.dylib"。
    Path string
    // Hash は "sha256:<hex>" 形式のハッシュ（キャッシュ有効性判定に使用）。
    Hash string
    // GetData は Mach-O バイト列を返すコールバック（キャッシュミス時のみ呼び出し）。
    GetData func() ([]byte, error)
}

// wellKnownLibsystemKernelPath は macOS 11+ 以前に libsystem_kernel.dylib が
// ファイルシステム上に存在する場合のウェルノウンパス。
const wellKnownLibsystemKernelPath = "/usr/lib/system/libsystem_kernel.dylib"

// libSystemBDylibInstallName は umbrella フレームワークのインストール名。
const libSystemBDylibInstallName = "/usr/lib/libSystem.B.dylib"

// libsystemKernelBaseName は libsystem_kernel.dylib のベース名。
const libsystemKernelBaseName = "libsystem_kernel.dylib"
```

### 10.2 `ResolveLibSystemKernel()` の実装

```go
// ResolveLibSystemKernel resolves the libsystem_kernel.dylib source from DynLibDeps.
//
// Returns nil, nil when:
//   - libSystem 系ライブラリが DynLibDeps に含まれない（非 libSystem バイナリ）
//   - dyld shared cache からの抽出も失敗した場合（フォールバック適用）
// Returns error only for unrecoverable conditions (permission errors, hash computation failures).
func ResolveLibSystemKernel(
    dynLibDeps []fileanalysis.LibEntry,
    fs safefileio.FileSystem,
) (*LibSystemKernelSource, error) {
    // Step 1: DynLibDeps から libSystem 系ライブラリを特定する（FR-3.1.5）。
    kernelEntry, hasLibSystem := findLibsystemKernelEntry(dynLibDeps)

    if !hasLibSystem {
        // libSystem 系が DynLibDeps に含まれていない: nil を返す（フォールバック条件 2）。
        return nil, nil
    }

    // Step 2: libsystem_kernel.dylib の実体パスを決定する。
    var candidatePath string
    if kernelEntry != nil {
        // libsystem_kernel.dylib が DynLibDeps に直接含まれている。
        candidatePath = kernelEntry.Path
    } else {
        // libSystem.B.dylib のみが含まれている: ウェルノウンパスを試行する（FR-3.1.5）。
        candidatePath = wellKnownLibsystemKernelPath
    }

    // Step 3: ファイルシステム上に存在するか確認する。
    if _, err := os.Stat(candidatePath); err == nil {
        // ファイルシステム上に存在する: ハッシュを計算する。
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

    // Step 4: dyld shared cache からの抽出を試みる（FR-3.1.6）。
    extracted, err := ExtractLibSystemKernelFromDyldCache()
    if err != nil {
        // 予期しないエラー（通常は発生しない: ExtractLibSystemKernelFromDyldCache は失敗時 nil, nil を返す）。
        return nil, fmt.Errorf("dyld shared cache extraction failed unexpectedly: %w", err)
    }
    if extracted == nil {
        // 抽出失敗: nil を返してフォールバックへ移行する（FR-3.4.1 条件 1）。
        slog.Info("libsystem_kernel.dylib: dyld shared cache からの抽出も失敗: フォールバックを適用します",
            "candidate_path", candidatePath)
        return nil, nil
    }

    // 抽出成功: インストール名をパスとして使用する（FR-3.1.6）。
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
// findLibsystemKernelEntry は DynLibDeps から libSystem 系ライブラリを特定する（FR-3.1.5）。
//
// 戻り値の解釈:
//   - kernelEntry != nil, hasLibSystem == true:
//       libsystem_kernel.dylib が直接 DynLibDeps に含まれる（優先順位 1）。
//       kernelEntry.Path を candidatePath として使用する。
//   - kernelEntry == nil, hasLibSystem == true:
//       libSystem.B.dylib のみが含まれる（優先順位 2）。
//       ウェルノウンパス wellKnownLibsystemKernelPath を candidatePath として試行する。
//   - kernelEntry == nil, hasLibSystem == false:
//       libSystem 系が含まれていない（非 libSystem バイナリ）。
//       ResolveLibSystemKernel は nil, nil を返してフォールバックへ移行する。
//
// 注意: libsystem_kernel.dylib が DynLibDeps に見つかった時点で即座に return するため、
// 同一 DynLibDeps 内に libsystem_kernel.dylib と libSystem.B.dylib の両方が含まれていても
// libsystem_kernel.dylib が優先される。
func findLibsystemKernelEntry(dynLibDeps []fileanalysis.LibEntry) (kernelEntry *fileanalysis.LibEntry, hasLibSystem bool) {
    for i, entry := range dynLibDeps {
        // 優先順位 1: libsystem_kernel.dylib のベース名一致（FR-3.1.5）。
        if filepath.Base(entry.SOName) == libsystemKernelBaseName {
            e := dynLibDeps[i]
            return &e, true
        }
        // 優先順位 2: libSystem.B.dylib のインストール名一致。
        // hasLibSystem を true にマークして走査を継続し、libsystem_kernel.dylib が
        // 後続エントリに存在するか確認する。
        if entry.SOName == libSystemBDylibInstallName {
            hasLibSystem = true
        }
    }
    return nil, hasLibSystem
}
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
// Mach-O arm64 svc #0x80 scan（現行）。
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
// Mach-O arm64 svc #0x80 スキャンと libSystem インポートシンボル照合。
// 両結果をマージして record.SyscallAnalysis に設定する（タスク 0100）。
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
// buildSVCSyscallEntries は svc #0x80 アドレスリストを []common.SyscallInfo に変換する。
// addrs が空の場合は nil を返す（変更前の buildSVCSyscallAnalysis の前半相当）。
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

// buildMachoSyscallAnalysisData は svc エントリと libSystem エントリをマージして
// SyscallAnalysisData を構築する。
// AnalysisWarnings は svc エントリが存在する場合のみ設定する。
// DetectedSyscalls は Number 昇順でソートする（FR-3.3.3）。
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

// mergeSyscallInfos は svc エントリと libSystem エントリを結合し Number 昇順でソートする。
// svc エントリは Number=-1（すべて異なる Location で重複なし）。
// libSystem エントリは Number>=0（GetSyscallInfos 内で dedup 済み）。
// 両者の Number が衝突することはないため追加の dedup は不要（FR-3.3.2）。
//
// ソート後の順序: Number=-1 の svc エントリが先頭（-1 < 0 < 正の番号）、
// libSystem エントリ（正の番号）がそれに続く。
// runner 側は Number の値ではなく DeterminationMethod で svc エントリを識別する。
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

// analyzeLibSystemImports は対象 Mach-O バイナリのインポートシンボルを取得し
// libSystem キャッシュと照合する（FR-3.3.2）。
// v.libSystemCache が nil の場合または DynLibDeps が空の場合は nil, nil を返す。
// 非 Mach-O ファイルは nil, nil を返す。
func (v *Validator) analyzeLibSystemImports(
    record *fileanalysis.Record,
    filePath string,
) ([]common.SyscallInfo, error) {
    if v.libSystemCache == nil || len(record.DynLibDeps) == 0 {
        return nil, nil
    }

    // debug/macho を使って Mach-O バイナリのインポートシンボルを取得する。
    // 非 Mach-O ファイル（ELF 等）は nil, nil を返す。
    importSymbols, err := getMachoImportSymbols(v.fileSystem, filePath)
    if err != nil || importSymbols == nil {
        return nil, err
    }

    // アンダースコアプレフィックスを除去する（FR-3.3.2）。
    // "_socket" → "socket", "_socket$UNIX2003" → "socket"
    normalized := make([]string, len(importSymbols))
    for i, sym := range importSymbols {
        normalized[i] = machoanalyzer.NormalizeSymbolName(sym)
    }

    return v.libSystemCache.GetSyscallInfos(record.DynLibDeps, normalized)
}

// getMachoImportSymbols は filePath を Mach-O として開き、インポートシンボルを返す。
// 非 Mach-O ファイルの場合は nil, nil を返す。
func getMachoImportSymbols(fs safefileio.FileSystem, filePath string) ([]string, error) {
    f, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
    if err != nil {
        return nil, err
    }
    defer func() { _ = f.Close() }()

    mf, err := macho.NewFile(f)
    if err != nil {
        // 非 Mach-O ファイル（ELF 等）: スキップする。
        return nil, nil //nolint:nilerr // Mach-O parse failure = non-Mach-O file; callers expect nil, nil
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
// normalizeSymbolName (小文字) はパッケージ内互換のため残す。
func NormalizeSymbolName(name string) string {
    return normalizeSymbolName(name)
}
```

## 13. `internal/runner/security/network_analyzer.go` の拡張

### 13.1 新規ヘルパー関数

```go
// syscallAnalysisHasNetworkSignal は SyscallAnalysisResult に IsNetwork==true エントリが
// あるかどうかを確認する（FR-3.6.2 優先順位 2）。
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
// 現行コード（変更なし）
case errors.Is(svcErr, fileanalysis.ErrNoSyscallAnalysis):
    // Fall through to SymbolAnalysis-based decision.
// ... その他の case ...
}
// ← ここに IsNetwork チェックを挿入する
```

**追加コード**:

```go
// IsNetwork チェック: libSystem 由来のネットワーク syscall を確認する（FR-3.6.2 優先順位 2）。
// direct_svc_0x80 チェック（switch 内）が優先されるため、ここに達した場合は
// svc #0x80 未検出が確定している。
if syscallAnalysisHasNetworkSignal(svcResult) {
    slog.Info("SyscallAnalysis cache indicates libSystem network syscall",
        "path", cmdPath)
    return true, false
}

// No svc #0x80 signal: determine result from SymbolAnalysis.
// （既存コード: data == nil { return false, false } ... 以降）
```

**優先順位の実現**: `direct_svc_0x80` チェック（switch 内の `syscallAnalysisHasSVCSignal` 呼び出し）が
この `IsNetwork` チェックより先に行われるため、FR-3.6.2 の優先順位 1 → 2 → 3 が自然に実現される。

## 14. `cmd/record/main.go` の拡張

`MachoLibSystemAdapter` を初期化して `Validator` に注入する。

```go
// 既存の LibcCacheManager 初期化の後に追加する。
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
| AC-2 | `internal/libccache/macho_cache_test.go` | `TestMachoLibSystemCacheManager_CreateAndLoad` |
| AC-3 | `internal/libccache/macho_cache_test.go` | `TestMachoLibSystemCacheManager_InvalidatesOnHashMismatch`, `TestMachoLibSystemCacheManager_InvalidatesOnSchemaMismatch` |
| AC-4 | `internal/libccache/adapters_test.go` | `TestMachoLibSystemAdapter_Fallback_NoLibSystem`, `TestMachoLibSystemAdapter_Fallback_DyldExtractFail` |
| AC-5 | `internal/libccache/adapters_test.go` | `TestMachoLibSystemAdapter_ImportSymbolMatching`, `TestMachoLibSystemAdapter_UnderscoreNormalization` |
| AC-6 | `internal/runner/security/network_analyzer_test.go` | `TestIsNetworkViaBinaryAnalysis_LibSystemNetworkSyscall` |
| AC-7 | `make test` | 全テスト |

### 15.2 `macho_analyzer_test.go`（主要テストケース）

```go
// TestMachoLibSystemAnalyzer_Analyze_SvcDetection はサイズフィルタと svc 検出の正常系。
func TestMachoLibSystemAnalyzer_Analyze_SvcDetection(t *testing.T) {
    // テスト用 Mach-O バイナリをメモリ上で構築する。
    // arm64 CpuType, __TEXT,__text セクション, Symtab を含む。
    // svc #0x80 命令 (0xD4001001) の直前に MOVZ X16, #97 (socket) を配置する。
}

// TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge は 256 バイト超の関数が除外されること。
func TestMachoLibSystemAnalyzer_Analyze_SizeTooLarge(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls は複数の異なる番号を持つ関数が除外されること。
func TestMachoLibSystemAnalyzer_Analyze_MultipleDistinctSyscalls(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval は 0x2000000 プレフィックスが除去されること（AC-2）。
func TestMachoLibSystemAnalyzer_Analyze_BSDPrefixRemoval(t *testing.T) { ... }

// TestMachoLibSystemAnalyzer_Analyze_NonArm64 は非 arm64 で UnsupportedArchitectureError が返ること。
func TestMachoLibSystemAnalyzer_Analyze_NonArm64(t *testing.T) { ... }
```

**テスト用 Mach-O バイナリ生成**: `debug/macho` の `NewFile` はバイナリ形式を必要とするため、
最小限の Mach-O ヘッダを手動で組み立てるか、既存の `internal/runner/security/machoanalyzer` の
テストパターンを参考にする。テスト用バイナリはファイルに書き出してテンプレートとして使用するか、
`bytes.Buffer` に直接組み立てる。

### 15.3 `adapters_test.go`（主要テストケース）

```go
// TestMachoLibSystemAdapter_GetSyscallInfos_CacheHit はキャッシュヒット時の正常系。
// LibSystemKernelSource を返すモックリゾルバーと、既存のキャッシュファイルを使用する。
func TestMachoLibSystemAdapter_GetSyscallInfos_CacheHit(t *testing.T) { ... }

// TestMachoLibSystemAdapter_GetSyscallInfos_Fallback_NoLibSystem は
// libSystem が DynLibDeps に含まれない場合のフォールバック。
func TestMachoLibSystemAdapter_GetSyscallInfos_Fallback_NoLibSystem(t *testing.T) { ... }

// TestMachoLibSystemAdapter_Fallback_DetectedSyscalls_Source は
// フォールバック時の Source が "libsystem_symbol_import" であること（AC-4）。
func TestMachoLibSystemAdapter_Fallback_DetectedSyscalls_Source(t *testing.T) { ... }

// TestMachoLibSystemAdapter_DeterminationMethod_LibCacheMatch は
// キャッシュヒット時の DeterminationMethod が "lib_cache_match" であること（AC-5）。
func TestMachoLibSystemAdapter_DeterminationMethod_LibCacheMatch(t *testing.T) { ... }
```

### 15.4 統合テスト条件

`//go:build !integration_only` または `t.Skip()` でプラットフォームをチェックする。

```go
// TestMachoLibSystem_Integration_DyldSharedCache は macOS arm64 のみで実行する。
func TestMachoLibSystem_Integration_DyldSharedCache(t *testing.T) {
    if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
        t.Skip("macOS arm64 only")
    }
    // record コマンドで動的 Mach-O バイナリを解析し、SyscallAnalysis に
    // ネットワーク syscall が検出されることを確認する。
}
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

`internal/libccache/errors.go` に以下を追加する。

```go
var (
    // 既存のエラーはそのまま

    // ErrLibsystemFileNotAccessible indicates that the libsystem_kernel.dylib file could not be read.
    ErrLibsystemFileNotAccessible = errors.New("libsystem_kernel.dylib file not accessible")

    // ErrLibsystemExportSymbolsFailed indicates that export symbol retrieval from libsystem_kernel.dylib failed.
    ErrLibsystemExportSymbolsFailed = errors.New("failed to get export symbols from libsystem_kernel.dylib")
)
```

**実装注意**: `ErrLibcFileNotAccessible` を Mach-O 版でも再利用することも可能だが、
エラーメッセージの可読性のため専用エラーを定義する。`MachoLibSystemCacheManager.GetOrCreate` は
`ErrLibcFileNotAccessible` を使用する（`cache.go` の既存パターンに合わせるため）。

## 18. `go.mod` / `go.sum` の更新

`blacktop/ipsw/pkg/dyld` を依存として追加する。

```
go get github.com/blacktop/ipsw@latest
```

**注意**: `blacktop/ipsw` は大きなモジュールであり、`pkg/dyld` サブパッケージのみが必要なため、
Go モジュールの最小バージョン選択（MVS）により実際の依存グラフが肥大化する可能性がある。
`pkg/dyld` のみを使用する最小依存パッケージを探すか、もしくは必要なコードを
`internal/machodylib/vendor/` 以下に手動で取り込む（ベンダリング）方法も検討する。
最終判断は実装フェーズで行う。
